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
var architectureRiskRE = regexp.MustCompile(`^RISK-ARCH-[0-9]{3}$`)

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

type riskScore struct {
	Severity   int `json:"severity"`
	Occurrence int `json:"occurrence"`
	Detection  int `json:"detection"`
	RPN        int `json:"rpn"`
}

type architectureRisk struct {
	ID                 string    `json:"id"`
	Title              string    `json:"title"`
	Category           string    `json:"category"`
	FailureMode        string    `json:"failure_mode"`
	Effect             string    `json:"effect"`
	Causes             []string  `json:"causes"`
	Controls           []string  `json:"controls"`
	Inherent           riskScore `json:"inherent"`
	Residual           riskScore `json:"residual"`
	Status             string    `json:"status"`
	Owner              string    `json:"owner"`
	PlanRefs           []string  `json:"plan_refs"`
	AuditRefs          []string  `json:"audit_refs,omitempty"`
	EvidenceRefs       []string  `json:"evidence_refs"`
	NextActions        []string  `json:"next_actions"`
	AcceptanceCriteria []string  `json:"acceptance_criteria"`
	ReviewTriggers     []string  `json:"review_triggers"`
	DecisionRef        string    `json:"decision_ref,omitempty"`
	LastReviewed       string    `json:"last_reviewed"`
}

type architectureRiskFile struct {
	Schema int                `json:"schema"`
	Risks  []architectureRisk `json:"risks"`
}

func main() {
	required := []string{
		"docs/ENGINEERING_STANDARD.md",
		"docs/TESTING_PLAYBOOK.md",
		"docs/DEFINITION_OF_DONE.md",
		"docs/ARCHITECTURE_FMEA.md",
		"docs/invariants/README.md",
		"docs/invariants/CANONICAL_SEMANTIC_UNITS.md",
		"docs/invariants/AUDIT_FINDINGS.md",
		"docs/TESTING.md",
		"docs/EXECUTABLE_QUALITY_GATES.md",
		"policy/invariant-compliance.json",
		"policy/nx-release-evidence.json",
		"policy/audit-findings.json",
		"policy/architecture-risks.json",
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
	if err := verifyAuditIndex(); err != nil {
		fatalf("audit finding index: %v", err)
	}
	riskCount, err := verifyArchitectureRisks()
	if err != nil {
		fatalf("architecture FMEA: %v", err)
	}
	if err := verifyPureGoBoundary(); err != nil {
		fatalf("pure-Go boundary: %v", err)
	}
	if err := verifyAgentSiemensBoundary(); err != nil {
		fatalf("Agent Siemens boundary: %v", err)
	}
	fmt.Printf("invariantcheck: PASS (%d invariant IDs, %d architecture risks, policy maps valid, Go/Agent dependency boundaries valid)\n", len(catalog), riskCount)
}

func verifyAuditIndex() error {
	registryBytes, err := os.ReadFile("policy/audit-findings.json")
	if err != nil {
		return err
	}
	var registry auditFindingFile
	if err := json.Unmarshal(registryBytes, &registry); err != nil {
		return fmt.Errorf("decode registry: %w", err)
	}
	indexBytes, err := os.ReadFile("docs/invariants/AUDIT_FINDINGS.md")
	if err != nil {
		return err
	}
	index := string(indexBytes)
	for _, finding := range registry.Findings {
		row := regexp.MustCompile(`(?m)^\| ` + regexp.QuoteMeta(finding.ID) + ` \| ([a-z_]+) \|`)
		match := row.FindStringSubmatch(index)
		if len(match) != 2 {
			return fmt.Errorf("audit index is missing %s", finding.ID)
		}
		if match[1] != finding.Status {
			return fmt.Errorf("audit index status for %s is %q, registry says %q", finding.ID, match[1], finding.Status)
		}
	}
	return nil
}

func verifyArchitectureRisks() (int, error) {
	b, err := os.ReadFile("policy/architecture-risks.json")
	if err != nil {
		return 0, err
	}
	var doc architectureRiskFile
	if err := json.Unmarshal(b, &doc); err != nil {
		return 0, fmt.Errorf("decode JSON: %w", err)
	}
	if doc.Schema != 1 {
		return 0, fmt.Errorf("unsupported schema %d", doc.Schema)
	}
	if len(doc.Risks) == 0 {
		return 0, fmt.Errorf("risk register is empty")
	}

	planBytes, err := os.ReadFile("MASTER_PLAN.md")
	if err != nil {
		return 0, err
	}
	plan := string(planBytes)
	docsBytes, err := os.ReadFile("docs/ARCHITECTURE_FMEA.md")
	if err != nil {
		return 0, err
	}
	docs := string(docsBytes)

	findingBytes, err := os.ReadFile("policy/audit-findings.json")
	if err != nil {
		return 0, err
	}
	var findings auditFindingFile
	if err := json.Unmarshal(findingBytes, &findings); err != nil {
		return 0, fmt.Errorf("decode audit finding registry: %w", err)
	}
	knownFindings := map[string]struct{}{}
	for _, finding := range findings.Findings {
		knownFindings[finding.ID] = struct{}{}
	}

	allowedStatus := map[string]bool{
		"open":       true,
		"mitigating": true,
		"watch":      true,
		"accepted":   true,
		"closed":     true,
	}
	activeStatus := map[string]bool{"open": true, "mitigating": true, "watch": true}
	seen := map[string]struct{}{}
	dateRE := regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

	for _, risk := range doc.Risks {
		if !architectureRiskRE.MatchString(risk.ID) {
			return 0, fmt.Errorf("invalid risk id %q", risk.ID)
		}
		if _, duplicate := seen[risk.ID]; duplicate {
			return 0, fmt.Errorf("duplicate risk id %q", risk.ID)
		}
		seen[risk.ID] = struct{}{}
		if strings.TrimSpace(risk.Title) == "" || strings.TrimSpace(risk.Category) == "" || strings.TrimSpace(risk.Owner) == "" {
			return 0, fmt.Errorf("%s requires title, category and owner", risk.ID)
		}
		if strings.TrimSpace(risk.FailureMode) == "" || strings.TrimSpace(risk.Effect) == "" {
			return 0, fmt.Errorf("%s requires failure_mode and effect", risk.ID)
		}
		if len(risk.Causes) == 0 || len(risk.Controls) == 0 {
			return 0, fmt.Errorf("%s requires causes and controls", risk.ID)
		}
		if !allowedStatus[risk.Status] {
			return 0, fmt.Errorf("%s has invalid status %q", risk.ID, risk.Status)
		}
		if err := verifyRiskScore(risk.ID+" inherent", risk.Inherent); err != nil {
			return 0, err
		}
		if err := verifyRiskScore(risk.ID+" residual", risk.Residual); err != nil {
			return 0, err
		}
		if risk.Residual.Severity != risk.Inherent.Severity {
			return 0, fmt.Errorf("%s changes severity from %d to %d; controls must not reduce consequence severity", risk.ID, risk.Inherent.Severity, risk.Residual.Severity)
		}
		if len(risk.PlanRefs) == 0 || len(risk.EvidenceRefs) == 0 || len(risk.AcceptanceCriteria) == 0 || len(risk.ReviewTriggers) == 0 {
			return 0, fmt.Errorf("%s requires plan_refs, evidence_refs, acceptance_criteria and review_triggers", risk.ID)
		}
		if (risk.Status == "open" || risk.Status == "mitigating") && len(risk.NextActions) == 0 {
			return 0, fmt.Errorf("%s is %s but has no next_actions", risk.ID, risk.Status)
		}
		if !dateRE.MatchString(risk.LastReviewed) {
			return 0, fmt.Errorf("%s has invalid last_reviewed %q", risk.ID, risk.LastReviewed)
		}
		for _, ref := range risk.EvidenceRefs {
			if strings.TrimSpace(ref) == "" {
				return 0, fmt.Errorf("%s contains an empty evidence_ref", risk.ID)
			}
			if _, err := os.Stat(ref); err != nil {
				return 0, fmt.Errorf("%s evidence_ref %q missing: %w", risk.ID, ref, err)
			}
		}
		for _, ref := range risk.AuditRefs {
			if _, ok := knownFindings[ref]; !ok {
				return 0, fmt.Errorf("%s references unknown audit finding %q", risk.ID, ref)
			}
		}
		if activeStatus[risk.Status] && !strings.Contains(plan, risk.ID) {
			return 0, fmt.Errorf("active risk %s is not explicitly tracked in MASTER_PLAN.md", risk.ID)
		}
		if !fmeaIndexHasStatus(docs, risk.ID, risk.Status) {
			return 0, fmt.Errorf("FMEA human index missing %s with status %s", risk.ID, risk.Status)
		}
		if (risk.Status == "accepted" || risk.Status == "closed") && (risk.Residual.RPN >= 150 || risk.Residual.Severity >= 9) && strings.TrimSpace(risk.DecisionRef) == "" {
			return 0, fmt.Errorf("%s is high-severity %s risk without decision_ref", risk.ID, risk.Status)
		}
	}
	return len(doc.Risks), nil
}

func verifyRiskScore(label string, score riskScore) error {
	if score.Severity < 1 || score.Severity > 10 || score.Occurrence < 1 || score.Occurrence > 10 || score.Detection < 1 || score.Detection > 10 {
		return fmt.Errorf("%s score dimensions must be within 1..10", label)
	}
	expected := score.Severity * score.Occurrence * score.Detection
	if score.RPN != expected {
		return fmt.Errorf("%s RPN is %d, expected %d", label, score.RPN, expected)
	}
	return nil
}

func fmeaIndexHasStatus(docs, id, status string) bool {
	for _, line := range strings.Split(docs, "\n") {
		if strings.Contains(line, "| "+id+" |") && strings.Contains(line, "| "+status+" |") {
			return true
		}
	}
	return false
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
