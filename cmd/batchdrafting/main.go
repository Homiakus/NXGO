package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Homiakus/NXGO/internal/supervisor"
	"github.com/Homiakus/NXGO/pkg/nxgo"
)

type DrawingGenerationResult struct {
	PartName     string   `json:"part_name"`
	PartPath     string   `json:"part_path"`
	IsAssembly   bool     `json:"is_assembly"`
	SheetName    string   `json:"sheet_name"`
	SheetSize    string   `json:"sheet_size"`
	ViewCount    int      `json:"view_count"`
	Views        []string `json:"views"`
	Dimensions   string   `json:"dimensions"`
	PDFPath      string   `json:"pdf_path"`
	PDFSizeBytes int64    `json:"pdf_size_bytes"`
	Error        string   `json:"error,omitempty"`
}

func main() {
	defaultAssembly := `C:\Users\KDFX Modes\Documents\vault_v2.0\20-Projects\201-Engineering-Projects\PKL.7.14\01-CAD\NX\PKL-ASM-000-V1.2.prt`
	assemblyFlag := flag.String("assembly", defaultAssembly, "Path to root assembly .prt")
	outputDirFlag := flag.String("output", `artifacts\drawings_pdf`, "Output directory for exported PDFs")
	maxPartsFlag := flag.Int("max-parts", 0, "Max parts to process (0 = all)")
	flag.Parse()

	absAssembly, err := filepath.Abs(*assemblyFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid assembly path: %v\n", err)
		os.Exit(1)
	}

	absOutputDir, err := filepath.Abs(*outputDirFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid output directory: %v\n", err)
		os.Exit(1)
	}
	_ = os.MkdirAll(absOutputDir, 0755)

	cadDir := filepath.Dir(absAssembly)
	fmt.Printf("=== NXGO Batch Drafting Pipeline ===\n")
	fmt.Printf("Root Assembly: %s\n", absAssembly)
	fmt.Printf("CAD Directory: %s\n", cadDir)
	fmt.Printf("PDF Output:    %s\n\n", absOutputDir)

	// Collect unique .prt files belonging to this project
	prtFiles, err := filepath.Glob(filepath.Join(cadDir, "*.prt"))
	if err != nil || len(prtFiles) == 0 {
		fmt.Fprintf(os.Stderr, "No .prt files found in %s\n", cadDir)
		os.Exit(1)
	}

	// Filter out temporary or step imported backup files
	var targetParts []string
	var rootAssemblyPath string

	for _, p := range prtFiles {
		base := filepath.Base(p)
		if strings.HasPrefix(base, "~") || strings.Contains(base, "_step") {
			continue
		}
		if strings.EqualFold(p, absAssembly) {
			rootAssemblyPath = p
		} else {
			targetParts = append(targetParts, p)
		}
	}

	// Place root assembly at the beginning
	if rootAssemblyPath != "" {
		targetParts = append([]string{rootAssemblyPath}, targetParts...)
	}

	if *maxPartsFlag > 0 && len(targetParts) > *maxPartsFlag {
		targetParts = targetParts[:*maxPartsFlag]
	}

	fmt.Printf("Found %d distinct CAD part(s)/assembly to draft:\n", len(targetParts))
	for i, p := range targetParts {
		fmt.Printf("  [%2d] %s\n", i+1, filepath.Base(p))
	}
	fmt.Println()

	// Launch Real NX Worker
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	repoRoot, _ := filepath.Abs(".")
	agentBin := filepath.Join(repoRoot, "agent", "bin")

	fmt.Println("Starting real Siemens NX worker...")
	worker, err := supervisor.StartWorker(ctx, supervisor.WorkerConfig{
		AgentMode:      supervisor.AgentModeCanonical,
		AgentBin:       agentBin,
		StartupTimeout: 45 * time.Second,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start NX worker: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		fmt.Println("\nStopping NX worker...")
		_ = worker.Kill()
	}()

	session := nxgo.WrapClient(worker.Client, worker.Manifest.ID, 1, worker.Manifest.NXRelease)
	if err := session.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Worker ping failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("NX worker connected: %s (PID: %d)\n\n", worker.Manifest.NXRelease, worker.Manifest.PID)

	var results []DrawingGenerationResult

	for idx, partPath := range targetParts {
		partBase := filepath.Base(partPath)
		partName := strings.TrimSuffix(partBase, filepath.Ext(partBase))
		isAssembly := strings.Contains(strings.ToUpper(partName), "ASM") || strings.EqualFold(partPath, absAssembly)

		fmt.Printf("[%d/%d] Generating drawing for: %s ...\n", idx+1, len(targetParts), partBase)

		res := processPartDrawing(ctx, session, partPath, partName, isAssembly, absOutputDir)
		results = append(results, res)

		if res.Error != "" {
			fmt.Printf("      -> ERROR: %s\n", res.Error)
		} else {
			fmt.Printf("      -> OK: %s (%d views, %d bytes) -> %s\n",
				res.SheetSize, res.ViewCount, res.PDFSizeBytes, filepath.Base(res.PDFPath))
		}
	}

	// Print Summary Table
	fmt.Printf("\n===============================================================================\n")
	fmt.Printf("                           BATCH DRAFTING SUMMARY                              \n")
	fmt.Printf("===============================================================================\n")
	successCount := 0
	for _, r := range results {
		status := "SUCCESS"
		if r.Error != "" {
			status = "FAILED"
		} else {
			successCount++
		}
		fmt.Printf("%-35s | %-7s | %-5s | %-25s | %s\n",
			r.PartName, status, r.SheetSize, r.Dimensions, filepath.Base(r.PDFPath))
	}
	fmt.Printf("\nTotal completed: %d / %d drawings exported to:\n%s\n",
		successCount, len(results), absOutputDir)
}

func processPartDrawing(
	ctx context.Context,
	session *nxgo.Session,
	partPath string,
	partName string,
	isAssembly bool,
	outputDir string,
) DrawingGenerationResult {
	res := DrawingGenerationResult{
		PartName:   partName,
		PartPath:   partPath,
		IsAssembly: isAssembly,
	}

	part, err := session.OpenPart(ctx, partPath)
	if err != nil {
		res.Error = fmt.Sprintf("OpenPart failed: %v", err)
		return res
	}
	defer func() {
		_ = part.Close(ctx, false) // discard drawing modifications on original PRT
	}()

	// Query summary & properties
	summary, _ := part.Summary(ctx)
	bbox, _ := part.BoundingBox(ctx)
	mp, _ := part.MassProperties(ctx)
	exprs, _ := part.ListExpressions(ctx)

	// Determine Sheet Size: A3 (420x297) for assemblies and larger parts, A4 (297x210) for small parts
	sheetWidth := 420.0
	sheetHeight := 297.0
	sheetSizeName := "A3"

	if !isAssembly && bbox != nil && bbox.Dimensions[0] < 50 && bbox.Dimensions[1] < 50 && bbox.Dimensions[2] < 50 {
		sheetWidth = 297.0
		sheetHeight = 210.0
		sheetSizeName = "A4"
	}
	res.SheetSize = sheetSizeName
	res.SheetName = "SHEET_1"

	sheet, err := part.CreateDrawingSheet(ctx, nxgo.CreateSheetParams{
		SheetName:        res.SheetName,
		Units:            "mm",
		Length:           sheetWidth,
		Height:           sheetHeight,
		ScaleNumerator:   1.0,
		ScaleDenominator: 1.0,
	})
	if err != nil {
		res.Error = fmt.Sprintf("CreateDrawingSheet failed: %v", err)
		return res
	}

	// 1. Add Standard Views (Front, Top, Right, Isometric)
	layout := "front_top_right_iso"
	viewRes, err := sheet.CreateStandardViews(ctx, nxgo.StandardViewsParams{
		Layout:             layout,
		MarginBetweenViews: 20.0,
		MarginToBorder:     25.0,
	})
	if err != nil {
		fmt.Printf("      [warn] CreateStandardViews: %v\n", err)
	} else {
		res.ViewCount = viewRes.ViewCount
		res.Views = viewRes.Views
	}

	// 2. Add Title Block / Stamp (Нижний правый угол)
	massStr := "0.00 кг"
	if mp != nil && mp.Mass > 0 {
		massStr = fmt.Sprintf("%.3f кг", mp.Mass)
	}
	volStr := ""
	if mp != nil && mp.Volume > 0 {
		volStr = fmt.Sprintf("%.1f см3", mp.Volume/1000.0)
	}

	dimStr := "Габариты не определены"
	if bbox != nil {
		dimStr = fmt.Sprintf("%.1f x %.1f x %.1f мм",
			bbox.Dimensions[0], bbox.Dimensions[1], bbox.Dimensions[2])
	}
	res.Dimensions = dimStr

	titleType := "ДЕТАЛЬ"
	if isAssembly {
		titleType = "СБОРОЧНЫЙ ЧЕРТЕЖ (СБ)"
	}

	stampLines := []string{
		"=========================================",
		fmt.Sprintf(" ОБОЗНАЧЕНИЕ: %s", partName),
		fmt.Sprintf(" ТИП:         %s", titleType),
		fmt.Sprintf(" ГАБАРИТЫ:    %s", dimStr),
		fmt.Sprintf(" МАССА:       %s   ОБЪЕМ: %s", massStr, volStr),
		" МАСШТАБ:     1:1    ЛИСТ: 1 ИЗ 1",
		" РАЗРАБОТАЛ:  NXGO CAD AUTOMATION",
		" СИСТЕМА:     SIEMENS DESIGN CENTER NX",
		"=========================================",
	}

	// Place stamp near bottom-right border (X = width - 190 mm, Y = 10 mm)
	stampX := sheetWidth - 190.0
	stampY := 10.0
	_, _ = sheet.AddNote(ctx, nxgo.AddNoteParams{
		TextLines: stampLines,
		OriginX:   stampX,
		OriginY:   stampY,
		Anchor:    "bottom_left",
		TextSize:  3.0,
	})

	// 3. Add Technical Requirements & Dimensions Table (Нижний левый угол)
	var techLines []string
	if isAssembly {
		techLines = append(techLines,
			"ТЕХНИЧЕСКИЕ ТРЕБОВАНИЯ И СПЕЦИФИКАЦИЯ СБОРКИ:",
			"1. * Размеры для справок.",
			fmt.Sprintf("2. Габаритные размеры сборочной единицы: %s.", dimStr),
			"3. Сборку производить в соответствии со сборочной схемой.",
			"4. Резьбовые соединения затянуть номинальным моментом.",
		)
		if summary != nil && summary.ComponentCount > 0 {
			techLines = append(techLines, fmt.Sprintf("5. Число компонентов в сборке: %d шт.", summary.ComponentCount))
		}
	} else {
		techLines = append(techLines,
			"ТЕХНИЧЕСКИЕ ТРЕБОВАНИЯ И РАЗМЕРЫ ДЕТАЛИ:",
			"1. * Размеры для справок.",
			fmt.Sprintf("2. Габаритные размеры заготовки: %s.", dimStr),
			"3. Неуказанные предельные отклонения: H14, h14, ±IT14/2.",
			"4. Острые кромки притупить R 0.2...0.5 мм.",
		)
		// Add top key parameters if available
		if len(exprs) > 0 {
			techLines = append(techLines, "5. ОСНОВНЫЕ ПАРАМЕТРЫ И ВЫРАЖЕНИЯ:")
			count := 0
			for _, e := range exprs {
				if count >= 8 {
					break
				}
				if e.Value != 0 || e.Formula != "" {
					u := e.Units
					if u == "" {
						u = "мм"
					}
					techLines = append(techLines, fmt.Sprintf("   %s = %s (%v %s)", e.Name, e.Formula, e.Value, u))
					count++
				}
			}
		}
	}

	// Place tech notes in lower-left area (X = 25 mm, Y = 15 mm)
	_, _ = sheet.AddNote(ctx, nxgo.AddNoteParams{
		TextLines: techLines,
		OriginX:   25.0,
		OriginY:   15.0,
		Anchor:    "bottom_left",
		TextSize:  3.0,
	})

	// 4. Export Sheet to PDF
	pdfFileName := partName + ".pdf"
	if isAssembly {
		pdfFileName = partName + "_СБ.pdf"
	}
	targetPDF := filepath.Join(outputDir, pdfFileName)

	pdfRes, err := part.ExportPDF(ctx, nxgo.ExportPDFParams{
		OutputPDFPath: targetPDF,
		SheetNames:    []string{res.SheetName},
		ColorMode:     "black_and_white",
	})
	if err != nil {
		res.Error = fmt.Sprintf("ExportPDF failed: %v", err)
		return res
	}

	res.PDFPath = pdfRes.ExportedPath
	res.PDFSizeBytes = pdfRes.FileSizeBytes
	return res
}
