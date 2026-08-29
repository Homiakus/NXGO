# NXGO design documentation

This directory is the normative design package for NXGO. Implementation is expected to follow these documents unless an ADR explicitly changes a decision.

## Start here

For day-to-day implementation use these documents first:

1. **[Engineering rules — quick reference](RULES_QUICK_REFERENCE.md)** — compact MUST/MUST NOT checklist.
2. **[Engineering Standard](ENGINEERING_STANDARD.md)** — normative programming/design rules for Go, C# Agent, NXOpen, IPC, state, codegen, observability and reviews.
3. **[Testing Playbook](TESTING_PLAYBOOK.md)** — test techniques, NX-in-the-loop execution, warm/isolated workers, matrix/chaos/soak/mutation/fuzz policy.
4. **[Executable quality gates](EXECUTABLE_QUALITY_GATES.md)** — what is already enforced by code, CI and a real-NX smoke runner.
5. **[Definition of Done](DEFINITION_OF_DONE.md)** — merge/release completion checklist.
6. **[Programming invariants](invariants/README.md)** — stable rules derived from concrete NX/NXOpen failure modes.
7. [Invariant enforcement plan](INVARIANT_ENFORCEMENT.md) — how prose rules become code, analyzers, guards and CI gates.
8. [`policy/invariant-compliance.json`](../policy/invariant-compliance.json) — machine-readable implementation status for invariant enforcement.

## Full reading order

1. [Product requirements](PRODUCT_REQUIREMENTS.md)
2. [Architecture](ARCHITECTURE.md)
3. [Engineering rules — quick reference](RULES_QUICK_REFERENCE.md)
4. [Engineering Standard](ENGINEERING_STANDARD.md)
5. [Programming invariants — MUST/MUST NOT rules](invariants/README.md)
6. [Invariant enforcement plan](INVARIANT_ENFORCEMENT.md)
7. [Executable quality gates](EXECUTABLE_QUALITY_GATES.md)
8. [Go API design](API_DESIGN.md)
9. [NX Agent](NX_AGENT.md)
10. [Protocol](PROTOCOL.md)
11. [Object and lifetime model](OBJECT_MODEL.md)
12. [Versioning and capabilities](VERSIONING.md)
13. [Observability](OBSERVABILITY.md)
14. [Testing architecture](TESTING.md)
15. [Testing Playbook](TESTING_PLAYBOOK.md)
16. [Definition of Done](DEFINITION_OF_DONE.md)
17. [Security](SECURITY.md)
18. [CLI](NXCTL.md)
19. [API scanner and code generation](CODEGEN.md)
20. [Deployment](DEPLOYMENT.md)
21. [Quality attributes](QUALITY_ATTRIBUTES.md)
22. [References](REFERENCES.md)
23. [ADRs](adr/)
24. [Implementation master plan](../MASTER_PLAN.md)

## Normative language

`MUST`, `MUST NOT`, `SHOULD`, `SHOULD NOT`, and `MAY` are used in the RFC 2119 sense. Where documents disagree, accepted ADRs take precedence, then P0/P1 invariant safety constraints, then the Engineering Standard, then `ARCHITECTURE.md`, then domain-specific documents.

## Architectural thesis

NXGO should feel like a native Go library while executing through Siemens-supported extension points. It therefore separates:

- a **Pure-Go client/domain SDK**;
- a **typed versioned protocol**;
- a **small in-process .NET NX Agent**;
- a **serialized/reentrancy-aware NX execution boundary**;
- **generated low-level bindings** for breadth;
- **handwritten high-level APIs** for ergonomics;
- a **Go supervisor/test runner** for process control, NX-in-the-loop testing and observability.

This separation is the central mechanism for hiding NX complexity without sacrificing access to advanced functionality.

## Safety contract

The [invariant catalog](invariants/README.md) converts observed NX/NXOpen failure modes into stable rules such as: no arbitrary-thread NXOpen calls, no stale-handle reuse, unconditional Builder cleanup, no blind mutation retries, no single-version compatibility assumptions and no screenshot-only correctness claims.

A P0/P1 invariant violation is an architectural defect, not a style preference.

Enforcement status MUST be represented honestly. `enforced`, `partially_enforced` and `planned` are distinct states; simulated Fake-Agent evidence must never be presented as proof of real Siemens NX behavior. `cmd/invariantcheck` validates the compliance metadata during the fast CI loop.

## Testing contract

NX is a normal test dependency for NX-dependent behavior. The project deliberately combines cheap pure tests with controlled real-NX workers:

```text
fast/no-NX
  -> fake Agent/contracts
  -> warm real NX
  -> isolated real NX for recovery/destructive cases
  -> supported-version matrix
  -> nightly mutation/fuzz/differential/chaos/soak/performance campaigns
```

Mock-only tests cannot establish compatibility or correctness of code that depends on NXOpen/kernel/session behavior. See [TESTING_PLAYBOOK.md](TESTING_PLAYBOOK.md).

## Executable commands already present

```text
go run ./cmd/nxctl test fast
go run ./cmd/nxctl test nx
go run ./cmd/nxctl test matrix
go run ./cmd/nxctl test chaos
go run ./cmd/nxctl test soak
go run ./cmd/nxctl test perf
```

The real-NX commands are intentionally fail-closed: without Windows, `NXGO_NX_HOME`, and a real Siemens `run_journal.exe`, the NX smoke cannot report PASS. See [EXECUTABLE_QUALITY_GATES.md](EXECUTABLE_QUALITY_GATES.md).
