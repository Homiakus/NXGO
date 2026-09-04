package nxgo

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestWorkflowEngineDryRun(t *testing.T) {
	engine := NewWorkflowEngine()
	ctx := context.Background()

	var events []WorkflowProgressEvent
	listener := func(ev WorkflowProgressEvent) {
		events = append(events, ev)
	}

	plan := WorkflowPlan{
		ID:     "plan-dryrun-1",
		Name:   "Dry Run Plan",
		DryRun: true,
		Steps: []WorkflowStep{
			{
				ID:            "step-1",
				Name:          "Open Part",
				MutationClass: MutationClassReadOnly,
			},
			{
				ID:            "step-2",
				Name:          "Create Extrude",
				MutationClass: MutationClassTransactional,
			},
		},
		ProgressListener: listener,
	}

	report, err := engine.ExecutePlan(ctx, nil, plan)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if !report.Success || report.CompletedSteps != 2 {
		t.Fatalf("unexpected dry run report: %+v", report)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 progress events, got %d", len(events))
	}
	for _, st := range report.StepReports {
		if st.Status != StepStatusSkipped {
			t.Fatalf("expected step status SKIPPED in dry-run, got %s", st.Status)
		}
	}
}

func TestWorkflowEngineSuccessWithProgress(t *testing.T) {
	engine := NewWorkflowEngine()
	ctx := context.Background()

	var events []WorkflowProgressEvent
	listener := func(ev WorkflowProgressEvent) {
		events = append(events, ev)
	}

	step1Executed := false
	step2Executed := false

	plan := WorkflowPlan{
		ID:   "plan-success-1",
		Name: "Successful Workflow",
		Steps: []WorkflowStep{
			{
				ID:            "step-1",
				Name:          "Read Geometry",
				MutationClass: MutationClassReadOnly,
				Execute: func(ctx context.Context, session *Session) (any, error) {
					step1Executed = true
					return "geometry-data", nil
				},
			},
			{
				ID:            "step-2",
				Name:          "Create Drawing",
				MutationClass: MutationClassDeterministicIdempotent,
				Execute: func(ctx context.Context, session *Session) (any, error) {
					step2Executed = true
					return "drawing-sheet-1", nil
				},
			},
		},
		ProgressListener: listener,
	}

	report, err := engine.ExecutePlan(ctx, nil, plan)
	if err != nil {
		t.Fatalf("workflow execution failed: %v", err)
	}
	if !report.Success || report.CompletedSteps != 2 || !step1Executed || !step2Executed {
		t.Fatalf("unexpected execution report: %+v", report)
	}
	if len(events) != 4 { // running + completed per step
		t.Fatalf("expected 4 progress events, got %d", len(events))
	}
}

func TestWorkflowEngineFailureAndCompensation(t *testing.T) {
	engine := NewWorkflowEngine()
	ctx := context.Background()

	compensated := false

	plan := WorkflowPlan{
		ID:   "plan-fail-comp-1",
		Name: "Failure & Compensation Workflow",
		Steps: []WorkflowStep{
			{
				ID:            "step-1",
				Name:          "Step with Compensation",
				MutationClass: MutationClassTransactional,
				Execute: func(ctx context.Context, session *Session) (any, error) {
					return "created-resource", nil
				},
				Compensate: func(ctx context.Context, session *Session, output any) error {
					if output == "created-resource" {
						compensated = true
					}
					return nil
				},
			},
			{
				ID:            "step-2",
				Name:          "Failing Step",
				MutationClass: MutationClassAmbiguousNonRetryable,
				Execute: func(ctx context.Context, session *Session) (any, error) {
					return nil, errors.New("simulated failure")
				},
			},
		},
	}

	report, err := engine.ExecutePlan(ctx, nil, plan)
	if err == nil {
		t.Fatalf("expected error from failing workflow, got nil")
	}
	if report.Success || !report.RollbackPerformed || !compensated {
		t.Fatalf("expected rollback and compensation performed, report: %+v, compensated: %v", report, compensated)
	}
	if len(report.CompensationReports) != 1 || !report.CompensationReports[0].Success {
		t.Fatalf("unexpected compensation reports: %+v", report.CompensationReports)
	}
}

func TestWorkflowRetryPlanner(t *testing.T) {
	rp := DefaultRetryPlanner()

	readStep := WorkflowStep{ID: "s1", MutationClass: MutationClassReadOnly}
	d1 := rp.Evaluate(readStep, errors.New("timeout"), 0)
	if !d1.Allowed {
		t.Fatalf("expected retry allowed for read-only attempt 0")
	}
	d2 := rp.Evaluate(readStep, errors.New("timeout"), 3)
	if d2.Allowed {
		t.Fatalf("expected retry denied after max attempts")
	}

	ambigStep := WorkflowStep{ID: "s2", MutationClass: MutationClassAmbiguousNonRetryable}
	d3 := rp.Evaluate(ambigStep, errors.New("pipe lost"), 0)
	if d3.Allowed {
		t.Fatalf("expected retry denied for ambiguous non-retryable mutation")
	}
}

func TestWorkflowCheckpointAndResume(t *testing.T) {
	engine := NewWorkflowEngine()
	ctx := context.Background()

	tmpDir := t.TempDir()
	checkpointPath := filepath.Join(tmpDir, "checkpoint.json")

	step1Count := 0
	step2Count := 0

	step1 := WorkflowStep{
		ID:            "step-1",
		Name:          "Step One",
		MutationClass: MutationClassDeterministicIdempotent,
		Execute: func(ctx context.Context, session *Session) (any, error) {
			step1Count++
			return "step-1-res", nil
		},
	}

	step2 := WorkflowStep{
		ID:            "step-2",
		Name:          "Step Two",
		MutationClass: MutationClassDeterministicIdempotent,
		Execute: func(ctx context.Context, session *Session) (any, error) {
			step2Count++
			return "step-2-res", nil
		},
	}

	// 1. Run step 1 only
	plan1 := WorkflowPlan{
		ID:             "wf-resume-test",
		Name:           "Resume Test",
		Steps:          []WorkflowStep{step1},
		CheckpointPath: checkpointPath,
	}

	r1, err := engine.ExecutePlan(ctx, nil, plan1)
	if err != nil || !r1.Success {
		t.Fatalf("plan1 failed: %v", err)
	}
	if step1Count != 1 {
		t.Fatalf("expected step1 run once, got %d", step1Count)
	}

	// 2. Resume plan with step1 and step2 (step1 should be skipped from checkpoint)
	plan2 := WorkflowPlan{
		ID:             "wf-resume-test",
		Name:           "Resume Test",
		Steps:          []WorkflowStep{step1, step2},
		CheckpointPath: checkpointPath,
	}

	r2, err := engine.ExecutePlan(ctx, nil, plan2)
	if err != nil || !r2.Success {
		t.Fatalf("plan2 failed: %v", err)
	}
	if step1Count != 1 {
		t.Fatalf("expected step1 skipped on resume, count was %d", step1Count)
	}
	if step2Count != 1 {
		t.Fatalf("expected step2 run once on resume, count was %d", step2Count)
	}
	if r2.CompletedSteps != 2 {
		t.Fatalf("expected 2 completed steps, got %d", r2.CompletedSteps)
	}
}
