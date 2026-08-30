package nx_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/pkg/nxgo"
)

func TestRealNXDraftingCreateSheetAndExportPDF(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	worker, session := startTestWorker(t, ctx)
	defer worker.Kill()

	tempDir := t.TempDir()
	partFilePath := filepath.Join(tempDir, "bracket_drafting.prt")
	pdfFilePath := filepath.Join(tempDir, "bracket_drawing.pdf")

	// 1. Create 3D Part
	t.Log("creating 3D part with block geometry...")
	part, err := session.NewPart(ctx, partFilePath, "mm")
	if err != nil {
		t.Fatalf("NewPart failed: %v", err)
	}

	_, err = part.CreateBlock(ctx, nxgo.BlockParams{
		Origin: nxgo.Point3D{0, 0, 0},
		Length: 100,
		Width:  50,
		Height: 25,
	})
	if err != nil {
		t.Fatalf("CreateBlock failed: %v", err)
	}

	// 2. Create A3 Drawing Sheet
	t.Log("creating A3 drawing sheet (297x420 mm)...")
	sheet, err := part.CreateDrawingSheet(ctx, nxgo.CreateSheetParams{
		SheetName:        "A3_MANUFACTURING",
		Units:            "mm",
		Height:           297.0,
		Length:           420.0,
		ScaleNumerator:   1.0,
		ScaleDenominator: 1.0,
	})
	if err != nil {
		t.Fatalf("CreateDrawingSheet failed: %v", err)
	}
	t.Logf("drawing sheet created: name=%s ref=%s height=%.1f length=%.1f",
		sheet.Name, sheet.Ref.ObjectID, sheet.Height, sheet.Length)

	// 3. Query Drawing Sheets
	t.Log("querying drawing sheets in part...")
	sheets, err := part.DrawingSheets(ctx)
	if err != nil {
		t.Fatalf("DrawingSheets query failed: %v", err)
	}
	if len(sheets) != 1 {
		t.Fatalf("expected 1 drawing sheet, got %d", len(sheets))
	}
	t.Logf("verified 1 drawing sheet found: name=%s", sheets[0].Name)

	// 4. Export to PDF
	t.Log("exporting drawing sheet to PDF...")
	exportRes, err := part.ExportPDF(ctx, nxgo.ExportPDFParams{
		OutputPDFPath: pdfFilePath,
		ColorMode:     "black_and_white",
	})
	if err != nil {
		t.Fatalf("ExportPDF failed: %v", err)
	}
	t.Logf("PDF export response: path=%s size=%d bytes", exportRes.ExportedPath, exportRes.FileSizeBytes)

	// 5. Verify PDF file on disk (NXGO-INV-COR-001)
	fi, err := os.Stat(pdfFilePath)
	if err != nil {
		t.Fatalf("stat exported PDF failed: %v", err)
	}
	if fi.Size() < 500 {
		t.Fatalf("exported PDF file size unexpectedly small: %d bytes", fi.Size())
	}

	content, err := os.ReadFile(pdfFilePath)
	if err != nil {
		t.Fatalf("reading exported PDF failed: %v", err)
	}
	if len(content) < 4 || string(content[:4]) != "%PDF" {
		t.Fatalf("exported PDF missing %%PDF header: got %q", string(content[:min(len(content), 10)]))
	}
	t.Logf("verified exported PDF on disk: %s (size: %d bytes, header: %s)",
		pdfFilePath, len(content), string(content[:5]))

	// 6. Save and Close
	_, _ = part.Save(ctx)
	_ = part.Close(ctx, false)
	_ = worker.Stop(ctx)
}
