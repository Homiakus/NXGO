package nx_test

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/pkg/nxgo"
)

func TestRealNXDeclarativeReleasePackageWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	worker, session := startTestWorker(t, ctx)
	defer worker.Kill()

	tempDir := t.TempDir()
	partFilePath := filepath.Join(tempDir, "machined_block.prt")

	// 1. Create and save base 3D CAD part (80x40x20 mm)
	t.Log("creating 3D model for release package...")
	part, err := session.NewPart(ctx, partFilePath, "mm")
	if err != nil {
		t.Fatalf("NewPart failed: %v", err)
	}
	_, err = part.CreateBlock(ctx, nxgo.BlockParams{
		Origin: nxgo.Point3D{0, 0, 0},
		Length: 80,
		Width:  40,
		Height: 20,
	})
	if err != nil {
		t.Fatalf("CreateBlock failed: %v", err)
	}
	_, err = part.Save(ctx)
	if err != nil {
		t.Fatalf("Save part failed: %v", err)
	}
	_ = part.Close(ctx, false)

	// 2. Execute Declarative Workflow: PrepareReleasePackage
	t.Log("executing PrepareReleasePackage workflow...")
	manifest, err := nxgo.PrepareReleasePackage(ctx, session, nxgo.ReleasePackageParams{
		PartPath:         partFilePath,
		OutputDir:        tempDir,
		DrawingSheetName: "PRODUCTION_RELEASE_A3",
		ColorMode:        "black_and_white",
	})
	if err != nil {
		t.Fatalf("PrepareReleasePackage failed: %v", err)
	}

	// 3. Verify Manifest & Artifacts (NXGO-INV-COR-001)
	if manifest.Status != "VERIFIED_RELEASE" {
		t.Fatalf("expected status VERIFIED_RELEASE, got %s", manifest.Status)
	}
	expectedVol := 80.0 * 40.0 * 20.0 // 64,000 mm³
	if math.Abs(manifest.MassProperties.Volume-expectedVol) > 0.1 {
		t.Fatalf("expected volume %.1f, got %.1f", expectedVol, manifest.MassProperties.Volume)
	}
	if manifest.PartSHA256 == "" || manifest.DrawingPDFSHA256 == "" {
		t.Fatalf("missing SHA256 hashes in manifest: %+v", manifest)
	}

	// 4. Verify Generated Files on Disk
	fi, err := os.Stat(manifest.DrawingPDFPath)
	if err != nil || fi.Size() == 0 {
		t.Fatalf("drawing PDF does not exist or empty: %v", err)
	}

	manifestJsonPath := filepath.Join(tempDir, "machined_block_manifest.json")
	if _, err := os.Stat(manifestJsonPath); err != nil {
		t.Fatalf("manifest.json missing on disk: %v", err)
	}
	t.Logf("release package generated successfully: PDF=%s (%d bytes) manifest=%s (volume=%.1f mm^3)",
		manifest.DrawingPDFPath, fi.Size(), manifestJsonPath, manifest.MassProperties.Volume)

	_ = worker.Stop(ctx)
}

func TestRealNXAssemblyValidationWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	worker, session := startTestWorker(t, ctx)
	defer worker.Kill()

	tempDir := t.TempDir()

	// 1. Create sub-components
	postPath := filepath.Join(tempDir, "post.prt")
	post, err := session.NewPart(ctx, postPath, "mm")
	if err != nil {
		t.Fatalf("NewPart post failed: %v", err)
	}
	_, _ = post.CreateCylinder(ctx, nxgo.CylinderParams{Origin: nxgo.Point3D{0, 0, 0}, Direction: nxgo.Vector3D{0, 0, 1}, Diameter: 12, Height: 50})
	_, _ = post.Save(ctx)
	_ = post.Close(ctx, false)

	basePath := filepath.Join(tempDir, "base.prt")
	base, err := session.NewPart(ctx, basePath, "mm")
	if err != nil {
		t.Fatalf("NewPart base failed: %v", err)
	}
	_, _ = base.CreateBlock(ctx, nxgo.BlockParams{Origin: nxgo.Point3D{0, 0, 0}, Length: 120, Width: 80, Height: 15})
	_, _ = base.Save(ctx)
	_ = base.Close(ctx, false)

	// 2. Create assembly with 1 base and 2 posts
	assyPath := filepath.Join(tempDir, "clamp_fixture.prt")
	assy, err := session.NewPart(ctx, assyPath, "mm")
	if err != nil {
		t.Fatalf("NewPart assembly failed: %v", err)
	}
	_, _ = assy.AddComponent(ctx, nxgo.AddComponentParams{PartPath: basePath, ComponentName: "BASE", Origin: nxgo.Point3D{0, 0, 0}})
	_, _ = assy.AddComponent(ctx, nxgo.AddComponentParams{PartPath: postPath, ComponentName: "POST_L", Origin: nxgo.Point3D{20, 40, 15}})
	_, _ = assy.AddComponent(ctx, nxgo.AddComponentParams{PartPath: postPath, ComponentName: "POST_R", Origin: nxgo.Point3D{100, 40, 15}})
	_, _ = assy.Save(ctx)
	_ = assy.Close(ctx, false)

	// 3. Execute Declarative Workflow: ValidateAssembly
	t.Log("executing ValidateAssembly workflow...")
	report, err := nxgo.ValidateAssembly(ctx, session, assyPath)
	if err != nil {
		t.Fatalf("ValidateAssembly failed: %v", err)
	}

	if !report.Valid {
		t.Fatalf("expected assembly to be valid, got invalid with issues: %v", report.Issues)
	}
	if report.TotalComponents != 3 {
		t.Fatalf("expected 3 total components, got %d", report.TotalComponents)
	}
	if report.UniqueParts != 2 {
		t.Fatalf("expected 2 unique parts in BOM, got %d", report.UniqueParts)
	}
	t.Logf("assembly validation report verified: valid=%v total_comps=%d unique_parts=%d BOM=%+v",
		report.Valid, report.TotalComponents, report.UniqueParts, report.BOM)

	_ = worker.Stop(ctx)
}
