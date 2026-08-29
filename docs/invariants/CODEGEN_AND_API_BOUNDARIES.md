# Code generation and API-boundary invariants

## NXGO-INV-GEN-001 — scanner models semantics, not names only

**MUST NOT:** generate bindings by mapping reflection signatures mechanically without classifying lifetime/ownership and special call semantics.

The scanner/generator MUST account for at least:
- overloads;
- `ref`/`out`/multi-result mapping;
- arrays/collections;
- enums and version moves;
- callbacks/delegates;
- nullable/null-object conventions;
- `Builder` lifecycle;
- `TransientObject`/`IDisposable` lifetime;
- tagged/live NX object references;
- `PartLoadStatus`-like disposable result objects;
- NXOpen.UF families;
- version-dependent method presence.

Unknown semantics should generate a lower-confidence/raw surface, not an unsafe high-level wrapper.

## NXGO-INV-GEN-002 — Siemens remoting is not NXGO remoting

**MUST NOT:** expose CLR `MarshalByRefObject`, `NXRemotableObject`, message sinks or Siemens remoting behavior as the cross-process protocol.

**MUST:** terminate Siemens runtime semantics at the Agent boundary and translate to NXGO protobuf DTOs/handles/events.

## Public API boundary rule

Generated raw types MUST NOT leak into handwritten domain signatures. Domain models remain stable Go-owned contracts and use release adapters internally.

## Generator validation

Every generated release must pass:
1. deterministic regeneration;
2. compile tests for Agent and Go raw layer;
3. manifest diff against previous supported release;
4. ownership/lifecycle lint;
5. selected real-NX invocation smoke tests;
6. capability publication only after NX-backed pass.