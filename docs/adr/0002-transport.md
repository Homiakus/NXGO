# ADR-0002: Typed protocol over secured local named pipes

- Status: Accepted for initial implementation
- Date: 2026-08-29

## Context

NXGO needs duplex requests, event/log streaming, cancellation and explicit version negotiation. Default use is one Go process communicating with NX on the same Windows workstation.

## Decision

Use a typed Protocol Buffers contract. Use Windows named pipes as the default local carrier with explicit current-user ACL/security. Do not expose a TCP port by default.

The implementation may use gRPC semantics/code generation where practical, but the architecture does not require exposing a network service.

## Consequences

Positive:

- typed Go/C# contracts;
- backward-compatible schema evolution;
- streaming and cancellation model;
- no default LAN attack surface;
- per-user Windows security primitives.

Negative:

- transport adapter work may be needed if standard gRPC named-pipe support is insufficient for selected runtime;
- debugging wire traffic is less trivial than JSON.

## Rejected alternatives

- JSON over localhost TCP: simple but weaker contracts and needless network listener.
- stdin/stdout only: poor fit for interactive attach and multiple streams.
- shared memory: premature complexity.
- cgo direct calls: violates ADR-0001.