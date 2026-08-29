# Contributing

NXGO is currently architecture-first. Changes should preserve the documented boundaries until an ADR changes them.

## Before implementing

1. Read `docs/ARCHITECTURE.md` and relevant domain docs.
2. Check `MASTER_PLAN.md` for the current phase.
3. For architectural changes, add/update an ADR first.
4. Do not add Siemens proprietary binaries or generated content derived by copying proprietary documentation.

## Code rules

- Go public SDK remains free of Siemens dependencies and should support `CGO_ENABLED=0`.
- Generated code is not edited manually.
- NX-specific behavior is isolated in Agent adapters/capabilities.
- Blocking public methods accept `context.Context`.
- New public operations require stable error semantics and tests.
- Avoid chatty RPC APIs.
- Do not invoke NXOpen from arbitrary transport threads.

## Required tests

A feature PR should include the lowest-cost meaningful layers plus real NX coverage when it touches NX behavior. See `docs/TESTING.md`.

## Commit expectations

Prefer atomic changes with conventional-style summaries, for example:

```text
feat(protocol): add capability handshake
feat(agent): add serialized command executor
test(nx): add 2606 drawing smoke fixture
docs(adr): define remote worker security boundary
```

## Documentation

Update `MASTER_PLAN.md` when implementation discoveries alter scope, ordering, risks or architectural assumptions.