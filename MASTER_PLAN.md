# NXGO MASTER PLAN

Status: **Living implementation plan**  
Created: 2026-08-29

The plan is deliberately staged so the project proves the risky boundaries before expanding API surface. Every phase has explicit exit gates. Unexpected discoveries update this document and, when architectural, create an ADR.

## North-star outcome

A Go developer can import NXGO and perform high-value Siemens NX automation through a stable API without managing NXOpen builders, Siemens DLL loading, NX main-thread execution, remote object lifetime, release differences, logs or process recovery.

## Implementation checkpoint — Agent safety boundary exists

NXGO has moved beyond architecture-only documentation. The repository now contains:

- Go module and initial safety/runtime primitives;
- `nxctl test fast|fuzz|nx|matrix|chaos|soak|perf` command surface;
- race-enabled/Pure-Go CI and invariant policy checker;
- machine-readable invariant-compliance map;
- stale session/epoch object-reference tests and fuzz targets;
- session-health state machine preventing in-place reuse of poisoned/lost sessions;
- Fake-Agent idempotency/ambiguous-response chaos contract;
- fail-closed real Siemens NX smoke via `run_journal.exe` + NXOpen Python journal;
- self-hosted Windows NX workflow that cannot pass without an explicit NX installation;
- **NX-independent .NET Agent core targeting `netstandard2.0`**;
- **`NxExecutor` that separates transport/background threads from the bound NX execution thread**;
- **single-attempt `BuilderScope<T>` with unconditional cleanup**;
- **bounded named-pipe framing/server plus cross-language Go/C# golden framing tests**;
- **`net48` dedicated-worker NXHost skeleton referencing only an installed NX through `NXGO_NX_MANAGED`**;
- executable quality-gate documentation and PR evidence template.

The NX-independent Agent core is verified in ordinary GitHub Actions together with Go fast/chaos gates. This still does **not** prove the new NXHost inside a licensed Siemens NX process; that evidence requires an authorized pinned Windows/NX runner.

---

## Phase 0 — Architecture baseline [DONE: documentation]

### Deliverables

- [x] product requirements;
- [x] process-boundary architecture;
- [x] API layering decision;
- [x] object lifetime model;
- [x] version/capability model;
- [x] observability strategy;
- [x] NX-backed testing strategy;
- [x] engineering standard/programming rules;
- [x] programming invariant catalog;
- [x] testing playbook and Definition of Done;
- [x] security baseline;
- [x] codegen/scanner design;
- [x] deployment and CLI specifications.

### Gate

No production implementation begins by adding ad-hoc cgo bindings or exposing Siemens types in public Go APIs.

---

## Phase 1 — Repository/build skeleton [IN PROGRESS]

### Tasks

- [x] create Go module and initial package layout;
- [x] create NX-independent .NET Agent core with NXHost/release boundary;
- [ ] create stable typed/protobuf protocol schema project;
- [x] create initial `nxctl` command and test-loop surface;
- [x] add formatting/linting/unit/race CI that does not require NX;
- [x] add .NET Agent Core CI that does not require NX;
- [x] add executable invariant checker and compliance metadata;
- [x] add fail-closed Windows real-NX smoke script/test journal;
- [x] add initial Windows Agent build script requiring installed `NXOpen.dll` path;
- [ ] validate Agent/NXHost build on authorized pinned NX developer machine;
- [ ] document exact supported compiler/runtime versions discovered from target NX builds in executable manifests;
- [ ] add version metadata generation.

### Tests

- [x] Go unit smoke;
- [x] race-enabled Go suite in GitHub Actions;
- [x] explicit `CGO_ENABLED=0 go test ./...` fast gate;
- [x] `go vet`;
- [x] invariant-policy gate;
- [x] Fake-Agent chaos/idempotency contract;
- [x] .NET Agent Core unit/contract smoke;
- [x] local named-pipe transport round trip without NX;
- [x] cross-language Go/C# framing golden;
- [ ] protobuf/codegen reproducibility.

### Exit gate

Fresh checkout builds/tests all non-NX components deterministically; NXHost additionally builds on an authorized pinned NX developer machine using installed Siemens assemblies without copying them into the repository.

---

## Phase 2 — Protocol + fake Agent vertical slice [IN PROGRESS]

### Tasks

- [ ] handshake messages;
- [ ] protocol version negotiation;
- [ ] capabilities;
- [ ] stable request/response envelope;
- [ ] stable error envelope;
- [ ] log/event stream;
- [x] cancellation semantics for queued executor work at core level;
- [x] bounded length-prefixed bootstrap framing;
- [x] named-pipe server transport primitive;
- [ ] Pure-Go Windows named-pipe production client;
- [ ] fake Agent server over production transport for Go tests;
- [x] initial in-process Fake-Agent failure semantics for request-ID idempotency and session poison.

### Tests

- [ ] protocol backward/forward minor compatibility;
- [x] bootstrap frame malformed/length mismatch tests;
- [x] bootstrap frame fuzz target;
- [x] local named-pipe roundtrip in Agent Core suite;
- [x] queued request cancellation before NX execution;
- [ ] broken pipe during real transport request;
- [ ] oversized production message;
- [ ] unauthorized local client;
- [ ] stream backpressure;
- [x] committed mutation + lost response does not duplicate on Fake-Agent replay;
- [x] poisoned simulated session rejects further work.

### Gate

The current string bootstrap protocol (`ping`, `nx.ping`, `shutdown`) is **temporary scaffolding** and MUST NOT become public API. Phase 2 exits only when Go client completes typed handshake/call/log-stream against the production transport with no NX installation.

---

## Phase 3 — Minimal real NX Agent [IN PROGRESS, REAL-NX EVIDENCE PENDING]

### Tasks

- [x] create dedicated-worker NXHost source entry point;
- [x] bind `NxExecutor` to NXHost entry thread in worker design;
- [x] keep pipe transport/background work outside direct NXOpen execution;
- [x] integrate Agent Core health state into worker skeleton;
- [x] add bootstrap `nx.ping` operation that queues the NXOpen log call through `NxExecutor`;
- [x] use `AtTermination` unload policy for long-lived worker host;
- [ ] validate Agent bootstrap inside pinned real NX;
- [ ] discover exact NX release/build through Agent;
- [ ] secured Windows pipe ACL endpoint;
- [ ] explicit callback reentrancy/call-depth policy;
- [ ] structured request/log correlation;
- [ ] stable first command: session information;
- [ ] first part command: open/query/save/close controlled fixture;
- [ ] separate Siemens-supported interactive-attach pump (do not reuse dedicated worker loop);
- [x] preliminary real-NX `run_journal.exe` smoke proving NXOpen session access without claiming Agent functionality.

### Tests

- [x] NX-independent executor proves producer thread != execution thread;
- [x] wrong-thread executor draining is rejected;
- [x] fail-closed external NX smoke harness exists;
- [ ] build NXHost against pinned installed NXOpen assemblies;
- [ ] run NXHost and prove `nx.ping` executes on the bound NX thread;
- [ ] execute smoke on authorized pinned NX builds and record exact versions;
- [ ] attach to real interactive NX through separate adapter;
- [ ] start controlled Agent worker from Go supervisor;
- [ ] exact build reported through Agent handshake;
- [ ] repeated connect/disconnect;
- [ ] timeout while command queued/running with safe cancellation semantics;
- [ ] Agent NXException translated;
- [ ] NX process termination detected.

### Exit gate

`nxctl status` and a Go integration test communicate reliably with real pinned NX through the NXGO Agent, not only through the preliminary journal smoke.

---

## Phase 4 — Supervisor and continuous logs

### Tasks

- [ ] NX installation discovery;
- [ ] exact version selection;
- [ ] worker launcher;
- [ ] ownership manifest;
- [ ] timeout/watchdog;
- [ ] NX syslog discovery/follow;
- [ ] merge NX/Agent/runner logs;
- [x] preliminary per-run artifact directory in real-NX smoke;
- [ ] failure artifact bundling;
- [ ] process crash/fatal-error classification;
- [ ] worker recycle policy;
- [ ] `nxctl doctor`, `status`, `logs --follow`, `diagnose`.

### Tests

- forced process kill;
- hung/fake worker timeout;
- log rotation/truncation;
- worker orphan cleanup;
- simultaneous side-by-side NX installations.

### Exit gate

Every worker failure is classified and leaves sufficient diagnostics for reproduction.

---

## Phase 5 — Object registry + transactions

### Tasks

- [x] define initial session/epoch/generation-aware `ObjectRef` safety primitive;
- [ ] opaque ObjectRef wire protocol;
- [ ] typed Go proxies;
- [ ] Agent object registry and quotas;
- [ ] scopes/batch release;
- [x] initial stale session/epoch validation + fuzz target;
- [x] generic `BuilderScope<T>` safety primitive;
- [ ] NX Builder adapter/factory recipes using `BuilderScope<T>`;
- [ ] NX undo-mark transaction manager;
- [ ] required UpdateManager/update semantics per recipe;
- [ ] rollback and dirty-session state;
- [ ] staged file outputs;
- [ ] semantic mutation postconditions.

### Tests

- [x] generic BuilderScope destroys after success;
- [x] generic BuilderScope destroys after failure and rejects same-builder retry;
- [ ] real NX Builder destroy/retry test;
- [ ] handle release/leak tests;
- [x] stale handle after epoch/session change at pure-Go layer;
- [ ] stale handle after real NX restart;
- [ ] rollback success;
- [ ] injected rollback failure;
- [ ] quota enforcement;
- [ ] transaction cancellation.

### Exit gate

Mutating real-NX integration tests cannot leak handles/builders and failed mutations either roll back or explicitly quarantine the worker.

---

## Phase 6 — API scanner + generated raw layer

### Tasks

- [ ] scan approved NXOpen assemblies/metadata;
- [ ] deterministic normalized manifest;
- [ ] API signature IDs;
- [ ] manifest diff tool;
- [ ] generated Go raw types/methods;
- [ ] generated C# dispatch glue where needed;
- [ ] capability catalog generation;
- [ ] `nxctl api scan/diff/find/inspect`;
- [ ] trace generated source to manifest/build.

### Tests

- deterministic generation;
- compile generated code;
- removed/changed fixture API diff;
- sample real NX raw invocation;
- no Siemens binary copied into repository artifacts.

### Exit gate

A new NX build can be scanned and broad raw bindings regenerated with a reproducible diff.

---

## Phase 7 — High-value domain API v0

Implement in Pareto order.

### Parts/attributes

- [ ] open/new/save/close;
- [ ] work/display part queries;
- [ ] batch attributes;
- [ ] units/basic metadata.

### Geometry/modeling

- [ ] bodies/faces/edges summaries;
- [ ] bounding box/mass properties;
- [ ] selected simple feature creation;
- [ ] semantic hole operation;
- [ ] bulk analysis request.

### Assemblies

- [ ] component tree;
- [ ] add/remove component;
- [ ] basic transforms;
- [ ] initial constraints;
- [ ] BOM-friendly metadata queries.

### Exit gate

Representative part/assembly workflows use only idiomatic Go domain API for common operations.

---

## Phase 8 — Drafting/PMI automation v0

### Tasks

- [ ] drawing sheet abstraction;
- [ ] base/projected/isometric/section views;
- [ ] automatic view-layout strategy interface;
- [ ] PMI retrieval/association;
- [ ] centerlines;
- [ ] hole callouts;
- [ ] title block attribute mapping;
- [ ] parts list/balloons for assemblies;
- [ ] PDF/DXF export;
- [ ] drawing validation report;
- [ ] initial ESKD-oriented policy layer without hardcoding organization-specific standards into core.

### Tests

- semantic golden drawings;
- visual layout regression;
- changed-model update test;
- no orphan annotation checks;
- title-block/BOM consistency;
- 2512/2606 behavior comparison.

### Exit gate

A controlled production-like part can produce a reviewable drawing package from one high-level Go request.

---

## Phase 9 — Workflow/declarative API

### Tasks

- [ ] operation plan schema;
- [ ] `GenerateDrawing` workflow;
- [ ] `PrepareReleasePackage`;
- [ ] `ValidatePart` / `ValidateAssembly`;
- [ ] dry-run/planning where meaningful;
- [ ] workflow progress events;
- [ ] staged outputs + manifest;
- [ ] retry policy limited to safe/idempotent boundaries.

### Exit gate

Applications can describe coarse CAD jobs without orchestrating individual NX operations.

---

## Phase 10 — Multi-version compatibility hardening

### Tasks

- [ ] formal compatibility matrix;
- [ ] validated pinned 2512 build;
- [ ] validated pinned 2606 build;
- [x] initial `nxctl test matrix` command accepts explicit side-by-side NX roots for real smoke;
- [ ] release adapter cleanup;
- [ ] capability fallback tests;
- [ ] automated NX upgrade report;
- [ ] document deprecation windows.

### Exit gate

Same high-level test suite passes on two release families with explicit, documented exceptions only.

---

## Phase 11 — Advanced escape hatches

### Tasks

- [ ] typed UF exposure strategy;
- [ ] restricted reflection invocation;
- [ ] external NXOpen library execution policy;
- [ ] journal execution/developer mode;
- [ ] recorded-journal analysis prototype;
- [ ] wrapper recipe generator prototype.

### Gate

Security tests prove hardened mode disables dangerous escape hatches.

---

## Phase 12 — Reliability/performance campaign

### Tasks

- [ ] benchmark no-op production IPC;
- [ ] batch vs N+1 benchmarks;
- [ ] large assembly enumeration benchmark;
- [ ] repeated worker memory/resource soak;
- [ ] crash recovery soak;
- [ ] handle leak detector;
- [ ] queue saturation behavior;
- [ ] log throughput tests;
- [ ] profile Go supervisor and Agent hotspots;
- [x] initial Fake-Agent `chaos`, `soak`, and `perf` command entry points (simulation only).

### Exit gate

Published performance baseline and no unbounded growth in defined soak workload.

---

## Phase 13 — Release engineering

### Tasks

- [ ] semantic versioning policy;
- [ ] signed/checksummed release artifacts;
- [ ] SBOM;
- [ ] compatibility table;
- [ ] install/update documentation;
- [ ] examples;
- [ ] migration guide;
- [ ] trademark/legal review;
- [ ] confirm Siemens licensing/redistribution obligations for deployment model.

### v1 gate

- stable high-level Go API for initial domains;
- documented raw escape hatch;
- real NX CI on supported builds;
- crash/log diagnostics;
- security baseline;
- no proprietary Siemens binaries in public release;
- end-to-end examples pass from clean setup.

---

# Cross-cutting backlog

## Documentation

- [ ] examples cookbook;
- [ ] troubleshooting guide based on real failures;
- [ ] NX error code mapping notes;
- [ ] ESKD drawing policy plugin specification;
- [ ] architecture diagrams as Mermaid/PlantUML if useful;
- [x] Engineering Standard, Testing Playbook, invariant catalog, executable quality-gate guide and Definition of Done;
- [x] Agent implementation/build boundary documentation.

## Developer experience

- [ ] `nxctl init` project scaffolding;
- [ ] local production-protocol Fake Agent mode;
- [ ] generated API searchable docs;
- [ ] VS Code/IDE task examples;
- [x] one-command fast developer gate (`nxctl test fast`);
- [x] one-command preliminary real-NX smoke (`nxctl test nx`) with fail-closed environment checks;
- [ ] one-command Agent Core test integrated into `nxctl test fast` (currently CI runs it as adjacent required step);
- [ ] warm-NX `--watch` development loop.

## Testing campaigns

- [x] exhaustive short-sequence session-health model test;
- [x] native Go fuzz target for stale epoch object references;
- [x] bootstrap protocol framing fuzz target;
- [ ] broader typed protocol/parser fuzz corpus;
- [ ] Go mutation testing campaign for session/retry/capability logic;
- [ ] C# mutation testing for Agent safety primitives;
- [ ] real-NX semantic fixture suite;
- [ ] metamorphic CAD tests;
- [ ] direct-NXOpen vs NXGO differential fixtures;
- [ ] Check-Mate independent oracle integration;
- [ ] destructive isolated-NX chaos suite;
- [ ] long-running real-NX resource soak.

## Future domains

- [ ] CAM;
- [ ] routing;
- [ ] CAE/Simcenter-specific adapters;
- [ ] Teamcenter-managed mode;
- [ ] remote worker gateway;
- [ ] worker pool/scheduler.

# Definition of done for every implementation iteration

An iteration is complete only when:

1. scope is implemented;
2. applicable `NXGO-INV-*` IDs are identified and compliance metadata is updated when enforcement changes;
3. relevant unit/contract/property/fuzz tests pass;
4. required real NX tests pass for affected validated builds before making real-NX compatibility claims;
5. semantic CAD postconditions are checked for CAD-affecting mutations;
6. logs contain correlation data where runtime integration exists;
7. resource cleanup/session health is verified;
8. docs/API examples are updated;
9. new architectural findings update this plan/ADR/invariant catalog;
10. no Siemens proprietary artifact is committed;
11. `main` remains buildable for non-NX CI;
12. the commit clearly distinguishes simulated/core evidence from real-NX evidence;
13. the change satisfies `docs/DEFINITION_OF_DONE.md`.
