# NXGO design documentation

This directory is the normative design package for NXGO. Implementation is expected to follow these documents unless an ADR explicitly changes a decision.

## Reading order

1. [Product requirements](PRODUCT_REQUIREMENTS.md)
2. [Architecture](ARCHITECTURE.md)
3. **[Programming invariants — MUST/MUST NOT rules](invariants/README.md)**
4. [Invariant enforcement plan](INVARIANT_ENFORCEMENT.md)
5. [Go API design](API_DESIGN.md)
6. [NX Agent](NX_AGENT.md)
7. [Protocol](PROTOCOL.md)
8. [Object and lifetime model](OBJECT_MODEL.md)
9. [Versioning and capabilities](VERSIONING.md)
10. [Observability](OBSERVABILITY.md)
11. [Testing](TESTING.md)
12. [Security](SECURITY.md)
13. [CLI](NXCTL.md)
14. [API scanner and code generation](CODEGEN.md)
15. [Deployment](DEPLOYMENT.md)
16. [Quality attributes](QUALITY_ATTRIBUTES.md)
17. [References](REFERENCES.md)
18. [ADRs](adr/)
19. [Implementation master plan](../MASTER_PLAN.md)

## Normative language

`MUST`, `MUST NOT`, `SHOULD`, `SHOULD NOT`, and `MAY` are used in the RFC 2119 sense. Where documents disagree, accepted ADRs take precedence, then the invariant catalog for safety constraints, then `ARCHITECTURE.md`, then domain-specific documents.

## Architectural thesis

NXGO should feel like a native Go library while executing through Siemens-supported extension points. It therefore separates:

- a **Pure-Go client/domain SDK**;
- a **typed versioned protocol**;
- a **small in-process .NET NX Agent**;
- a **serialized NX execution boundary**;
- **generated low-level bindings** for breadth;
- **handwritten high-level APIs** for ergonomics;
- a **Go supervisor/test runner** for process control and observability.

This separation is the central mechanism for hiding NX complexity without sacrificing access to advanced functionality.

## Safety contract

The [invariant catalog](invariants/README.md) converts observed NX/NXOpen failure modes into stable rules such as: no arbitrary-thread NXOpen calls, no stale-handle reuse, unconditional Builder cleanup, no blind mutation retries, no single-version compatibility assumptions and no screenshot-only correctness claims.

A P0/P1 invariant violation is an architectural defect, not a style preference.