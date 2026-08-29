# NXGO Definition of Done

Status: **normative merge/release checklist**.

A change is done only when all applicable sections below are satisfied. "Code compiles" is not sufficient for NX-dependent behavior.

## 1. Scope and architecture

- [ ] The change has a clear responsibility and correct package/component boundary.
- [ ] Applicable `NXGO-INV-*` rules were identified.
- [ ] No P0/P1 invariant is violated.
- [ ] New architectural exception has an accepted ADR.
- [ ] Siemens types/runtime details do not leak into public Go API unintentionally.
- [ ] Version-specific logic is isolated in the proper adapter/capability layer.

## 2. Public API

- [ ] Naming expresses domain intent, not incidental NXOpen Builder mechanics.
- [ ] Blocking/remote operation accepts `context.Context`.
- [ ] Dimensional values are unit-aware.
- [ ] Error semantics are stable and documented.
- [ ] Capability/license/unsupported semantics are defined.
- [ ] Mutation retry/idempotency semantics are defined.
- [ ] Bulk behavior avoids obvious N+1 RPC design.
- [ ] Returned object lifetime/session-epoch semantics are defined.

## 3. NX safety

- [ ] NXOpen/NXOpen.UF execution goes through the safe executor.
- [ ] Reentrancy/callback behavior is considered.
- [ ] Builders/resources are destroyed/disposed on every path.
- [ ] Failed Builders are not reused.
- [ ] Required NX update semantics are executed.
- [ ] Mutation rollback/quarantine behavior is defined.
- [ ] Semantic postconditions validate important engineering intent.
- [ ] Poison/suspect classification cannot leave worker incorrectly Healthy.
- [ ] Work/Display Part and assembly load-state assumptions are explicit.

## 4. Tests

- [ ] Lowest-cost unit/table tests exist.
- [ ] Property/model-based tests exist where state/value space benefits from them.
- [ ] Parser/protocol boundaries have negative/fuzz coverage where relevant.
- [ ] Fake Agent tests cover transport/failure semantics where relevant.
- [ ] Changed NX behavior has a real-NX test.
- [ ] Mutating operation has semantic postcondition testing.
- [ ] Failure/recovery behavior has a negative/fault test.
- [ ] Crash/poison/startup behavior uses isolated NX where required.
- [ ] Assembly-wide behavior tests partial/incomplete loading where relevant.
- [ ] Compatibility matrix is updated for supported releases.
- [ ] Critical decision logic has mutation testing or an explicit reason it is not yet practical.
- [ ] Long-lived resource behavior has leak/soak coverage when ownership/lifecycle changes.

## 5. Test evidence

For NX-backed changes:

- [ ] Exact NX release/build is recorded.
- [ ] Fixture/environment manifest is recorded.
- [ ] Test/run/request IDs correlate with logs.
- [ ] Session health before/after is known.
- [ ] Failure produces useful syslog/Agent/runner artifacts.
- [ ] Warm-worker reuse/reset is proven safe or worker is recycled.

## 6. Concurrency and performance

- [ ] Shared Go state passes relevant `go test -race` coverage.
- [ ] No new unbounded queue/registry/cache growth is introduced.
- [ ] New high-volume API is bulk/coarse enough.
- [ ] Performance-sensitive change has a representative benchmark.
- [ ] Benchmark compares RPC count/latency/memory where useful, not wall time alone.

## 7. Security

- [ ] Input is validated at trust boundaries.
- [ ] Dangerous raw/reflection/journal execution remains controlled by policy.
- [ ] Local/remote transport exposure is intentional.
- [ ] Logs/artifacts do not intentionally expose credentials/secrets.
- [ ] No proprietary Siemens binaries or copied proprietary documentation are committed.

## 8. Observability

- [ ] New important operation emits structured correlation-aware events.
- [ ] Native NX diagnostic context is preserved on failure.
- [ ] Severe failures preserve artifacts before recycle.
- [ ] Metrics/logs distinguish NX, Agent, transport and runner sources.

## 9. Documentation

- [ ] Public API/docs/examples updated with behavior.
- [ ] New recurring NX failure mode is documented/invariantized.
- [ ] `MASTER_PLAN.md` updated if scope/risk/order changed.
- [ ] Compatibility notes updated if behavior differs across NX releases.
- [ ] Examples obey current Engineering Standard.

## 10. Merge gate by change category

### Pure Go change

Requires fast loop, race coverage where concurrent, and mutation/property/fuzz techniques according to risk.

### Protocol change

Requires contract compatibility + fake Agent + real-NX smoke if Agent/client interaction changes.

### NX Agent/adapter change

Requires warm real NX; isolated NX for lifecycle/recovery changes; matrix when release-specific.

### Public domain API

Requires unit validation + fake Agent + real NX + semantic oracle + failure/cleanup cases + matrix entry.

### Drawing/PMI

Requires semantic drawing assertions + export validation + optional visual golden + version differential where supported.

### Supervisor/recovery

Requires real process termination/hang/failure tests and diagnostic artifact verification.

### New NX release support

Requires API scan/diff, generated build, contract tests and complete applicable real-NX compatibility matrix. Compilation alone is not acceptance.

## 11. Iteration completion

An implementation iteration is complete only when:

```text
implemented
+ reviewed against invariants
+ tests green at required layers
+ real NX evidence where applicable
+ failure/recovery verified
+ diagnostics preserved
+ docs updated
+ compatibility impact known
= DONE
```

Unfinished test/recovery/documentation work remains implementation work and must not be described as "done later" unless explicitly tracked as a non-release-blocking future enhancement.