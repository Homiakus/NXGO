# Invariant enforcement plan

The invariant catalog is normative only if violations are progressively made difficult or impossible.

## 1. Compile-time/package boundaries

- Only approved Agent namespaces/projects may reference Siemens `NXOpen*` assemblies.
- Transport/server code must depend on an `INxExecutor`, not `Session`/`UFSession` directly.
- Public Go domain packages must not import generated raw packages or Siemens artifacts.
- Generated code is reproducible and never hand-edited.

Add architecture tests/analyzers for these dependency rules.

## 2. Safe wrappers

Implement central primitives that encode invariants:

- `NxExecutor` — serialized, reentrancy-aware execution;
- `BuilderScope<T>` — unconditional destroy + single-attempt semantics;
- `ModelMutationScope` — undo/update/postcondition lifecycle;
- `ObjectRegistry` — session epoch/generation + quotas/scopes;
- `SubscriptionRegistry` — callback ownership/unregister;
- `SessionHealth` — healthy/suspect/poisoned state machine;
- `CapabilityRegistry` — release/license/headless/managed requirements;
- `IdempotencyStore` — duplicate mutation request protection;
- `ArtifactCollector` — bounded crash evidence collection.

Developers SHOULD use these primitives instead of implementing local variants.

## 3. Static/repository checks

CI SHOULD eventually fail on:

- direct NXOpen references outside approved Agent layer;
- `Create*Builder` not dominated by a Builder scope in handwritten Agent recipes;
- forbidden raw `FindObject` in production recipe directories;
- public Go dimensional fields using unqualified numeric types where a quantity type is required;
- registered callback without registry ownership;
- generated code modified by hand;
- unsupported API manifest drift.

Some checks require analyzers rather than regex; start with high-signal rules and avoid noisy gates.

## 4. Runtime guards

Agent MUST reject:

- stale epoch handles;
- operations on poisoned session;
- worker-incompatible UI/graphics capability;
- unsupported release/capability;
- registry quota overflow;
- invalid current Work/Display context where the operation requires explicit target;
- strict full-assembly operations with partial load.

## 5. Test gates

Every P0 invariant needs at least one executable negative test. The test should demonstrate the forbidden behavior is rejected/prevented or that the recovery state machine reacts safely.

Create an `invariant-test-map` artifact in CI mapping `NXGO-INV-*` IDs to test names. Missing P0 coverage is a release blocker after the corresponding component exists.

## 6. Code review format

PRs touching Agent/NX behavior should include:

```text
Invariants affected:
- NXGO-INV-MUT-001: preserved by BuilderScope
- NXGO-INV-COR-001: adds body-count/volume postcondition
- NXGO-INV-VER-001: tested on 2512 + 2606
```

Any deliberate exception must cite an ADR.

## 7. Error/log integration

Invariant-related runtime rejection SHOULD emit the invariant ID in structured diagnostics, for example:

```json
{"kind":"stale_object","invariant":"NXGO-INV-OBJ-002","epoch":43,"handle_epoch":42}
```

This makes operational incidents traceable back to architectural rules.