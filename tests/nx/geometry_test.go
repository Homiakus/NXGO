package nx_test

import (
	"context"
	"math"
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
	t.Logf("mp returned: volume=%.4f area=%.4f mass=%.4f centroid=%+v",
		mp.Volume, mp.Area, mp.Mass, mp.Centroid)

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
	t.Logf("bounding box verified: dimensions=%+v min=%+v max=%+v",
		bbox.Dimensions, bbox.MinCorner, bbox.MaxCorner)

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

	// 7. Save and Close
	_, _ = part.Save(ctx)
	_ = part.Close(ctx, false)
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
