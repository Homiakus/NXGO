package main

import (
    "fmt"
    "os"
    "path/filepath"
    "regexp"
    "strings"
)

var invariantRE = regexp.MustCompile(`NXGO-INV-[A-Z]+-[0-9]{3}`)

func main() {
    required := []string{
        "docs/ENGINEERING_STANDARD.md",
        "docs/TESTING_PLAYBOOK.md",
        "docs/DEFINITION_OF_DONE.md",
        "docs/invariants/README.md",
        "docs/TESTING.md",
        "scripts/nx-real-smoke.ps1",
        ".github/workflows/fast.yml",
        ".github/workflows/nx-self-hosted.yml",
    }
    for _, path := range required {
        if _, err := os.Stat(path); err != nil { fatalf("required quality artifact missing: %s: %v", path, err) }
    }

    b, err := os.ReadFile("docs/invariants/README.md")
    if err != nil { fatalf("read invariant catalog: %v", err) }
    matches := invariantRE.FindAllString(string(b), -1)
    unique := map[string]struct{}{}
    for _, m := range matches { unique[m] = struct{}{} }
    if len(unique) < 40 { fatalf("expected at least 40 stable invariant IDs, found %d", len(unique)) }

    if err := verifyPureGoBoundary(); err != nil { fatalf("pure-Go boundary: %v", err) }
    fmt.Printf("invariantcheck: PASS (%d invariant IDs, required artifacts present)\n", len(unique))
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
