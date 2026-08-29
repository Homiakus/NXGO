# Contributing

NXGO is currently architecture-first. Changes should preserve the documented boundaries until an ADR changes them.

## Before implementing

1. Read `docs/ARCHITECTURE.md`.
2. Read the applicable **normative** rules in `docs/invariants/README.md`; P0/P1 violations are merge blockers.
3. Check `MASTER_PLAN.md` for the current phase.
4. For architectural changes or deliberate invariant exceptions, add/update an ADR first.
5. Do not add Siemens proprietary binaries or generated content derived by copying proprietary documentation.

## Code rules

- Go public SDK remains free of Siemens dependencies and should support `CGO_ENABLED=0`.
- Generated code is not edited manually.
- NX-specific behavior is isolated in Agent adapters/capabilities.
- Blocking public methods accept `context.Context`.
- New public operations require stable error semantics, semantic postconditions and tests.
- Avoid chatty RPC APIs; prefer bulk/coarse operations.
- Do not invoke NXOpen from arbitrary transport/background threads.
- Do not keep/revive handles across NX session epochs.
- Do not reuse failed Builders; destroy all Builders on every path.
- Do not blindly retry mutations.
- Do not use raw recorded `FindObject` selectors as production domain logic.
- Do not claim cross-version/headless/managed compatibility without the corresponding real-NX tests.

## Invariant declaration in PRs

Changes touching Agent/NX behavior should state which invariant IDs are relevant, for example:

```text
Invariants affected:
- NXGO-INV-MUT-001 — preserved by BuilderScope
- NXGO-INV-COR-001 — adds semantic body-count/volume postcondition
- NXGO-INV-VER-001 — exercised on supported NX matrix
```

If a design cannot satisfy an invariant, do not silently work around it. Propose an ADR explaining the failure mode, replacement safety mechanism and test evidence.

## Required tests

A feature PR should include the lowest-cost meaningful layers plus real NX coverage when it touches NX behavior. See `docs/TESTING.md` and `docs/INVARIANT_ENFORCEMENT.md`.

## Commit expectations

Prefer atomic changes with conventional-style summaries, for example:

```text
feat(protocol): add capability handshake
feat(agent): add serialized command executor
test(nx): add 2606 drawing smoke fixture
docs(adr): define remote worker security boundary
```

## Documentation

Update `MASTER_PLAN.md` when implementation discoveries alter scope, ordering, risks or architectural assumptions. Add new recurring failure modes to the invariant catalog rather than leaving them as tribal knowledge.