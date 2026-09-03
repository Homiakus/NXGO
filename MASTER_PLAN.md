# NXGO MASTER PLAN

Status: **Living implementation plan**  
Created: 2026-08-29  
Last major audit update: 2026-09-02

This file is the single execution roadmap for NXGO. Do not create a parallel roadmap for implementation work. New architectural findings, production incidents, failed tests, NX-version incompatibilities, and audit findings must update this plan and, when appropriate, the ADR and invariant catalogs.

---

# 0. North-star outcome

A Go developer can import NXGO and perform high-value Siemens NX / Designcenter automation through a stable, idiomatic API without directly managing:

- NXOpen builder lifecycles;
- Siemens DLL loading;
- NX main-thread execution constraints;
- remote NX object lifetime;
- undo/update semantics;
- NX release differences;
- crash recovery;
- logs and syslog correlation;
- retry ambiguity;
- process isolation;
- staged output publication.

The target is **not** a handwritten 1:1 Go wrapper around NXOpen. NXGO is a safe automation runtime with layered APIs:

1. workflow/declarative API;
2. idiomatic domain API;
3. generated raw API;
4. UF/raw escape hatch;
5. controlled journal/library fallback.

---

# 1. Evidence policy and status model

The previous plan used broad labels such as `DELIVERED` or `REAL-NX EVIDENCE VERIFIED`. The 2026-09-02 audit found that those labels can overstate maturity when the implementation, fake-agent proof, and real-NX proof are at different levels.

Every major feature now tracks four independent dimensions:

- **D — Design**: architecture/contracts/invariants are specified;
- **I — Implementation**: production code exists;
- **S — Simulated proof**: no-NX / Fake-Agent / unit / fuzz / model tests exist;
- **R — Real-NX proof**: the production path passed semantic tests on a pinned NX build and produced retained evidence.

A feature is production-ready only when all required dimensions reach the release gate.

## Evidence classes

- `E0` — documentation only;
- `E1` — unit/static/model evidence;
- `E2` — production protocol with Fake Agent / local process evidence;
- `E3` — one pinned real NX release, local or self-hosted, with semantic assertions;
- `E4` — two supported NX release families with retained compatibility evidence;
- `E5` — soak/chaos/performance/security evidence at release scale.

**Rule:** documentation must never claim a higher evidence class than the repository can reproduce or point to through retained CI/test artifacts.

---

# 2. 2026-09-02 production-hardening audit checkpoint

The architecture remains sound, but the audit identified safety-critical gaps in the execution kernel. These findings override previous completion labels where they conflict.

## Confirmed strengths

- Pure-Go public SDK boundary;
- separate NX process / Agent architecture;
- explicit NX execution-thread gateway;
- framed local IPC;
- protocol envelopes and request IDs;
- session health model;
- object handles and registry concept;
- undo transaction layer;
- Fake-Agent chaos/idempotency model;
- supervisor and syslog harvesting;
- real-NX test harness structure;
- API metadata scanner;
- high-level domain/workflow direction;
- fuzz/chaos/soak campaign structure.

## Blocking audit findings

### A-001 — late response can corrupt RPC sequencing — **P0**

Current Go transport creates a per-call reader goroutine. When a context expires, `Call` can return while that reader remains blocked on the same pipe. A later call may create another reader. The client also does not strictly reject a response whose `request_id` differs from the outstanding request.

Risk: a late response to request A can be consumed as request B, including for mutating operations.

### A-002 — timed-out NX work may still execute — **P0**

The production bundled Agent can return timeout while queued work remains executable by the NX thread.

Risk: caller observes failure, mutation later commits, caller retries, mutation is duplicated or state becomes ambiguous.

### A-003 — Fake-Agent idempotency is stronger than production Agent — **P0**

Fake-Agent records request results and can model committed-mutation/lost-response replay. Production Agent does not yet provide equivalent request-journal/idempotency guarantees.

Risk: chaos tests prove behavior that production does not enforce.

### A-004 — object resolution is not consistently fail-closed — **P0**

Some production resolver paths catch object-resolution errors and fall back to work/display part or the first body.

Risk: a stale/foreign/invalid handle can cause an operation to target a different NX object rather than fail before mutation.

### A-005 — runtime ObjectRef contract diverges from invariant model — **P0**

The internal invariant type carries session/epoch/object/generation identity, while the wire handle does not consistently carry generation and the production registry does not use one canonical reference implementation.

### A-006 — mass/bounding-box unit contract conflicts with semantic tests — **P0**

The production UF conversion path and the Go real-NX tests currently expect incompatible units. This makes current geometry evidence unreliable until normalized against explicit unit contracts and independent oracles.

### A-007 — `Part.MassProperties` can resolve to one body — **P0**

A part-level query must represent all applicable bodies, not silently the first body.

### A-008 — public parameters can be accepted but ignored — **P0/P1**

Examples include feature boolean-operation/target-body fields whose backend behavior can remain hardcoded to create-new-body semantics.

Risk: syntactically successful call produces semantically wrong CAD.

### A-009 — save-on-close can suppress save failure — **P0**

Explicit save requests must not report a successful close after swallowing a failed save.

### A-010 — production Agent duplicates tested Agent.Core primitives — **P1**

The former `agent/bundle/AgentWorker.cs` contained duplicate execution, framing, lifecycle, registry, and parsing logic; it has been removed from the production source tree in favor of the canonical tested Agent Core implementation.

Risk: safety fixes/tests can apply to one path but not the actual real-NX runtime.

### A-011 — handwritten JSON parsing/formatting in production Agent — **P1**

String-search JSON parsing and manual response construction are not a stable protocol implementation for arbitrary paths, names, Unicode, quotes, braces, and control characters.

### A-012 — API scanner is not yet a generated raw API — **P1**

Scanner/diff exists, but signature IDs, overload-safe diffing, generated Go bindings, generated C# dispatch and source-to-manifest traceability are incomplete.

### A-013 — registry lifetime/quotas need hard limits — **P1**

Repeated queries can create registered handles without a fully enforced lease/quota/lifetime model.

### A-014 — local pipe security is development-grade — **P1**

Production endpoint still needs explicit per-user/worker ACL and peer validation.

### A-015 — public real-NX evidence is incomplete — **P1**

Fast CI is reproducible. Real-NX workflow exists, but release claims must require retained real-NX run evidence and semantic artifact validation rather than checklist status alone.

### A-016 — NX2512 canonical Agent bootstrap cannot acquire Session — **P0**

On the installed NX 2512 runner, `tests/nx/TestRealNXAgentBootstrapAndSessionQuery` starts `run_journal` and loads the canonical Protocol/Core/NXHost assemblies, but `NXOpen.Session.GetSession()` fails with `InvalidCastException` (`NXOpen.Session` to `NXOpen.TaggedObject`). The Python NX smoke still passes, so NX itself is available; the failure is isolated to the C# Agent entrypoint/load context. Two load-policy experiments (`Assembly.Load(byte[])` and `Assembly.LoadFrom`) and an NXOpen-free bootstrap produced the same failure. Until a Siemens-compatible C# entrypoint/assembly-binding fix is verified, canonical Agent real-NX semantic claims remain blocked.

---

# 3. Immediate development policy — HARDENING FREEZE

Until **Hardening Gate H6** is passed:

## Allowed

- protocol correctness fixes;
- Agent Core consolidation;
- object lifetime fixes;
- transaction/rollback fixes;
- unit normalization;
- supervisor/recovery work;
- semantic real-NX fixtures;
- API scanner correctness;
- security hardening;
- diagnostics and test infrastructure;
- documentation required to support the above.

## Frozen by default

Do not materially expand public NX feature coverage with new large domains such as:

- CAM;
- routing;
- CAE;
- broad PMI;
- advanced drafting;
- large feature catalogs;
- remote gateway;
- worker pool;
- arbitrary reflection escape hatches.

Small additions are allowed only when required to construct a hardening fixture or prove a safety invariant.

**Reason:** adding API surface before fixing the execution kernel multiplies unsafe semantics and migration cost.

---

# 4. Hardening program

<!-- HARDENING_STATUS_BEGIN -->
## Current hardening execution status — 2026-09-02

Evidence policy for this status block:

- **E2 / no-NX confirmed** means the canonical fast gate is reproducibly green and the stated invariant has deterministic automated coverage.
- **E3 / real-NX pending** means the production code path exists but Siemens NX semantic evidence has not yet been retained from the self-hosted Windows/NX runner.
- A phase is **not closed** merely because its no-NX subset is complete.

### H1 — protocol sequencing / cancellation — **E2 substantially complete, E3 pending**

Confirmed in `main`:

- one connection-owned Go receive loop with exact RequestID routing;
- bounded pending RPC set and duplicate in-flight RequestID rejection;
- unknown/duplicate response IDs and malformed frames are protocol-fatal;
- timeout/cancellation after send returns `ErrOutcomeUnknown`, quarantines the connection, and supervisor termination makes the NX worker disposable;
- production executor distinguishes queued, started, completed and cancelled-before-start work; a pre-start timeout cannot execute later;
- production execution-started timeout becomes `OUTCOME_UNKNOWN` and marks the session lost;
- 1,000 deterministic cancel-after-send/late-response cycles prove request B never receives or follows request A on the quarantined stream;
- explicit duplicate-response regression coverage is green.

Still required before H1 exit:

- real-NX blocked-queue cancellation fixture with before/after CAD state;
- real-NX timeout-after-start fixture;
- Agent/NX termination after mutation commit but before response delivery;
- retained long-cycle leak/resource evidence on the production Windows worker.

### H2 — mutation idempotency / outcome journal — **E2 partial, E3 pending**

Confirmed in `main`:

- bounded Agent.Core request journal and production runtime mutation journal;
- RequestID + operation + SHA-256 payload identity;
- same RequestID with different payload/operation fails closed;
- committed same-worker replay returns the cached response without repeating NX execution;
- duplicate in-flight request is rejected deterministically;
- cancelled-before-start result is cacheable and replayable as non-executed;
- started operation with unprovable result becomes `OUTCOME_UNKNOWN` and cannot be automatically replayed;
- journal quota is fail-closed.

Still required before H2 exit:

- durable journal state across Agent/NX process loss;
- explicit persistence/fsync/recovery policy and crash-transition tests;
- shared Fake-Agent / production-Agent conformance suite;
- real-NX committed-mutation/lost-response replay proving feature count remains one.

### H3 — ObjectRef / registry lifetime — **E2 substantially complete, E3 pending**

Confirmed in `main`:

- protocol v2 makes `Generation` mandatory in `ObjectHandleWire`;
- SDK validates SessionID, Epoch, ObjectID, Generation and expected Kind before IPC;
- production Agent validates the same identity and rejects missing/wrong/stale generation;
- implicit invalid-handle fallback to work/display part or first body is removed;
- released/unknown handle resolution fails closed;
- production registry is bounded to 4,096 live handles and records a high-watermark;
- protocol v1 peers are rejected at version negotiation rather than failing later during CAD execution.

Still required before H3 exit:

- explicit lease-scope lifecycle and scope release;
- dependent-handle invalidation on part/component close;
- per-request produced-handle quota in addition to the worker-wide bound;
- registry diagnostics surfaced through session/doctor telemetry;
- real-NX invalid-reference fixtures proving zero unintended CAD mutation.

### H4 — Agent Core consolidation — **OPEN**

The former legacy AgentWorker has been removed; H4 remains open for the remaining canonical dispatch/corpus and real-NX evidence work.

### H5/H6 semantic fixes already landed — **E2 confirmed, E3 pending**

- metric mass-property normalization now uses the explicit kg/m UF contract and converts once to mm/mm²/mm³/kg;
- UF bounding boxes remain in owning-part length units without the previous `/1000` corruption;
- part-level mass properties and bounding boxes aggregate all bodies instead of silently choosing the first body;
- `part.close(save=true)` no longer swallows save failures;
- unsupported Boolean/TargetBody feature options are rejected instead of being accepted and ignored.

Required evidence remains real-NX metric/imperial oracle fixtures, multi-body fixtures, save-failure fixtures and retained artifacts.

### Immediate next execution order

1. H4: make `NXGO.Agent.Core` the canonical production execution/protocol/journal layer with a minimal compiled NX adapter/bootstrap.
2. H2: add durable mutation journal recovery so process death after commit cannot permit blind replay.
3. H3: implement lease scopes, dependent invalidation, per-request handle budgets and registry telemetry.
4. Run the self-hosted `real-nx-quality-gate` on pinned NX 2512 and retain semantic artifacts for H1/H2/H3/H5/H6.
5. Only after those gates, reconsider the hardening freeze on broader NX API surface.
<!-- HARDENING_STATUS_END -->


# H0 — Baseline, reproducibility, and audit-lock

Priority: **P0**  
Target evidence: `E1 → E2`

## Tasks

- [x] record the audit findings A-001..A-015 in the invariant/ADR system where applicable;
- [x] add an `AUDIT_FINDING` or equivalent reference field to remediation tests/commits;
- [x] change completion language in docs so simulated evidence and real-NX evidence cannot be conflated;
- [x] capture current protocol fixtures and golden frames before refactoring;
- [x] capture current supported Go/.NET/NX runtime assumptions;
- [x] add a machine-readable capability/evidence manifest per tested NX release;
- [x] define canonical semantic units for every public geometry quantity;
- [x] define mutation classes: read-only / deterministic-idempotent / transactional / ambiguous-nonretryable;
- [x] document connection/session quarantine rules.

## Tests / gates

- [ ] fast CI remains green;
- [x] invariant checker rejects an implementation marked production-safe without required evidence references;
- [ ] no new public CAD mutation API is merged while H0-H6 freeze is active unless explicitly justified.

## Exit gate

Audit findings are represented in executable policy and all following work has unambiguous acceptance criteria.

---

# H1 — Protocol sequencing, cancellation, and ambiguity semantics

Priority: **P0**  
Addresses: A-001, A-002  
Target evidence: `E2 → E3`

## Architecture decision

Use **one reader** per production connection. Never permit independent concurrent `Receive` calls on the same framed stream.

Recommended design:

```text
pipe reader goroutine
      |
      v
validate frame/envelope
      |
      +--> pending[requestID] response channel
      +--> stream/event dispatcher
      +--> protocol-fatal => close connection
```

## Go transport tasks

- [x] replace per-call read goroutines with one connection-owned receive loop;
- [x] maintain a bounded `pending map[RequestID]callState`;
- [x] reject duplicate in-flight RequestIDs;
- [x] require exact `ResponseEnvelope.RequestID` correlation;
- [x] treat unknown/duplicate response IDs as protocol violation;
- [x] on framing/decode/correlation violation close the connection and mark session lost/ambiguous;
- [x] make request ID generation collision-resistant and testable;
- [x] distinguish caller cancellation from server-confirmed cancellation;
- [x] expose structured `ErrOutcomeUnknown` / `ErrSessionLost` rather than generic timeout for ambiguous mutation state;
- [x] ensure `Close` unblocks reader/writers and completes all pending calls exactly once;
- [x] bound pending-call count and payload memory.

## Agent executor tasks

- [x] queue items carry explicit states: queued / started / committed / completed / cancelled-before-start;
- [x] cancellation before NX execution removes/skips the item deterministically;
- [x] once NX execution starts, client cancellation must not imply rollback or non-execution;
- [x] return an explicit final outcome whenever the connection remains healthy;
- [x] if outcome cannot be proven after transport loss, quarantine the worker/session;
- [x] define cancellation semantics separately for read-only and mutating operations;
- [x] remove timeout behavior that allows an operation to execute later while caller assumes it did not.

## Required tests

### No-NX deterministic tests

- [x] timeout A, late response A, request B — B must never receive A;
- [x] 1,000 randomized request/timeout/late-response sequences;
- [x] duplicate response ID;
- [x] unknown response ID;
- [x] connection close with N pending calls;
- [x] server sends malformed frame between valid calls;
- [x] cancellation before dequeue prevents work execution;
- [x] cancellation after execution starts produces explicit ambiguous/final state, never false `not executed` semantics;
- [x] race test with concurrent callers;
- [x] goroutine leak test after repeated timeout cycles.

### Real-NX tests

- [ ] queue a mutation behind an intentionally blocked NX operation, cancel it before start, verify CAD state unchanged;
- [ ] force timeout after mutation start and validate quarantine/outcome policy;
- [ ] kill Agent/NX after commit but before normal response and verify retry policy does not blindly duplicate mutation.

## Exit gate H1

No scenario exists in which a timed-out/late response can be mistaken for another RPC, and no cancelled-before-start mutation executes later.

---

# H2 — Production idempotency and mutation outcome journal

Priority: **P0**  
Addresses: A-003  
Target evidence: `E2 → E3`

## Required model

Every mutating request gets a stable identity and payload fingerprint.

Suggested record:

```text
RequestID
Operation
PayloadHash
SessionID
Epoch
TxID
State: RECEIVED | STARTED | COMMITTED | ROLLED_BACK | FAILED | OUTCOME_UNKNOWN
ResultEnvelope
CreatedAt
CompletedAt
```

## Tasks

- [x] move idempotency behavior from Fake-Agent-only semantics into production Agent Core;
- [x] reject reuse of RequestID with different operation/payload hash;
- [x] return cached committed result for safe replay within the same compatible session epoch;
- [x] define which read-only operations may bypass journal persistence;
- [x] define journal retention and memory/disk bounds;
- [x] persist enough state for supervisor recovery policy where process loss occurs;
- [x] classify operations by replay policy;
- [x] prevent automatic retry of non-idempotent operations without proven outcome;
- [x] integrate correlation IDs and transaction IDs into journal entries;
- [x] emit diagnostic evidence for ambiguous transport loss.

## Tests

- [ ] committed mutation + lost response + same RequestID => mutation count remains one;
- [x] same RequestID + different payload => hard protocol error;
- [x] duplicate request while original is executing => deterministic dedup/wait/reject policy;
- [x] rollback result replay;
- [x] journal quota behavior;
- [ ] process crash during each state transition;
- [ ] real-NX feature creation replay does not create duplicate features.

## Exit gate H2

Fake-Agent and production Agent implement the same mutation replay contract, verified by a shared conformance suite.

---

# H3 — Canonical ObjectRef, leases, and fail-closed NX object resolution

Priority: **P0**  
Addresses: A-004, A-005, A-013  
Target evidence: `E2 → E3`

## Canonical handle

Use one wire/runtime model everywhere:

```text
SessionID
Epoch
ObjectID
Generation
Kind
LeaseScopeID
```

`NativeTag` may be diagnostics-only; it must not be trusted as cross-session identity.

## Tasks

- [ ] make one canonical ObjectRef definition the source of truth for Go protocol, Go proxies and C# Agent;
- [x] add generation to production wire protocol and registry;
- [x] increment generation/revoke identity where handle reuse could occur;
- [x] remove all `catch {}` fallbacks that convert invalid handles into work/display part or first body;
- [x] if a reference field is supplied and cannot be resolved, return an error before any NX mutation;
- [x] distinguish absent optional reference from invalid supplied reference;
- [x] enforce expected `Kind` on every resolver;
- [x] invalidate dependent handles when a part/component lifecycle invalidates them;
- [x] implement explicit lease scopes for query-created ephemeral handles;
- [x] add per-session handle quotas and per-request produced-handle limits;
- [x] add bulk release and scope release in public SDK;
- [x] avoid creating persistent handles for value-snapshot results when no object proxy is needed;
- [x] record registry size/high-watermark diagnostics.

## Required adversarial tests

- [x] stale session handle;
- [x] stale epoch;
- [x] wrong generation;
- [x] wrong kind;
- [x] released handle;
- [x] closed-part handle;
- [x] handle from another worker;
- [x] body handle passed where part is expected;
- [x] explicitly invalid handle while a valid work part exists — operation must fail, never fall back;
- [x] repeated tree/body queries stay below defined registry bound after scope release;
- [x] fuzz reference decoding and lifecycle transition sequences.

## Real-NX semantic gate

For every invalid-reference mutation fixture, record before/after semantic state and prove zero unintended CAD changes.

## Exit gate H3

NXGO object identity is fail-closed, generation-aware and bounded for long-running workers.

---

# H4 — Unify Agent Core and remove production-only duplicate safety logic

Priority: **P1**, but required before broader production claims  
Addresses: A-010, A-011  
Target evidence: `E2 → E3`

## Target structure

```text
agent/
  NXGO.Protocol/
  NXGO.Agent.Core/
  NXGO.Agent.NXAdapter/
  NXGO.Agent.NXHost/
```

The runtime loaded/executed by real NX must consume the same Core primitives exercised by ordinary CI.

## Tasks

- [x] eliminate duplicate `NxExecutor` implementation from the production launch path;
- [x] eliminate duplicate frame codec/server implementation by removing the legacy runtime;
- [x] eliminate duplicate SessionHealth implementation by removing the legacy runtime;
- [x] eliminate duplicate ObjectRegistry implementation by removing the legacy runtime;
- [x] eliminate duplicate BuilderScope implementation by removing the legacy runtime;
- [x] eliminate manual request dispatch parsing from the production runtime;
- [x] introduce strongly typed request/response envelope DTOs;
- [x] use a real JSON serializer compatible with supported NX/.NET runtime;
- [x] make operation admission and handshake capabilities derive from one explicit canonical registry;
- [ ] keep NX-specific adapters thin and release-aware;
- [ ] keep bootstrap/journal entry point minimal;
- [x] make the production build fail if it bypasses canonical Core packages;
- [x] add cross-language golden protocol tests using production serializer;
- [x] add malformed/escaped/unicode payload corpus.

## JSON/path test corpus

Must include:

- quotes in names;
- backslashes;
- braces inside strings;
- Unicode/Cyrillic/CJK;
- newline/control characters;
- long Windows paths;
- empty strings/null/omitted optional values;
- nested payloads;
- arrays with escaped content.

## Exit gate H4

There is one safety kernel. A bug fixed in Agent Core cannot remain unfixed in the production real-NX path because the production path uses that same implementation.

---

# H5 — Semantic CAD correctness, units, builders, save behavior

Priority: **P0/P1**  
Addresses: A-006, A-007, A-008, A-009  
Target evidence: `E3 → E4`

## H5.1 Canonical units contract

Public domain API v0 uses explicit units and never relies on undocumented implicit conversion.

Recommended initial contract:

- length: part units or explicit `LengthUnit` in request/response;
- area: square of declared length unit;
- volume: cube of declared length unit;
- mass: explicit mass unit;
- density: explicit unit;
- angles: radians internally, documented conversion helpers if exposed;
- transforms: homogeneous/unit-aware conventions documented.

## Tasks

- [x] write a unit table for every geometry protocol field;
- [x] replace magic UF unit conversions with named conversion functions/constants;
- [ ] verify UF call parameter semantics against installed NX API docs/metadata;
- [x] include returned unit metadata where ambiguity is possible;
- [ ] normalize `BoundingBox`, centroid, area, volume and mass consistently;
- [x] implement `Part.MassProperties` over all applicable bodies;
- [x] expose body-level and part-level mass properties as separate semantics;
- [ ] define behavior for sheet bodies/non-solid bodies/mixed parts;
- [ ] add density/material semantics instead of silently assuming a misleading mass value.

## H5.2 Feature parameter honesty

- [x] every public parameter is either implemented and verified or rejected as unsupported;
- [x] implement/reject `BooleanOp` deterministically;
- [x] implement/reject `TargetBodyRef` deterministically;
- [x] validate dimensions/direction vectors before creating NX builders;
- [x] normalize zero/negative/tolerance edge cases;
- [ ] add semantic postconditions after builder commit;
- [ ] integrate required UpdateManager/update semantics per operation recipe.

## H5.3 Builder lifecycle

- [x] all NX builders use canonical `BuilderScope`;
- [x] no same-builder retry after commit attempt;
- [x] destroy runs on success, NXException, cancellation path and validation failure;
- [ ] add builder leak diagnostics where NX permits;
- [ ] real-NX test builder failure followed by fresh-builder retry.

## H5.4 Save/close/data durability

- [x] never swallow an explicitly requested save failure;
- [x] `Close(save=true)` returns error and leaves state diagnosable when save fails;
- [ ] separate `Close`, `Save`, `SaveAs`, `ForceCloseDiscard` semantics;
- [ ] stage externally published files before atomic publish where possible;
- [x] validate output exists/non-zero and matches intended part/sheet before reporting success;
- [ ] propagate PartLoadStatus/PartSaveStatus diagnostics.

## Independent semantic oracles

For core geometry fixtures use at least two of:

- known analytical geometry values;
- direct NXOpen reference implementation fixture;
- NX Check-Mate/validation where suitable;
- exported neutral geometry inspected independently;
- differential NXGO vs direct NXOpen results.

## Mandatory real-NX fixtures

### Geometry

- [ ] 100 × 50 × 25 mm block: volume, area, centroid, bounding box;
- [ ] inch-unit equivalent of same physical solid;
- [ ] cylinder analytical fixture;
- [ ] multi-body part aggregate mass properties;
- [ ] body-level mass properties;
- [ ] rollback restores zero-body initial state;
- [ ] failed boolean operation leaves model unchanged/quarantined according to contract.

### Parts

- [ ] new/save/close/open;
- [ ] forced save failure;
- [ ] Unicode/path escaping;
- [ ] stale handle after close;
- [ ] work/display part does not mask invalid explicit ref.

### Assemblies

- [ ] add/remove component semantic tree assertions;
- [ ] transform correctness;
- [ ] BOM quantity aggregation;
- [ ] invalid component handle produces zero mutation.

### Drafting

- [ ] create/query sheet;
- [ ] PDF output exists and passes basic artifact validation;
- [ ] explicit unit/size checks;
- [ ] invalid sheet/part ref fails closed.

## Exit gate H5

Core part/geometry/assembly/drafting v0 semantics pass on pinned NX 2512 with independent numerical/structural postconditions.

---

# H6 — Real-NX evidence matrix, security, soak, and release re-entry gate

Priority: **P1**  
Addresses: A-014, A-015  
Target evidence: `E4 → E5`

## H6.1 Real-NX CI evidence

- [ ] self-hosted workflow stores exact NX release/build;
- [ ] retain test logs, syslog, Agent logs and semantic result manifest;
- [ ] retain failure artifacts;
- [ ] make release compatibility claims depend on a successful referenced run;
- [ ] add pinned NX 2512 lane;
- [ ] add pinned NX 2606 lane;
- [ ] run the same high-value semantic suite on both;
- [ ] generate machine-readable compatibility delta report;
- [ ] fail when a claimed supported build has no current evidence.

## H6.2 Named-pipe security

- [ ] explicit Windows DACL scoped to intended user/service identity;
- [ ] validate pipe ownership/peer expectations where practical;
- [ ] supervisor generates high-entropy worker secret/nonce;
- [ ] handshake binds client/worker/session identity;
- [ ] operation allowlist remains default;
- [ ] hardened mode disables journal/reflection/raw execution escape hatches;
- [ ] unauthorized local client test;
- [ ] malformed/flooding client test;
- [ ] payload and pending-request quotas.

## H6.3 Reliability campaign

- [ ] 8-hour real-NX warm-worker soak on representative workload;
- [ ] repeated create/save/close cycles;
- [ ] repeated assembly enumeration;
- [ ] repeated drawing/PDF cycles;
- [ ] worker memory high-watermark tracking;
- [ ] registry size high-watermark tracking;
- [ ] pipe/goroutine/thread leak tracking;
- [ ] crash-recovery cycle campaign;
- [ ] queue saturation behavior;
- [ ] log rotation/truncation campaign;
- [ ] forced NX termination at defined mutation phases.

## H6.4 Performance baseline

Publish at least:

- no-op/health RPC p50/p95/p99;
- batch vs N+1 query cost;
- part open/save/close timings;
- body/tree enumeration timings;
- startup/recycle timing;
- IPC payload throughput;
- memory growth during soak.

Performance is secondary to correctness: no optimization may weaken outcome, object-lifetime or thread-safety invariants.

## Hardening Gate H6 — criteria to unfreeze feature expansion

All must be true:

- [ ] H1 protocol/cancellation gate passed;
- [ ] H2 production idempotency gate passed;
- [ ] H3 fail-closed ObjectRef gate passed;
- [ ] H4 canonical Agent Core used by real NX;
- [ ] H5 semantic geometry/part/assembly/drafting v0 passed on NX 2512;
- [ ] same core semantic suite passed on NX 2606 or an explicit documented temporary exception exists;
- [ ] named-pipe ACL/security baseline passed;
- [ ] no known P0 correctness issue remains;
- [ ] soak shows no unbounded resource growth in the defined workload;
- [ ] retained real-NX evidence exists for compatibility claims.

Only after this gate may broad NX API expansion resume.

---

# 5. API scanner and generated raw layer — completion program

Current state: scanner/search/diff foundation exists; generated raw layer is incomplete.

## Tasks

- [x] scan approved NXOpen assemblies/metadata;
- [x] deterministic normalized manifest foundation;
- [x] type/method search and inspection;
- [x] basic manifest diff;
- [ ] define canonical overload-safe signature format;
- [ ] compute stable API signature IDs;
- [ ] include parameter type, ref/out, generic arity, static/instance and return type in diff;
- [ ] detect changed overloads, not only added/removed names;
- [ ] capture enum members and relevant constants;
- [ ] capture inheritance/interface relationships needed for binding generation;
- [ ] generate Go raw types/methods;
- [ ] generate C# dispatch glue;
- [ ] generate capability IDs from bindings;
- [ ] trace every generated symbol to source assembly/release/signature ID;
- [ ] produce reproducible `2512 -> 2606` compatibility report;
- [ ] compare scanner results against NX Open Reporter output on a representative subset where tooling is available;
- [ ] ensure no Siemens binaries are committed or published.

## Exit gate

A new supported NX build can be scanned and raw bindings regenerated deterministically; overload/signature breaking changes are identified reproducibly and mapped to capabilities.

---

# 6. Domain API roadmap after Hardening Gate

# D1 — Parts and attributes

- [x] open/new/save/close foundation;
- [x] work/display part queries foundation;
- [x] units/basic metadata foundation;
- [ ] batch attributes;
- [ ] explicit SaveAs/export semantics;
- [ ] bulk metadata query;
- [ ] part dependency/load-status diagnostics.

# D2 — Modeling

- [x] body summaries foundation;
- [x] block/cylinder foundation;
- [x] bounding box/mass properties foundation subject to H5 correction;
- [ ] semantic boolean create/unite/subtract/intersect;
- [ ] semantic hole;
- [ ] datum/plane/axis basics;
- [ ] sketch/profile strategy;
- [ ] extrude/revolve;
- [ ] fillet/chamfer;
- [ ] pattern;
- [ ] expression/parameter API;
- [ ] bulk geometry analysis.

Every operation requires:

- input validation;
- BuilderScope recipe;
- UpdateManager recipe;
- transaction policy;
- semantic postcondition;
- capability/version mapping;
- real-NX fixture.

# D3 — Assemblies

- [x] tree foundation;
- [x] add/remove component foundation;
- [x] basic transforms foundation;
- [x] BOM-friendly metadata foundation;
- [ ] constraints;
- [ ] arrangements/reference sets;
- [ ] suppression/load-state semantics;
- [ ] interpart references policy;
- [ ] large-assembly bulk query path.

# D4 — Drafting / PMI

- [x] drawing sheet foundation;
- [x] PDF/DXF export foundation;
- [ ] base/projected/isometric/section views;
- [ ] automatic view layout interface;
- [ ] PMI retrieval and association;
- [ ] dimensions/centerlines/hole callouts;
- [ ] title-block attribute mapping;
- [ ] assembly parts list/balloons;
- [ ] drawing validation report;
- [ ] ESKD-oriented policy plugin without organization-specific rules in core;
- [ ] semantic + visual regression fixtures.

# D5 — Workflow/declarative API

- [x] coarse operation-plan foundation;
- [x] PrepareReleasePackage foundation;
- [x] ValidatePart / ValidateAssembly foundation;
- [x] staged-output manifest foundation;
- [ ] dry-run/planning;
- [ ] progress events;
- [ ] compensation/rollback reporting;
- [ ] safe retry planner based on mutation classification;
- [ ] workflow resume semantics;
- [ ] deterministic release manifest with input/output hashes.

---

# 7. Multi-version compatibility

Target initial families:

- NX / Designcenter 2512;
- NX / Designcenter 2606.

## Tasks

- [ ] formal compatibility matrix;
- [ ] exact pinned build manifests;
- [ ] side-by-side discovery test;
- [ ] same domain conformance suite on both releases;
- [ ] capability fallback tests;
- [ ] release adapter isolation;
- [ ] automated API scan diff;
- [ ] automated behavioral diff report;
- [ ] deprecation policy/windows;
- [ ] explicit unsupported-capability errors rather than silent fallback.

## Exit gate

Supported-version claims are based on semantic conformance evidence, not only successful assembly loading or compilation.

---

# 8. Advanced escape hatches

Do not start broad implementation before H6.

- [ ] typed UF exposure strategy;
- [ ] restricted reflection invocation;
- [ ] external NXOpen library execution policy;
- [ ] journal execution developer mode;
- [ ] recorded-journal analysis prototype;
- [ ] wrapper recipe generator prototype;
- [ ] hardened-mode deny policy;
- [ ] security tests proving dangerous capabilities are unavailable by default.

---

# 9. Supervisor / lifecycle / observability

Existing foundation includes discovery, worker launch, status/doctor, syslog harvesting and worker diagnostics. Continue with:

- [ ] explicit worker state machine: starting / ready / busy / draining / dirty / poisoned / lost / stopped;
- [ ] quarantine reason codes;
- [ ] outcome-unknown propagation from protocol to supervisor;
- [ ] graceful drain before recycle where safe;
- [ ] orphan cleanup with ownership verification;
- [ ] structured process manifest with exact Agent/protocol/NX versions;
- [ ] request/transaction/session/run correlation across Go/Agent/NX syslog;
- [ ] artifact manifest containing hashes and test metadata;
- [ ] crash signature classification;
- [ ] resource high-watermark telemetry;
- [ ] optional worker recycling thresholds based on measured soak evidence.

---

# 10. Testing architecture

NXGO uses a layered verification ladder.

## Tier T0 — static and compile

- formatting;
- `go vet`;
- race-enabled Go tests;
- .NET compile/tests;
- invariant policy;
- generated-code reproducibility.

## Tier T1 — unit/property/fuzz

- protocol codec;
- response correlation;
- object references;
- session-health transitions;
- transaction planner;
- capability negotiation;
- unit conversion;
- API signature normalization.

## Tier T2 — production transport + Fake Agent

- actual framed transport;
- same DTO/serializer contract;
- idempotency conformance;
- timeout/late response;
- broken pipe;
- backpressure;
- chaos and outcome ambiguity.

## Tier T3 — warm real NX

Use for high-frequency development validation:

- part operations;
- geometry;
- assembly;
- drafting;
- transaction rollback;
- semantic assertions.

## Tier T4 — isolated destructive real NX

Use fresh NX process for:

- poison/session loss;
- process kill;
- hung operations;
- crash recovery;
- malformed client/security;
- ambiguous mutation outcomes.

## Tier T5 — compatibility matrix

Run the same semantic suite across every claimed NX release family.

## Tier T6 — reliability campaigns

- fuzz;
- mutation testing;
- metamorphic CAD tests;
- direct-NXOpen differential tests;
- chaos;
- soak;
- performance.

---

# 11. Required mutation-testing targets

## Go

- protocol response correlation;
- request-id handling;
- timeout/quarantine logic;
- capability negotiation;
- ObjectRef validation;
- transaction/outcome classification;
- unit conversion;
- supervisor recovery.

## C# Agent Core

- executor thread checks;
- cancellation-before-start;
- idempotency journal transitions;
- ObjectRegistry validation;
- BuilderScope cleanup;
- transaction rollback health transitions;
- dispatch validation;
- serializer/error mapping.

Mutation score is not a vanity metric. Surviving mutations in safety-critical branches become explicit remediation items.

---

# 12. Performance strategy

Do not optimize individual NXOpen calls prematurely. First eliminate N+1 IPC and repeated object marshaling.

Preferred order:

1. semantic bulk operations;
2. batch protocol calls;
3. value snapshots instead of handles when possible;
4. cache immutable release/capability metadata;
5. reduce Agent allocations only after profiling;
6. recycle workers only when soak data demonstrates benefit.

Performance regressions must not weaken synchronization, validation, rollback or evidence generation.

---

# 13. Security baseline

Before v1:

- [ ] per-user/service pipe ACL;
- [ ] random per-worker authentication material;
- [ ] bounded frame size;
- [ ] bounded pending request count;
- [ ] bounded registry handles;
- [ ] bounded logs/event buffers;
- [ ] allowlisted operations;
- [ ] safe filesystem output policy;
- [ ] hardened mode without arbitrary journal/library/reflection execution;
- [ ] no proprietary Siemens assemblies in public artifacts;
- [ ] dependency/SBOM generation;
- [ ] release checksums/signatures;
- [ ] threat-model review for shared workstation/self-hosted runner scenarios.

---

# 14. Developer experience

After hardening:

- [ ] `nxctl init` scaffolding;
- [ ] production-protocol local Fake Agent;
- [ ] generated searchable API documentation;
- [ ] `nxctl doctor --json`;
- [ ] `nxctl capabilities`;
- [ ] `nxctl api scan/diff/find/inspect` completion;
- [ ] `nxctl test fast` includes every NX-independent Agent Core gate;
- [ ] warm-NX `nxctl test --watch` loop;
- [ ] fixture generator;
- [ ] one-command compatibility matrix on configured self-hosted runners;
- [ ] troubleshooting command that bundles Agent/NX/syslog/manifests without proprietary binaries.

Public SDK requirements:

- idiomatic Go;
- `context.Context` on operations that may block;
- typed errors with `errors.Is/As` support;
- no Siemens types in public packages;
- no stringly typed capability checks in ordinary user code;
- explicit unsupported feature errors;
- no accepted-but-ignored parameters;
- stable semantic abstractions over NX builder details.

---

# 15. Future domains — post-v1 kernel

Only after H6 and stable initial domain release:

- CAM;
- routing;
- CAE/Simcenter adapters;
- Teamcenter-managed mode;
- remote worker gateway;
- worker pool/scheduler;
- enterprise policy/authorization;
- declarative drawing/package automation at organization scale.

---

# 16. Release engineering

## v0.x hardening releases

- unstable API allowed with migration notes;
- every release lists evidence level per domain;
- no `production-ready` wording before H6.

## v1 release gate

All required:

- [ ] H6 passed;
- [ ] stable initial public Go API;
- [ ] production Agent uses canonical tested Core;
- [ ] no known P0 correctness defect;
- [ ] safe mutation outcome/idempotency contract;
- [ ] fail-closed generation-aware object identity;
- [ ] semantic fixtures pass on supported NX releases;
- [ ] real-NX CI evidence retained;
- [ ] documented raw escape hatch;
- [ ] security baseline complete;
- [ ] crash/log diagnostics complete;
- [ ] compatibility table complete;
- [ ] examples pass from clean setup;
- [ ] SBOM/checksums/signing policy complete;
- [ ] Siemens licensing/redistribution review complete;
- [ ] no proprietary Siemens binary is published.

---

# 17. Execution order

The remediation sequence is intentionally strict:

```text
H0 evidence baseline
    ↓
H1 protocol + cancellation
    ↓
H2 idempotency + outcome journal
    ↓
H3 ObjectRef + leases
    ↓
H4 one production Agent Core
    ↓
H5 semantic CAD correctness
    ↓
H6 NX2512/NX2606 + security + soak
    ↓
resume API expansion
    ↓
complete generated raw layer
    ↓
expand Modeling/Assembly/Drafting/PMI
    ↓
advanced domains
    ↓
v1
```

Some implementation work may overlap, but **exit gates may not be skipped**.

---

# 18. Recommended first implementation batch

The first coding batch after this plan update should be limited to the highest-risk correctness layer:

1. add regression test reproducing timeout-A / late-response-A / request-B corruption;
2. redesign Go pipe client around one receive loop and strict request-ID correlation;
3. add `ErrOutcomeUnknown` and session quarantine semantics;
4. add regression test proving cancelled-before-start NX work never executes later;
5. remove resolver fallback from invalid explicit ObjectRefs;
6. add generation to the canonical wire reference;
7. add regression tests for stale/wrong-kind/foreign refs while a valid work part exists;
8. fix/lock public geometry unit contract and analytical fixtures;
9. correct part-level multi-body mass properties;
10. make save-on-close fail loudly on save error;
11. reject currently ignored feature parameters until implemented;
12. begin migration of real AgentWorker onto canonical Agent Core.

Do not bundle broad new NX feature coverage into this batch.

---

# 19. Definition of done for every implementation iteration

An iteration is complete only when:

1. scope is implemented in the production path, not only a Fake Agent/model;
2. applicable `NXGO-INV-*` IDs and audit findings are identified;
3. unit/contract/property/fuzz tests pass;
4. production transport tests pass where applicable;
5. required real-NX tests pass before making real-NX claims;
6. CAD mutations have semantic postconditions;
7. invalid/stale references fail before mutation;
8. timeout/cancellation outcome is explicit and safe;
9. builder/object/resource cleanup is verified;
10. logs contain request/session/transaction correlation;
11. evidence artifacts are retained for claimed compatibility;
12. documentation and capability manifests are updated;
13. no accepted public parameter is silently ignored;
14. no proprietary Siemens artifact is committed;
15. non-NX CI remains deterministic and green;
16. the change distinguishes simulated evidence from real-NX evidence;
17. architectural findings update this plan/ADR/invariant catalog;
18. the change satisfies `docs/DEFINITION_OF_DONE.md`.

---

# 20. Current maturity statement after 2026-09-02 audit

NXGO has a strong architecture and a meaningful implementation foundation, but the execution kernel is in **production-hardening**, not production-ready, state.

The project should be treated as:

- architecture: advanced;
- no-NX test architecture: advanced;
- domain API: early but usable for controlled development;
- real-NX semantic evidence: must be rebuilt under the H1-H6 gates;
- production mutation safety: not yet release-grade;
- broad NXOpen coverage: intentionally secondary until the hardening gate passes.

The shortest path to a strong NXGO is not adding more methods. It is proving that a small, representative vertical slice — `Connect → Part → Geometry → Transaction → Save → Failure/Recovery` — is correct under timeout, stale handles, lost responses, process failure and two supported NX releases. Once that kernel is trustworthy, the existing architecture can scale safely to much broader Siemens NX automation.
