# NX API scanner and code generation

## 1. Purpose

Handwriting wrappers for the entire NXOpen surface is not maintainable. NXGO uses code generation for breadth and handwritten APIs for ergonomics.

## 2. Inputs

Scanner operates against an approved installed NX version and reads metadata from relevant NXOpen assemblies and other publicly exposed API description sources available with the installation.

It MUST NOT require copying Siemens proprietary binaries into the NXGO source repository.

## 3. Normalized manifest

Output is deterministic JSON (or equivalent) containing:

- NX release/build;
- source assembly identity/hash metadata;
- namespaces/types;
- constructors/methods/properties/events;
- parameter/return types;
- enums/constants;
- generic/array/ref/out information;
- inheritance/interfaces;
- obsolete markers;
- stable normalized signature ID.

Paths and machine-specific details are removed from deterministic manifests.

## 4. Generator outputs

### C# Agent bindings/adapters
Strongly typed dispatch stubs and conversion glue where useful.

### Go raw bindings
Typed proxies in an internal/generated or public `raw` package. They use NXGO primitive/handle types, never .NET objects.

### Capability catalog
Map of operations/types available in the scanned release.

### Documentation index
Searchable names/signatures/examples metadata used by `nxctl api`.

## 5. Handwritten facade

Generated code is never manually edited. Handwritten domain modules call stable internal adapter interfaces and may combine multiple generated calls.

Directory intention:

```text
internal/generated/<manifest-version>/...
internal/agentgenerated/...
pkg/domain/...        # handwritten
```

Exact package layout will be validated by a prototype before freezing.

## 6. API diff algorithm

Match primarily by normalized fully-qualified signature, with secondary rename heuristics only for reporting. Never silently map a likely rename into runtime behavior without tests.

Report:

- additions;
- removals;
- signature/property changes;
- enum member changes;
- inheritance changes;
- deprecations;
- unresolved mapping candidates.

## 7. Documentation generation

Generated raw Go methods include:

- source NX type/signature;
- minimum validated NX capability/build where known;
- lifecycle warnings;
- link/key into local Siemens NXOpen help if discoverable without bundling proprietary docs.

## 8. Search

Create an index enabling:

```text
nxctl api find "hole callout"
```

Results rank:

1. NXGO high-level API;
2. generated NXOpen type/member matches;
3. UF matches;
4. known recipes/examples.

## 9. Journal-assisted discovery

Developer tooling MAY ingest a recorded journal and extract referenced NXOpen type/member sequences to propose a recipe or high-level wrapper skeleton. Generated suggestions require review and real NX tests.

## 10. Reproducibility

A manifest includes hashes/identifiers for source API assemblies so generated output can be traced to an exact installed environment.