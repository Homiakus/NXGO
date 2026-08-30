# Pre-flight: 0004-real-nx-agent-bootstrap-and-session-query

## Task
Implement Real Siemens NX 2512 Agent bootstrap and first session query (`internal/supervisor/worker.go`, `agent/bundle/AgentWorker.cs`, `tests/nx/agent_smoke_test.go`).

## Root Cause & Characterization
Phase 3 requires validating that the Agent starts inside a real licensed Siemens NX process, binds `NxExecutor` to the NX main thread, opens a named pipe, responds to typed handshakes and queries, and executes NXOpen operations safely on the bound thread.

## Invariants Maintained
- `NXGO-INV-EXEC-001`: Transport pipe reader runs on background thread; NXOpen session calls are strictly dispatched and drained on the bound NX main thread.
- `NXGO-INV-EXEC-002`: Execution serialization avoids deadlock and checks caller thread.
- `NXGO-INV-SES-001`: Session health state machine is initialized and guarded.
- `NXGO-INV-TEST-003`: Validated directly against real pinned Siemens NX 2512 (`NXGO_NX_HOME`).

## Protected Surfaces
- Pure-Go boundary: Supervisor launches `run_journal.exe` with Agent bootstrap and connects via `internal/transport/pipe`.
- Siemens assembly boundary: Agent source does not copy proprietary Siemens binaries into the repository.

## Verification Ladder & Edge Space
1. Static analysis: `go vet ./...` and `go run ./cmd/invariantcheck`.
2. Unit tests for bundle generation and worker supervisor.
3. Real NX Integration test:
   - Launch real NX worker in background via supervisor.
   - Pure-Go client connects to worker pipe.
   - Handshake: exchanges protocol versions, NX build ("2512"), process ID, session ID.
   - Query: `nx.ping` and `session.info` executed on bound NX thread.
   - Graceful shutdown: sends shutdown request and waits for NX process exit code 0.
