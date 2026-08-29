package main

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "regexp"
    "strings"
)

var invariantRE = regexp.MustCompile(`NXGO-INV-[A-Z]+-[0-9]{3}`)

type complianceFile struct {
    Schema int `json:"schema"`
    Entries []complianceEntry `json:"entries"`
}

type complianceEntry struct {
    ID string `json:"id"`
    Status string `json:"status"`
    Mechanisms []string `json:"mechanisms"`
    Note string `json:"note,omitempty"`
}

func main() {
    required := []string{
        "docs/ENGINEERING_STANDARD.md",
        "docs/TESTING_PLAYBOOK.md",
        "docs/DEFINITION_OF_DONE.md",
        "docs/invariants/README.md",
        "docs/TESTING.md",
        "docs/EXECUTABLE_QUALITY_GATES.md",
        "policy/invariant-compliance.json",
        "scripts/nx-real-smoke.ps1",
        ".github/workflows/fast.yml",
        ".github/workflows/nx-self-hosted.yml",
    }
    for _, path := range required {
        if _, err := os.Stat(path); err != nil { fatalf("required quality artifact missing: %s: %v", path, err) }
    }

    catalogBytes, err := os.ReadFile("docs/invariants/README.md")
    if err != nil { fatalf("read invariant catalog: %v", err) }
    matches := invariantRE.FindAllString(string(catalogBytes), -1)
    catalog := map[string]struct{}{}
    for _, m := range matches { catalog[m] = struct{}{} }
    if len(catalog) < 40 { fatalf("expected at least 40 stable invariant IDs, found %d", len(catalog)) }

    if err := verifyCompliance(catalog); err != nil { fatalf("compliance map: %v", err) }
    if err := verifyPureGoBoundary(); err != nil { fatalf("pure-Go boundary: %v", err) }
    fmt.Printf("invariantcheck: PASS (%d invariant IDs, compliance map valid, required artifacts present)\n", len(catalog))
}

func verifyCompliance(catalog map[string]struct{}) error {
    b, err := os.ReadFile("policy/invariant-compliance.json")
    if err != nil { return err }
    var doc complianceFile
    if err := json.Unmarshal(b, &doc); err != nil { return fmt.Errorf("decode JSON: %w", err) }
    if doc.Schema != 1 { return fmt.Errorf("unsupported schema %d", doc.Schema) }

    allowed := map[string]bool{"enforced": true, "partially_enforced": true, "planned": true}
    seen := map[string]struct{}{}
    for _, e := range doc.Entries {
        if _, ok := catalog[e.ID]; !ok { return fmt.Errorf("unknown invariant %q", e.ID) }
        if _, dup := seen[e.ID]; dup { return fmt.Errorf("duplicate invariant %q", e.ID) }
        seen[e.ID] = struct{}{}
        if !allowed[e.Status] { return fmt.Errorf("%s has invalid status %q", e.ID, e.Status) }
        if e.Status == "enforced" && len(e.Mechanisms) == 0 { return fmt.Errorf("%s is enforced but has no mechanisms", e.ID) }
        for _, mechanism := range e.Mechanisms {
            if _, err := os.Stat(mechanism); err != nil { return fmt.Errorf("%s mechanism %q missing: %w", e.ID, mechanism, err) }
        }
    }
    return nil
}

func verifyPureGoBoundary() error {
    roots := []string{"sdk", "pkg"}
    for _, root := range roots {
        info, err := os.Stat(root)
        if err != nil {
            if os.IsNotExist(err) { continue }
            return err
        }
        if !info.IsDir() { continue }
        err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
            if err != nil { return err }
            if d.IsDir() || !strings.HasSuffix(path, ".go") { return nil }
            b, err := os.ReadFile(path)
            if err != nil { return err }
            s := string(b)
            if strings.Contains(s, `import "C"`) || strings.Contains(s, "NXOpen.dll") || strings.Contains(s, "NXOpen.UF.dll") {
                return fmt.Errorf("%s leaks Siemens/cgo dependency into public Go boundary", path)
            }
            return nil
        })
        if err != nil { return err }
    }
    return nil
}

func fatalf(format string, args ...any) {
    fmt.Fprintf(os.Stderr, "invariantcheck: FAIL: "+format+"\n", args...)
    os.Exit(1)
}
