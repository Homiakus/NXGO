# NXGO

**NXGO** is a Go-first automation platform for Siemens NX / Designcenter.

The project goal is to let ordinary Go applications safely invoke NX capabilities through an idiomatic, version-tolerant API while hiding NXOpen session management, builder lifecycles, UI-thread constraints, object handles, version differences, logging, retries, rollback, and test orchestration.

> NXGO is intentionally **not** a 1:1 handwritten Go wrapper around NXOpen. The public SDK is Pure Go. A thin in-process .NET agent talks to NXOpen/NXOpen.UF, while generated bindings and escape hatches preserve broad NX coverage.

## Target architecture

```text
Go application
    |
    v
Pure-Go SDK (high-level + workflow + generated raw API)
    |
    | typed protocol / local IPC
    v
NXGO Agent (.NET, loaded in NX)
    |
    +--> main-thread command executor
    +--> NXOpen Common API
    +--> NXOpen.UF / Open C wrappers
    +--> journal / library execution fallback
    +--> syslog + structured events
    |
    v
Siemens NX / Designcenter
```

Default local transport is a Windows named pipe. The SDK itself must remain free of Siemens binaries and should build with `CGO_ENABLED=0`.

## Current implementation status

The repository now contains executable quality gates and the first implementation slice for the NX integration boundary:

- Pure-Go safety/session primitives;
- `nxctl` test-loop commands;
- invariant enforcement and fuzz/state-machine tests;
- Fake Agent failure contracts;
- fail-closed real-NX smoke runner;
- .NET NXGO.Agent core skeleton with `NxExecutor`, `BuilderScope<T>` and named-pipe request loop;
- contract tests for Agent primitives that do **not** require Siemens NX binaries;
- self-hosted Windows hooks for real NX integration.

The Agent project deliberately separates the NX-independent core from release-specific NXOpen adapters so ordinary CI can test executor/lifetime/transport behavior without redistributing Siemens assemblies.

See [`agent/README.md`](agent/README.md) for the .NET boundary implementation.

## Developer experience target

```go
ctx := context.Background()

nx, err := nxgo.Connect(ctx)
if err != nil {
    return err
}
defer nx.Close()

part, err := nx.Parts.Open(ctx, `D:\cad\bracket.prt`)
if err != nil {
    return err
}

drawing, err := part.Drawings.Generate(ctx, nxgo.DrawingRequest{
    Standard:   nxgo.ESKD,
    Sheet:      nxgo.AutoSheet(),
    Views:      nxgo.AutoViews(),
    Dimensions: nxgo.FromPMI(),
    Validate:   true,
})
if err != nil {
    return err
}

return drawing.ExportPDF(ctx, `D:\release\bracket.pdf`)
```

The implementation behind a call such as `Generate` may use dozens or hundreds of NXOpen calls, builders, undo marks, temporary objects and validation rules. None of that complexity should leak into ordinary application code.

## API layers

NXGO deliberately exposes several levels rather than forcing every use case through one abstraction:

1. **Workflow API** — business operations such as `PrepareReleasePackage`, `GenerateDrawing`, or `ValidateAssembly`.
2. **Idiomatic domain API** — `Parts`, `Modeling`, `Assemblies`, `Drawings`, `PMI`, `CAM`, `Validation`, etc.
3. **Generated raw API** — broad machine-generated coverage derived from the installed NXOpen assemblies/API metadata.
4. **UF/raw escape hatch** — access to capabilities not yet represented by the domain layer.
5. **Journal/library/command fallback** — controlled last-resort compatibility route for automatable operations not exposed cleanly elsewhere.

## Core design rules

- Public Go packages do not reference Siemens DLLs.
- NX objects never cross IPC boundaries as .NET objects; they are represented by scoped handles.
- NXOpen execution is serialized through an NX-safe executor/main-thread boundary.
- High-frequency operations use batch/bulk RPCs to avoid chatty IPC.
- Every operation accepts `context.Context` and supports deadlines/cancellation where NX permits it.
- Mutating workflows use undo/rollback boundaries when possible.
- Protocol and API evolution use explicit capabilities and semantic versioning.
- Logs, NX syslog, bridge events, test events and artifacts share correlation IDs.
- Interactive NX sessions and isolated worker/test sessions are separate first-class modes.
- CI uses pinned NX builds; upgrades are validated through a compatibility matrix before adoption.

## Engineering and testing rules

NXGO treats programming rules and testing rules as one engineering contract. Start here:

- [`docs/RULES_QUICK_REFERENCE.md`](docs/RULES_QUICK_REFERENCE.md) — daily MUST/MUST NOT checklist;
- [`docs/ENGINEERING_STANDARD.md`](docs/ENGINEERING_STANDARD.md) — normative programming/design standard;
- [`docs/TESTING_PLAYBOOK.md`](docs/TESTING_PLAYBOOK.md) — fast tests, fake Agent, warm NX, isolated NX, matrix, fuzz, mutation, metamorphic, differential, chaos, soak and performance testing;
- [`docs/DEFINITION_OF_DONE.md`](docs/DEFINITION_OF_DONE.md) — merge/release completion gate;
- [`docs/invariants/README.md`](docs/invariants/README.md) — stable safety invariants derived from real NX/NXOpen failure modes;
- [`docs/EXECUTABLE_QUALITY_GATES.md`](docs/EXECUTABLE_QUALITY_GATES.md) — what is currently enforced by code/CI.

For NX-dependent behavior, mock-only confidence is insufficient. The target development loop is:

```text
edit
 -> fast/no-NX tests
 -> fake Agent/contracts
 -> affected test in warm pinned NX
 -> semantic CAD assertions + logs/session health
 -> reuse or recycle worker
 -> result
```

Crash/poison/startup/recovery cases use isolated NX processes, while supported-version claims require the NX compatibility matrix.

## Repository documentation

The design package lives in [`docs/`](docs/README.md) and covers architecture, engineering rules, Go SDK/API contracts, Agent/protocol, object lifetime, versioning, observability, testing, security, code generation and the living implementation plan.

## Initial supported environment

The first implementation track targets Windows and modern Siemens NX / Designcenter Continuous Release builds, with a compatibility baseline covering at least the 2512 and 2606 release families. Exact NXOpen runtime dependencies are discovered from the installed NX environment rather than hard-coded into the Go SDK.

Siemens documents NX Open as an extensibility platform using languages such as C/C++, Visual Basic, C#, Java and Python; Go is therefore kept outside the NX process and talks through the supported in-process bridge. Siemens also provides NX Open Reporter tooling for assessing API changes between Continuous Release updates, which informs NXGO's compatibility strategy.

## Non-goals

NXGO does not attempt to replace NX or bypass Siemens licensing, expose unsafe arbitrary remote command execution by default, make NXOpen thread-safe, serialize live Siemens/.NET objects across process boundaries, or promise deterministic parallel mutation of one NX session.

## Status

**Implementation started.** The architecture and testing contract are backed by executable Go gates and the first NX-independent .NET Agent core. The next milestones are validating the NX worker host against a pinned Siemens NX installation and adding the first stable typed Go↔Agent request/response contract.

See [`MASTER_PLAN.md`](MASTER_PLAN.md) for implementation sequence and gates.

## Trademark notice

Siemens, NX, NX Open and Designcenter are trademarks or product names of Siemens and/or its affiliates. NXGO is an independent integration/automation project and is not presented as an official Siemens product.
