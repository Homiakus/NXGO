# Product requirements

## 1. Mission

NXGO provides a stable, idiomatic Go surface for automating Siemens NX / Designcenter while keeping Siemens runtime dependencies and NXOpen-specific complexity behind an adapter boundary.

## 2. Primary users

### Go application developer
Wants to call CAD operations without becoming an NXOpen lifecycle expert.

### NX automation engineer
Needs access to advanced NXOpen/UF capabilities, journals, diagnostics and version-specific escape hatches.

### CAD platform owner
Needs repeatable deployment, pinned versions, permission controls, logging, supportability and upgrade reports.

### Test/CI engineer
Needs isolated NX workers, deterministic fixtures, timeouts, crash capture, artifacts and a compatibility matrix.

## 3. Primary use cases

- discover installed and running NX versions;
- start or attach to an NX session;
- open/create/save/close parts;
- inspect and mutate geometry and features;
- work with assemblies and constraints;
- generate and validate drawings;
- work with PMI;
- export PDF/DXF/DWG/STEP/JT and other supported artifacts;
- invoke validation/Check-Mate-style workflows where available;
- execute supported NXOpen/UF functions not yet wrapped by the high-level API;
- record and consume structured runtime events;
- follow NX system logs continuously;
- run NX-backed integration/golden tests;
- compare behavior across pinned NX releases.

## 4. UX requirements

A common operation SHOULD require one domain call rather than exposing Builder setup/commit/destroy mechanics.

Typical code:

```go
nx, err := nxgo.Connect(ctx)
part, err := nx.Parts.Open(ctx, path)
result, err := part.Drawings.Generate(ctx, request)
```

Every public operation MUST:

- accept `context.Context` when it can block;
- return stable Go errors;
- document mutation, rollback and idempotency semantics;
- avoid exposing NXOpen/.NET types;
- avoid requiring callers to know the installed NX filesystem layout.

## 5. Coverage model

NXGO does not claim that every private NX UI command is programmatically exposed. Instead it MUST provide a layered access strategy:

1. workflow API;
2. idiomatic domain API;
3. generated raw NXOpen layer;
4. UF/raw layer;
5. controlled library/journal/command escape hatches.

A missing high-level wrapper MUST NOT permanently block use of a capability available through lower layers.

## 6. Supported modes

### Interactive mode
Attach to a user-facing NX session. Suitable for commands, UI-integrated automation and assisted engineering.

### Worker mode
Launch an isolated controlled NX instance for batch automation and tests. No assumption is made that GUI automation is safe or desirable.

## 7. Initial platform scope

- Windows first.
- Go SDK should remain portable and `CGO_ENABLED=0` where possible.
- Agent runs in the NX-supported .NET environment of each target release.
- Initial compatibility target: NX/Designcenter 2512 and 2606 families, pinned to validated maintenance builds.

## 8. Non-functional goals

- crash isolation between Go host and NX;
- explicit compatibility negotiation;
- low IPC overhead through batching;
- deterministic cleanup of remote object handles;
- rich diagnostics with correlation IDs;
- no unauthenticated network listener by default;
- reproducible CI workers;
- maintainable generated code separated from handwritten domain code.

## 9. Non-goals

- license circumvention;
- replacement of Siemens NX;
- uncontrolled arbitrary code execution as a convenience API;
- parallel mutation of a single NX session;
- copying Siemens proprietary SDK binaries into public distributions;
- pretending version-specific NX behavior does not exist.

## 10. Success criteria for v1

v1 is successful when a fresh Go project can:

1. discover/start/attach to NX;
2. open a fixture part;
3. query basic model metadata;
4. perform at least one safe modeled mutation in a transaction;
5. create/export a simple drawing;
6. stream logs/events;
7. recover cleanly from timeout/session loss;
8. run the same integration suite against at least two pinned NX release families;
9. use a generated raw call for a capability absent from the high-level layer.