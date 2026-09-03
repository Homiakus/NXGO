package nxgo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type ReleasePackageParams struct {
	PartPath                string
	OutputDir               string
	DrawingSheetName        string
	DrawingScaleNumerator   float64
	DrawingScaleDenominator float64
	ColorMode               string
}

type ReleaseManifest struct {
	PartPath         string         `json:"part_path"`
	DrawingPDFPath   string         `json:"drawing_pdf_path"`
	PartSHA256       string         `json:"part_sha256"`
	DrawingPDFSHA256 string         `json:"drawing_pdf_sha256"`
	MassProperties   MassProperties `json:"mass_properties"`
	BoundingBox      BoundingBox    `json:"bounding_box"`
	Timestamp        string         `json:"timestamp"`
	Status           string         `json:"status"`
}

type AssemblyValidationReport struct {
	AssemblyPath    string    `json:"assembly_path"`
	TotalComponents int       `json:"total_components"`
	UniqueParts     int       `json:"unique_parts"`
	BOM             []BOMItem `json:"bom"`
	Valid           bool      `json:"valid"`
	Issues          []string  `json:"issues,omitempty"`
}

func PrepareReleasePackage(ctx context.Context, session *Session, params ReleasePackageParams) (*ReleaseManifest, error) {
	if params.OutputDir == "" {
		params.OutputDir = filepath.Dir(params.PartPath)
	}
	if err := os.MkdirAll(params.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// 1. Open Part
	part, err := session.OpenPart(ctx, params.PartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open part for release package: %w", err)
	}
	defer func() {
		_ = part.Close(ctx, false)
	}()

	// 2. Measure Mass Properties and Bounding Box
	mp, err := part.MassProperties(ctx)
	if err != nil {
		return nil, fmt.Errorf("mass properties calculation failed: %w", err)
	}
	bbox, err := part.BoundingBox(ctx)
	if err != nil {
		return nil, fmt.Errorf("bounding box calculation failed: %w", err)
	}

	// 3. Create Release Drawing Sheet
	sheetName := params.DrawingSheetName
	if sheetName == "" {
		sheetName = "A3_RELEASE_DRAWING"
	}
	num := params.DrawingScaleNumerator
	if num == 0 {
		num = 1.0
	}
	den := params.DrawingScaleDenominator
	if den == 0 {
		den = 1.0
	}

	_, err = part.CreateDrawingSheet(ctx, CreateSheetParams{
		SheetName:        sheetName,
		Units:            "mm",
		Height:           297.0,
		Length:           420.0,
		ScaleNumerator:   num,
		ScaleDenominator: den,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create release drawing sheet: %w", err)
	}

	// 4. Export PDF Drawing
	baseName := filepath.Base(params.PartPath)
	ext := filepath.Ext(baseName)
	rawName := baseName[:len(baseName)-len(ext)]
	pdfPath := filepath.Join(params.OutputDir, rawName+"_drawing.pdf")

	colorMode := params.ColorMode
	if colorMode == "" {
		colorMode = "black_and_white"
	}

	_, err = part.ExportPDF(ctx, ExportPDFParams{
		OutputPDFPath: pdfPath,
		ColorMode:     colorMode,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to export release PDF: %w", err)
	}

	// 5. Compute SHA256 Hashes
	partHash, err := hashFileSHA256(params.PartPath)
	if err != nil {
		partHash = "unknown"
	}
	pdfHash, err := hashFileSHA256(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to hash generated PDF: %w", err)
	}

	manifest := &ReleaseManifest{
		PartPath:         params.PartPath,
		DrawingPDFPath:   pdfPath,
		PartSHA256:       partHash,
		DrawingPDFSHA256: pdfHash,
		MassProperties:   *mp,
		BoundingBox:      *bbox,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		Status:           "VERIFIED_RELEASE",
	}

	// 6. Stage and atomically write Manifest to Disk
	manifestPath := filepath.Join(params.OutputDir, rawName+"_manifest.json")
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}
	tmpManifestPath := manifestPath + ".tmp"
	if err := os.WriteFile(tmpManifestPath, manifestBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed to write staged manifest: %w", err)
	}
	if err := os.Rename(tmpManifestPath, manifestPath); err != nil {
		_ = os.Remove(tmpManifestPath)
		return nil, fmt.Errorf("failed to atomically publish manifest: %w", err)
	}

	return manifest, nil
}

func ValidatePart(ctx context.Context, session *Session, partPath string) (*MassProperties, *BoundingBox, error) {
	part, err := session.OpenPart(ctx, partPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open part failed: %w", err)
	}
	defer func() { _ = part.Close(ctx, false) }()

	mp, err := part.MassProperties(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("mass properties query failed: %w", err)
	}
	bbox, err := part.BoundingBox(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("bounding box query failed: %w", err)
	}
	return mp, bbox, nil
}

func ValidateAssembly(ctx context.Context, session *Session, assemblyPath string) (*AssemblyValidationReport, error) {
	part, err := session.OpenPart(ctx, assemblyPath)
	if err != nil {
		return nil, fmt.Errorf("open assembly failed: %w", err)
	}
	defer func() { _ = part.Close(ctx, false) }()

	tree, err := part.ComponentTree(ctx)
	if err != nil {
		return nil, fmt.Errorf("component tree query failed: %w", err)
	}

	bom, err := part.BOM(ctx)
	if err != nil {
		return nil, fmt.Errorf("BOM query failed: %w", err)
	}

	report := &AssemblyValidationReport{
		AssemblyPath:    assemblyPath,
		TotalComponents: countComponents(tree),
		UniqueParts:     len(bom),
		BOM:             bom,
		Valid:           true,
	}

	// Check for missing prototype paths
	checkTreePrototypes(tree, report)

	return report, nil
}

func countComponents(node *ComponentNode) int {
	if node == nil {
		return 0
	}
	count := 0
	for _, ch := range node.Children {
		count += 1 + countComponents(&ch)
	}
	return count
}

func checkTreePrototypes(node *ComponentNode, report *AssemblyValidationReport) {
	if node == nil {
		return
	}
	for _, ch := range node.Children {
		if ch.PrototypePath == "" {
			report.Valid = false
			report.Issues = append(report.Issues, fmt.Sprintf("component %q has unresolvable prototype", ch.DisplayName))
		}
		checkTreePrototypes(&ch, report)
	}
}

func hashFileSHA256(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
