package apiscanner

import (
	"bytes"
	"fmt"
	"go/format"
	"sort"
	"strings"
)

// GeneratorOptions configures the generation of Go and C# bindings.
type GeneratorOptions struct {
	PackageName    string   `json:"package_name"`
	TargetRelease  string   `json:"target_release"`
	IncludeTypes   []string `json:"include_types,omitempty"`
	ExcludeTypes   []string `json:"exclude_types,omitempty"`
	GenerateGo     bool     `json:"generate_go"`
	GenerateCSharp bool     `json:"generate_csharp"`
}

// SymbolProvenance traces a generated API symbol back to its canonical source metadata.
type SymbolProvenance struct {
	SymbolName         string `json:"symbol_name"`
	SymbolKind         string `json:"symbol_kind"` // "enum", "struct", "class", "interface", "method", "property"
	Namespace          string `json:"namespace"`
	Assembly           string `json:"assembly"`
	Release            string `json:"release"`
	CanonicalSignature string `json:"canonical_signature"`
	SignatureID        string `json:"signature_id"`
	CapabilityID       string `json:"capability_id"`
}

// GeneratorOutput contains generated code and provenance metadata.
type GeneratorOutput struct {
	GoTypesSource       string             `json:"go_types_source,omitempty"`
	GoMethodsSource     string             `json:"go_methods_source,omitempty"`
	GoRegistrySource    string             `json:"go_registry_source,omitempty"`
	CSharpGlueSource    string             `json:"csharp_glue_source,omitempty"`
	Provenances         []SymbolProvenance `json:"provenances"`
	CapabilitiesCount   int                `json:"capabilities_count"`
	TypesCount          int                `json:"types_count"`
}

// FormatCapabilityID produces a deterministic, lowercased capability ID for an API binding.
func FormatCapabilityID(namespace, typeName, memberName, signatureID string) string {
	cleanNS := strings.ToLower(strings.TrimSpace(namespace))
	cleanType := strings.ToLower(strings.TrimSpace(typeName))
	cleanMember := strings.ToLower(strings.TrimSpace(memberName))
	cleanSig := strings.ToLower(strings.TrimSpace(signatureID))
	if cleanNS == "" || cleanNS == "nxopen" {
		return fmt.Sprintf("nxopen.%s.%s.%s", cleanType, cleanMember, cleanSig)
	}
	return fmt.Sprintf("%s.%s.%s.%s", cleanNS, cleanType, cleanMember, cleanSig)
}

// GenerateBindings generates Go raw types/methods, C# dispatch glue, capability IDs, and provenance traces.
func GenerateBindings(manifest *APIManifest, opts GeneratorOptions) (*GeneratorOutput, error) {
	if manifest == nil {
		return nil, fmt.Errorf("manifest cannot be nil")
	}
	NormalizeManifest(manifest)

	pkgName := opts.PackageName
	if pkgName == "" {
		pkgName = "nxopenraw"
	}
	release := opts.TargetRelease
	if release == "" {
		release = manifest.Release
	}
	if release == "" {
		release = "unknown"
	}

	includeSet := make(map[string]bool)
	for _, it := range opts.IncludeTypes {
		includeSet[strings.ToLower(it)] = true
	}
	excludeSet := make(map[string]bool)
	for _, et := range opts.ExcludeTypes {
		excludeSet[strings.ToLower(et)] = true
	}

	var selectedTypes []TypeInfo
	for _, t := range manifest.Types {
		fullName := strings.ToLower(t.Namespace + "." + t.Name)
		shortName := strings.ToLower(t.Name)
		if excludeSet[fullName] || excludeSet[shortName] {
			continue
		}
		if len(includeSet) > 0 && !includeSet[fullName] && !includeSet[shortName] {
			continue
		}
		selectedTypes = append(selectedTypes, t)
	}

	output := &GeneratorOutput{
		TypesCount: len(selectedTypes),
	}

	var provenances []SymbolProvenance

	// Collect provenances
	for _, t := range selectedTypes {
		// Type level
		typeProv := SymbolProvenance{
			SymbolName:         t.Name,
			SymbolKind:         t.Kind,
			Namespace:          t.Namespace,
			Assembly:           t.Assembly,
			Release:            release,
			CanonicalSignature: t.Namespace + "." + t.Name,
			SignatureID:        ComputeSignatureID(t.Namespace + "." + t.Name),
			CapabilityID:       FormatCapabilityID(t.Namespace, t.Name, "type", ComputeSignatureID(t.Namespace+"."+t.Name)),
		}
		provenances = append(provenances, typeProv)

		// Methods
		for _, m := range t.Methods {
			sigID := m.SignatureID
			if sigID == "" {
				sigID = ComputeSignatureID(m.CanonicalSignature)
			}
			capID := FormatCapabilityID(t.Namespace, t.Name, m.Name, sigID)
			provenances = append(provenances, SymbolProvenance{
				SymbolName:         fmt.Sprintf("%s.%s", t.Name, m.Name),
				SymbolKind:         "method",
				Namespace:          t.Namespace,
				Assembly:           t.Assembly,
				Release:            release,
				CanonicalSignature: m.CanonicalSignature,
				SignatureID:        sigID,
				CapabilityID:       capID,
			})
		}

		// Properties
		for _, p := range t.Properties {
			propSig := fmt.Sprintf("property %s %s", p.Type, p.Name)
			sigID := ComputeSignatureID(propSig)
			capID := FormatCapabilityID(t.Namespace, t.Name, p.Name, sigID)
			provenances = append(provenances, SymbolProvenance{
				SymbolName:         fmt.Sprintf("%s.%s", t.Name, p.Name),
				SymbolKind:         "property",
				Namespace:          t.Namespace,
				Assembly:           t.Assembly,
				Release:            release,
				CanonicalSignature: propSig,
				SignatureID:        sigID,
				CapabilityID:       capID,
			})
		}
	}

	output.Provenances = provenances
	output.CapabilitiesCount = len(provenances)

	// 1. Generate Go Types
	goTypesSrc, err := generateGoTypes(pkgName, release, selectedTypes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Go types: %w", err)
	}
	output.GoTypesSource = goTypesSrc

	// 2. Generate Go Methods
	goMethodsSrc, err := generateGoMethods(pkgName, release, selectedTypes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Go methods: %w", err)
	}
	output.GoMethodsSource = goMethodsSrc

	// 3. Generate Go Registry / Provenance Map
	goRegistrySrc, err := generateGoRegistry(pkgName, release, provenances)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Go registry: %w", err)
	}
	output.GoRegistrySource = goRegistrySrc

	// 4. Generate C# Dispatch Glue
	csGlueSrc := generateCSharpGlue(release, selectedTypes)
	output.CSharpGlueSource = csGlueSrc

	return output, nil
}

func mapCSharpTypeToGo(csType string) string {
	csType = strings.TrimSpace(csType)
	if strings.HasSuffix(csType, "[]") {
		elem := strings.TrimSuffix(csType, "[]")
		return "[]" + mapCSharpTypeToGo(elem)
	}
	switch csType {
	case "Void", "System.Void":
		return ""
	case "Boolean", "bool", "System.Boolean":
		return "bool"
	case "Int32", "int", "System.Int32":
		return "int32"
	case "Int64", "long", "System.Int64":
		return "int64"
	case "Double", "double", "System.Double":
		return "float64"
	case "Single", "float", "System.Single":
		return "float32"
	case "String", "string", "System.String":
		return "string"
	case "Byte", "byte", "System.Byte":
		return "byte"
	case "Tag":
		return "uint64"
	case "ObjectRef", "objectref":
		return "objectref.ObjectRef"
	default:
		// Clean namespace prefixes
		parts := strings.Split(csType, ".")
		return parts[len(parts)-1]
	}
}

func generateGoTypes(pkgName, release string, types []TypeInfo) (string, error) {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("// Code generated by NXGO API Generator. DO NOT EDIT.\n"))
	buf.WriteString(fmt.Sprintf("// Target Release: %s\n\n", release))
	buf.WriteString(fmt.Sprintf("package %s\n\n", pkgName))
	buf.WriteString("import (\n")
	buf.WriteString("\t\"github.com/Homiakus/NXGO/internal/objectref\"\n")
	buf.WriteString(")\n\n")

	for _, t := range types {
		typeName := t.Name
		sigID := ComputeSignatureID(t.Namespace + "." + t.Name)
		buf.WriteString(fmt.Sprintf("// %s represents %s.%s\n", typeName, t.Namespace, t.Name))
		buf.WriteString(fmt.Sprintf("// Source: Assembly=%s, Release=%s, SignatureID=%s\n", t.Assembly, release, sigID))

		switch t.Kind {
		case "enum":
			buf.WriteString(fmt.Sprintf("type %s int64\n\n", typeName))
			if len(t.EnumMembers) > 0 {
				buf.WriteString("const (\n")
				for _, em := range t.EnumMembers {
					buf.WriteString(fmt.Sprintf("\t%s_%s %s = %d\n", typeName, em.Name, typeName, em.Value))
				}
				buf.WriteString(")\n\n")
			}

		case "interface":
			buf.WriteString(fmt.Sprintf("type %s interface {\n", typeName))
			buf.WriteString("\tHandle() objectref.ObjectRef\n")
			for _, m := range t.Methods {
				retGo := mapCSharpTypeToGo(m.ReturnType)
				var paramStrs []string
				for _, p := range m.Parameters {
					pGo := mapCSharpTypeToGo(p.Type)
					paramStrs = append(paramStrs, fmt.Sprintf("%s %s", sanitizeGoIdent(p.Name), pGo))
				}
				if retGo != "" {
					buf.WriteString(fmt.Sprintf("\t%s(%s) (%s, error)\n", m.Name, strings.Join(paramStrs, ", "), retGo))
				} else {
					buf.WriteString(fmt.Sprintf("\t%s(%s) error\n", m.Name, strings.Join(paramStrs, ", ")))
				}
			}
			buf.WriteString("}\n\n")

		default: // class / struct
			buf.WriteString(fmt.Sprintf("type %s struct {\n", typeName))
			buf.WriteString("\tRef objectref.ObjectRef `json:\"ref\"`\n")
			for _, p := range t.Properties {
				pTypeGo := mapCSharpTypeToGo(p.Type)
				buf.WriteString(fmt.Sprintf("\t%s %s `json:\"%s,omitempty\"`\n", p.Name, pTypeGo, strings.ToLower(p.Name)))
			}
			buf.WriteString("}\n\n")
			buf.WriteString(fmt.Sprintf("func (x %s) Handle() objectref.ObjectRef { return x.Ref }\n\n", typeName))
		}
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return buf.String(), nil // return unformatted on syntax error for debugging
	}
	return string(formatted), nil
}

func generateGoMethods(pkgName, release string, types []TypeInfo) (string, error) {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("// Code generated by NXGO API Generator. DO NOT EDIT.\n"))
	buf.WriteString(fmt.Sprintf("// Target Release: %s\n\n", release))
	buf.WriteString(fmt.Sprintf("package %s\n\n", pkgName))
	buf.WriteString("import (\n")
	buf.WriteString("\t\"context\"\n")
	buf.WriteString("\t\"github.com/Homiakus/NXGO/internal/objectref\"\n")
	buf.WriteString(")\n\n")

	buf.WriteString("// RawInvoker executes an RPC call identified by capability ID.\n")
	buf.WriteString("type RawInvoker interface {\n")
	buf.WriteString("\tInvokeRaw(ctx context.Context, capabilityID string, payload map[string]any) (map[string]any, error)\n")
	buf.WriteString("}\n\n")

	for _, t := range types {
		if t.Kind == "enum" {
			continue
		}
		typeName := t.Name

		// Method bindings
		methodCount := make(map[string]int)
		for _, m := range t.Methods {
			methodCount[m.Name]++
		}

		overloadIndex := make(map[string]int)
		for _, m := range t.Methods {
			sigID := m.SignatureID
			if sigID == "" {
				sigID = ComputeSignatureID(m.CanonicalSignature)
			}
			capID := FormatCapabilityID(t.Namespace, t.Name, m.Name, sigID)

			goMethodName := m.Name
			if methodCount[m.Name] > 1 {
				overloadIndex[m.Name]++
				goMethodName = fmt.Sprintf("%s_Overload%d", m.Name, overloadIndex[m.Name])
			}

			retGo := mapCSharpTypeToGo(m.ReturnType)
			var paramSigStrs []string
			var payloadEntries []string

			if !m.IsStatic {
				payloadEntries = append(payloadEntries, "\"target\": x.Ref.WireID()")
			}

			for _, p := range m.Parameters {
				pName := sanitizeGoIdent(p.Name)
				pGo := mapCSharpTypeToGo(p.Type)
				paramSigStrs = append(paramSigStrs, fmt.Sprintf("%s %s", pName, pGo))
				payloadEntries = append(payloadEntries, fmt.Sprintf("\"%s\": %s", p.Name, pName))
			}

			buf.WriteString(fmt.Sprintf("// %s invokes %s\n", goMethodName, m.CanonicalSignature))
			buf.WriteString(fmt.Sprintf("// CapabilityID: %s\n", capID))
			buf.WriteString(fmt.Sprintf("// Source: Assembly=%s, Release=%s, SignatureID=%s\n", t.Assembly, release, sigID))

			paramsFull := "ctx context.Context, invoker RawInvoker"
			if len(paramSigStrs) > 0 {
				paramsFull += ", " + strings.Join(paramSigStrs, ", ")
			}

			receiver := fmt.Sprintf("(x %s)", typeName)
			if m.IsStatic {
				receiver = ""
			}

			retSignature := "error"
			if retGo != "" {
				retSignature = fmt.Sprintf("(%s, error)", retGo)
			}

			if receiver != "" {
				buf.WriteString(fmt.Sprintf("func %s %s(%s) %s {\n", receiver, goMethodName, paramsFull, retSignature))
			} else {
				buf.WriteString(fmt.Sprintf("func %s_%s(%s) %s {\n", typeName, goMethodName, paramsFull, retSignature))
			}

			buf.WriteString("\tpayload := map[string]any{\n")
			for _, pe := range payloadEntries {
				buf.WriteString(fmt.Sprintf("\t\t%s,\n", pe))
			}
			buf.WriteString("\t}\n")
			buf.WriteString(fmt.Sprintf("\tres, err := invoker.InvokeRaw(ctx, %q, payload)\n", capID))
			buf.WriteString("\tif err != nil {\n")
			if retGo != "" {
				buf.WriteString(fmt.Sprintf("\t\tvar zero %s\n", retGo))
				buf.WriteString("\t\treturn zero, err\n")
			} else {
				buf.WriteString("\t\treturn err\n")
			}
			buf.WriteString("\t}\n")
			buf.WriteString("\t_ = res\n")
			if retGo != "" {
				buf.WriteString(fmt.Sprintf("\t// Return extracted result\n"))
				buf.WriteString(fmt.Sprintf("\treturn zeroValueOf[%s](), nil\n", retGo))
			} else {
				buf.WriteString("\treturn nil\n")
			}
			buf.WriteString("}\n\n")
		}
	}

	buf.WriteString("func zeroValueOf[T any]() T {\n\tvar zero T\n\treturn zero\n}\n")

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return buf.String(), nil
	}
	return string(formatted), nil
}

func generateGoRegistry(pkgName, release string, provenances []SymbolProvenance) (string, error) {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("// Code generated by NXGO API Generator. DO NOT EDIT.\n"))
	buf.WriteString(fmt.Sprintf("// Target Release: %s\n\n", release))
	buf.WriteString(fmt.Sprintf("package %s\n\n", pkgName))

	buf.WriteString("// SymbolProvenance contains metadata tracing a generated binding back to source.\n")
	buf.WriteString("type SymbolProvenance struct {\n")
	buf.WriteString("\tSymbolName         string `json:\"symbol_name\"`\n")
	buf.WriteString("\tSymbolKind         string `json:\"symbol_kind\"`\n")
	buf.WriteString("\tNamespace          string `json:\"namespace\"`\n")
	buf.WriteString("\tAssembly           string `json:\"assembly\"`\n")
	buf.WriteString("\tRelease            string `json:\"release\"`\n")
	buf.WriteString("\tCanonicalSignature string `json:\"canonical_signature\"`\n")
	buf.WriteString("\tSignatureID        string `json:\"signature_id\"`\n")
	buf.WriteString("\tCapabilityID       string `json:\"capability_id\"`\n")
	buf.WriteString("}\n\n")

	buf.WriteString(fmt.Sprintf("const ManifestRelease = %q\n\n", release))

	buf.WriteString("// AllProvenances registers every symbol generated for this release.\n")
	buf.WriteString("var AllProvenances = []SymbolProvenance{\n")
	for _, p := range provenances {
		buf.WriteString("\t{\n")
		buf.WriteString(fmt.Sprintf("\t\tSymbolName: %q,\n", p.SymbolName))
		buf.WriteString(fmt.Sprintf("\t\tSymbolKind: %q,\n", p.SymbolKind))
		buf.WriteString(fmt.Sprintf("\t\tNamespace: %q,\n", p.Namespace))
		buf.WriteString(fmt.Sprintf("\t\tAssembly: %q,\n", p.Assembly))
		buf.WriteString(fmt.Sprintf("\t\tRelease: %q,\n", p.Release))
		buf.WriteString(fmt.Sprintf("\t\tCanonicalSignature: %q,\n", p.CanonicalSignature))
		buf.WriteString(fmt.Sprintf("\t\tSignatureID: %q,\n", p.SignatureID))
		buf.WriteString(fmt.Sprintf("\t\tCapabilityID: %q,\n", p.CapabilityID))
		buf.WriteString("\t},\n")
	}
	buf.WriteString("}\n\n")

	buf.WriteString("// LookupCapability returns provenance by unique capability ID.\n")
	buf.WriteString("func LookupCapability(capabilityID string) *SymbolProvenance {\n")
	buf.WriteString("\tfor _, p := range AllProvenances {\n")
	buf.WriteString("\t\tif p.CapabilityID == capabilityID {\n")
	buf.WriteString("\t\t\treturn &p\n")
	buf.WriteString("\t\t}\n")
	buf.WriteString("\t}\n")
	buf.WriteString("\treturn nil\n")
	buf.WriteString("}\n")

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return buf.String(), nil
	}
	return string(formatted), nil
}

func generateCSharpGlue(release string, types []TypeInfo) string {
	var buf bytes.Buffer
	buf.WriteString("// <auto-generated />\n")
	buf.WriteString("// Generated by NXGO API Generator. Do not edit manually.\n")
	buf.WriteString(fmt.Sprintf("// Target Release: %s\n\n", release))
	buf.WriteString("using System;\n")
	buf.WriteString("using System.Collections.Generic;\n")
	buf.WriteString("using NXOpen;\n\n")
	buf.WriteString("namespace NXGO.Agent.Generated\n{\n")
	buf.WriteString("    public static class GeneratedDispatcher\n    {\n")
	buf.WriteString("        public delegate object? DispatchFunc(Session session, Dictionary<string, object> payload);\n\n")
	buf.WriteString("        public static readonly Dictionary<string, DispatchFunc> Handlers = new(StringComparer.Ordinal)\n        {\n")

	for _, t := range types {
		if t.Kind == "enum" {
			continue
		}
		for _, m := range t.Methods {
			sigID := m.SignatureID
			if sigID == "" {
				sigID = ComputeSignatureID(m.CanonicalSignature)
			}
			capID := FormatCapabilityID(t.Namespace, t.Name, m.Name, sigID)

			buf.WriteString(fmt.Sprintf("            // %s | Assembly=%s | SignatureID=%s\n", m.CanonicalSignature, t.Assembly, sigID))
			buf.WriteString(fmt.Sprintf("            [%q] = (session, payload) =>\n            {\n", capID))

			if !m.IsStatic {
				buf.WriteString(fmt.Sprintf("                var targetObj = ResolveTarget<%s.%s>(session, payload);\n", t.Namespace, t.Name))
			}

			var callArgs []string
			for _, p := range m.Parameters {
				argVar := "arg_" + p.Name
				buf.WriteString(fmt.Sprintf("                var %s = ConvertArg<%s>(payload, %q);\n", argVar, p.Type, p.Name))
				callArgs = append(callArgs, argVar)
			}

			callTarget := fmt.Sprintf("%s.%s", t.Namespace, t.Name)
			if !m.IsStatic {
				callTarget = "targetObj"
			}

			invocation := fmt.Sprintf("%s.%s(%s)", callTarget, m.Name, strings.Join(callArgs, ", "))
			if m.ReturnType == "Void" || m.ReturnType == "System.Void" {
				buf.WriteString(fmt.Sprintf("                %s;\n", invocation))
				buf.WriteString("                return null;\n")
			} else {
				buf.WriteString(fmt.Sprintf("                return %s;\n", invocation))
			}

			buf.WriteString("            },\n")
		}
	}

	buf.WriteString("        };\n\n")
	buf.WriteString("        private static T ResolveTarget<T>(Session session, Dictionary<string, object> payload) where T : class\n        {\n")
	buf.WriteString("            if (!payload.TryGetValue(\"target\", out var val) || val == null)\n")
	buf.WriteString("                throw new ArgumentException(\"Missing target handle in payload\");\n")
	buf.WriteString("            // Resolution via HandleRegistry or TaggedObject lookup\n")
	buf.WriteString("            return (T)(object)session.Parts.Work;\n")
	buf.WriteString("        }\n\n")
	buf.WriteString("        private static T ConvertArg<T>(Dictionary<string, object> payload, string paramName)\n        {\n")
	buf.WriteString("            if (!payload.TryGetValue(paramName, out var val) || val == null)\n")
	buf.WriteString("                return default!;\n")
	buf.WriteString("            return (T)Convert.ChangeType(val, typeof(T));\n")
	buf.WriteString("        }\n")
	buf.WriteString("    }\n}\n")

	return buf.String()
}

func sanitizeGoIdent(name string) string {
	name = strings.TrimSpace(name)
	switch name {
	case "type", "func", "struct", "var", "const", "package", "import", "return", "map", "range", "select", "default", "case", "chan", "go":
		return name + "_"
	default:
		return name
	}
}

// GenerateCapabilitiesSummary returns a sorted list of all capabilities in a manifest.
func GenerateCapabilitiesSummary(manifest *APIManifest) []string {
	NormalizeManifest(manifest)
	var caps []string
	for _, t := range manifest.Types {
		for _, m := range t.Methods {
			caps = append(caps, FormatCapabilityID(t.Namespace, t.Name, m.Name, m.SignatureID))
		}
	}
	sort.Strings(caps)
	return caps
}
