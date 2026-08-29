# NXGO MASTER PLAN

Status: **Living implementation plan**  
Created: 2026-08-29

The plan is deliberately staged so the project proves the risky boundaries before expanding API surface. Every phase has explicit exit gates. Unexpected discoveries update this document and, when architectural, create an ADR.

## North-star outcome

A Go developer can import NXGO and perform high-value Siemens NX automation through a stable API without managing NXOpen builders, Siemens DLL loading, NX main-thread execution, remote object lifetime, release differences, logs or process recovery.

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
- [x] security baseline;
- [x] codegen/scanner design;
- [x] deployment and CLI specifications.

### Gate

No production implementation begins by adding ad-hoc cgo bindings or exposing Siemens types in public Go APIs.

---

## Phase 1 — Repository/build skeleton

### Tasks

- [ ] create Go module and package layout;
- [ ] create .NET Agent solution with release adapter boundary;
- [ ] create protocol schema project;
- [ ] create `nxctl` command;
- [ ] add formatting/linting/unit-test CI that does not require NX;
- [ ] add build scripts for Windows developer environment;
- [ ] document exact supported compiler/runtime versions discovered from target NX builds;
- [ ] add version metadata generation.

### Tests

- Go unit smoke;
- .NET unit smoke;
- protobuf/codegen reproducibility;
- `CGO_ENABLED=0 go test ./...` for Go-side packages where applicable.

### Exit gate

Fresh checkout can build non-NX components deterministically; Agent skeleton builds on an authorized NX developer machine.

---

## Phase 2 — Protocol + fake Agent vertical slice

### Tasks

- [ ] handshake messages;
- [ ] protocol version negotiation;
- [ ] capabilities;
- [ ] request/response envelope;
- [ ] stable error envelope;
- [ ] log/event stream;
- [ ] context cancellation/deadlines;
- [ ] named-pipe transport abstraction;
- [ ] fake Agent server for Go tests.

### Tests

- protocol backward/forward minor compatibility;
- cancelled request;
- broken pipe;
- malformed message;
- oversized message;
- unauthorized local client;
- stream backpressure.

### Exit gate

Go client completes handshake/call/log-stream against fake Agent with no NX installation.

---

## Phase 3 — Minimal real NX Agent

### Tasks

- [ ] Agent bootstrap inside NX;
- [ ] discover exact NX release/build;
- [ ] secured pipe endpoint;
- [ ] serialized NX command queue/main-thread gateway;
- [ ] health state;
- [ ] structured logging;
- [ ] correlation markers in NX log where supported;
- [ ] first safe command: session information;
- [ ] first part command: open/query/save/close controlled fixture.

### Tests

- attach to real interactive NX;
- start controlled worker;
- exact build reported;
- repeated connect/disconnect;
- timeout while command queued;
- Agent exception translated;
- NX process termination detected.

### Exit gate

`nxctl status` and a Go integration test communicate reliably with real pinned NX.

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

- [ ] opaque ObjectRef protocol;
- [ ] typed Go proxies;
- [ ] registry quotas;
- [ ] scopes/batch release;
- [ ] stale-handle detection;
- [ ] NX undo-mark transaction manager;
- [ ] rollback and dirty-session state;
- [ ] staging for file outputs.

### Tests

- handle release/leak tests;
- stale handle after restart;
- rollback success;
- injected rollback failure;
- quota enforcement;
- transaction cancellation.

### Exit gate

Mutating integration tests cannot leak handles and failed mutations either roll back or explicitly quarantine the worker.

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

- [ ] benchmark no-op IPC;
- [ ] batch vs N+1 benchmarks;
- [ ] large assembly enumeration benchmark;
- [ ] repeated worker memory/resource soak;
- [ ] crash recovery soak;
- [ ] handle leak detector;
- [ ] queue saturation behavior;
- [ ] log throughput tests;
- [ ] profile Go supervisor and Agent hotspots.

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
- [ ] architecture diagrams as Mermaid/PlantUML if useful.

## Developer experience

- [ ] `nxctl init` project scaffolding;
- [ ] local fake Agent mode;
- [ ] generated API searchable docs;
- [ ] VS Code/IDE task examples;
- [ ] one-command developer smoke test.

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
2. relevant unit/contract tests pass;
3. required real NX tests pass for affected validated builds;
4. logs contain correlation data;
5. resource cleanup is verified;
6. docs/API examples are updated;
7. new architectural findings update this plan/ADR;
8. no new Siemens proprietary artifact is committed;
9. `main` remains buildable for non-NX CI;
10. the commit clearly states what was verified.