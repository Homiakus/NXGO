package nx_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/pkg/nxgo"
)

func TestRealNXAssemblyComponentAddTreeAndBOM(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	worker, session := startTestWorker(t, ctx)
	defer worker.Kill()

	tempDir := t.TempDir()

	// 1. Create Sub-component A: Pin (cylinder d=10, h=40)
	pinPath := filepath.Join(tempDir, "pin.prt")
	t.Log("creating sub-part pin.prt...")
	pinPart, err := session.NewPart(ctx, pinPath, "mm")
	if err != nil {
		t.Fatalf("NewPart pin failed: %v", err)
	}
	_, err = pinPart.CreateCylinder(ctx, nxgo.CylinderParams{
		Origin:    nxgo.Point3D{0, 0, 0},
		Direction: nxgo.Vector3D{0, 0, 1},
		Diameter:  10,
		Height:    40,
	})
	if err != nil {
		t.Fatalf("CreateCylinder on pin failed: %v", err)
	}
	_, err = pinPart.Save(ctx)
	if err != nil {
		t.Fatalf("Save pin failed: %v", err)
	}
	_ = pinPart.Close(ctx, false)

	// 2. Create Sub-component B: Plate (block 100x100x10)
	platePath := filepath.Join(tempDir, "plate.prt")
	t.Log("creating sub-part plate.prt...")
	platePart, err := session.NewPart(ctx, platePath, "mm")
	if err != nil {
		t.Fatalf("NewPart plate failed: %v", err)
	}
	_, err = platePart.CreateBlock(ctx, nxgo.BlockParams{
		Origin: nxgo.Point3D{0, 0, 0},
		Length: 100,
		Width:  100,
		Height: 10,
	})
	if err != nil {
		t.Fatalf("CreateBlock on plate failed: %v", err)
	}
	_, err = platePart.Save(ctx)
	if err != nil {
		t.Fatalf("Save plate failed: %v", err)
	}
	_ = platePart.Close(ctx, false)

	// 3. Create Main Assembly Part
	assyPath := filepath.Join(tempDir, "fixture_assembly.prt")
	t.Log("creating main assembly fixture_assembly.prt...")
	assy, err := session.NewPart(ctx, assyPath, "mm")
	if err != nil {
		t.Fatalf("NewPart assembly failed: %v", err)
	}

	// 4. Add Plate Component at [0, 0, 0]
	t.Log("adding plate component to assembly...")
	compPlate, err := assy.AddComponent(ctx, nxgo.AddComponentParams{
		PartPath:      platePath,
		ComponentName: "BASE_PLATE",
		Origin:        nxgo.Point3D{0, 0, 0},
	})
	if err != nil {
		t.Fatalf("AddComponent plate failed: %v", err)
	}
	t.Logf("plate component added: name=%s handle=%s", compPlate.Name, compPlate.Ref.ObjectID)

	// 5. Add Pin 1 at [20, 20, 10]
	t.Log("adding pin component 1 to assembly...")
	compPin1, err := assy.AddComponent(ctx, nxgo.AddComponentParams{
		PartPath:      pinPath,
		ComponentName: "LOCATING_PIN_1",
		Origin:        nxgo.Point3D{20, 20, 10},
	})
	if err != nil {
		t.Fatalf("AddComponent pin 1 failed: %v", err)
	}
	t.Logf("pin 1 added: name=%s handle=%s", compPin1.Name, compPin1.Ref.ObjectID)

	// 6. Add Pin 2 at [80, 80, 10]
	t.Log("adding pin component 2 to assembly...")
	compPin2, err := assy.AddComponent(ctx, nxgo.AddComponentParams{
		PartPath:      pinPath,
		ComponentName: "LOCATING_PIN_2",
		Origin:        nxgo.Point3D{80, 80, 10},
	})
	if err != nil {
		t.Fatalf("AddComponent pin 2 failed: %v", err)
	}
	t.Logf("pin 2 added: name=%s handle=%s", compPin2.Name, compPin2.Ref.ObjectID)

	// 7. Query Assembly Component Tree
	t.Log("querying assembly component tree...")
	tree, err := assy.ComponentTree(ctx)
	if err != nil {
		t.Fatalf("ComponentTree failed: %v", err)
	}
	if len(tree.Children) != 3 {
		t.Fatalf("expected 3 top-level children in assembly tree, got %d", len(tree.Children))
	}
	for i, ch := range tree.Children {
		t.Logf("tree child %d: name=%s display=%s pos=%+v proto=%s",
			i+1, ch.Name, ch.DisplayName, ch.Position, ch.PrototypePath)
	}

	// 8. Query Assembly BOM Summary
	t.Log("querying assembly BOM...")
	bom, err := assy.BOM(ctx)
	if err != nil {
		t.Fatalf("BOM failed: %v", err)
	}
	if len(bom) != 2 {
		t.Fatalf("expected 2 unique BOM part items, got %d", len(bom))
	}

	var pinQty, plateQty int
	for _, item := range bom {
		t.Logf("BOM item: part=%s qty=%d components=%v", item.PartName, item.Quantity, item.ComponentNames)
		if strings.Contains(strings.ToLower(item.PartName), "pin") {
			pinQty = item.Quantity
		}
		if strings.Contains(strings.ToLower(item.PartName), "plate") {
			plateQty = item.Quantity
		}
	}
	if plateQty != 1 {
		t.Fatalf("expected plate quantity 1, got %d", plateQty)
	}
	if pinQty != 2 {
		t.Fatalf("expected pin quantity 2, got %d", pinQty)
	}
	t.Log("BOM quantities verified: 1x Plate, 2x Pin")

	// 9. Remove Component Pin 2
	t.Log("removing component LOCATING_PIN_2...")
	if err := compPin2.Remove(ctx); err != nil {
		t.Fatalf("Remove component failed: %v", err)
	}
	t.Log("component removed cleanly")

	// 10. Verify Tree after Removal (Children count = 2)
	treeAfter, err := assy.ComponentTree(ctx)
	if err != nil {
		t.Fatalf("ComponentTree after removal failed: %v", err)
	}
	if len(treeAfter.Children) != 2 {
		t.Fatalf("expected 2 children in assembly tree after removal, got %d", len(treeAfter.Children))
	}
	t.Log("tree verified after removal: exactly 2 components remain")

	// 11. Save and Close Assembly
	_, _ = assy.Save(ctx)
	_ = assy.Close(ctx, false)
	_ = worker.Stop(ctx)
}
