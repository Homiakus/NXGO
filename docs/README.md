# NXGO design documentation

This directory is the normative design package for NXGO. Implementation is expected to follow these documents unless an ADR explicitly changes a decision.

## Reading order

1. [Product requirements](PRODUCT_REQUIREMENTS.md)
2. [Architecture](ARCHITECTURE.md)
3. [Go API design](API_DESIGN.md)
4. [NX Agent](NX_AGENT.md)
5. [Protocol](PROTOCOL.md)
6. [Object and lifetime model](OBJECT_MODEL.md)
7. [Versioning and capabilities](VERSIONING.md)
8. [Observability](OBSERVABILITY.md)
9. [Testing](TESTING.md)
10. [Security](SECURITY.md)
11. [CLI](NXCTL.md)
12. [API scanner and code generation](CODEGEN.md)
13. [Deployment](DEPLOYMENT.md)
14. [Quality attributes](QUALITY_ATTRIBUTES.md)
15. [References](REFERENCES.md)
16. [ADRs](adr/)
17. [Implementation master plan](../MASTER_PLAN.md)

## Normative language

`MUST`, `MUST NOT`, `SHOULD`, `SHOULD NOT`, and `MAY` are used in the RFC 2119 sense. Where documents disagree, accepted ADRs take precedence, then `ARCHITECTURE.md`, then domain-specific documents.

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