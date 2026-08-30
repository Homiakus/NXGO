# Pre-flight: 0005-object-registry-transactions-and-part-ops

## Task
Implement Phase 5 (Object Registry & Undo-Mark Transaction Manager) and Phase 7 (Part Basic Operations: new, open, save, close, query_summary) in `agent/bundle/AgentWorker.cs`, `internal/protocol/`, and `nxgo` client layer with comprehensive integration tests against real Siemens NX 2512.

## Root Cause & Characterization
Automating CAD operations requires robust handle lifetime tracking, transaction rollback safety (NX Undo Marks), and fail-closed session isolation. Siemens NX objects (tagged objects) cannot be exposed directly to Go client code; instead, opaque, session- and epoch-bound `ObjectHandleWire` references are used. When CAD mutations fail or are aborted, NX undo marks guarantee rollback to a clean state or mark the session as `Dirty`/`Poisoned`.

## Invariants Maintained
- `NXGO-INV-OBJ-001` / `002`: Opaque `ObjectRef` with `SessionID`, `Epoch`, `ObjectID`, `NativeTag`. Validation rejects stale handles across epochs or sessions.
- `NXGO-INV-SES-001` / `002`: Session health transitions; errors categorized with recoverability and session status.
- `NXGO-INV-EXEC-001` / `002`: All NXOpen calls (undo marks, part operations) strictly execute on the bound NX main thread via `NxExecutor`.
- `NXGO-INV-MUT-001` / `002`: `BuilderScope<T>` guarantees single-attempt commit and unconditional builder destruction.
- `NXGO-INV-MEM-001` / `002` / `003`: Memory and handle lifecycle management. Unconditional cleanup of temporary NX builders, part descriptors, and undo marks.
- `NXGO-INV-IPC-001`..`004`: Pure-Go typed envelopes, context cancellation, and idempotent request handling.

## Protected Surfaces
- Pure-Go client boundary: `CGO_ENABLED=0`, no proprietary Siemens DLLs or cgo bindings.
- Opaque wire protocol: Go client receives only handles, never raw pointers or C++ tags directly.

## Verification Ladder & Edge Space
1. Protocol schemas and Go structs for transactions, parts, and object registry.
2. Invariant verification via `go run ./cmd/invariantcheck` and `go vet ./...`.
3. Unit tests for `internal/objectref`, `internal/protocol`, `internal/sessionhealth`.
4. Integration tests against live Siemens NX 2512 in `tests/nx/`:
   - `TestRealNXPartLifecycle`: `part.new`, `part.query_summary`, `part.save`, `part.close`, `part.open`.
   - `TestRealNXTransactionRollback`: `transaction.begin`, mutation / part check, `transaction.rollback`, `transaction.commit`.
   - `TestRealNXStaleHandleRejection`: closed parts or foreign epoch handles are rejected fail-closed.
