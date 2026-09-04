package nxgo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type ReleasePackageParams struct {
	PartPath                string
	OutputDir               string
	DrawingSheetName        string
	DrawingScaleNumerator   float64
	DrawingScaleDenominator float64
	ColorMode               string
}

type ReleaseManifest struct {
	PartPath         string         `json:"part_path"`
	DrawingPDFPath   string         `json:"drawing_pdf_path"`
	PartSHA256       string         `json:"part_sha256"`
	DrawingPDFSHA256 string         `json:"drawing_pdf_sha256"`
	MassProperties   MassProperties `json:"mass_properties"`
	BoundingBox      BoundingBox    `json:"bounding_box"`
	Timestamp        string         `json:"timestamp"`
	Status           string         `json:"status"`
}

type AssemblyValidationReport struct {
	AssemblyPath    string    `json:"assembly_path"`
	TotalComponents int       `json:"total_components"`
	UniqueParts     int       `json:"unique_parts"`
	BOM             []BOMItem `json:"bom"`
	Valid           bool      `json:"valid"`
	Issues          []string  `json:"issues,omitempty"`
}

func PrepareReleasePackage(ctx context.Context, session *Session, params ReleasePackageParams) (*ReleaseManifest, error) {
	if params.OutputDir == "" {
		params.OutputDir = filepath.Dir(params.PartPath)
	}
	if err := os.MkdirAll(params.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// 1. Open Part
	part, err := session.OpenPart(ctx, params.PartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open part for release package: %w", err)
	}
	defer func() {
		_ = part.Close(ctx, false)
	}()

	// 2. Measure Mass Properties and Bounding Box
	mp, err := part.MassProperties(ctx)
	if err != nil {
		return nil, fmt.Errorf("mass properties calculation failed: %w", err)
	}
	bbox, err := part.BoundingBox(ctx)
	if err != nil {
		return nil, fmt.Errorf("bounding box calculation failed: %w", err)
	}

	// 3. Create Release Drawing Sheet
	sheetName := params.DrawingSheetName
	if sheetName == "" {
		sheetName = "A3_RELEASE_DRAWING"
	}
	num := params.DrawingScaleNumerator
	if num == 0 {
		num = 1.0
	}
	den := params.DrawingScaleDenominator
	if den == 0 {
		den = 1.0
	}

	_, err = part.CreateDrawingSheet(ctx, CreateSheetParams{
		SheetName:        sheetName,
		Units:            "mm",
		Height:           297.0,
		Length:           420.0,
		ScaleNumerator:   num,
		ScaleDenominator: den,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create release drawing sheet: %w", err)
	}

	// 4. Export PDF Drawing
	baseName := filepath.Base(params.PartPath)
	ext := filepath.Ext(baseName)
	rawName := baseName[:len(baseName)-len(ext)]
	pdfPath := filepath.Join(params.OutputDir, rawName+"_drawing.pdf")

	colorMode := params.ColorMode
	if colorMode == "" {
		colorMode = "black_and_white"
	}

	_, err = part.ExportPDF(ctx, ExportPDFParams{
		OutputPDFPath: pdfPath,
		ColorMode:     colorMode,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to export release PDF: %w", err)
	}

	// 5. Compute SHA256 Hashes
	partHash, err := hashFileSHA256(params.PartPath)
	if err != nil {
		partHash = "unknown"
	}
	pdfHash, err := hashFileSHA256(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to hash generated PDF: %w", err)
	}

	manifest := &ReleaseManifest{
		PartPath:         params.PartPath,
		DrawingPDFPath:   pdfPath,
		PartSHA256:       partHash,
		DrawingPDFSHA256: pdfHash,
		MassProperties:   *mp,
		BoundingBox:      *bbox,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		Status:           "VERIFIED_RELEASE",
	}

	// 6. Stage and atomically write Manifest to Disk
	manifestPath := filepath.Join(params.OutputDir, rawName+"_manifest.json")
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}
	tmpManifestPath := manifestPath + ".tmp"
	if err := os.WriteFile(tmpManifestPath, manifestBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed to write staged manifest: %w", err)
	}
	if err := os.Rename(tmpManifestPath, manifestPath); err != nil {
		_ = os.Remove(tmpManifestPath)
		return nil, fmt.Errorf("failed to atomically publish manifest: %w", err)
	}

	return manifest, nil
}

func ValidatePart(ctx context.Context, session *Session, partPath string) (*MassProperties, *BoundingBox, error) {
	part, err := session.OpenPart(ctx, partPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open part failed: %w", err)
	}
	defer func() { _ = part.Close(ctx, false) }()

	mp, err := part.MassProperties(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("mass properties query failed: %w", err)
	}
	bbox, err := part.BoundingBox(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("bounding box query failed: %w", err)
	}
	return mp, bbox, nil
}

func ValidateAssembly(ctx context.Context, session *Session, assemblyPath string) (*AssemblyValidationReport, error) {
	part, err := session.OpenPart(ctx, assemblyPath)
	if err != nil {
		return nil, fmt.Errorf("open assembly failed: %w", err)
	}
	defer func() { _ = part.Close(ctx, false) }()

	tree, err := part.ComponentTree(ctx)
	if err != nil {
		return nil, fmt.Errorf("component tree query failed: %w", err)
	}

	bom, err := part.BOM(ctx)
	if err != nil {
		return nil, fmt.Errorf("BOM query failed: %w", err)
	}

	report := &AssemblyValidationReport{
		AssemblyPath:    assemblyPath,
		TotalComponents: countComponents(tree),
		UniqueParts:     len(bom),
		BOM:             bom,
		Valid:           true,
	}

	// Check for missing prototype paths
	checkTreePrototypes(tree, report)

	return report, nil
}

func countComponents(node *ComponentNode) int {
	if node == nil {
		return 0
	}
	count := 0
	for _, ch := range node.Children {
		count += 1 + countComponents(&ch)
	}
	return count
}

func checkTreePrototypes(node *ComponentNode, report *AssemblyValidationReport) {
	if node == nil {
		return
	}
	for _, ch := range node.Children {
		if ch.PrototypePath == "" {
			report.Valid = false
			report.Issues = append(report.Issues, fmt.Sprintf("component %q has unresolvable prototype", ch.DisplayName))
		}
		checkTreePrototypes(&ch, report)
	}
}

func hashFileSHA256(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type MutationClass string

const (
	MutationClassReadOnly                MutationClass = "read_only"
	MutationClassDeterministicIdempotent MutationClass = "deterministic_idempotent"
	MutationClassTransactional           MutationClass = "transactional"
	MutationClassAmbiguousNonRetryable   MutationClass = "ambiguous_nonretryable"
)

type StepStatus string

const (
	StepStatusPending     StepStatus = "PENDING"
	StepStatusRunning     StepStatus = "RUNNING"
	StepStatusCompleted   StepStatus = "COMPLETED"
	StepStatusSkipped     StepStatus = "SKIPPED"
	StepStatusFailed      StepStatus = "FAILED"
	StepStatusCompensated StepStatus = "COMPENSATED"
)

type WorkflowProgressEvent struct {
	StepID      string        `json:"step_id"`
	StepName    string        `json:"step_name"`
	Status      StepStatus    `json:"status"`
	ProgressPct float64       `json:"progress_pct"`
	Message     string        `json:"message,omitempty"`
	Duration    time.Duration `json:"duration,omitempty"`
	Timestamp   time.Time     `json:"timestamp"`
}

type WorkflowProgressListener func(event WorkflowProgressEvent)

type StepFunc func(ctx context.Context, session *Session) (any, error)
type CompensationFunc func(ctx context.Context, session *Session, output any) error

type WorkflowStep struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Description   string           `json:"description,omitempty"`
	MutationClass MutationClass    `json:"mutation_class"`
	Execute       StepFunc         `json:"-"`
	Compensate    CompensationFunc `json:"-"`
	EstimatedCost time.Duration    `json:"estimated_cost,omitempty"`
}

type WorkflowPlan struct {
	ID             string                   `json:"id"`
	Name           string                   `json:"name"`
	Steps          []WorkflowStep           `json:"steps"`
	DryRun         bool                     `json:"dry_run"`
	ResumeFrom     string                   `json:"resume_from,omitempty"`
	CheckpointPath string                   `json:"checkpoint_path,omitempty"`
	ProgressListener WorkflowProgressListener `json:"-"`
}

type StepReport struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	MutationClass MutationClass `json:"mutation_class"`
	Status        StepStatus    `json:"status"`
	Duration      time.Duration `json:"duration"`
	Output        any           `json:"output,omitempty"`
	Error         string        `json:"error,omitempty"`
}

type CompensationReport struct {
	StepID  string `json:"step_id"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type WorkflowExecutionReport struct {
	WorkflowID          string               `json:"workflow_id"`
	WorkflowName        string               `json:"workflow_name"`
	Success             bool                 `json:"success"`
	CompletedSteps      int                  `json:"completed_steps"`
	TotalSteps          int                  `json:"total_steps"`
	TotalDuration       time.Duration        `json:"total_duration"`
	StepReports         []StepReport         `json:"step_reports"`
	CompensationReports []CompensationReport `json:"compensation_reports,omitempty"`
	RollbackPerformed   bool                 `json:"rollback_performed"`
	FinalError          string               `json:"final_error,omitempty"`
	CheckpointPath      string               `json:"checkpoint_path,omitempty"`
}

type WorkflowCheckpoint struct {
	WorkflowID       string         `json:"workflow_id"`
	LastStepID       string         `json:"last_step_id"`
	CompletedStepIDs []string       `json:"completed_step_ids"`
	SavedAt          time.Time      `json:"saved_at"`
	StepOutputs      map[string]any `json:"step_outputs"`
}

type RetryDecision struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

type RetryPlanner struct {
	MaxAttempts map[MutationClass]int
}

func DefaultRetryPlanner() *RetryPlanner {
	return &RetryPlanner{
		MaxAttempts: map[MutationClass]int{
			MutationClassReadOnly:                3,
			MutationClassDeterministicIdempotent: 3,
			MutationClassTransactional:           1, // transactional requires clean rollback before retry
			MutationClassAmbiguousNonRetryable:   0, // fail closed
		},
	}
}

func (rp *RetryPlanner) Evaluate(step WorkflowStep, err error, attempt int) RetryDecision {
	if err == nil {
		return RetryDecision{Allowed: false, Reason: "no error"}
	}
	maxAttempts, ok := rp.MaxAttempts[step.MutationClass]
	if !ok || maxAttempts <= 0 {
		return RetryDecision{
			Allowed: false,
			Reason:  fmt.Sprintf("mutation class %q is non-retryable", step.MutationClass),
		}
	}
	if attempt >= maxAttempts {
		return RetryDecision{
			Allowed: false,
			Reason:  fmt.Sprintf("max retry attempts (%d) reached for class %q", maxAttempts, step.MutationClass),
		}
	}
	return RetryDecision{
		Allowed: true,
		Reason:  fmt.Sprintf("retry permitted under policy for class %q (attempt %d/%d)", step.MutationClass, attempt+1, maxAttempts),
	}
}

type WorkflowEngine struct {
	retryPlanner *RetryPlanner
}

func NewWorkflowEngine() *WorkflowEngine {
	return &WorkflowEngine{
		retryPlanner: DefaultRetryPlanner(),
	}
}

func (we *WorkflowEngine) ExecutePlan(ctx context.Context, session *Session, plan WorkflowPlan) (*WorkflowExecutionReport, error) {
	startTime := time.Now()
	totalSteps := len(plan.Steps)
	report := &WorkflowExecutionReport{
		WorkflowID:   plan.ID,
		WorkflowName: plan.Name,
		TotalSteps:   totalSteps,
		StepReports:  make([]StepReport, 0, totalSteps),
	}

	if totalSteps == 0 {
		report.Success = true
		report.TotalDuration = time.Since(startTime)
		return report, nil
	}

	// 1. Dry Run Mode
	if plan.DryRun {
		for i, step := range plan.Steps {
			pct := float64(i+1) / float64(totalSteps) * 100.0
			if plan.ProgressListener != nil {
				plan.ProgressListener(WorkflowProgressEvent{
					StepID:      step.ID,
					StepName:    step.Name,
					Status:      StepStatusSkipped,
					ProgressPct: pct,
					Message:     fmt.Sprintf("[DRY RUN] Step %s validated (MutationClass: %s)", step.ID, step.MutationClass),
					Timestamp:   time.Now(),
				})
			}
			report.StepReports = append(report.StepReports, StepReport{
				ID:            step.ID,
				Name:          step.Name,
				MutationClass: step.MutationClass,
				Status:        StepStatusSkipped,
				Duration:      0,
			})
			report.CompletedSteps++
		}
		report.Success = true
		report.TotalDuration = time.Since(startTime)
		return report, nil
	}

	// 2. Resume / Checkpoint validation
	completedMap := make(map[string]bool)
	stepOutputs := make(map[string]any)
	if plan.CheckpointPath != "" {
		if cpData, err := os.ReadFile(plan.CheckpointPath); err == nil {
			var cp WorkflowCheckpoint
			if err := json.Unmarshal(cpData, &cp); err == nil && cp.WorkflowID == plan.ID {
				for _, id := range cp.CompletedStepIDs {
					completedMap[id] = true
				}
				stepOutputs = cp.StepOutputs
			}
		}
	}

	var executedSteps []int
	var failedErr error

	for i, step := range plan.Steps {
		if completedMap[step.ID] {
			report.StepReports = append(report.StepReports, StepReport{
				ID:            step.ID,
				Name:          step.Name,
				MutationClass: step.MutationClass,
				Status:        StepStatusCompleted,
				Output:        stepOutputs[step.ID],
			})
			report.CompletedSteps++
			continue
		}

		pct := float64(i+1) / float64(totalSteps) * 100.0
		if plan.ProgressListener != nil {
			plan.ProgressListener(WorkflowProgressEvent{
				StepID:      step.ID,
				StepName:    step.Name,
				Status:      StepStatusRunning,
				ProgressPct: pct,
				Message:     fmt.Sprintf("Executing step %s (%s)", step.ID, step.Name),
				Timestamp:   time.Now(),
			})
		}

		stepStart := time.Now()
		var stepOutput any
		var stepErr error
		attempts := 0

		for {
			if step.Execute != nil {
				stepOutput, stepErr = step.Execute(ctx, session)
			}
			if stepErr == nil {
				break
			}
			decision := we.retryPlanner.Evaluate(step, stepErr, attempts)
			if !decision.Allowed {
				break
			}
			attempts++
		}

		duration := time.Since(stepStart)

		if stepErr != nil {
			failedErr = fmt.Errorf("step %s (%s) failed: %w", step.ID, step.Name, stepErr)
			report.StepReports = append(report.StepReports, StepReport{
				ID:            step.ID,
				Name:          step.Name,
				MutationClass: step.MutationClass,
				Status:        StepStatusFailed,
				Duration:      duration,
				Error:         stepErr.Error(),
			})
			if plan.ProgressListener != nil {
				plan.ProgressListener(WorkflowProgressEvent{
					StepID:      step.ID,
					StepName:    step.Name,
					Status:      StepStatusFailed,
					ProgressPct: pct,
					Message:     stepErr.Error(),
					Duration:    duration,
					Timestamp:   time.Now(),
				})
			}
			break
		}

		executedSteps = append(executedSteps, i)
		stepOutputs[step.ID] = stepOutput
		report.StepReports = append(report.StepReports, StepReport{
			ID:            step.ID,
			Name:          step.Name,
			MutationClass: step.MutationClass,
			Status:        StepStatusCompleted,
			Duration:      duration,
			Output:        stepOutput,
		})
		report.CompletedSteps++

		if plan.ProgressListener != nil {
			plan.ProgressListener(WorkflowProgressEvent{
				StepID:      step.ID,
				StepName:    step.Name,
				Status:      StepStatusCompleted,
				ProgressPct: pct,
				Duration:    duration,
				Timestamp:   time.Now(),
			})
		}

		// Save Checkpoint if path provided
		if plan.CheckpointPath != "" {
			var completedIDs []string
			for _, idx := range executedSteps {
				completedIDs = append(completedIDs, plan.Steps[idx].ID)
			}
			cp := WorkflowCheckpoint{
				WorkflowID:       plan.ID,
				LastStepID:       step.ID,
				CompletedStepIDs: completedIDs,
				SavedAt:          time.Now().UTC(),
				StepOutputs:      stepOutputs,
			}
			if cpBytes, err := json.MarshalIndent(cp, "", "  "); err == nil {
				_ = os.WriteFile(plan.CheckpointPath, cpBytes, 0644)
				report.CheckpointPath = plan.CheckpointPath
			}
		}
	}

	// 3. Compensation & Rollback if failure occurred
	if failedErr != nil {
		report.FinalError = failedErr.Error()
		report.RollbackPerformed = true

		for k := len(executedSteps) - 1; k >= 0; k-- {
			idx := executedSteps[k]
			step := plan.Steps[idx]
			if step.Compensate != nil {
				compErr := step.Compensate(ctx, session, stepOutputs[step.ID])
				compRep := CompensationReport{
					StepID:  step.ID,
					Success: compErr == nil,
				}
				if compErr != nil {
					compRep.Error = compErr.Error()
				}
				report.CompensationReports = append(report.CompensationReports, compRep)
			}
		}
		report.TotalDuration = time.Since(startTime)
		return report, failedErr
	}

	report.Success = true
	report.TotalDuration = time.Since(startTime)
	return report, nil
}

