package apiscanner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type APIManifest struct {
	Release    string     `json:"release"`
	Timestamp  string     `json:"timestamp"`
	Assemblies []string   `json:"assemblies"`
	Types      []TypeInfo `json:"types"`
}

type TypeInfo struct {
	Name        string           `json:"name"`
	Namespace   string           `json:"namespace"`
	Assembly    string           `json:"assembly"`
	Kind        string           `json:"kind"`
	BaseType    string           `json:"base_type,omitempty"`
	Interfaces  []string         `json:"interfaces,omitempty"`
	EnumMembers []EnumMemberInfo `json:"enum_members,omitempty"`
	Methods     []MethodInfo     `json:"methods,omitempty"`
	Properties  []PropertyInfo   `json:"properties,omitempty"`
}

type EnumMemberInfo struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

type MethodInfo struct {
	Name               string      `json:"name"`
	ReturnType         string      `json:"return_type"`
	Parameters         []ParamInfo `json:"parameters,omitempty"`
	IsStatic           bool        `json:"is_static"`
	GenericArity       int         `json:"generic_arity,omitempty"`
	CanonicalSignature string      `json:"canonical_signature"`
	SignatureID        string      `json:"signature_id"`
}

type ParamInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	IsByRef bool   `json:"is_by_ref,omitempty"`
	IsOut   bool   `json:"is_out,omitempty"`
}

type PropertyInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	CanRead  bool   `json:"can_read"`
	CanWrite bool   `json:"can_write"`
}

type ChangedOverload struct {
	TypeName       string `json:"type_name"`
	MethodName     string `json:"method_name"`
	OldSignature   string `json:"old_signature"`
	OldSignatureID string `json:"old_signature_id"`
	NewSignature   string `json:"new_signature"`
	NewSignatureID string `json:"new_signature_id"`
}

type APIDiffReport struct {
	ReleaseA         string            `json:"release_a"`
	ReleaseB         string            `json:"release_b"`
	AddedTypes       []string          `json:"added_types,omitempty"`
	RemovedTypes     []string          `json:"removed_types,omitempty"`
	AddedMethods     []string          `json:"added_methods,omitempty"`
	RemovedMethods   []string          `json:"removed_methods,omitempty"`
	ChangedOverloads []ChangedOverload `json:"changed_overloads,omitempty"`
}

func FormatCanonicalSignature(m MethodInfo) string {
	var b strings.Builder
	if m.IsStatic {
		b.WriteString("static ")
	}
	b.WriteString(m.Name)
	if m.GenericArity > 0 {
		fmt.Fprintf(&b, "<`%d>", m.GenericArity)
	}
	b.WriteString("(")
	for i, p := range m.Parameters {
		if i > 0 {
			b.WriteString(", ")
		}
		if p.IsOut {
			b.WriteString("out ")
		} else if p.IsByRef {
			b.WriteString("ref ")
		}
		b.WriteString(p.Type)
		if p.Name != "" {
			b.WriteString(" ")
			b.WriteString(p.Name)
		}
	}
	b.WriteString("): ")
	b.WriteString(m.ReturnType)
	return b.String()
}

func ComputeSignatureID(canonicalSig string) string {
	h := sha256.Sum256([]byte(canonicalSig))
	return hex.EncodeToString(h[:8])
}

func NormalizeManifest(manifest *APIManifest) {
	for i := range manifest.Types {
		t := &manifest.Types[i]
		for j := range t.Methods {
			m := &t.Methods[j]
			if m.CanonicalSignature == "" {
				m.CanonicalSignature = FormatCanonicalSignature(*m)
			}
			if m.SignatureID == "" {
				m.SignatureID = ComputeSignatureID(m.CanonicalSignature)
			}
		}

		sort.Slice(t.Methods, func(a, b int) bool {
			if t.Methods[a].Name == t.Methods[b].Name {
				return t.Methods[a].CanonicalSignature < t.Methods[b].CanonicalSignature
			}
			return t.Methods[a].Name < t.Methods[b].Name
		})

		sort.Slice(t.Properties, func(a, b int) bool {
			return t.Properties[a].Name < t.Properties[b].Name
		})

		sort.Slice(t.EnumMembers, func(a, b int) bool {
			return t.EnumMembers[a].Value < t.EnumMembers[b].Value
		})
	}

	sort.Slice(manifest.Types, func(i, j int) bool {
		if manifest.Types[i].Namespace == manifest.Types[j].Namespace {
			return manifest.Types[i].Name < manifest.Types[j].Name
		}
		return manifest.Types[i].Namespace < manifest.Types[j].Namespace
	})
}

func ScanManagedAssemblies(ctx context.Context, managedDir string, assemblyNames ...string) (*APIManifest, error) {
	if len(assemblyNames) == 0 {
		assemblyNames = []string{"NXOpen.dll", "NXOpen.UF.dll", "NXOpen.Utilities.dll"}
	}
	var asmQuotes []string
	for _, a := range assemblyNames {
		asmQuotes = append(asmQuotes, "'"+strings.ReplaceAll(a, "'", "''")+"'")
	}
	asmArrayStr := strings.Join(asmQuotes, ",")

	psScript := fmt.Sprintf(`
$ManagedDir = '%s'
$Assemblies = @(%s)

$result = @()
foreach ($dll in $Assemblies) {
    $fullPath = Join-Path $ManagedDir $dll
    if (-not (Test-Path $fullPath)) { continue }
    try {
        $asm = [System.Reflection.Assembly]::LoadFrom($fullPath)
        foreach ($t in $asm.GetExportedTypes()) {
            $methods = @()
            foreach ($m in $t.GetMethods([System.Reflection.BindingFlags]::Public -bor [System.Reflection.BindingFlags]::Instance -bor [System.Reflection.BindingFlags]::Static -bor [System.Reflection.BindingFlags]::DeclaredOnly)) {
                if ($m.IsSpecialName) { continue }
                $params = @()
                foreach ($p in $m.GetParameters()) {
                    $pType = $p.ParameterType
                    $isRef = $pType.IsByRef
                    $typeName = if ($isRef) { $pType.GetElementType().Name } else { $pType.Name }
                    $params += @{
                        name = $p.Name
                        type = $typeName
                        is_by_ref = $isRef
                        is_out = $p.IsOut
                    }
                }
                $genArity = 0
                if ($m.IsGenericMethod) {
                    $genArity = $m.GetGenericArguments().Length
                }
                $methods += @{
                    name = $m.Name
                    return_type = $m.ReturnType.Name
                    parameters = $params
                    is_static = $m.IsStatic
                    generic_arity = $genArity
                }
            }
            $props = @()
            foreach ($pr in $t.GetProperties([System.Reflection.BindingFlags]::Public -bor [System.Reflection.BindingFlags]::Instance -bor [System.Reflection.BindingFlags]::Static -bor [System.Reflection.BindingFlags]::DeclaredOnly)) {
                $props += @{
                    name = $pr.Name
                    type = $pr.PropertyType.Name
                    can_read = $pr.CanRead
                    can_write = $pr.CanWrite
                }
            }
            $kind = "class"
            if ($t.IsInterface) { $kind = "interface" }
            elseif ($t.IsEnum) { $kind = "enum" }
            elseif ($t.IsValueType) { $kind = "struct" }

            $baseType = ""
            if ($t.BaseType) { $baseType = $t.BaseType.FullName }

            $ifaces = @()
            foreach ($iface in $t.GetInterfaces()) {
                $ifaces += $iface.FullName
            }

            $enumMembers = @()
            if ($t.IsEnum) {
                $names = [System.Enum]::GetNames($t)
                $vals = [System.Enum]::GetValues($t)
                for ($idx = 0; $idx -lt $names.Length; $idx++) {
                    $enumMembers += @{
                        name = $names[$idx]
                        value = [int64]$vals[$idx]
                    }
                }
            }

            $result += @{
                name = $t.Name
                namespace = $t.Namespace
                assembly = $dll
                kind = $kind
                base_type = $baseType
                interfaces = $ifaces
                enum_members = $enumMembers
                methods = $methods
                properties = $props
            }
        }
    } catch {
        Write-Error "failed scanning $dll - $_"
    }
}
$result | ConvertTo-Json -Depth 6 -Compress
`, strings.ReplaceAll(managedDir, "'", "''"), asmArrayStr)

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("powershell scanner failed: %w (stderr: %s)", err, stderr.String())
	}

	outStr := strings.TrimSpace(stdout.String())
	if outStr == "" {
		return nil, fmt.Errorf("no output from assembly scanner (stderr: %s)", stderr.String())
	}

	var rawTypes []TypeInfo
	if err := json.Unmarshal([]byte(outStr), &rawTypes); err != nil {
		var single TypeInfo
		if err2 := json.Unmarshal([]byte(outStr), &single); err2 == nil {
			rawTypes = []TypeInfo{single}
		} else {
			return nil, fmt.Errorf("failed to parse scanner JSON: %w", err)
		}
	}

	manifest := &APIManifest{
		Release:    filepath.Base(filepath.Dir(filepath.Dir(managedDir))),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Assemblies: assemblyNames,
		Types:      rawTypes,
	}

	NormalizeManifest(manifest)
	return manifest, nil
}

func SearchTypes(manifest *APIManifest, query string) []TypeInfo {
	q := strings.ToLower(query)
	var matches []TypeInfo
	for _, t := range manifest.Types {
		if strings.Contains(strings.ToLower(t.Name), q) ||
			strings.Contains(strings.ToLower(t.Namespace), q) {
			matches = append(matches, t)
			continue
		}
		for _, m := range t.Methods {
			if strings.Contains(strings.ToLower(m.Name), q) ||
				strings.Contains(strings.ToLower(m.CanonicalSignature), q) {
				matches = append(matches, t)
				break
			}
		}
	}
	return matches
}

func InspectType(manifest *APIManifest, typeName string) *TypeInfo {
	q := strings.ToLower(typeName)
	for _, t := range manifest.Types {
		if strings.ToLower(t.Name) == q || strings.ToLower(t.Namespace+"."+t.Name) == q {
			return &t
		}
	}
	return nil
}

func DiffManifests(a, b *APIManifest) *APIDiffReport {
	NormalizeManifest(a)
	NormalizeManifest(b)

	report := &APIDiffReport{
		ReleaseA: a.Release,
		ReleaseB: b.Release,
	}

	typesA := make(map[string]TypeInfo)
	for _, t := range a.Types {
		typesA[t.Namespace+"."+t.Name] = t
	}

	typesB := make(map[string]TypeInfo)
	for _, t := range b.Types {
		typesB[t.Namespace+"."+t.Name] = t
	}

	for k := range typesB {
		if _, exists := typesA[k]; !exists {
			report.AddedTypes = append(report.AddedTypes, k)
		}
	}

	for k, ta := range typesA {
		tb, exists := typesB[k]
		if !exists {
			report.RemovedTypes = append(report.RemovedTypes, k)
			continue
		}

		methodsABySig := make(map[string]MethodInfo)
		methodsAByName := make(map[string][]MethodInfo)
		for _, m := range ta.Methods {
			methodsABySig[m.CanonicalSignature] = m
			methodsAByName[m.Name] = append(methodsAByName[m.Name], m)
		}

		methodsBBySig := make(map[string]MethodInfo)
		methodsBByName := make(map[string][]MethodInfo)
		for _, m := range tb.Methods {
			methodsBBySig[m.CanonicalSignature] = m
			methodsBByName[m.Name] = append(methodsBByName[m.Name], m)
		}

		// Detect added and changed methods
		for sig, mb := range methodsBBySig {
			if _, exactMatch := methodsABySig[sig]; !exactMatch {
				// Check if there are methods with the same name in A (changed overload)
				if oldList, hasName := methodsAByName[mb.Name]; hasName && len(oldList) > 0 {
					// Correlate with the closest old overload
					report.ChangedOverloads = append(report.ChangedOverloads, ChangedOverload{
						TypeName:       k,
						MethodName:     mb.Name,
						OldSignature:   oldList[0].CanonicalSignature,
						OldSignatureID: oldList[0].SignatureID,
						NewSignature:   mb.CanonicalSignature,
						NewSignatureID: mb.SignatureID,
					})
				} else {
					report.AddedMethods = append(report.AddedMethods, fmt.Sprintf("%s.%s [%s]", k, mb.CanonicalSignature, mb.SignatureID))
				}
			}
		}

		// Detect removed methods
		for sig, ma := range methodsABySig {
			if _, exactMatch := methodsBBySig[sig]; !exactMatch {
				// If not mapped as changed overload
				if _, hasName := methodsBByName[ma.Name]; !hasName {
					report.RemovedMethods = append(report.RemovedMethods, fmt.Sprintf("%s.%s [%s]", k, ma.CanonicalSignature, ma.SignatureID))
				}
			}
		}
	}

	sort.Strings(report.AddedTypes)
	sort.Strings(report.RemovedTypes)
	sort.Strings(report.AddedMethods)
	sort.Strings(report.RemovedMethods)
	sort.Slice(report.ChangedOverloads, func(i, j int) bool {
		if report.ChangedOverloads[i].TypeName == report.ChangedOverloads[j].TypeName {
			return report.ChangedOverloads[i].NewSignature < report.ChangedOverloads[j].NewSignature
		}
		return report.ChangedOverloads[i].TypeName < report.ChangedOverloads[j].TypeName
	})

	return report
}
