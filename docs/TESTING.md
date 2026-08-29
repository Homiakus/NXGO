# Testing strategy

## 1. Philosophy

NXGO must test both ordinary software behavior and behavior of real Siemens NX. Mock-only confidence is insufficient, while running NX for every tiny unit test is too slow. Use a layered pyramid.

## 2. Levels

### L0 — pure Go unit tests
Test domain validation, unit conversion, retry policy, capability logic, error mapping, workflow planning and serializers without NX.

### L1 — protocol/contract tests
Run Go client against an in-memory/fake Agent and C# protocol tests against generated contracts. Include compatibility fixtures for older protocol minors.

### L2 — Agent adapter tests
Where practical test reflection/scanner, object registry and adapters without a full interactive workflow. Siemens-dependent tests are tagged.

### L3 — real NX integration tests
Start pinned NX worker, load controlled `.prt` fixtures and execute domain operations.

### L4 — semantic/golden artifact tests
Compare generated model/drawing/export properties against normalized expected manifests. Visual PDF/image comparison may complement but never replace semantic checks.

### L5 — GUI smoke tests
Small set only: Agent load, menu/command integration if provided, interactive attach and selected UI-facing flows.

## 3. Test isolation

Default critical integration isolation:

- fresh working directory per test;
- read-only source fixtures copied to workspace;
- unique run ID;
- bounded timeout;
- deterministic environment;
- recycle worker after severe NX errors;
- configurable tests-per-worker for performance.

Tests that validate crash recovery SHOULD use one test per process.

## 4. Result taxonomy

Do not collapse all failures into `failed`:

```text
PASS
ASSERTION_FAIL
NX_EXCEPTION
NX_FATAL_ERROR
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

Visual golden comparison checks layout drift, overlap and clipping with tolerances.

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
```

Never compare unstable binary `.prt` bytes directly as the primary assertion.

## 7. Contract test for a new domain API

Every new public domain operation requires:

- input validation tests;
- capability missing test;
- happy-path real NX test;
- native NX exception mapping test where feasible;
- cancellation/timeout behavior;
- rollback test if mutating;
- log correlation assertion;
- resource/handle cleanup assertion;
- version matrix entry.

## 8. Fault injection

Build controllable faults into fake Agent/supervisor tests:

- delayed reply;
- broken pipe;
- stale handle;
- Agent restart;
- queue saturation;
- rollback failure;
- malformed response;
- capability mismatch.

Real NX environment tests include forced process termination to verify supervisor recovery.

## 9. Performance tests

Measure:

- attach/start latency;
- no-op RPC latency;
- batch vs chatty query performance;
- large face/feature enumeration;
- drawing generation time;
- export time;
- handle registry growth;
- memory growth across repeated jobs.

Performance regressions use statistical thresholds rather than single-run absolute assertions.

## 10. Test-of-tests

Important validation logic SHOULD be mutation-tested on pure Go components. Golden checker rules receive deliberately broken artifacts/manifests to prove failures are detected.

## 11. CI policy

Public cloud CI can run pure tests. NX-backed jobs require authorized self-hosted Windows runners with valid Siemens installation/licensing and protected fixture handling.

A commit is not considered compatible with a new NX build until its NX-backed matrix is green.