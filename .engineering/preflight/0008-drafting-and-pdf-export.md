# Pre-flight: 0008-drafting-and-pdf-export

## Task
Implement Phase 8 Drafting & Drawing Export domain APIs (`drafting.create_sheet`, `drafting.export_pdf`, `drafting.query_sheets`) in `AgentWorker.cs`, protocol schemas in `internal/protocol/`, and Go client methods in `pkg/nxgo/` with live validation on Siemens NX 2512.

## Root Cause & Characterization
Downstream manufacturing and engineering workflows rely on production drawings (PDF, DXF, sheet definitions) generated from 3D models. Siemens NX provides `DrawingSheet` and `PrintPDFBuilder` capabilities which must be wrapped into safe, transactional `BuilderScope<T>` operations with opaque handles and verified output artifact generation.

## Invariants Maintained
- `NXGO-INV-MUT-001` / `002`: `PrintPDFBuilder` and `DrawingSheetBuilder` wrapped with `BuilderScope<T>` ensuring single commit and deterministic builder cleanup.
- `NXGO-INV-OBJ-001` / `002`: Drawing sheets and drafting views tracked via epoch/session-scoped opaque `ObjectHandleWire`.
- `NXGO-INV-COR-001`: Semantic postcondition validation (verifying PDF output existence, header validity, and non-empty byte size).
- `NXGO-INV-EXEC-001` / `002`: All drafting operations executed on bound NX main thread.

## Verification Ladder & Edge Space
1. Protocol envelope definitions in `internal/protocol/messages.go`.
2. Go client types and methods in `pkg/nxgo/drafting.go`.
3. Agent worker request handlers in `agent/bundle/AgentWorker.cs`.
4. Static check: `go vet ./...` and `go run ./cmd/invariantcheck`.
5. Real NX 2512 integration test in `tests/nx/drafting_test.go`:
   - Creates part with 3D geometry.
   - Creates A3 drawing sheet.
   - Exports drawing sheet to PDF.
   - Validates PDF file header `%PDF-` and file size > 1 KB on disk.
