package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Status string

const (
	StatusTODO       Status = "TODO"
	StatusReady      Status = "READY"
	StatusInProgress Status = "IN_PROGRESS"
	StatusVerifying  Status = "VERIFYING"
	StatusDone       Status = "DONE"
	StatusBlocked    Status = "BLOCKED"
	StatusSuperseded Status = "SUPERSEDED"
	StatusCancelled  Status = "CANCELLED"
)

func (s Status) Valid() bool {
	switch s {
	case StatusTODO, StatusReady, StatusInProgress, StatusVerifying, StatusDone, StatusBlocked, StatusSuperseded, StatusCancelled:
		return true
	default:
		return false
	}
}

type Task struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Phase        string   `json:"phase"`
	Status       Status   `json:"status"`
	Priority     string   `json:"priority,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
}

type Plan struct {
	CurrentPhase string          `json:"current_phase,omitempty"`
	Tasks        map[string]Task `json:"tasks"`
}

type Candidate struct {
	Task           Task `json:"task"`
	NeedsPromotion bool `json:"needs_promotion"`
}

type ContinuationDecision struct {
	Continue bool       `json:"continue"`
	Next     *Candidate `json:"next,omitempty"`
	Reason   string     `json:"reason"`
}

var (
	phaseHeading = regexp.MustCompile(`^##\s+Phase\s+([A-Za-z0-9_-]+)\s+(?:—|-)\s+(.+)$`)
	taskHeading  = regexp.MustCompile(`^###\s+(T-[0-9]+)\s+(?:—|-)\s+(.+)$`)
)

func Parse(markdown string) (*Plan, error) {
	result := &Plan{Tasks: map[string]Task{}}
	scanner := bufio.NewScanner(strings.NewReader(markdown))
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	currentPhase := ""
	currentTaskID := ""
	awaitCurrentPhaseValue := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "## Current phase" {
			awaitCurrentPhaseValue = true
			currentTaskID = ""
			continue
		}
		if awaitCurrentPhaseValue && line != "" {
			value := strings.Trim(line, "` ")
			if fields := strings.Fields(value); len(fields) > 0 {
				result.CurrentPhase = fields[0]
			}
			awaitCurrentPhaseValue = false
		}

		if match := phaseHeading.FindStringSubmatch(line); match != nil {
			currentPhase = match[1]
			currentTaskID = ""
			continue
		}
		if match := taskHeading.FindStringSubmatch(line); match != nil {
			id := match[1]
			if _, exists := result.Tasks[id]; exists {
				return nil, fmt.Errorf("duplicate task id %s", id)
			}
			result.Tasks[id] = Task{ID: id, Title: strings.TrimSpace(match[2]), Phase: currentPhase}
			currentTaskID = id
			continue
		}
		if currentTaskID == "" {
			continue
		}

		task := result.Tasks[currentTaskID]
		switch {
		case strings.HasPrefix(line, "Status:"):
			status := Status(strings.TrimSpace(strings.TrimPrefix(line, "Status:")))
			if !status.Valid() {
				return nil, fmt.Errorf("task %s has invalid status %q", task.ID, status)
			}
			task.Status = status
		case strings.HasPrefix(line, "Priority:"):
			task.Priority = strings.TrimSpace(strings.TrimPrefix(line, "Priority:"))
		case strings.HasPrefix(line, "Dependencies:"):
			task.Dependencies = parseDependencies(strings.TrimSpace(strings.TrimPrefix(line, "Dependencies:")))
		}
		result.Tasks[currentTaskID] = task
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan plan: %w", err)
	}
	if len(result.Tasks) == 0 {
		return nil, fmt.Errorf("plan contains no tasks")
	}
	for id, task := range result.Tasks {
		if task.Status == "" {
			return nil, fmt.Errorf("task %s has no Status field", id)
		}
		for _, dependency := range task.Dependencies {
			if _, ok := result.Tasks[dependency]; !ok {
				return nil, fmt.Errorf("task %s references unknown dependency %s", id, dependency)
			}
		}
	}
	return result, nil
}

func parseDependencies(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "none") {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func (p *Plan) dependenciesDone(task Task) bool {
	for _, dependency := range task.Dependencies {
		dep, ok := p.Tasks[dependency]
		if !ok || dep.Status != StatusDone {
			return false
		}
	}
	return true
}

func priorityRank(priority string) int {
	priority = strings.TrimSpace(strings.ToUpper(priority))
	if len(priority) < 2 || priority[0] != 'P' {
		return 1 << 30
	}
	n, err := strconv.Atoi(priority[1:])
	if err != nil || n < 0 {
		return 1 << 30
	}
	return n
}

func stateRank(status Status, promotable bool) int {
	switch status {
	case StatusInProgress:
		return 0
	case StatusVerifying:
		return 1
	case StatusReady:
		return 2
	case StatusTODO:
		if promotable {
			return 3
		}
	}
	return 1 << 30
}

func (p *Plan) SelectNext() (*Candidate, bool) {
	decision := p.ShouldContinue()
	if !decision.Continue || decision.Next == nil {
		return nil, false
	}
	return decision.Next, true
}

func (p *Plan) ShouldContinue() ContinuationDecision {
	type item struct {
		candidate Candidate
		sRank     int
		pRank     int
		phaseEq   int
		id        string
	}
	items := make([]item, 0)
	for id, task := range p.Tasks {
		if task.Status == StatusDone || task.Status == StatusBlocked || task.Status == StatusCancelled || task.Status == StatusSuperseded {
			continue
		}
		promotable := task.Status == StatusTODO && p.dependenciesDone(task)
		sRank := stateRank(task.Status, promotable)
		if sRank > 3 {
			continue
		}
		phaseEq := 1
		if p.CurrentPhase != "" && strings.EqualFold(task.Phase, p.CurrentPhase) {
			phaseEq = 0
		}
		items = append(items, item{
			candidate: Candidate{Task: task, NeedsPromotion: promotable},
			sRank:     sRank,
			pRank:     priorityRank(task.Priority),
			phaseEq:   phaseEq,
			id:        id,
		})
	}
	if len(items) == 0 {
		return ContinuationDecision{Continue: false, Reason: "no executable or promotable work remains"}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].sRank != items[j].sRank {
			return items[i].sRank < items[j].sRank
		}
		if items[i].phaseEq != items[j].phaseEq {
			return items[i].phaseEq < items[j].phaseEq
		}
		if items[i].pRank != items[j].pRank {
			return items[i].pRank < items[j].pRank
		}
		return items[i].id < items[j].id
	})
	next := items[0].candidate
	reason := fmt.Sprintf("selected active %s task", next.Task.Status)
	if next.NeedsPromotion {
		reason = "TODO work has all dependencies DONE and is promotable to READY"
	}
	return ContinuationDecision{Continue: true, Next: &next, Reason: reason}
}

func main() {
	planPath := "MASTER_PLAN.md"
	if custom := os.Getenv("AUTOLOOP_PLAN_PATH"); custom != "" {
		planPath = custom
	}

	data, err := os.ReadFile(planPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", planPath, err)
		os.Exit(1)
	}
	plan, err := Parse(string(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse %s: %v\n", planPath, err)
		os.Exit(1)
	}

	cmd := "status"
	if len(os.Args) >= 2 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "status":
		counts := make(map[string]int)
		for _, task := range plan.Tasks {
			counts[string(task.Status)]++
		}
		decision := plan.ShouldContinue()
		out := map[string]any{
			"current_phase":   plan.CurrentPhase,
			"status_counts":   counts,
			"task_count":      len(plan.Tasks),
			"should_continue": decision.Continue,
			"reason":          decision.Reason,
		}
		if decision.Next != nil {
			out["next_task_id"] = decision.Next.Task.ID
			out["needs_promotion"] = decision.Next.NeedsPromotion
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)

	case "next":
		candidate, ok := plan.SelectNext()
		if !ok {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(map[string]any{
				"found":  false,
				"reason": "no executable or promotable work remains",
			})
			return
		}
		reason := "selected executable task"
		if candidate.NeedsPromotion {
			reason = "selected TODO task whose dependencies are DONE; promote it to READY before execution"
		}
		out := map[string]any{
			"found":           true,
			"needs_promotion": candidate.NeedsPromotion,
			"reason":          reason,
			"task":            candidate.Task,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)

	case "packet":
		taskID := ""
		if len(os.Args) >= 3 {
			taskID = os.Args[2]
		}
		var task Task
		if taskID != "" {
			var ok bool
			task, ok = plan.Tasks[taskID]
			if !ok {
				fmt.Fprintf(os.Stderr, "task %s not found\n", taskID)
				os.Exit(1)
			}
		} else {
			candidate, ok := plan.SelectNext()
			if !ok {
				fmt.Fprintf(os.Stderr, "no next task available\n")
				os.Exit(1)
			}
			task = candidate.Task
		}
		out := map[string]any{
			"task": task,
			"revision": "HEAD",
			"risk": "R1",
			"context_budget_tokens": 4000,
			"invariants": []string{
				"NXGO-INV-D2-MODELING",
				"NXGO-INV-FAIL-CLOSED",
			},
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)

	case "set-status":
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "usage: autoloop set-status <task_id> <new_status>\n")
			os.Exit(1)
		}
		targetID := os.Args[2]
		newStatus := Status(os.Args[3])
		if !newStatus.Valid() {
			fmt.Fprintf(os.Stderr, "invalid status: %s\n", newStatus)
			os.Exit(1)
		}

		lines := strings.Split(string(data), "\n")
		targetHeading := fmt.Sprintf("### %s", targetID)
		inTargetTask := false
		replaced := false

		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "### ") {
				if strings.HasPrefix(trimmed, targetHeading) {
					inTargetTask = true
				} else if inTargetTask {
					break
				}
			}
			if inTargetTask && strings.HasPrefix(trimmed, "Status:") {
				lines[i] = fmt.Sprintf("Status: %s", newStatus)
				replaced = true
				break
			}
		}

		if !replaced {
			fmt.Fprintf(os.Stderr, "could not find Status field for %s\n", targetID)
			os.Exit(1)
		}

		if err := os.WriteFile(planPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", planPath, err)
			os.Exit(1)
		}
		fmt.Printf("Updated %s status to %s\n", targetID, newStatus)

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		os.Exit(1)
	}
}
