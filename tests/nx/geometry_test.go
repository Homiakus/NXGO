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

func TestRealNXGeometryCreationAndMassProperties(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	worker, session := startTestWorker(t, ctx)
	defer worker.Kill()

	tempDir := t.TempDir()
	partFilePath := filepath.Join(tempDir, "nxgo_geo_test.prt")

	// 1. Create a new part in millimeters
	t.Log("creating new part for geometry testing...")
	part, err := session.NewPart(ctx, partFilePath, "mm")
	if err != nil {
		t.Fatalf("NewPart failed: %v", err)
	}

	// 2. Create a 100 x 50 x 25 mm Block Feature
	t.Log("creating 100x50x25 block feature...")
	blockFeat, err := part.CreateBlock(ctx, nxgo.BlockParams{
		Origin: nxgo.Point3D{0, 0, 0},
		Length: 100,
		Width:  50,
		Height: 25,
	})
	if err != nil {
		t.Fatalf("CreateBlock failed: %v", err)
	}
	if blockFeat.Ref.ObjectID == "" || blockFeat.BodyRef.ObjectID == "" {
		t.Fatalf("invalid block feature or body ref: %+v", blockFeat)
	}
	t.Logf("block created: name=%s feat_ref=%s body_ref=%s",
		blockFeat.Name, blockFeat.Ref.ObjectID, blockFeat.BodyRef.ObjectID)

	// 3. Verify Mass Properties (NXGO-INV-COR-001)
	t.Log("verifying mass properties of block...")
	mp, err := part.MassProperties(ctx)
	if err != nil {
		t.Fatalf("MassProperties failed: %v", err)
	}
	t.Logf("mp returned: volume=%.4f area=%.4f mass=%.4f centroid=%+v units=%s",
		mp.Volume, mp.Area, mp.Mass, mp.Centroid, mp.Units)

	expectedVolume := 100.0 * 50.0 * 25.0 // 125,000 mm³
	if math.Abs(mp.Volume-expectedVolume) > 0.1 {
		t.Fatalf("expected volume ~%.1f, got %.4f", expectedVolume, mp.Volume)
	}

	expectedArea := 2.0 * (100.0*50.0 + 100.0*25.0 + 50.0*25.0) // 17,500 mm²
	if math.Abs(mp.Area-expectedArea) > 0.1 {
		t.Fatalf("expected area ~%.1f, got %.4f", expectedArea, mp.Area)
	}

	if math.Abs(mp.Centroid[0]-50.0) > 0.1 || math.Abs(mp.Centroid[1]-25.0) > 0.1 || math.Abs(mp.Centroid[2]-12.5) > 0.1 {
		t.Fatalf("expected centroid [50, 25, 12.5], got %+v", mp.Centroid)
	}
	if mp.Units != "mm" {
		t.Fatalf("expected units 'mm', got %q", mp.Units)
	}
	t.Logf("mass properties verified: volume=%.2f mm^3 area=%.2f mm^2 centroid=%+v",
		mp.Volume, mp.Area, mp.Centroid)

	// 4. Verify Bounding Box
	t.Log("verifying bounding box of block...")
	bbox, err := part.BoundingBox(ctx)
	if err != nil {
		t.Fatalf("BoundingBox failed: %v", err)
	}
	if math.Abs(bbox.Dimensions[0]-100.0) > 0.01 || math.Abs(bbox.Dimensions[1]-50.0) > 0.01 || math.Abs(bbox.Dimensions[2]-25.0) > 0.01 {
		t.Fatalf("expected bounding box dimensions [100, 50, 25], got %+v", bbox.Dimensions)
	}
	if bbox.Units != "mm" {
		t.Fatalf("expected bbox units 'mm', got %q", bbox.Units)
	}
	t.Logf("bounding box verified: dimensions=%+v min=%+v max=%+v units=%s",
		bbox.Dimensions, bbox.MinCorner, bbox.MaxCorner, bbox.Units)

	// 5. Create a Cylinder Feature (d=20, h=30)
	t.Log("creating cylinder feature d=20, h=30...")
	cylFeat, err := part.CreateCylinder(ctx, nxgo.CylinderParams{
		Origin:    nxgo.Point3D{200, 0, 0},
		Direction: nxgo.Vector3D{0, 0, 1},
		Diameter:  20,
		Height:    30,
	})
	if err != nil {
		t.Fatalf("CreateCylinder failed: %v", err)
	}
	t.Logf("cylinder created: name=%s feat_ref=%s body_ref=%s",
		cylFeat.Name, cylFeat.Ref.ObjectID, cylFeat.BodyRef.ObjectID)

	// 6. Query Bodies in Part
	t.Log("querying all bodies in part...")
	bodies, err := part.Bodies(ctx)
	if err != nil {
		t.Fatalf("Bodies query failed: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("expected 2 bodies in part, got %d", len(bodies))
	}
	for i, b := range bodies {
		t.Logf("body %d: ref=%s solid=%s faces=%d edges=%d",
			i+1, b.Ref.ObjectID, b.SolidType, b.FaceCount, b.EdgeCount)
		if b.FaceCount == 0 || b.EdgeCount == 0 {
			t.Fatalf("body %d has invalid geometry: faces=%d edges=%d", i+1, b.FaceCount, b.EdgeCount)
		}
	}

	// 7. Verify body-level mass properties for cylinder alone
	t.Log("verifying body-level mass properties of cylinder...")
	cylBody := bodies[1]
	cylMp, err := cylBody.MassProperties(ctx)
	if err != nil {
		t.Fatalf("Cylinder body MassProperties failed: %v", err)
	}
	expectedCylVol := math.Pi * 10.0 * 10.0 * 30.0 // ~9424.778 mm³
	expectedCylArea := 2.0*math.Pi*10.0*30.0 + 2.0*math.Pi*10.0*10.0 // ~2513.274 mm²
	if math.Abs(cylMp.Volume-expectedCylVol) > 1.0 {
		t.Fatalf("expected cylinder volume ~%.2f, got %.4f", expectedCylVol, cylMp.Volume)
	}
	if math.Abs(cylMp.Area-expectedCylArea) > 1.0 {
		t.Fatalf("expected cylinder area ~%.2f, got %.4f", expectedCylArea, cylMp.Area)
	}
	if math.Abs(cylMp.Centroid[0]-200.0) > 0.1 || math.Abs(cylMp.Centroid[1]-0.0) > 0.1 || math.Abs(cylMp.Centroid[2]-15.0) > 0.1 {
		t.Fatalf("expected cylinder centroid [200, 0, 15], got %+v", cylMp.Centroid)
	}
	t.Logf("cylinder body mass properties verified: volume=%.2f area=%.2f centroid=%+v",
		cylMp.Volume, cylMp.Area, cylMp.Centroid)

	// 8. Verify multi-body aggregation at Part level
	t.Log("verifying multi-body aggregate MassProperties at part level...")
	aggMp, err := part.MassProperties(ctx)
	if err != nil {
		t.Fatalf("aggregate MassProperties failed: %v", err)
	}
	expectedAggVol := expectedVolume + expectedCylVol
	expectedAggArea := expectedArea + expectedCylArea
	if math.Abs(aggMp.Volume-expectedAggVol) > 2.0 {
		t.Fatalf("expected aggregate volume ~%.2f, got %.4f", expectedAggVol, aggMp.Volume)
	}
	if math.Abs(aggMp.Area-expectedAggArea) > 2.0 {
		t.Fatalf("expected aggregate area ~%.2f, got %.4f", expectedAggArea, aggMp.Area)
	}
	expectedCentroidX := (expectedVolume*50.0 + expectedCylVol*200.0) / expectedAggVol
	if math.Abs(aggMp.Centroid[0]-expectedCentroidX) > 0.5 {
		t.Fatalf("expected aggregate centroid X ~%.2f, got %.4f", expectedCentroidX, aggMp.Centroid[0])
	}
	if aggMp.Units != "mm" {
		t.Fatalf("expected aggregate units 'mm', got %q", aggMp.Units)
	}
	t.Logf("multi-body aggregate mass properties verified: vol=%.2f area=%.2f centroid=%+v",
		aggMp.Volume, aggMp.Area, aggMp.Centroid)

	// 9. Verify multi-body aggregation for BoundingBox
	t.Log("verifying multi-body aggregate BoundingBox at part level...")
	aggBbox, err := part.BoundingBox(ctx)
	if err != nil {
		t.Fatalf("aggregate BoundingBox failed: %v", err)
	}
	// Block: [0, 100], [0, 50], [0, 25]
	// Cylinder: [190, 210], [-10, 10], [0, 30]
	// Combined: min=[0, -10, 0], max=[210, 50, 30], dim=[210, 60, 30]
	if math.Abs(aggBbox.MinCorner[0]-0.0) > 0.1 || math.Abs(aggBbox.MinCorner[1]-(-10.0)) > 0.1 || math.Abs(aggBbox.MinCorner[2]-0.0) > 0.1 {
		t.Fatalf("expected aggregate min corner [0, -10, 0], got %+v", aggBbox.MinCorner)
	}
	if math.Abs(aggBbox.MaxCorner[0]-210.0) > 0.1 || math.Abs(aggBbox.MaxCorner[1]-50.0) > 0.1 || math.Abs(aggBbox.MaxCorner[2]-30.0) > 0.1 {
		t.Fatalf("expected aggregate max corner [210, 50, 30], got %+v", aggBbox.MaxCorner)
	}
	if math.Abs(aggBbox.Dimensions[0]-210.0) > 0.1 || math.Abs(aggBbox.Dimensions[1]-60.0) > 0.1 || math.Abs(aggBbox.Dimensions[2]-30.0) > 0.1 {
		t.Fatalf("expected aggregate dimensions [210, 60, 30], got %+v", aggBbox.Dimensions)
	}
	if aggBbox.Units != "mm" {
		t.Fatalf("expected aggregate bbox units 'mm', got %q", aggBbox.Units)
	}
	t.Logf("multi-body aggregate bounding box verified: min=%+v max=%+v dim=%+v",
		aggBbox.MinCorner, aggBbox.MaxCorner, aggBbox.Dimensions)

	// 10. Save and Close
	_, _ = part.Save(ctx)
	_ = part.Close(ctx, false)
	_ = worker.Stop(ctx)
}

func TestRealNXImperialGeometryAndUnitsNormalization(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	worker, session := startTestWorker(t, ctx)
	defer worker.Kill()

	tempDir := t.TempDir()
	partFilePath := filepath.Join(tempDir, "nxgo_imperial_geo_test.prt")

	// 1. Create a new part in inches
	t.Log("creating new part in inches...")
	part, err := session.NewPart(ctx, partFilePath, "in")
	if err != nil {
		t.Fatalf("NewPart in inches failed: %v", err)
	}

	// 2. Create a 4 x 2 x 1 inch Block Feature
	t.Log("creating 4x2x1 inch block feature...")
	blockFeat, err := part.CreateBlock(ctx, nxgo.BlockParams{
		Origin: nxgo.Point3D{0, 0, 0},
		Length: 4,
		Width:  2,
		Height: 1,
	})
	if err != nil {
		t.Fatalf("CreateBlock in inches failed: %v", err)
	}
	if blockFeat.Ref.ObjectID == "" || blockFeat.BodyRef.ObjectID == "" {
		t.Fatalf("invalid block feature or body ref: %+v", blockFeat)
	}

	// 3. Verify single-body mass properties in inches
	t.Log("verifying mass properties of inch block...")
	mp, err := part.MassProperties(ctx)
	if err != nil {
		t.Fatalf("MassProperties in inches failed: %v", err)
	}
	t.Logf("inch mp returned: volume=%.4f area=%.4f mass=%.4f centroid=%+v units=%s",
		mp.Volume, mp.Area, mp.Mass, mp.Centroid, mp.Units)

	expectedVolume := 4.0 * 2.0 * 1.0 // 8.0 in³
	if math.Abs(mp.Volume-expectedVolume) > 0.05 {
		t.Fatalf("expected volume ~%.1f in^3, got %.4f", expectedVolume, mp.Volume)
	}

	expectedArea := 2.0 * (4.0*2.0 + 4.0*1.0 + 2.0*1.0) // 28.0 in²
	if math.Abs(mp.Area-expectedArea) > 0.05 {
		t.Fatalf("expected area ~%.1f in^2, got %.4f", expectedArea, mp.Area)
	}

	if math.Abs(mp.Centroid[0]-2.0) > 0.05 || math.Abs(mp.Centroid[1]-1.0) > 0.05 || math.Abs(mp.Centroid[2]-0.5) > 0.05 {
		t.Fatalf("expected centroid [2, 1, 0.5] in, got %+v", mp.Centroid)
	}
	if mp.Units != "inch" {
		t.Fatalf("expected units 'inch', got %q", mp.Units)
	}

	// 4. Verify single-body bounding box in inches
	t.Log("verifying bounding box of inch block...")
	bbox, err := part.BoundingBox(ctx)
	if err != nil {
		t.Fatalf("BoundingBox in inches failed: %v", err)
	}
	if math.Abs(bbox.Dimensions[0]-4.0) > 0.01 || math.Abs(bbox.Dimensions[1]-2.0) > 0.01 || math.Abs(bbox.Dimensions[2]-1.0) > 0.01 {
		t.Fatalf("expected bounding box dimensions [4, 2, 1], got %+v", bbox.Dimensions)
	}
	if bbox.Units != "inch" {
		t.Fatalf("expected bbox units 'inch', got %q", bbox.Units)
	}

	// 5. Create a Cylinder Feature in inches (d=1 in, h=2 in) at origin (10, 0, 0)
	t.Log("creating cylinder feature d=1, h=2 in inches...")
	cylFeat, err := part.CreateCylinder(ctx, nxgo.CylinderParams{
		Origin:    nxgo.Point3D{10, 0, 0},
		Direction: nxgo.Vector3D{0, 0, 1},
		Diameter:  1,
		Height:    2,
	})
	if err != nil {
		t.Fatalf("CreateCylinder in inches failed: %v", err)
	}
	t.Logf("cylinder created: name=%s feat_ref=%s", cylFeat.Name, cylFeat.Ref.ObjectID)

	// 6. Verify multi-body aggregation in inches
	t.Log("verifying multi-body aggregate in inches...")
	aggMp, err := part.MassProperties(ctx)
	if err != nil {
		t.Fatalf("aggregate MassProperties in inches failed: %v", err)
	}
	expectedCylVol := math.Pi * 0.5 * 0.5 * 2.0 // 0.5*pi ~ 1.570796 in³
	expectedCylArea := 2.0*math.Pi*0.5*2.0 + 2.0*math.Pi*0.25 // 2.5*pi ~ 7.85398 in²
	expectedAggVol := expectedVolume + expectedCylVol
	expectedAggArea := expectedArea + expectedCylArea
	if math.Abs(aggMp.Volume-expectedAggVol) > 0.1 {
		t.Fatalf("expected aggregate volume ~%.4f in^3, got %.4f", expectedAggVol, aggMp.Volume)
	}
	if math.Abs(aggMp.Area-expectedAggArea) > 0.1 {
		t.Fatalf("expected aggregate area ~%.4f in^2, got %.4f", expectedAggArea, aggMp.Area)
	}
	if aggMp.Units != "inch" {
		t.Fatalf("expected aggregate units 'inch', got %q", aggMp.Units)
	}

	// 7. Verify multi-body bounding box in inches
	aggBbox, err := part.BoundingBox(ctx)
	if err != nil {
		t.Fatalf("aggregate BoundingBox in inches failed: %v", err)
	}
	// Block: [0, 4], [0, 2], [0, 1]
	// Cylinder: center (10, 0, 0), radius 0.5, height 2 along Z => [9.5, 10.5], [-0.5, 0.5], [0, 2]
	// Combined: min=[0, -0.5, 0], max=[10.5, 2, 2], dim=[10.5, 2.5, 2]
	if math.Abs(aggBbox.MinCorner[0]-0.0) > 0.05 || math.Abs(aggBbox.MinCorner[1]-(-0.5)) > 0.05 || math.Abs(aggBbox.MinCorner[2]-0.0) > 0.05 {
		t.Fatalf("expected aggregate min corner [0, -0.5, 0], got %+v", aggBbox.MinCorner)
	}
	if math.Abs(aggBbox.MaxCorner[0]-10.5) > 0.05 || math.Abs(aggBbox.MaxCorner[1]-2.0) > 0.05 || math.Abs(aggBbox.MaxCorner[2]-2.0) > 0.05 {
		t.Fatalf("expected aggregate max corner [10.5, 2, 2], got %+v", aggBbox.MaxCorner)
	}
	if math.Abs(aggBbox.Dimensions[0]-10.5) > 0.05 || math.Abs(aggBbox.Dimensions[1]-2.5) > 0.05 || math.Abs(aggBbox.Dimensions[2]-2.0) > 0.05 {
		t.Fatalf("expected aggregate dimensions [10.5, 2.5, 2], got %+v", aggBbox.Dimensions)
	}
	if aggBbox.Units != "inch" {
		t.Fatalf("expected aggregate bbox units 'inch', got %q", aggBbox.Units)
	}
	t.Logf("imperial multi-body verified successfully: vol=%.4f area=%.4f bbox_dim=%+v",
		aggMp.Volume, aggMp.Area, aggBbox.Dimensions)

	_ = part.Close(ctx, false)
	_ = worker.Stop(ctx)
}

func TestRealNXPartCloseSaveFailureReadOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	worker, session := startTestWorker(t, ctx)
	defer worker.Kill()

	tempDir := t.TempDir()
	partFilePath := filepath.Join(tempDir, "nxgo_readonly_save_test.prt")

	// 1. Create a new part and save it to disk
	t.Log("creating and saving initial part...")
	part, err := session.NewPart(ctx, partFilePath, "mm")
	if err != nil {
		t.Fatalf("NewPart failed: %v", err)
	}
	saveResp, err := part.Save(ctx)
	if err != nil {
		t.Fatalf("initial part.Save failed: %v", err)
	}
	if !saveResp.Saved {
		t.Fatalf("expected Saved=true, got %+v", saveResp)
	}

	// 2. Modify the part by adding a block feature
	t.Log("modifying part with block feature...")
	_, err = part.CreateBlock(ctx, nxgo.BlockParams{
		Origin: nxgo.Point3D{0, 0, 0},
		Length: 20,
		Width:  20,
		Height: 20,
	})
	if err != nil {
		t.Fatalf("CreateBlock failed: %v", err)
	}

	// 3. Mark the saved file on disk as Read-Only
	t.Log("setting part file on disk to read-only...")
	if err := os.Chmod(partFilePath, 0400); err != nil {
		t.Fatalf("failed to set file read-only: %v", err)
	}
	defer func() {
		_ = os.Chmod(partFilePath, 0666)
	}()

	// 4. Attempt part.Close(save=true) - MUST FAIL and not swallow the save failure!
	t.Log("calling part.Close(save=true) on read-only file; expecting error...")
	err = part.Close(ctx, true)
	if err == nil {
		t.Fatal("expected part.Close(save=true) to fail on read-only file, but it succeeded!")
	}
	t.Logf("verified: part.Close(save=true) failed as expected: %v", err)

	// 5. Restore write permission and stop worker (faulted mutation safely quarantines worker)
	t.Log("restoring write permission and stopping worker...")
	_ = os.Chmod(partFilePath, 0666)
	_ = worker.Stop(ctx)
}

func TestRealNXTransactionRollbackOnFeatureCreation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	worker, session := startTestWorker(t, ctx)
	defer worker.Kill()

	tempDir := t.TempDir()
	partFilePath := filepath.Join(tempDir, "nxgo_tx_rollback_geo.prt")

	// 1. Create new part
	part, err := session.NewPart(ctx, partFilePath, "mm")
	if err != nil {
		t.Fatalf("NewPart failed: %v", err)
	}

	// 2. Begin Undo Transaction
	t.Log("beginning transaction before feature creation...")
	tx, err := session.BeginTx(ctx, "tx_block_rollback")
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	// 3. Create Block inside Transaction
	t.Log("creating block inside transaction...")
	_, err = part.CreateBlock(ctx, nxgo.BlockParams{
		Origin: nxgo.Point3D{0, 0, 0},
		Length: 50,
		Width:  50,
		Height: 50,
	})
	if err != nil {
		t.Fatalf("CreateBlock failed: %v", err)
	}

	// 4. Verify Body exists (count = 1)
	summary, err := part.Summary(ctx)
	if err != nil {
		t.Fatalf("Summary failed: %v", err)
	}
	if summary.BodyCount != 1 {
		t.Fatalf("expected body count 1, got %d", summary.BodyCount)
	}
	t.Log("block feature created; body count = 1")

	// 5. Rollback Transaction (Undo Mark)
	t.Log("rolling back transaction to undo mark...")
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	t.Log("rollback completed successfully")

	// 6. Verify Body and Feature rolled back (count = 0)
	summaryAfter, err := part.Summary(ctx)
	if err != nil {
		t.Fatalf("Summary after rollback failed: %v", err)
	}
	if summaryAfter.BodyCount != 0 {
		t.Fatalf("expected body count 0 after rollback, got %d", summaryAfter.BodyCount)
	}
	t.Log("verified body count returned to 0 after rollback!")

	_ = part.Close(ctx, false)
	_ = worker.Stop(ctx)
}
