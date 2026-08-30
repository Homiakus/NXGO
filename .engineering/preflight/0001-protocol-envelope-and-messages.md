# Pre-flight: 0001-protocol-envelope-and-messages

## Task
Implement stable typed protocol messages and validation codecs in Pure-Go (`internal/protocol`) conforming to `docs/PROTOCOL.md`.

## Root Cause & Characterization
The current prototype in `internal/protocol` only has raw byte framing (`frame.go`). To progress Phase 1 and Phase 2 exit gates, NXGO requires typed structures for:
1. `HandshakeRequest` / `HandshakeResponse` (protocol negotiation, SDK version, PID, server limits, capability flags).
2. `RequestEnvelope` (request ID, correlation ID, operation name, deadline/timeout, optional transaction ID, payload, trace metadata).
3. `ResponseEnvelope` (request ID, status, produced object handles, timing metrics, payload, warnings).
4. `ErrorEnvelope` (category, native NX error code, safe message, session health state, recoverability, diagnostic correlation).
5. `StreamEvent` (stream kind: logs, events, progress; correlation ID, sequence, timestamp, payload, loss marker).
6. Protocol version compatibility evaluator (major version strict match, minor version backward compatibility).

## Invariants Maintained
- `NXGO-INV-IPC-001`: Request envelope contains explicit deadline, timeout, and cancellation indicators.
- `NXGO-INV-IPC-002`: Object handles in response envelope conform to session/epoch/generation contracts.
- `NXGO-INV-IPC-003` & `NXGO-INV-IPC-004`: Request ID and correlation ID are mandatory on all requests and responses to prevent double-execution on replay.
- `NXGO-INV-SES-001`: Error envelope explicitly conveys session health (`healthy`, `dirty`, `lost`).
- `NXGO-INV-VER-001`: Handshake negotiator fails closed on major version mismatch.
- `NXGO-INV-OBS-001`: All envelopes carry correlation and trace IDs.

## Protected Surfaces
- Pure-Go boundary: No CGO or Siemens DLL references.
- Framing layer (`internal/protocol/frame.go`): Remains intact and composes directly with encoded messages.

## Verification Ladder & Edge Space
1. Static analysis: `go vet ./...` and `go run ./cmd/invariantcheck`.
2. Unit tests (`internal/protocol/messages_test.go`):
   - Handshake negotiation (equal versions, client newer minor, client older minor, major mismatch).
   - Envelopes serialization & deserialization round-trip.
   - Validation failures: empty request ID, negative timeouts, empty operation name.
   - Error envelope with session health states (`healthy`, `dirty`, `lost`).
   - Stream event serialization and loss marker preservation.
3. Race detection & benchmarks for allocations and throughput.
