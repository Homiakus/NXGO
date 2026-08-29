# NX Agent design

## 1. Responsibility

`NXGO.Agent` is the smallest practical trusted component loaded into Siemens NX. It converts typed NXGO commands into supported NXOpen/NXOpen.UF operations and emits structured results/events.

It must not become a second application server with business logic. Domain orchestration belongs primarily in Go unless running it inside NX materially reduces round trips or is required by NX state semantics.

## 2. Internal modules

```text
Agent bootstrap
  |- handshake/security
  |- protocol endpoint
  |- request validator
  |- command dispatcher
  |- NX executor/main-thread gateway
  |- object registry
  |- transaction/undo manager
  |- NX adapters
  |    |- Common NXOpen
  |    |- UF
  |    |- release shims
  |    `- controlled fallback execution
  |- event publisher
  `- diagnostics
```

## 3. Bootstrap

Agent startup MUST:

1. identify NX release/build and process ID;
2. initialize structured logging;
3. create a cryptographically random session ID;
4. create the secured local endpoint;
5. build a capability manifest;
6. expose a health state only after NX execution gateway is ready.

## 4. NX execution gateway

Transport callbacks enqueue commands. Only the gateway may execute NX API calls.

Required properties:

- FIFO within a transaction;
- bounded queue;
- priority reserved for cancellation/health where safe;
- queue metrics;
- protection against reentrant deadlock;
- clear policy for callbacks raised by NX during a command.

No assumption of general NXOpen thread safety is allowed.

## 5. Adapter model

Domain command handlers call interfaces implemented by release-specific adapters. Example:

```text
DrawingAdapter
  |- 2512 implementation/shims
  `- 2606 implementation/shims
```

Common code is used when behavior is verified identical. Capability flags represent optional behavior.

## 6. Builder lifecycle

All Builder-style APIs follow `create -> configure -> validate -> commit -> destroy` with `finally` cleanup. Builders MUST NOT be registered as long-lived client handles unless an explicit raw API requires it.

## 7. Undo/rollback

Mutating domain commands SHOULD establish an undo mark. Transaction manager tracks nested logical operations while mapping them safely onto NX undo behavior.

If rollback fails:

- mark session `dirty`;
- emit critical diagnostic event;
- return `ErrRollbackFailed`-class error;
- worker mode SHOULD recycle the NX process before further mutable tests.

## 8. Object registry

Registry entries include:

- NXGO object ID;
- weak/strong reference as appropriate;
- NX type metadata;
- session ID;
- lease scope;
- creation timestamp;
- optional native tag.

Registry MUST reject stale session IDs and enforce limits.

## 9. Dynamic/raw execution

Dynamic reflection invocation is an advanced escape hatch, not the default dispatcher.

Policies:

- allowlist public NXOpen assemblies/types;
- block arbitrary filesystem assembly loading by default;
- block process/shell execution;
- validate argument types;
- apply per-call limits;
- log invocation metadata.

## 10. Journal/library execution

Journal or external NXOpen library execution is optional and policy controlled. Hardened mode may disable it entirely.

## 11. Diagnostics

Agent emits structured events and can also write correlation markers into the NX system log where supported. It records original NX error codes and sanitized exception information.

## 12. Failure states

Health states:

- `starting`;
- `healthy`;
- `busy`;
- `dirty`;
- `draining`;
- `lost`.

The Agent cannot recover from process-level Fatal Error; the Go supervisor detects termination, preserves artifacts and starts a replacement worker according to policy.