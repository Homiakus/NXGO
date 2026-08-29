## Scope

Describe the change and affected architecture boundary.

## Invariants affected

List applicable stable IDs, for example:

- `NXGO-INV-MUT-001` — preserved by ...
- `NXGO-INV-COR-001` — semantic postcondition ...

## Test evidence

- [ ] `nxctl test fast`
- [ ] real NX required for this change
- [ ] `nxctl test nx` completed on pinned NX build (if applicable)
- [ ] isolated/chaos/matrix testing completed (if applicable)
- [ ] exact NX release/build recorded in evidence
- [ ] failure artifacts/log correlation checked

## Engineering result

For CAD-affecting changes, state the semantic oracle: body/feature count, mass/volume, assembly completeness, drawing objects, Check-Mate result, export metadata, etc.

## Definition of Done

- [ ] Reviewed against `docs/DEFINITION_OF_DONE.md`
- [ ] Documentation and `MASTER_PLAN.md` updated if architecture/scope changed
