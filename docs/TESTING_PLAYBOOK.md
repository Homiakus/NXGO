# NXGO Testing Playbook

Status: **normative operational testing standard**. This document defines which test techniques apply to NXGO, where real Siemens NX enters the loop, and which gates are required for different classes of change.

The existing [TESTING.md](TESTING.md) defines the high-level testing architecture. This playbook defines execution policy.

## 1. Testing objective

NXGO has four things to prove simultaneously:

1. software logic is correct;
2. protocol/runtime behavior is correct;
3. NX session state remains safe/recoverable;
4. resulting CAD/documentation satisfies engineering intent.

No single test technique covers all four.

## 2. The four nested test loops

### Loop A — Fast / no NX

Runs continuously during development and on every PR.

Includes:

- format/lint/static analysis;
- Go unit/table tests;
- C# unit tests;
- architecture/dependency tests;
- protocol contract tests;
- fake Agent integration;
- property/model-based tests where cheap;
- selected fuzz corpus;
- deterministic codegen tests.

Target: seconds to low minutes.

### Loop B — Warm real NX

A pinned NX worker is started once and reused across a controlled set of tests.

Includes:

- real NXOpen adapter calls;
- fixture open/query/save/close;
- modeling operations;
- object lifetime tests;
- drawing/PMI tests;
- semantic assertions;
- selected Check-Mate/validation runs;
- export tests;
- log correlation and cleanup checks.

Between tests the runner applies a defined reset level and evaluates `SessionHealth` before reuse.

### Loop C — Isolated real NX

One test/scenario gets a fresh NX process.

Required for:

- process crash/fatal-error handling;
- session poison classification;
- Agent startup/load/unload behavior;
- startup environment/Customer Defaults;
- license acquisition/error cases;
- modal/UI interaction risk;
- forced process kill;
- ambiguous transport failure after mutation;
- callback lifecycle/reload cases;
- tests that intentionally corrupt/dirty state.

### Loop D — Matrix / campaign

Runs nightly, pre-release, or when compatibility/runtime changes.

Includes:

- multiple pinned NX releases/builds;
- native/managed modes where supported;
- mm/inch fixtures;
- differential behavior;
- extended chaos;
- mutation testing;
- fuzz campaigns;
- soak/resource leak campaigns;
- performance regression;
- large assembly workload;
- upgrade compatibility scanning.

## 3. Standard developer commands

The intended interface is eventually:

```text
nxctl test fast
nxctl test nx
nxctl test nx --isolated
nxctl test matrix
nxctl test chaos
nxctl test soak
nxctl test perf
```

`nxctl test nx --watch` SHOULD support a warm-worker edit/test loop for NX-dependent development.

## 4. Test technique catalog

### 4.1 Unit tests

Use for deterministic pure logic:

- units/conversions;
- version parsing;
- capability resolution;
- retry/idempotency decisions;
- error classification;
- session-health transitions;
- handle validation;
- request planning;
- normalized semantic comparison;
- path/output policies.

Rule: if a behavior can be proven without NX, test it without NX first.

### 4.2 Table-driven tests

Preferred for large decision matrices such as:

- native NX error -> NXGO error mapping;
- version/capability combinations;
- load status policy;
- unit conversions;
- file conflict policy;
- retryability/recoverability classifications.

### 4.3 Property-based testing

Use when the space of sequences/values is more important than individual examples.

High-value targets:

- session state machines;
- ObjectRef/epoch lifecycle;
- request/idempotency state;
- unit round trips;
- capability/version negotiation;
- normalized manifest transformations.

Generated sequences SHOULD be shrinkable so failing cases become minimal reproducible scenarios.

### 4.4 Model-based/state-machine testing

Maintain a small reference model independent from production state code.

Example model states:

```text
Stopped -> Starting -> Ready -> Busy -> Ready
                         |        |
                         v        v
                      Suspect -> Poisoned
                         |
                         v
                       Lost
```

Generate actions such as:

- start/connect;
- open/close part;
- create/release handle;
- request/cancel;
- restart/kill;
- poison/recover;
- reconnect.

Production behavior must match model invariants.

### 4.5 Fuzz testing

Coverage-guided fuzzing is required/preferred for untrusted or highly variable parsers:

- protocol decoder boundaries;
- manifest parser;
- version parser;
- syslog classifier/parser;
- journal analysis parser;
- path normalization;
- codegen metadata normalization.

Fuzz tests MUST assert no panic/hang/unbounded allocation and no acceptance of structurally impossible handles/messages.

Do not run a real NX process for every fuzz input. Promote minimized/high-value corpus cases to real-NX replay when relevant.

### 4.6 Mutation testing

Use mutation testing as test-of-tests for safety-critical pure logic.

Priority targets:

- session health/error classifier;
- retry/idempotency policy;
- stale-handle/epoch validation;
- unit conversion;
- capability negotiation;
- semantic validators;
- invariant enforcement helpers.

Generated code and thin mechanically generated bindings are normally excluded.

A surviving high-risk mutant is treated as a missing/weak test rather than a harmless score issue.

### 4.7 Contract tests

Prove Go/C# wire compatibility using frozen fixtures and bidirectional encode/decode cases.

Must cover:

- current protocol;
- prior supported minor versions;
- unknown fields/capabilities;
- malformed messages;
- oversized messages;
- cancellation/deadline metadata;
- event streams;
- duplicate request/idempotency behavior.

### 4.8 Fake Agent integration

Run the real Go client/transport against a programmable fake Agent.

The fake Agent MUST support controlled faults such as:

```text
DelayNextResponse
DropConnection
DropResponseAfterCommit
ReturnStaleHandle
PoisonSession
FailRollback
LoseCapability
MalformedResponse
QueueSaturation
```

This layer validates distributed-systems behavior cheaply before using licensed NX workers.

### 4.9 Real NX integration

Any code whose correctness depends on NXOpen/NXOpen.UF/kernel/session behavior requires real-NX coverage.

Tests MUST record:

- exact NX release/build;
- Agent/NXGO version;
- execution mode;
- fixture/environment manifest;
- request/test IDs;
- session health before/after;
- syslog/artifact references on failure.

### 4.10 Semantic CAD tests

Primary oracle for engineering operations.

Depending on operation, assert:

- bodies/features count/type;
- volume/mass/area;
- bounding box;
- expected topology characteristics;
- expression/dimension values;
- object associativity;
- assembly component structure;
- BOM metadata;
- drawing sheet/view/annotation structure;
- PMI associations;
- update state.

`err == nil` is never sufficient for high-value CAD mutation.

### 4.11 Metamorphic testing

Use when an exact expected CAD model is difficult to specify but invariant relationships are known.

Examples:

- rigid translation/rotation preserves mass and volume;
- save/reopen preserves normalized semantics;
- supported unit conversion round-trip preserves geometry within tolerance;
- reordering independent operations preserves equivalent semantic result;
- equivalent input representation produces equivalent modeled result.

### 4.12 Differential testing

Compare the same normalized operation/result across:

- NX 2512 vs 2606 (or supported releases);
- NXGO high-level operation vs a small verified direct NXOpen reference implementation;
- native vs alternative supported path where two implementations exist.

Compare semantics, not binary `.prt` bytes.

Differences must be classified as:

```text
expected release difference
NXGO adapter difference
NX behavioral regression
fixture/environment difference
unknown -> investigate
```

### 4.13 Independent validation / Check-Mate

Where applicable use an independent NX validation mechanism in addition to NXGO assertions. A generator and validator sharing the same bug must not be the only oracle.

### 4.14 Golden artifact testing

Goldens contain normalized expected manifests, tolerances and optional references.

Do not use raw `.prt` binary equality as the primary assertion.

For drawings, combine:

1. semantic object checks;
2. export/PDF metadata/text checks;
3. optional visual/layout diff.

Visual equality alone cannot prove correctness.

### 4.15 Chaos/fault testing

Required for recovery logic.

Real/fake scenarios include:

- kill NX during request;
- break pipe before/after commit;
- rollback failure;
- stale handle after Undo/restart;
- missing component;
- partial assembly load;
- missing license;
- read-only output;
- worker hang/timeout;
- duplicate idempotency request;
- simulated/real session poison signature;
- modal UI/hung interactive condition where safely reproducible.

Assertions cover not only returned error but session health, handle invalidation, artifact preservation and recycle decision.

### 4.16 Soak/resource-leak testing

Long-lived NX/Agent tests repeatedly execute representative workloads and monitor:

- NX memory;
- Agent memory;
- Go supervisor memory;
- registry handle count;
- callback/subscription count;
- thread count;
- open part count;
- temp files;
- process/OS handles where practical.

Workloads include repeated open/close, modeling, drawing generation, export and mixed workflows.

### 4.17 Performance regression

Maintain representative workloads:

- trivial part;
- complex machined part;
- 50-component assembly;
- target ~300-component assembly;
- larger stress assembly;
- drawing generation;
- BOM/attributes;
- export.

Track at least:

- median/p95 wall time;
- peak memory;
- RPC count;
- IPC bytes when available;
- worker recycle rate;
- NX operation counts if instrumented.

Benchmark regressions use statistical/baseline rules, not one noisy run.

### 4.18 Concurrency/race testing

Go shared state is exercised under `go test -race` with concurrent:

- requests;
- logs/events subscriptions;
- context cancellation;
- Close/reconnect;
- capability queries;
- worker allocation.

NX calls remain serialized according to the executor invariant.

### 4.19 UI smoke testing

UI testing is deliberately small.

Use only for:

- NX startup/Agent load;
- menu/button integration if supplied;
- settings dialog;
- interactive attach;
- a small number of UI-only capabilities.

Do not validate modeling correctness through coordinate clicking.

## 5. Real NX worker reset policy

Warm workers require deterministic reset decisions.

Suggested levels:

```text
R0: no mutation / no reset
R1: rollback to known undo mark
R2: close fixture and reopen clean copy
R3: close all owned parts + clear owned subscriptions/handles
R4: recycle NX process
```

After each test, worker reuse requires checks for:

- `SessionHealth == Healthy`;
- no unexpected registry growth;
- no leaked owned callbacks;
- expected open-part set;
- no severe syslog signature;
- known transaction/update state.

Any uncertainty escalates to process recycle rather than optimistic reuse.

## 6. Fixture standard

Fixtures are immutable source inputs copied into per-test workspaces.

Each important real-NX fixture SHOULD have a manifest containing:

```text
case ID
fixture version/hash
expected units
native/managed mode
minimum capabilities/licenses
load policy
expected semantic properties
allowed tolerances
known release-specific exceptions
```

CI MUST NOT depend on arbitrary user templates/defaults.

## 7. Required test matrix by change type

| Change | Fast | Fake Agent | Warm NX | Isolated NX | Matrix/campaign |
|---|---|---|---|---|---|
| pure Go value logic | required | as relevant | no | no | mutation/property as relevant |
| protocol | required | required | smoke | if startup/reconnect affected | compatibility matrix |
| Agent executor | required | required | required | required for failures | chaos/soak |
| NX adapter | required | optional | required | severe-failure cases | version differential |
| public domain API | required | required | required | as relevant | supported-version matrix |
| object registry | required | required | required | restart/stale cases | soak/chaos |
| retry/idempotency | required | required | required where mutation involved | lost-response case | chaos |
| codegen/API scanner | required | no | generated smoke | no | multi-version scan/diff |
| drawing/PMI | required | workflow tests | required | as relevant | semantic+visual+differential |
| supervisor/process | required | fake process faults | required | required | soak/chaos |
| UI | required supporting logic | optional | attach smoke | UI isolated | minimal UI matrix |
| managed/Teamcenter | pure contract | fake where possible | managed required | failure cases | managed matrix |

## 8. Required tests for a new public NX operation

Before merge, a normal new NX-backed public operation requires, where applicable:

1. input validation/unit tests;
2. capability/license missing case;
3. fake Agent protocol behavior;
4. real NX happy path;
5. semantic postcondition;
6. native NX error mapping case;
7. cleanup/handle leak assertion;
8. cancellation/deadline behavior;
9. rollback/failure behavior if mutating;
10. stale-handle/restart behavior if returning proxies;
11. partial-load behavior if assembly-wide;
12. version matrix entry;
13. correlation/log evidence on failure.

## 9. Invariant-to-test traceability

Every implemented P0/P1 invariant must map to one or more executable controls:

```text
Invariant ID -> implementation guard -> test(s) -> CI job
```

Examples:

```text
NXGO-INV-EXEC-001 -> NxExecutor boundary -> arbitrary-thread negative test
NXGO-INV-OBJ-002 -> epoch validation -> restart/stale handle test
NXGO-INV-MUT-001 -> BuilderScope -> exception-path destruction test
NXGO-INV-SES-001 -> SessionHealth -> poison/recycle chaos test
NXGO-INV-IPC-003 -> idempotency policy -> lost response after commit test
NXGO-INV-COR-001 -> semantic postconditions -> wrong-result fixture test
```

The compliance report SHOULD be generated by CI as the project matures.

## 10. Coverage philosophy

Line coverage is diagnostic, not the primary quality gate. NXGO prioritizes:

- critical branch/decision coverage;
- invariant negative tests;
- mutation resistance;
- real-NX semantic coverage;
- failure/recovery coverage.

Do not raise line coverage by testing getters/generated code while recovery/state logic remains weak.

## 11. Failure artifacts

Any failed real-NX test SHOULD produce an artifact bundle with:

```text
run manifest
exact NX build
Agent/NXGO versions
test/fixture manifest
request/event trace
NX syslog
Agent log
runner stdout/stderr
semantic result diff
exported artifacts when useful
session-health history
```

A failure that cannot be diagnosed after CI is a test infrastructure defect.

## 12. PR gates

A PR cannot be considered verified when:

- only mocks were run for changed NX behavior;
- tests ignore an applicable P0/P1 invariant;
- mutation/recovery behavior changed without negative testing;
- a generated drawing is judged only visually;
- session poison/restart code lacks isolated-NX tests;
- version compatibility is claimed from compilation only;
- flaky tests are simply retried until green without root-cause classification.

## 13. Nightly/release campaigns

Recommended campaign order:

```text
fast suite
-> real NX warm suite
-> isolated recovery suite
-> supported-version differential matrix
-> mutation campaign
-> extended fuzz corpus/campaign
-> chaos
-> soak/leak
-> performance
-> selected UI/managed smoke
```

Release support requires a stored report of the applicable campaign results.

## 14. Flakiness policy

A flaky real-NX test is not automatically deleted or endlessly retried. Classify it:

- nondeterministic product bug;
- NX/environment instability;
- fixture contamination;
- timing/race bug;
- insufficient reset isolation;
- license/infrastructure issue;
- unstable visual oracle.

Retries MAY collect evidence but MUST NOT hide repeatable product failures.

## 15. Test code quality

Tests follow production-quality rules:

- deterministic names/fixtures;
- no hidden dependence on developer machine paths;
- explicit tolerances with engineering rationale;
- helpers do not silently weaken assertions;
- reference implementations are kept minimal and auditable;
- test cleanup cannot mask the original failure;
- test logs use run/test correlation IDs.

## 16. Test loop north star

The desired development cycle for NX-dependent code is:

```text
edit
 -> fast tests
 -> affected NX test selected
 -> warm pinned NX executes real NXOpen
 -> semantic + independent assertions
 -> syslog/session-health checked
 -> worker safely reused or recycled
 -> immediate result
```

Real NX is therefore a normal executable test dependency, not a manual final validation step.