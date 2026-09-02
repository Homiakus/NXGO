package nxgo

import (
	"math"
	"testing"
)

func TestAggregateMassPropertiesUsesAllBodies(t *testing.T) {
	got := aggregateMassProperties([]*MassProperties{
		{
			Volume:   100,
			Area:     60,
			Mass:     2,
			Centroid: Point3D{0, 0, 0},
		},
		{
			Volume:   300,
			Area:     140,
			Mass:     6,
			Centroid: Point3D{20, 10, 0},
		},
	})

	if got.Volume != 400 || got.Area != 200 || got.Mass != 8 {
		t.Fatalf("unexpected totals: %+v", got)
	}
	want := Point3D{15, 7.5, 0}
	for axis := range want {
		if math.Abs(got.Centroid[axis]-want[axis]) > 1e-12 {
			t.Fatalf("centroid axis %d: got %v want %v", axis, got.Centroid, want)
		}
	}
	if got.SolidType != "aggregate" {
		t.Fatalf("expected aggregate solid type, got %q", got.SolidType)
	}
}

func TestAggregateMassPropertiesFallsBackToVolumeWeightWhenMassUnavailable(t *testing.T) {
	got := aggregateMassProperties([]*MassProperties{
		{Volume: 1, Centroid: Point3D{0, 0, 0}},
		{Volume: 3, Centroid: Point3D{8, 0, 0}},
	})
	if math.Abs(got.Centroid[0]-6) > 1e-12 {
		t.Fatalf("expected volume-weighted centroid 6, got %+v", got.Centroid)
	}
}

func TestAggregateBoundingBoxesIncludesEveryBody(t *testing.T) {
	got := aggregateBoundingBoxes([]*BoundingBox{
		{
			MinCorner: Point3D{-1, 2, 3},
			MaxCorner: Point3D{4, 5, 6},
		},
		{
			MinCorner: Point3D{10, -2, 1},
			MaxCorner: Point3D{20, 8, 9},
		},
	})

	wantMin := Point3D{-1, -2, 1}
	wantMax := Point3D{20, 8, 9}
	wantDim := Point3D{21, 10, 8}
	if got.MinCorner != wantMin || got.MaxCorner != wantMax || got.Dimensions != wantDim {
		t.Fatalf("unexpected aggregate bounding box: %+v", got)
	}
}

func TestAggregateEmptyPartIsExplicit(t *testing.T) {
	mass := aggregateMassProperties(nil)
	if mass.SolidType != "empty" || mass.Volume != 0 || mass.Mass != 0 {
		t.Fatalf("unexpected empty mass properties: %+v", mass)
	}
	box := aggregateBoundingBoxes(nil)
	if box.MinCorner != (Point3D{}) || box.MaxCorner != (Point3D{}) || box.Dimensions != (Point3D{}) {
		t.Fatalf("unexpected empty bounding box: %+v", box)
	}
}
