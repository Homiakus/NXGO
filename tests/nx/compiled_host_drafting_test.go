package nx_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/supervisor"
	"github.com/Homiakus/NXGO/pkg/nxgo"
)

func TestRealNXCanonicalCompiledHostDrafting(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("canonical compiled NXHost Drafting oracle requires Windows/Siemens NX")
	}
	if os.Getenv("NXGO_RUN_REAL_NX") == "" {
		t.Skip("set NXGO_RUN_REAL_NX=1 or run via the real-NX quality gate")
	}

	repoRoot := repoRootFromTestFile(t)
	agentBin := os.Getenv("NXGO_AGENT_BIN")
	if agentBin == "" {
		agentBin = filepath.Join(repoRoot, "agent", "bin")
	}
	for _, required := range []string{
		filepath.Join(agentBin, "NXGO.Agent.Core.dll"),
		filepath.Join(agentBin, "NXGO.Agent.NXHost.dll"),
	} {
		if _, err := os.Stat(required); err != nil {
			if os.Getenv("NXGO_REQUIRE_COMPILED_HOST") == "1" {
				t.Fatalf("canonical compiled Agent output is mandatory but missing: %s", required)
			}
			t.Skipf("canonical compiled Agent not built: %s", required)
		}
	}

	oldBin, hadBin := os.LookupEnv("NXGO_AGENT_BIN")
	if err := os.Setenv("NXGO_AGENT_BIN", agentBin); err != nil {
		t.Fatalf("set NXGO_AGENT_BIN: %v", err)
	}
	defer func() {
		if hadBin {
			_ = os.Setenv("NXGO_AGENT_BIN", oldBin)
		} else {
			_ = os.Unsetenv("NXGO_AGENT_BIN")
		}
	}()

	bootstrap := filepath.Join(repoRoot, "agent", "bundle", "CompiledHostBootstrap.cs")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	worker, err := supervisor.StartWorker(ctx, supervisor.WorkerConfig{
		NXHome:         getNXHome(t),
		JournalPath:    bootstrap,
		StartupTimeout: 45 * time.Second,
	})
	if err != nil {
		t.Fatalf("start canonical compiled NXHost for Drafting: %v", err)
	}
	defer worker.Kill()

	session := nxgo.WrapClient(worker.Client, worker.Manifest.ID, 1, worker.Manifest.NXRelease)
	artifactDir := filepath.Join(repoRoot, "artifacts", "nx-smoke", "canonical-drafting")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("create Drafting artifact directory: %v", err)
	}

	partPath := filepath.Join(t.TempDir(), "canonical_drafting.prt")
	part, err := session.NewPart(ctx, partPath, "mm")
	if err != nil {
		t.Fatalf("Drafting part.new failed: %v", err)
	}

	sheet, err := part.CreateDrawingSheet(ctx, nxgo.CreateSheetParams{
		SheetName:        "A3_MAIN",
		Units:            "mm",
		Height:           297,
		Length:           420,
		ScaleNumerator:   1,
		ScaleDenominator: 1,
	})
	if err != nil {
		t.Fatalf("canonical drafting.create_sheet failed: %v", err)
	}
	if sheet.Ref.ObjectID == "" || sheet.Ref.Generation == 0 || sheet.Ref.Kind != "DrawingSheet" {
		t.Fatalf("created DrawingSheet must have persistent generation-aware handle: %+v", sheet.Ref)
	}

	sheets, err := part.DrawingSheets(ctx)
	if err != nil {
		t.Fatalf("canonical drafting.query_sheets failed: %v", err)
	}
	if len(sheets) != 1 || sheets[0].Name != "A3_MAIN" {
		t.Fatalf("canonical sheet snapshot mismatch: %+v", sheets)
	}
	if sheets[0].Ref.ObjectID != "" || sheets[0].Ref.Generation != 0 {
		t.Fatalf("query_sheets snapshot unexpectedly allocated operational handle: %+v", sheets[0].Ref)
	}

	pdfPath := filepath.Join(artifactDir, "canonical-a3-main.pdf")
	_ = os.Remove(pdfPath)
	pdf, err := part.ExportPDF(ctx, nxgo.ExportPDFParams{
		OutputPDFPath: pdfPath,
		SheetNames:    []string{"A3_MAIN"},
		ColorMode:     "black_and_white",
	})
	if err != nil {
		t.Fatalf("canonical drafting.export_pdf failed: %v", err)
	}
	if pdf.FileSizeBytes <= 0 {
		t.Fatalf("canonical PDF export returned non-positive size: %+v", pdf)
	}
	if info, err := os.Stat(pdfPath); err != nil || info.Size() <= 0 {
		t.Fatalf("canonical PDF artifact missing/empty: info=%v err=%v", info, err)
	}

	// Unsupported options must fail before PDF commit and leave the worker
	// reusable. Legacy silently ignored these fields; canonical behavior is
	// deliberately fail-closed until verified NX semantics are implemented.
	unsupportedPath := filepath.Join(artifactDir, "must-not-exist-grayscale.pdf")
	_ = os.Remove(unsupportedPath)
	if _, err := part.ExportPDF(ctx, nxgo.ExportPDFParams{
		OutputPDFPath: unsupportedPath,
		SheetNames:    []string{"A3_MAIN"},
		ColorMode:     "grayscale",
	}); err == nil {
		t.Fatal("unsupported grayscale PDF mode unexpectedly succeeded")
	}
	if _, err := os.Stat(unsupportedPath); !os.IsNotExist(err) {
		t.Fatalf("unsupported color mode created an artifact unexpectedly: err=%v", err)
	}
	if err := session.Ping(ctx); err != nil {
		t.Fatalf("pre-start color validation poisoned canonical worker: %v", err)
	}

	missingPath := filepath.Join(artifactDir, "must-not-exist-missing-sheet.pdf")
	_ = os.Remove(missingPath)
	if _, err := part.ExportPDF(ctx, nxgo.ExportPDFParams{
		OutputPDFPath: missingPath,
		SheetNames:    []string{"MISSING_SHEET"},
		ColorMode:     "black_and_white",
	}); err == nil {
		t.Fatal("PDF export with missing requested sheet unexpectedly succeeded")
	}
	if _, err := os.Stat(missingPath); !os.IsNotExist(err) {
		t.Fatalf("missing-sheet preflight created an artifact unexpectedly: err=%v", err)
	}
	if _, err := part.Summary(ctx); err != nil {
		t.Fatalf("missing-sheet failed-before-start made session unusable: %v", err)
	}

	if _, err := part.Save(ctx); err != nil {
		t.Fatalf("Drafting part.save failed: %v", err)
	}
	if err := part.Close(ctx, false); err != nil {
		t.Fatalf("Drafting part.close failed: %v", err)
	}
	if err := session.ReleaseObjects(ctx, sheet.Ref); err == nil {
		t.Fatal("DrawingSheet handle unexpectedly survived owning Part close")
	}

	if err := worker.Stop(ctx); err != nil {
		t.Fatalf("canonical Drafting worker shutdown failed: %v", err)
	}
	t.Logf("canonical Drafting verified: sheet=%s pdf=%s bytes=%d", sheet.Name, pdf.ExportedPath, pdf.FileSizeBytes)
}
