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
	Schema  int               `json:"schema"`
	Entries []complianceEntry `json:"entries"`
}

type complianceEntry struct {
	ID         string   `json:"id"`
	Status     string   `json:"status"`
	Mechanisms []string `json:"mechanisms"`
	Note       string   `json:"note,omitempty"`
}

type releaseEvidence struct {
	Release         string            `json:"release"`
	Evidence        map[string]string `json:"evidence"`
	FindingRefs     []string          `json:"finding_refs,omitempty"`
	ClaimsAllowed   []string          `json:"claims_allowed"`
	ClaimsForbidden []string          `json:"claims_forbidden"`
}

type releaseEvidenceFile struct {
	Schema   int               `json:"schema"`
	Releases []releaseEvidence `json:"releases"`
}

type auditFindingFile struct {
	Schema   int `json:"schema"`
	Findings []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"findings"`
}

func main() {
	required := []string{
		"docs/ENGINEERING_STANDARD.md",
		"docs/TESTING_PLAYBOOK.md",
		"docs/DEFINITION_OF_DONE.md",
		"docs/invariants/README.md",
		"docs/invariants/CANONICAL_SEMANTIC_UNITS.md",
		"docs/TESTING.md",
		"docs/EXECUTABLE_QUALITY_GATES.md",
		"policy/invariant-compliance.json",
		"policy/nx-release-evidence.json",
		"policy/audit-findings.json",
		"scripts/nx-real-smoke.ps1",
		"scripts/build-agent.ps1",
		"agent/NXGO.Agent.Core/NxExecutor.cs",
		"agent/NXGO.Agent.Core/BuilderScope.cs",
		"agent/NXGO.Agent.Core/NamedPipeRequestServer.cs",
		"agent/NXGO.Agent.Core.Tests/AgentCoreTests.cs",
		"agent/NXGO.Agent.NXHost/EntryPoint.cs",
		".github/workflows/fast.yml",
		".github/workflows/nx-self-hosted.yml",
	}
	for _, path := range required {
		if _, err := os.Stat(path); err != nil {
			fatalf("required quality artifact missing: %s: %v", path, err)
		}
	}

	catalogBytes, err := os.ReadFile("docs/invariants/README.md")
	if err != nil {
		fatalf("read invariant catalog: %v", err)
	}
	matches := invariantRE.FindAllString(string(catalogBytes), -1)
	catalog := map[string]struct{}{}
	for _, m := range matches {
		catalog[m] = struct{}{}
	}
	if len(catalog) < 40 {
		fatalf("expected at least 40 stable invariant IDs, found %d", len(catalog))
	}

	if err := verifyCompliance(catalog); err != nil {
		fatalf("compliance map: %v", err)
	}
	if err := verifyReleaseEvidence(); err != nil {
		fatalf("release evidence: %v", err)
	}
	if err := verifyPureGoBoundary(); err != nil {
		fatalf("pure-Go boundary: %v", err)
	}
	if err := verifyAgentSiemensBoundary(); err != nil {
		fatalf("Agent Siemens boundary: %v", err)
	}
	fmt.Printf("invariantcheck: PASS (%d invariant IDs, compliance map valid, Go/Agent dependency boundaries valid)\n", len(catalog))
}

func verifyReleaseEvidence() error {
	findingBytes, err := os.ReadFile("policy/audit-findings.json")
	if err != nil {
		return err
	}
	var findings auditFindingFile
	if err := json.Unmarshal(findingBytes, &findings); err != nil || findings.Schema != 1 {
		return fmt.Errorf("audit findings registry must use schema 1")
	}
	known := map[string]struct{}{}
	for _, finding := range findings.Findings {
		if !regexp.MustCompile(`^A-[0-9]{3}$`).MatchString(finding.ID) {
			return fmt.Errorf("audit findings registry has invalid id %q", finding.ID)
		}
		if finding.Status != "open" && finding.Status != "mitigated" {
			return fmt.Errorf("audit finding %s has invalid status %q", finding.ID, finding.Status)
		}
		if _, exists := known[finding.ID]; exists {
			return fmt.Errorf("audit findings registry contains duplicate id %q", finding.ID)
		}
		known[finding.ID] = struct{}{}
	}
	b, err := os.ReadFile("policy/nx-release-evidence.json")
	if err != nil {
		return err
	}
	var doc releaseEvidenceFile
	if err := json.Unmarshal(b, &doc); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	if doc.Schema != 1 || len(doc.Releases) == 0 {
		return fmt.Errorf("schema must be 1 with at least one release")
	}
	for _, release := range doc.Releases {
		if strings.TrimSpace(release.Release) == "" {
			return fmt.Errorf("release identifier is required")
		}
		if len(release.Evidence) == 0 {
			return fmt.Errorf("release %s has no evidence", release.Release)
		}
		for name, status := range release.Evidence {
			if strings.EqualFold(status, "failed") || strings.HasPrefix(strings.ToLower(status), "blocked_by_") {
				if len(release.FindingRefs) == 0 {
					return fmt.Errorf("release %s evidence %q has status %q without audit finding reference", release.Release, name, status)
				}
			}
		}
		for _, finding := range release.FindingRefs {
			if !regexp.MustCompile(`^A-[0-9]{3}$`).MatchString(finding) {
				return fmt.Errorf("release %s has invalid audit finding reference %q", release.Release, finding)
			}
			if _, ok := known[finding]; !ok {
				return fmt.Errorf("release %s references unknown audit finding %q", release.Release, finding)
			}
		}
		for _, claim := range release.ClaimsAllowed {
			lowerClaim := strings.ToLower(claim)
			if strings.Contains(lowerClaim, "production") || strings.Contains(lowerClaim, "compatibility") || strings.Contains(lowerClaim, "semantic") {
				matched := false
				for _, status := range release.Evidence {
					if strings.EqualFold(status, "passed") {
						matched = true
						break
					}
				}
				if !matched {
					return fmt.Errorf("release %s allows claim %q without passed evidence", release.Release, claim)
				}
			}
			for _, forbidden := range release.ClaimsForbidden {
				if claim == forbidden {
					return fmt.Errorf("release %s both allows and forbids claim %q", release.Release, claim)
				}
			}
		}
	}
	return nil
}

func verifyCompliance(catalog map[string]struct{}) error {
	b, err := os.ReadFile("policy/invariant-compliance.json")
	if err != nil {
		return err
	}
	var doc complianceFile
	if err := json.Unmarshal(b, &doc); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	if doc.Schema != 1 {
		return fmt.Errorf("unsupported schema %d", doc.Schema)
	}

	allowed := map[string]bool{"enforced": true, "partially_enforced": true, "planned": true}
	seen := map[string]struct{}{}
	for _, e := range doc.Entries {
		if _, ok := catalog[e.ID]; !ok {
			return fmt.Errorf("unknown invariant %q", e.ID)
		}
		if _, dup := seen[e.ID]; dup {
			return fmt.Errorf("duplicate invariant %q", e.ID)
		}
		seen[e.ID] = struct{}{}
		if !allowed[e.Status] {
			return fmt.Errorf("%s has invalid status %q", e.ID, e.Status)
		}
		if e.Status == "enforced" && len(e.Mechanisms) == 0 {
			return fmt.Errorf("%s is enforced but has no mechanisms", e.ID)
		}
		for _, mechanism := range e.Mechanisms {
			if _, err := os.Stat(mechanism); err != nil {
				return fmt.Errorf("%s mechanism %q missing: %w", e.ID, mechanism, err)
			}
		}
	}
	return nil
}

func verifyPureGoBoundary() error {
	roots := []string{"sdk", "pkg"}
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			s := string(b)
			if strings.Contains(s, `import "C"`) || strings.Contains(s, "NXOpen.dll") || strings.Contains(s, "NXOpen.UF.dll") {
				return fmt.Errorf("%s leaks Siemens/cgo dependency into public Go boundary", path)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func verifyAgentSiemensBoundary() error {
	root := "agent"
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		slashPath := filepath.ToSlash(path)
		if strings.HasPrefix(slashPath, "agent/NXGO.Agent.NXHost/") || strings.HasPrefix(slashPath, "agent/bundle/") {
			return nil
		}
		if !strings.HasSuffix(path, ".cs") && !strings.HasSuffix(path, ".csproj") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s := string(b)
		forbidden := []string{"using NXOpen", "<Reference Include=\"NXOpen", "NXOpen.dll", "NXOpen.UF.dll"}
		for _, marker := range forbidden {
			if strings.Contains(s, marker) {
				return fmt.Errorf("%s references Siemens NXOpen outside the approved NXHost boundary (%q)", path, marker)
			}
		}
		return nil
	})
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "invariantcheck: FAIL: "+format+"\n", args...)
	os.Exit(1)
}
