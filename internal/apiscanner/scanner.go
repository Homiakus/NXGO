package apiscanner

import (
	"bytes"
	"context"
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
	Name       string         `json:"name"`
	Namespace  string         `json:"namespace"`
	Assembly   string         `json:"assembly"`
	Kind       string         `json:"kind"`
	Methods    []MethodInfo   `json:"methods,omitempty"`
	Properties []PropertyInfo `json:"properties,omitempty"`
}

type MethodInfo struct {
	Name       string      `json:"name"`
	ReturnType string      `json:"return_type"`
	Parameters []ParamInfo `json:"parameters,omitempty"`
	IsStatic   bool        `json:"is_static"`
}

type ParamInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type PropertyInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	CanRead  bool   `json:"can_read"`
	CanWrite bool   `json:"can_write"`
}

type APIDiffReport struct {
	ReleaseA       string   `json:"release_a"`
	ReleaseB       string   `json:"release_b"`
	AddedTypes     []string `json:"added_types,omitempty"`
	RemovedTypes   []string `json:"removed_types,omitempty"`
	AddedMethods   []string `json:"added_methods,omitempty"`
	RemovedMethods []string `json:"removed_methods,omitempty"`
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
                    $params += @{ name = $p.Name; type = $p.ParameterType.Name }
                }
                $methods += @{
                    name = $m.Name
                    return_type = $m.ReturnType.Name
                    parameters = $params
                    is_static = $m.IsStatic
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

            $result += @{
                name = $t.Name
                namespace = $t.Namespace
                assembly = $dll
                kind = $kind
                methods = $methods
                properties = $props
            }
        }
    } catch {
        Write-Error "failed scanning $dll - $_"
    }
}
$result | ConvertTo-Json -Depth 5 -Compress
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
		// Single object fallback
		var single TypeInfo
		if err2 := json.Unmarshal([]byte(outStr), &single); err2 == nil {
			rawTypes = []TypeInfo{single}
		} else {
			return nil, fmt.Errorf("failed to parse scanner JSON: %w", err)
		}
	}

	// Sort types, methods, properties lexicographically for determinism
	for i := range rawTypes {
		sort.Slice(rawTypes[i].Methods, func(a, b int) bool {
			if rawTypes[i].Methods[a].Name == rawTypes[i].Methods[b].Name {
				return len(rawTypes[i].Methods[a].Parameters) < len(rawTypes[i].Methods[b].Parameters)
			}
			return rawTypes[i].Methods[a].Name < rawTypes[i].Methods[b].Name
		})
		sort.Slice(rawTypes[i].Properties, func(a, b int) bool {
			return rawTypes[i].Properties[a].Name < rawTypes[i].Properties[b].Name
		})
	}

	sort.Slice(rawTypes, func(i, j int) bool {
		if rawTypes[i].Namespace == rawTypes[j].Namespace {
			return rawTypes[i].Name < rawTypes[j].Name
		}
		return rawTypes[i].Namespace < rawTypes[j].Namespace
	})

	return &APIManifest{
		Release:    filepath.Base(filepath.Dir(filepath.Dir(managedDir))),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Assemblies: assemblyNames,
		Types:      rawTypes,
	}, nil
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
			if strings.Contains(strings.ToLower(m.Name), q) {
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

		methodsA := make(map[string]bool)
		for _, m := range ta.Methods {
			methodsA[m.Name] = true
		}
		methodsB := make(map[string]bool)
		for _, m := range tb.Methods {
			methodsB[m.Name] = true
		}

		for m := range methodsB {
			if !methodsA[m] {
				report.AddedMethods = append(report.AddedMethods, k+"."+m)
			}
		}
		for m := range methodsA {
			if !methodsB[m] {
				report.RemovedMethods = append(report.RemovedMethods, k+"."+m)
			}
		}
	}

	sort.Strings(report.AddedTypes)
	sort.Strings(report.RemovedTypes)
	sort.Strings(report.AddedMethods)
	sort.Strings(report.RemovedMethods)

	return report
}
