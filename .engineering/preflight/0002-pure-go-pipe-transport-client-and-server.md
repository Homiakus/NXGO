# Pre-flight: 0002-pure-go-pipe-transport-client-and-server

## Task
Implement Pure-Go Windows named-pipe client (`internal/transport/pipe`) and fake Agent transport server over framed typed protocol.

## Root Cause & Characterization
Phase 2 requires Pure-Go client transport that connects to the Agent's length-prefixed named pipe server, performs the initial handshake, sends requests, and receives responses/stream events with timeout and cancellation protection.
Additionally, tests must be able to run a Fake-Agent transport server without needing an installed NX or C# runtime.

## Invariants Maintained
- `NXGO-INV-EXEC-001`: Transport client is completely decoupled from direct NX execution.
- `NXGO-INV-IPC-001`: Client requests respect context cancellation and timeout boundaries without force-aborting execution.
- `NXGO-INV-IPC-002`: Session Epoch mismatch fails immediately on client side.
- `NXGO-INV-IPC-003` & `NXGO-INV-IPC-004`: Idempotency keys/request IDs are preserved over the wire.
- `NXGO-INV-SES-001` & `NXGO-INV-SES-002`: Client checks session health returned in responses and rejects reuse of dirty/lost sessions.
- `NXGO-INV-VER-001`: Handshake failure on major version mismatch terminates the connection.

## Protected Surfaces
- Pure-Go boundary: Zero CGO dependencies. Must use standard Go file/net APIs for Windows pipe I/O.
- Protocol envelopes: Uses `internal/protocol` codecs without leaking transport details into domain logic.

## Verification Ladder & Edge Space
1. Static analysis: `go vet ./...` and `go run ./cmd/invariantcheck`.
2. Unit and Integration tests:
   - Full transport client-server handshake over named pipe / pipe listener.
   - Request-response dispatch over framed pipe transport.
   - Broken pipe / unexpected EOF error handling.
   - Server timeout and client context cancellation.
   - Duplicate request ID handling and session poisoning over real transport.
3. Concurrency & Race detection: `go test -race` on transport and fakeagent.
