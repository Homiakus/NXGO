# Architecture

## 1. Context

NXGO must reconcile two incompatible realities:

- users want a normal Go library;
- NXOpen executes inside a Siemens-supported runtime and has session/UI/thread/lifecycle constraints.

The architecture therefore uses a process boundary instead of forcing Siemens runtime dependencies into the Go process.

## 2. Component model

```text
+----------------------------+
| Go application             |
|                            |
| nxgo workflow API          |
| nxgo domain API            |
| nxgo generated raw API     |
+-------------+--------------+
              |
       typed requests/events
              |
+-------------v--------------+
| Go transport/client        |
| discovery, deadlines,      |
| retries, handle scopes     |
+-------------+--------------+
              |
      Named Pipe (default)
              |
+-------------v--------------+
| NXGO Agent (.NET in NX)    |
|                            |
| auth/handshake             |
| request validation         |
| object registry            |
| NX command queue           |
| capability adapters        |
| structured logging         |
+-------------+--------------+
              |
      serialized execution
              |
+-------------v--------------+
| NXOpen / NXOpen.UF / NX    |
+----------------------------+
```

A separate Go process, `nxctl`/supervisor, discovers installations, launches workers, tails external logs, applies timeouts and collects crash artifacts.

## 3. Dependency rule

Dependencies point inward toward stable NXGO contracts:

- domain API MUST NOT import generated raw packages;
- generated packages MUST NOT dictate public domain models;
- transport DTOs MUST NOT become public domain types;
- Agent adapters MAY reference Siemens APIs;
- Go SDK MUST NOT reference Siemens binaries.

## 4. Execution model

RPC handlers MUST NOT call NXOpen directly from arbitrary transport threads.

Every NX-affecting request is converted into an internal command and scheduled on the NX-safe executor. Ordering is explicit. Reads MAY be optimized later only after proving a given NX API is safe; default is serialized execution.

Long operations report progress/events but remain one logical operation.

## 5. Domain modules

Initial domain boundaries:

- `session` / discovery;
- `parts`;
- `modeling`;
- `geometry`;
- `assemblies`;
- `drawings`;
- `pmi`;
- `attributes`;
- `validation`;
- `export`;
- `events`;
- `logs`;
- `raw`.

Future modules MAY include CAM, routing, CAE and Teamcenter integration without changing core transport contracts.

## 6. Three API planes

### Workflow plane
Coarse operations optimized for product workflows and low IPC overhead.

### Domain plane
Idiomatic Go objects and operations.

### Raw plane
Generated API enabling broad coverage and fast adoption of new NX releases.

High-level code SHOULD prefer workflow/domain APIs. Raw access is supported but carries weaker stability guarantees.

## 7. Reliability boundaries

NX is treated as an external stateful engine that can:

- hang;
- raise NX exceptions;
- terminate with a fatal error;
- lose a license;
- present version-specific behavior;
- leave a partially modified work part.

Therefore the supervisor owns process-level recovery. The Agent owns in-session cleanup/rollback. The Go SDK owns client-visible deadlines and typed error translation.

## 8. Transaction strategy

Mutations SHOULD execute under named NX undo marks when the API supports it.

```text
begin logical transaction
  create undo mark
  execute commands
  validate
  success -> retain/commit state
  failure -> rollback to mark
end
```

Transactions are session-local and do not pretend to provide ACID semantics across NX plus external services.

## 9. Performance strategy

Avoid N+1 IPC patterns. Provide bulk operations such as:

- `Body.Faces()` rather than per-index RPC;
- `Part.Analyze(AnalysisRequest)` returning a composite result;
- batch attribute get/set;
- batch export;
- one drawing-generation request rather than dozens of remote Builder manipulations.

Protocol messages carry compact stable IDs/handles rather than repeated full object descriptions.

## 10. Extension strategy

New NX release support proceeds through:

1. scan installed NXOpen assemblies/metadata;
2. generate release manifest;
3. diff against previous manifest;
4. regenerate low-level bindings/adapters;
5. compile Agent;
6. run contract tests;
7. run NX-backed regression matrix;
8. mark capabilities supported only after passing gates.

## 11. Architectural invariants

The following require an ADR to change:

- Pure-Go public SDK;
- out-of-process SDK/Agent boundary;
- no live NX object serialization;
- serialized NX executor by default;
- typed versioned protocol;
- capability negotiation;
- generated raw + handwritten high-level split;
- local IPC secured to the current user by default.