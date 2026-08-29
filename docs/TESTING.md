# Testing strategy

## 1. Philosophy

NXGO must test both ordinary software behavior and behavior of real Siemens NX. Mock-only confidence is insufficient, while running NX for every tiny unit test is too slow. Use a layered pyramid. The normative [programming invariants](invariants/README.md) define mandatory negative/recovery cases in addition to feature happy paths.

## 2. Levels

### L0 — pure Go unit tests
Test domain validation, unit conversion, retry/idempotency policy, capability logic, error mapping, workflow planning and serializers without NX.

### L1 — protocol/contract tests
Run Go client against an in-memory/fake Agent and C# protocol tests against generated contracts. Include compatibility fixtures for older protocol minors.

### L2 — Agent adapter/architecture tests
Where practical test reflection/scanner, object registry, callback registry, Builder scopes, session-health state machine and adapters without a full interactive workflow. Architecture tests enforce allowed NXOpen dependency boundaries.

### L3 — real NX integration tests
Start pinned NX worker, load controlled `.prt` fixtures and execute domain operations.

### L4 — semantic/golden artifact tests
Compare generated model/drawing/export properties against normalized expected manifests. Visual PDF/image comparison may complement but never replace semantic checks.

### L5 — GUI smoke tests
Small set only: Agent load, menu/command integration if provided, interactive attach and selected UI-facing flows. Coordinate/ribbon automation is never the primary correctness path.

## 3. Test isolation

Default critical integration isolation:

- fresh working directory per test;
- read-only source fixtures copied to workspace;
- unique run ID;
- bounded timeout;
- pinned NX/Agent build;
- deterministic locale/customer defaults/templates/environment;
- explicit native/managed and load policy;
- recycle worker after severe NX errors;
- configurable tests-per-worker for performance.

Tests that validate crash/poison recovery SHOULD use one test per process.

## 4. Result taxonomy

Do not collapse all failures into `failed`:

```text
PASS
ASSERTION_FAIL
NX_EXCEPTION
STALE_OBJECT
PARTIAL_LOAD
NX_FATAL_ERROR
SESSION_POISONED
PROCESS_CRASH
TIMEOUT
LICENSE_ERROR
UNSUPPORTED
ROLLBACK_FAILED
ARTIFACT_MISMATCH
INFRA_ERROR
```

## 5. Drawing quality tests

For automated drawings check semantically where possible:

- sheet count/format/orientation;
- intended scale;
- required view types;
- view update state;
- annotations/dimensions counts and associations;
- centerlines/hole callouts;
- title block attributes;
- material/designation/mass presence;
- BOM/balloon consistency for assemblies;
- validation rule results;
- export success;
- no orphaned/stale annotations.

Visual golden comparison checks layout drift, overlap and clipping with tolerances, but cannot be the only oracle (`NXGO-INV-TEST-002`).

## 6. Golden data

Each golden case contains:

```text
tests/golden/<case>/
  source/
  request.json
  expected-semantic.json
  expected-export-metadata.json
  optional-reference.pdf
  tolerances.json
  environment.json
```

Never compare unstable binary `.prt` bytes directly as the primary assertion.

## 7. Contract test for a new domain API

Every new public domain operation requires:

- input/unit validation tests;
- capability/license missing test;
- happy-path real NX test;
- native NX exception mapping test where feasible;
- cancellation/timeout state behavior;
- rollback test if mutating;
- postcondition test for engineering intent;
- log correlation assertion;
- resource/handle cleanup assertion;
- stale-handle/session-restart behavior where objects are returned;
- partial-load behavior for assembly-wide operations;
- version matrix entry.

## 8. Fault/chaos injection

Build controllable faults into fake Agent/supervisor tests:

- delayed reply;
- broken pipe after mutation commit;
- stale handle;
- Agent/NX restart;
- queue saturation;
- rollback failure;
- malformed response;
- capability mismatch;
- duplicate/idempotent request;
- simulated session poison.

Real NX environment tests include forced process termination and selected failure-inducing fixtures to verify supervisor recovery. See `NXGO-INV-TEST-004`.

## 9. Performance tests

Measure:

- attach/start latency;
- no-op RPC latency;
- batch vs chatty query performance;
- large face/feature enumeration;
- representative ~300-component assembly inspection;
- drawing generation time;
- export time;
- handle registry growth;
- memory growth across repeated jobs.

Performance regressions use statistical thresholds rather than single-run absolute assertions.

## 10. Test-of-tests

Important validation logic SHOULD be mutation-tested on pure Go components. Golden checker rules receive deliberately broken artifacts/manifests to prove failures are detected.

## 11. Invariant compliance matrix

CI SHOULD publish a generated mapping from every implemented P0/P1 `NXGO-INV-*` rule to one or more tests/enforcement mechanisms. An invariant with no enforcement/test is visible technical debt; an implemented P0 subsystem may not be released while its applicable P0 invariant has no negative test.

Examples:

- `EXEC-001` -> concurrent RPC/main-thread executor test;
- `OBJ-002` -> restart + stale epoch handle test;
- `MUT-001` -> exception path proves Builder destruction;
- `SES-001` -> poison classification forces worker recycle;
- `IPC-003/004` -> lost response after commit does not duplicate mutation;
- `TEST-002` -> deliberately semantically wrong but visually similar artifact fails.

## 12. CI policy

Public cloud CI can run pure tests. NX-backed jobs require authorized self-hosted Windows runners with valid Siemens installation/licensing and protected fixture handling.

A commit is not considered compatible with a new NX build until its real-NX matrix is green. Compilation/code generation alone is insufficient.