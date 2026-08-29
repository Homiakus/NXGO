# Contributing

NXGO is architecture-first and test-driven around real NX behavior. Changes must preserve documented safety boundaries unless an ADR explicitly changes them.

## Before implementing

1. Read `docs/RULES_QUICK_REFERENCE.md`.
2. Read the applicable sections of `docs/ENGINEERING_STANDARD.md`.
3. Read the applicable **normative** rules in `docs/invariants/README.md`; P0/P1 violations are merge blockers.
4. Select the required test layers from `docs/TESTING_PLAYBOOK.md`.
5. Check `MASTER_PLAN.md` for the current phase.
6. For architectural changes or deliberate invariant exceptions, add/update an ADR first.
7. Do not add Siemens proprietary binaries or generated content copied from proprietary documentation.

## Code rules

- Go public SDK remains free of Siemens dependencies and should support `CGO_ENABLED=0` for client-side packages.
- Generated code is never edited manually.
- NX-specific behavior is isolated in Agent adapters/capabilities.
- Blocking public methods accept `context.Context`.
- Public dimensional inputs use explicit unit-aware types.
- New public operations require stable error semantics, semantic postconditions and tests.
- Avoid chatty RPC APIs; prefer bulk/coarse operations.
- Do not invoke NXOpen from arbitrary transport/background threads.
- Do not keep/revive handles across NX session epochs.
- Do not reuse failed Builders; destroy Builders/resources on every path.
- Do not blindly retry mutations; define idempotency when retry is allowed.
- Do not use raw recorded `FindObject` selectors as production domain logic.
- Do not treat partial assembly load as complete.
- Do not claim cross-version/headless/managed compatibility without corresponding real-NX tests.
- Do not use screenshots/visual goldens as the sole CAD correctness oracle.

The complete rules are normative in `docs/ENGINEERING_STANDARD.md` and `docs/invariants/`.

## Change classification

Mark a nontrivial change mentally or in the PR as one or more:

```text
pure-go
protocol
agent-core
nx-adapter
domain-api
codegen
workflow
observability
security
test-infrastructure
managed-mode
ui
```

Use the change-type matrix in `docs/TESTING_PLAYBOOK.md` to select required tests.

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

Testing is part of implementation.

Use the cheapest valid layers first, but NX-dependent behavior must eventually run against real NX.

Typical sequence:

```text
fast/no-NX
 -> fake Agent/contracts
 -> warm real NX
 -> isolated real NX when lifecycle/recovery requires it
 -> compatibility/campaign matrix where applicable
```

Important test techniques include unit/table, property/model-based, fuzz, mutation, contract, fake Agent integration, real NX integration, semantic CAD assertions, metamorphic, differential, golden, chaos, soak, performance, race and minimal UI smoke testing. See `docs/TESTING_PLAYBOOK.md`.

## Real NX evidence

A real-NX failure must be diagnosable. Tests should capture exact NX build, fixture/environment identity, run/test/request IDs, session health and relevant syslog/Agent/runner artifacts.

A test that leaves an uncertain worker state must recycle the worker rather than optimistically reuse it.

## Definition of Done

Before declaring work complete, apply `docs/DEFINITION_OF_DONE.md`.

At minimum a finished change has:

- implementation;
- invariant review;
- required test layers green;
- real-NX evidence when applicable;
- cleanup/recovery verification;
- diagnostics/correlation;
- documentation and compatibility impact updated.

## Commit expectations

Prefer atomic changes with conventional-style summaries, for example:

```text
feat(protocol): add capability handshake
feat(agent): add serialized command executor
test(nx): add 2606 drawing smoke fixture
docs(adr): define remote worker security boundary
```

## Documentation

Update `MASTER_PLAN.md` when implementation discoveries alter scope, ordering, risks or architectural assumptions. Add new recurring failure modes to the invariant catalog rather than leaving them as tribal knowledge. Examples and test helpers must obey the same engineering rules as production code.