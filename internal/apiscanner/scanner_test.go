package apiscanner_test

import (
	"strings"
	"testing"

	"github.com/Homiakus/NXGO/internal/apiscanner"
)

func TestAPISearchAndInspect(t *testing.T) {
	manifest := &apiscanner.APIManifest{
		Release: "v2512",
		Types: []apiscanner.TypeInfo{
			{
				Name:       "Part",
				Namespace:  "NXOpen",
				Assembly:   "NXOpen.dll",
				Kind:       "class",
				BaseType:   "NXOpen.NXObject",
				Interfaces: []string{"NXOpen.INXObject"},
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
			{
				Name:      "PartUnits",
				Namespace: "NXOpen",
				Assembly:  "NXOpen.dll",
				Kind:      "enum",
				EnumMembers: []apiscanner.EnumMemberInfo{
					{Name: "Inches", Value: 1},
					{Name: "Millimeters", Value: 2},
				},
			},
		},
	}

	apiscanner.NormalizeManifest(manifest)

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
	if info.BaseType != "NXOpen.NXObject" || len(info.Interfaces) != 1 {
		t.Fatalf("unexpected inheritance info: %+v", info)
	}

	// Verify signature generation
	saveMethod := info.Methods[1]
	if saveMethod.Name == "Close" {
		saveMethod = info.Methods[0]
	}
	if saveMethod.CanonicalSignature == "" || saveMethod.SignatureID == "" {
		t.Fatalf("expected computed canonical signature and id, got: %+v", saveMethod)
	}

	enumInfo := apiscanner.InspectType(manifest, "PartUnits")
	if enumInfo == nil || len(enumInfo.EnumMembers) != 2 {
		t.Fatalf("expected 2 enum members, got: %+v", enumInfo)
	}
}

func TestAPIDiffAndChangedOverloads(t *testing.T) {
	manifestA := &apiscanner.APIManifest{
		Release: "v2512",
		Types: []apiscanner.TypeInfo{
			{
				Name:      "Part",
				Namespace: "NXOpen",
				Methods: []apiscanner.MethodInfo{
					{
						Name:       "Save",
						ReturnType: "Void",
						Parameters: []apiscanner.ParamInfo{
							{Name: "force", Type: "Boolean"},
						},
					},
					{
						Name:       "OldMethod",
						ReturnType: "Void",
					},
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
					{
						Name:       "Save",
						ReturnType: "Void",
						Parameters: []apiscanner.ParamInfo{
							{Name: "force", Type: "Boolean"},
							{Name: "closeAfterSave", Type: "Boolean"},
						},
					},
					{
						Name:       "NewAsyncSave",
						ReturnType: "Task",
					},
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
	if len(diff.AddedMethods) != 1 || !strings.Contains(diff.AddedMethods[0], "NewAsyncSave") {
		t.Fatalf("expected AddedMethod NewAsyncSave, got %+v", diff.AddedMethods)
	}
	if len(diff.RemovedMethods) != 1 || !strings.Contains(diff.RemovedMethods[0], "OldMethod") {
		t.Fatalf("expected RemovedMethod OldMethod, got %+v", diff.RemovedMethods)
	}
	if len(diff.ChangedOverloads) != 1 {
		t.Fatalf("expected 1 ChangedOverload for Save, got %+v", diff.ChangedOverloads)
	}
	ch := diff.ChangedOverloads[0]
	if ch.MethodName != "Save" || ch.OldSignatureID == "" || ch.NewSignatureID == "" {
		t.Fatalf("unexpected ChangedOverload: %+v", ch)
	}
}
