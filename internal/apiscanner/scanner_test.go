package apiscanner_test

import (
	"testing"

	"github.com/Homiakus/NXGO/internal/apiscanner"
)

func TestAPISearchAndInspect(t *testing.T) {
	manifest := &apiscanner.APIManifest{
		Release: "v2512",
		Types: []apiscanner.TypeInfo{
			{
				Name:      "Part",
				Namespace: "NXOpen",
				Assembly:  "NXOpen.dll",
				Kind:      "class",
				Methods: []apiscanner.MethodInfo{
					{Name: "Save", ReturnType: "PartSaveStatus"},
					{Name: "Close", ReturnType: "Void"},
				},
				Properties: []apiscanner.PropertyInfo{
					{Name: "FullPath", Type: "String", CanRead: true, CanWrite: false},
				},
			},
			{
				Name:      "BlockBuilder",
				Namespace: "NXOpen.Features",
				Assembly:  "NXOpen.dll",
				Kind:      "class",
				Methods: []apiscanner.MethodInfo{
					{Name: "CommitFeature", ReturnType: "Feature"},
				},
			},
		},
	}

	// 1. Search
	results := apiscanner.SearchTypes(manifest, "block")
	if len(results) != 1 || results[0].Name != "BlockBuilder" {
		t.Fatalf("expected 1 match BlockBuilder, got %+v", results)
	}

	methodResults := apiscanner.SearchTypes(manifest, "save")
	if len(methodResults) != 1 || methodResults[0].Name != "Part" {
		t.Fatalf("expected 1 match Part for method 'save', got %+v", methodResults)
	}

	// 2. Inspect
	info := apiscanner.InspectType(manifest, "Part")
	if info == nil || info.Name != "Part" {
		t.Fatalf("expected to find Part, got %+v", info)
	}
	if len(info.Methods) != 2 || len(info.Properties) != 1 {
		t.Fatalf("unexpected methods/props on Part: %+v", info)
	}
}

func TestAPIDiff(t *testing.T) {
	manifestA := &apiscanner.APIManifest{
		Release: "v2512",
		Types: []apiscanner.TypeInfo{
			{
				Name:      "Part",
				Namespace: "NXOpen",
				Methods: []apiscanner.MethodInfo{
					{Name: "Save", ReturnType: "Void"},
					{Name: "OldMethod", ReturnType: "Void"},
				},
			},
			{
				Name:      "DeprecatedBuilder",
				Namespace: "NXOpen.Features",
			},
		},
	}

	manifestB := &apiscanner.APIManifest{
		Release: "v2606",
		Types: []apiscanner.TypeInfo{
			{
				Name:      "Part",
				Namespace: "NXOpen",
				Methods: []apiscanner.MethodInfo{
					{Name: "Save", ReturnType: "Void"},
					{Name: "NewAsyncSave", ReturnType: "Task"},
				},
			},
			{
				Name:      "NewModernBuilder",
				Namespace: "NXOpen.Features",
			},
		},
	}

	diff := apiscanner.DiffManifests(manifestA, manifestB)
	if len(diff.AddedTypes) != 1 || diff.AddedTypes[0] != "NXOpen.Features.NewModernBuilder" {
		t.Fatalf("expected AddedType NewModernBuilder, got %+v", diff.AddedTypes)
	}
	if len(diff.RemovedTypes) != 1 || diff.RemovedTypes[0] != "NXOpen.Features.DeprecatedBuilder" {
		t.Fatalf("expected RemovedType DeprecatedBuilder, got %+v", diff.RemovedTypes)
	}
	if len(diff.AddedMethods) != 1 || diff.AddedMethods[0] != "NXOpen.Part.NewAsyncSave" {
		t.Fatalf("expected AddedMethod NewAsyncSave, got %+v", diff.AddedMethods)
	}
	if len(diff.RemovedMethods) != 1 || diff.RemovedMethods[0] != "NXOpen.Part.OldMethod" {
		t.Fatalf("expected RemovedMethod OldMethod, got %+v", diff.RemovedMethods)
	}
}
