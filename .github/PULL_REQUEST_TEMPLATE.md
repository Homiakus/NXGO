## Scope

Describe the change and affected architecture boundary.

## Invariants affected

List applicable stable IDs, for example:

- `NXGO-INV-MUT-001` — preserved by ...
- `NXGO-INV-COR-001` — semantic postcondition ...

## Architecture risk impact

List every applicable `RISK-ARCH-*` ID from `docs/ARCHITECTURE_FMEA.md` / `policy/architecture-risks.json`, or state `none` with a short rationale.

- Applicable risks: `RISK-ARCH-...`
- Does this change alter Severity / Occurrence / Detection / status? Explain and cite evidence.
- Does it introduce a new architectural failure mode? If yes, add the risk to the register and `MASTER_PLAN.md` in this PR.

- [ ] Applicable architecture risks reviewed
- [ ] Any score/status change is backed by implementation + evidence
- [ ] Active residual risk `>= 150` has explicit remediation work and exit criteria in `MASTER_PLAN.md`
- [ ] `policy/architecture-risks.json`, `docs/ARCHITECTURE_FMEA.md`, and `MASTER_PLAN.md` are synchronized when risk state changes

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
- [ ] `go run ./cmd/invariantcheck` passes
