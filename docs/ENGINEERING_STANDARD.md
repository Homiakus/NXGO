# NXGO Engineering Standard

Status: **normative**. This document defines how NXGO is designed, implemented, reviewed and evolved. The short daily checklist is [RULES_QUICK_REFERENCE.md](RULES_QUICK_REFERENCE.md). Safety-specific rules are in the [programming invariant catalog](invariants/README.md).

## 1. Normative precedence

When documents disagree, apply this order:

1. accepted ADRs;
2. P0/P1 programming invariants;
3. this Engineering Standard;
4. `ARCHITECTURE.md` and domain-specific design documents;
5. examples/cookbooks.

A developer MUST NOT bypass a higher-level rule by citing lower-level example code.

## 2. Engineering goals

NXGO optimizes for these properties in order:

1. **engineering correctness** — generated/modified CAD state matches declared intent;
2. **session safety** — NX state is never silently reused after corruption/suspect failure;
3. **API stability** — ordinary Go callers are insulated from NXOpen implementation churn;
4. **diagnosability** — failures can be reproduced from logs, manifests and artifacts;
5. **testability** — important behavior can be exercised both without NX and with real NX;
6. **performance** — minimize unnecessary IPC/NX work after correctness and safety are established;
7. **developer ergonomics** — hide NX incidental complexity while keeping advanced escape hatches available.

## 3. Repository architecture rules

### 3.1 Go SDK

The public Go SDK MUST:

- remain Pure Go where possible and support `CGO_ENABLED=0` for client-side packages;
- expose stable NXGO domain types, not Siemens classes;
- use `context.Context` for blocking/remote work;
- expose explicit unit-aware dimensional values;
- surface capability and compatibility information;
- expose typed errors and preserve underlying NX diagnostic information;
- separate workflow/domain/raw planes.

The public Go SDK MUST NOT:

- load `NXOpen.dll`, `NXOpen.UF.dll` or other Siemens binaries directly;
- expose .NET remoting objects;
- require callers to manually create/destroy NX builders in normal use;
- make callers understand NX main-thread rules for normal domain operations;
- encode a single NX release into public domain semantics;
- make thread safety claims stronger than the Agent/session model actually provides.

### 3.2 NX Agent

The NX Agent is the only normal in-process Siemens integration boundary. It MUST own:

- NX session acquisition;
- safe NX execution scheduling;
- object registry/lifetime;
- builder/resource cleanup;
- release adapters/capability probing;
- NX callback registration/removal;
- session-health classification;
- structured Agent logging;
- mapping native errors to NXGO error semantics.

### 3.3 Supervisor / `nxctl`

The supervisor owns process-level responsibilities:

- discovering installations;
- selecting pinned builds;
- launching/stopping workers;
- collecting NX syslog/stdout/stderr/crash artifacts;
- watchdog/timeout policy;
- process restart/recycle;
- test worker pools;
- compatibility matrix execution.

The Agent MUST NOT attempt to replace the supervisor for process-level recovery after fatal process failure.

## 4. Dependency boundaries

Allowed dependency direction:

```text
workflow API
    -> domain API
        -> protocol/client abstractions
            -> transport

Agent domain adapters
    -> NX-safe executor
        -> NXOpen / NXOpen.UF

codegen
    -> normalized API manifest
        -> generated raw contracts
```

Forbidden examples:

- public domain package importing a release-specific Siemens adapter;
- transport package defining public CAD concepts;
- handwritten high-level API returning generated raw objects directly;
- NX Agent transport callback directly invoking NXOpen;
- tests reaching private Siemens types to avoid exercising NXGO contracts unless explicitly marked as reference/differential tests.

## 5. Go coding rules

### 5.1 API design

Prefer intent-oriented operations:

```go
part.Modeling.CreateHole(ctx, Hole{...})
```

over exposing NX mechanisms:

```go
builder := part.Raw().CreateHoleBuilder(...)
```

Raw access MAY exist for advanced use but MUST be clearly separated and have weaker stability guarantees.

### 5.2 Errors

Public errors SHOULD support `errors.Is` / `errors.As`. Error payloads SHOULD carry:

- stable NXGO kind/code;
- operation name;
- NX native code/message where available;
- run/request/session identifiers;
- retryability/recoverability classification;
- session-health consequence;
- diagnostic artifact/syslog reference.

Error strings are for humans; behavior MUST NOT depend on parsing public error strings when a structured code is available.

### 5.3 Units

Public dimensional inputs MUST be explicit:

```go
nx.MM(25)
nx.Deg(30)
```

not naked unit-dependent numbers. Internal conversion occurs at a documented boundary and is covered by round-trip/property tests.

### 5.4 Concurrency

Client APIs MAY be goroutine-safe, but this does not imply NX operations execute concurrently. Shared client/session state MUST be race-tested. A client Close racing with requests/subscriptions MUST have defined behavior.

### 5.5 Resource ownership

Every Go-side remote proxy has explicit ownership semantics: persistent session proxy, scoped handle, or value DTO. Cleanup APIs MUST be idempotent where feasible. Finalizers MUST NOT be the only cleanup mechanism.

### 5.6 Performance

Do not optimize by exposing unsafe NX details. Prefer:

- bulk fetch/update;
- request planning;
- server-side iteration;
- compact value DTOs;
- streaming only when the result genuinely benefits from incremental delivery.

Normal domain workflows MUST NOT require an RPC per face/edge/component unless intentionally requested.

## 6. C# Agent coding rules

### 6.1 NXOpen execution

Every NXOpen/NXOpen.UF call MUST be attributable to an NX-safe execution context. Arbitrary RPC/background threads MUST NOT call NXOpen directly.

The executor MUST model:

- queued/running/completed state;
- reentrancy/callback context;
- cancellation before execution;
- non-interruptible operations;
- session health before/after execution;
- correlation metadata.

### 6.2 Builders

Builders are single-attempt disposable resources.

Required pattern:

```text
create
configure
validate
commit
update/postcondition
finally destroy
```

A failed Builder is destroyed and recreated for any explicit retry.

### 6.3 Mutation scopes

Mutation helpers SHOULD create named undo marks where meaningful. After mutation they MUST run required update and semantic postconditions. Rollback failure escalates session health; code MUST NOT pretend rollback succeeded.

### 6.4 Callbacks

Callbacks have one owner and one unregister path. Reload/development code MUST not accumulate duplicate subscriptions. Shutdown disposes subscriptions deterministically.

### 6.5 Release adapters

Release-specific differences belong in adapters/capability tables. Avoid scattered code such as:

```text
if NXVersion >= ...
```

through unrelated domain logic.

### 6.6 Runtime target

Agent runtime/compiler choices follow the actual supported runtime of the target NX release. "Newest .NET" is not itself a valid reason to change the Agent target.

## 7. Protocol and distributed-systems rules

### 7.1 Identity

Every session has a unique ID and monotonically/non-repeating epoch concept. Object references contain enough identity to reject objects from old sessions.

### 7.2 Requests

Every request has:

- request ID;
- operation identity;
- deadline/cancellation metadata when applicable;
- client/session identity;
- correlation metadata.

Mutation/workflow requests that may be safely retried additionally define an idempotency key/contract.

### 7.3 Retry

Read-only queries MAY be retryable if their semantics are stable. Mutations MUST NOT be blindly retried after ambiguous transport failure. Lost response after a successful commit is explicitly tested.

### 7.4 Cancellation

Cancellation means "caller no longer waits" unless the operation is known cooperatively cancellable. Running arbitrary NX code is never killed by aborting its execution thread.

### 7.5 Compatibility

Minor compatible changes tolerate unknown fields/capabilities. Breaking wire changes require major protocol evolution/migration policy.

## 8. Object and state rules

- Live Siemens objects stay in the Agent.
- Handles are opaque and epoch-bound.
- Stale/dead objects produce stable NXGO errors.
- Registry growth is bounded and observable.
- Work Part, Display Part and Work Component are explicit state.
- Interactive and worker sessions expose different capability/state policy.
- Session health is first-class: `Healthy`, `Busy`, `Suspect`, `Poisoned`, `Lost`, or equivalent documented model.
- A poisoned/lost session can never return to Healthy without a new worker/session epoch.

## 9. CAD correctness rules

### 9.1 Success is semantic

No exception does not prove CAD correctness. High-value mutating operations SHOULD specify postconditions such as:

- body/feature count change;
- expected topology class;
- bounding box/volume/mass tolerance;
- associativity;
- annotation/view/BOM consistency;
- update state.

### 9.2 Assemblies

Assembly-wide operations MUST define load completeness requirements. If results are partial, completeness/load issues are part of the result. Silent incomplete mass/BOM/inspection results are forbidden.

### 9.3 Files

File operations explicitly define:

- overwrite policy;
- existing-open-part behavior;
- workspace/staging path;
- permission/path failure semantics;
- native vs managed mode;
- output atomicity where feasible.

## 10. Journal and UI rules

Recorded journals are discovery/reference material, not production source of truth. Raw recorded `FindObject` identifiers are not stable semantic selectors.

Coordinate/ribbon UI automation MUST NOT be the primary automation path. UI automation is limited to small smoke coverage where no stronger API-level test is possible.

Worker automation capable of producing modal UI MUST be denied, modeled or isolated; indefinite hidden modal waits are treated as defects.

## 11. Version and codegen rules

Each supported NX build has a normalized manifest. New support follows:

```text
scan -> normalize -> diff -> generate -> compile -> contract test -> real NX test -> matrix green -> supported
```

Generated files:

- are deterministic;
- include source manifest/build metadata;
- are never hand-edited;
- do not copy proprietary Siemens documentation into the repository;
- distinguish lifecycle categories and special parameters (`ref/out`, callbacks, arrays, disposable/transient objects).

## 12. Logging and diagnostics rules

Each operation can be traced across:

```text
Go client -> protocol -> Agent -> NX syslog -> artifacts
```

with shared IDs.

NX syslog remains available as raw forensic evidence. Parsers/classifiers MAY derive events from it but MUST retain the relevant raw evidence for severe failures.

Severe failure collection SHOULD include, when available:

- run/test/request manifest;
- exact NX/Agent/NXGO versions;
- syslog tail/full retained file;
- Agent log;
- stdout/stderr;
- fixture/workspace manifest;
- relevant output artifacts;
- session-health transition reason.

## 13. Security rules

- local pipe access is restricted by default;
- remote transport is disabled unless explicitly configured and secured;
- arbitrary reflection/journal/library execution is disabled in hardened modes;
- untrusted protocol input is size/type validated;
- no Siemens proprietary binaries are committed/distributed by NXGO;
- secrets/credentials/private Teamcenter data are never intentionally logged.

## 14. Testing standard

Testing is part of implementation, not a later QA phase. Use [TESTING_PLAYBOOK.md](TESTING_PLAYBOOK.md) as the operational standard.

At minimum:

- pure logic gets unit/table/property tests;
- parsers/protocol boundaries get fuzz/negative tests;
- state/recovery logic gets model-based/fault tests;
- NX behavior gets real-NX integration coverage;
- engineering outputs get semantic postconditions;
- compatibility claims get differential/matrix tests;
- recovery mechanisms get chaos/fault tests;
- long-lived resources get soak/leak tests;
- critical validators/retry/error logic get mutation testing where practical.

## 15. Testability as a design requirement

Production components MUST expose deterministic seams for testing without exposing unsafe public API. Examples:

- injectable clock/process/transport in pure Go;
- fake Agent protocol server;
- controlled fault injection points;
- deterministic session-health classifier inputs;
- fixture/environment manifests;
- replaceable NX worker allocator in test runner;
- semantic inspectors independent from generator where possible.

A component that can only be tested manually in NX requires explicit design justification.

## 16. Code review rules

Reviewer checks, in order:

1. Is the change inside the correct architectural boundary?
2. Which `NXGO-INV-*` rules apply?
3. Can this leave NX/session/object state inconsistent?
4. Are units/load completeness/version/capability semantics explicit?
5. Is failure/recovery behavior defined?
6. Are retries/idempotency safe?
7. Is the API coarse enough to avoid routine N+1 RPC?
8. Is there a cheaper pure test plus required real-NX test?
9. Do tests prove negative/recovery cases, not only happy path?
10. Are diagnostics sufficient to reproduce failures?

## 17. Documentation rules

A public behavior change updates relevant API/design docs in the same change. Repeated failure modes become invariants or troubleshooting entries rather than remaining tribal knowledge.

Examples MUST obey current invariant rules; obsolete examples are defects.

## 18. Change classification

Every nontrivial change SHOULD be classified as one or more:

- `pure-go`;
- `protocol`;
- `agent-core`;
- `nx-adapter`;
- `domain-api`;
- `codegen`;
- `workflow`;
- `observability`;
- `security`;
- `test-infrastructure`;
- `managed-mode`;
- `ui`.

The classification determines required tests using the matrix in [TESTING_PLAYBOOK.md](TESTING_PLAYBOOK.md).

## 19. Merge/release governance

P0/P1 invariant violations block merge unless replaced by an accepted ADR with equivalent or stronger safety controls. A release MUST NOT advertise support for an NX build/mode that has not passed the required real-NX matrix.

The complete completion gate is [DEFINITION_OF_DONE.md](DEFINITION_OF_DONE.md).