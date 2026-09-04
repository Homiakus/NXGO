package apiscanner_test

import (
	"strings"
	"testing"

	"github.com/Homiakus/NXGO/internal/apiscanner"
)

func sampleManifest() *apiscanner.APIManifest {
	return &apiscanner.APIManifest{
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
					{
						Name:       "Save",
						ReturnType: "PartSaveStatus",
						Parameters: []apiscanner.ParamInfo{
							{Name: "force", Type: "Boolean"},
						},
					},
					{
						Name:       "Close",
						ReturnType: "Void",
						Parameters: []apiscanner.ParamInfo{
							{Name: "save", Type: "Boolean"},
						},
					},
					{
						Name:       "Reopen",
						ReturnType: "Void",
						IsStatic:   true,
						Parameters: []apiscanner.ParamInfo{
							{Name: "path", Type: "String"},
						},
					},
				},
				Properties: []apiscanner.PropertyInfo{
					{Name: "FullPath", Type: "String", CanRead: true, CanWrite: false},
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
			{
				Name:      "INXObject",
				Namespace: "NXOpen",
				Assembly:  "NXOpen.dll",
				Kind:      "interface",
				Methods: []apiscanner.MethodInfo{
					{
						Name:       "GetTag",
						ReturnType: "Tag",
					},
				},
			},
		},
	}
}

func TestGenerateBindingsGoAndCSharp(t *testing.T) {
	manifest := sampleManifest()

	opts := apiscanner.GeneratorOptions{
		PackageName:    "nxopenraw",
		TargetRelease:  "v2512",
		GenerateGo:     true,
		GenerateCSharp: true,
	}

	out, err := apiscanner.GenerateBindings(manifest, opts)
	if err != nil {
		t.Fatalf("GenerateBindings failed: %v", err)
	}

	if out.TypesCount != 3 {
		t.Fatalf("expected 3 types, got %d", out.TypesCount)
	}

	// 1. Verify Go Types Source
	if !strings.Contains(out.GoTypesSource, "type Part struct") {
		t.Errorf("GoTypesSource missing 'type Part struct':\n%s", out.GoTypesSource)
	}
	if !strings.Contains(out.GoTypesSource, "type PartUnits int64") {
		t.Errorf("GoTypesSource missing 'type PartUnits int64':\n%s", out.GoTypesSource)
	}
	if !strings.Contains(out.GoTypesSource, "PartUnits_Millimeters PartUnits = 2") {
		t.Errorf("GoTypesSource missing enum value Millimeters:\n%s", out.GoTypesSource)
	}
	if !strings.Contains(out.GoTypesSource, "type INXObject interface") {
		t.Errorf("GoTypesSource missing 'type INXObject interface':\n%s", out.GoTypesSource)
	}

	// 2. Verify Go Methods Source
	if !strings.Contains(out.GoMethodsSource, "func (x Part) Save(") {
		t.Errorf("GoMethodsSource missing Part.Save method:\n%s", out.GoMethodsSource)
	}
	if !strings.Contains(out.GoMethodsSource, "func Part_Reopen(") {
		t.Errorf("GoMethodsSource missing static Part_Reopen func:\n%s", out.GoMethodsSource)
	}
	if !strings.Contains(out.GoMethodsSource, "invoker.InvokeRaw(ctx,") {
		t.Errorf("GoMethodsSource missing InvokeRaw call:\n%s", out.GoMethodsSource)
	}

	// 3. Verify Capability IDs and Symbol Provenance Tracing
	if len(out.Provenances) == 0 {
		t.Fatalf("expected non-empty provenances")
	}

	foundPartSave := false
	for _, p := range out.Provenances {
		if p.SymbolName == "Part.Save" {
			foundPartSave = true
			if !strings.HasPrefix(p.CapabilityID, "nxopen.part.save.") {
				t.Errorf("unexpected CapabilityID for Part.Save: %s", p.CapabilityID)
			}
			if p.Assembly != "NXOpen.dll" {
				t.Errorf("unexpected Assembly: %s", p.Assembly)
			}
			if p.Release != "v2512" {
				t.Errorf("unexpected Release: %s", p.Release)
			}
			if p.SignatureID == "" {
				t.Errorf("missing SignatureID for Part.Save")
			}
			// Verify provenance appears in Go method comment
			expectedComment := "// CapabilityID: " + p.CapabilityID
			if !strings.Contains(out.GoMethodsSource, expectedComment) {
				t.Errorf("GoMethodsSource missing capability comment %s", expectedComment)
			}
		}
	}
	if !foundPartSave {
		t.Errorf("provenance for Part.Save not found in %+v", out.Provenances)
	}

	// 4. Verify Go Registry Source
	if !strings.Contains(out.GoRegistrySource, "LookupCapability") {
		t.Errorf("GoRegistrySource missing LookupCapability function:\n%s", out.GoRegistrySource)
	}
	if !strings.Contains(out.GoRegistrySource, "AllProvenances = []SymbolProvenance") {
		t.Errorf("GoRegistrySource missing AllProvenances:\n%s", out.GoRegistrySource)
	}

	// 5. Verify C# Dispatch Glue
	if !strings.Contains(out.CSharpGlueSource, "namespace NXGO.Agent.Generated") {
		t.Errorf("CSharpGlueSource missing namespace:\n%s", out.CSharpGlueSource)
	}
	if !strings.Contains(out.CSharpGlueSource, "public static class GeneratedDispatcher") {
		t.Errorf("CSharpGlueSource missing GeneratedDispatcher class:\n%s", out.CSharpGlueSource)
	}
	if !strings.Contains(out.CSharpGlueSource, "targetObj.Save(arg_force)") {
		t.Errorf("CSharpGlueSource missing targetObj.Save call:\n%s", out.CSharpGlueSource)
	}
	if !strings.Contains(out.CSharpGlueSource, "NXOpen.Part.Reopen(arg_path)") {
		t.Errorf("CSharpGlueSource missing static NXOpen.Part.Reopen call:\n%s", out.CSharpGlueSource)
	}
}

func TestGenerateBindingsFiltering(t *testing.T) {
	manifest := sampleManifest()

	opts := apiscanner.GeneratorOptions{
		PackageName:  "nxopenraw",
		IncludeTypes: []string{"Part"},
	}

	out, err := apiscanner.GenerateBindings(manifest, opts)
	if err != nil {
		t.Fatalf("GenerateBindings failed: %v", err)
	}

	if out.TypesCount != 1 {
		t.Fatalf("expected 1 type with filter, got %d", out.TypesCount)
	}
	if !strings.Contains(out.GoTypesSource, "type Part struct") {
		t.Errorf("missing Part in filtered output")
	}
	if strings.Contains(out.GoTypesSource, "type PartUnits") {
		t.Errorf("PartUnits should have been filtered out")
	}
}

func TestGenerateCapabilitiesSummary(t *testing.T) {
	manifest := sampleManifest()
	caps := apiscanner.GenerateCapabilitiesSummary(manifest)
	if len(caps) != 4 { // Part.Save, Part.Close, Part.Reopen, INXObject.GetTag
		t.Fatalf("expected 4 capabilities, got %d: %+v", len(caps), caps)
	}
	for _, c := range caps {
		if !strings.HasPrefix(c, "nxopen.") {
			t.Errorf("capability %s does not start with nxopen.", c)
		}
	}
}
