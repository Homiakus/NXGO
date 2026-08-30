# Pre-flight: 0009-declarative-workflow-release-package

## Task
Implement Phase 9 Declarative CAD Workflows (`PrepareReleasePackage`, `ValidatePart`, `ValidateAssembly`, and `ReleaseManifest` generation) in `pkg/nxgo/workflow.go` with end-to-end verification against real Siemens NX 2512.

## Root Cause & Characterization
High-level CAD pipeline consumers and enterprise automations require one-shot declarative workflows to produce complete, audited release packages (validation reports, mass property proofs, manufacturing drawing PDFs, and release manifests) rather than orchestrating low-level imperative NX operations individually.

## Invariants Maintained
- `NXGO-INV-COR-001`: Postcondition verification (automated validation of 3D geometry mass properties, drawing sheet integrity, PDF artifact on disk, and release manifest checksums).
- `NXGO-INV-MEM-001` / `002`: Clean part closure and object handle disposal upon workflow completion.
- `NXGO-INV-EXEC-001` / `002`: Multi-step declarative pipelines execute strictly over verified bound NX agent session.

## Verification Ladder & Edge Space
1. Typed declarative schema in `pkg/nxgo/workflow.go`.
2. Static checks: `go vet ./...` and `go run ./cmd/invariantcheck`.
3. Real NX 2512 integration tests in `tests/nx/workflow_test.go`:
   - `TestRealNXDeclarativeReleasePackageWorkflow`:
     - Executes `PrepareReleasePackage` for a machined bracket.
     - Validates that `manifest.json`, `<part>.prt`, and `<part>_drawing.pdf` are generated with analytical mass properties and valid SHA-256 hashes.
   - `TestRealNXAssemblyValidationWorkflow`:
     - Executes `ValidateAssembly` on a multi-part fixture.
     - Validates component tree, prototype resolution, and BOM consistency.
