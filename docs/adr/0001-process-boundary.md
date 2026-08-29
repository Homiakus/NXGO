# ADR-0001: Keep Go outside the NX runtime

- Status: Accepted
- Date: 2026-08-29

## Context

NX Open publicly targets supported languages/runtimes such as C/C++, .NET/C#, Java and Python. Go is not the natural in-process NXOpen runtime. Direct cgo/Open C binding would tightly couple the Go build to Siemens native headers, DLL loading, ABI/version differences and crash behavior.

## Decision

The public NXGO SDK remains Pure Go and communicates with a small supported-language Agent loaded into NX. Initial Agent language is C#/.NET because it provides strong typing, reflection useful for API scanning, mature IPC support and broad NXOpen/UF access.

## Consequences

Positive:

- ordinary `go get` experience;
- Siemens DLLs stay outside Go process;
- NX crash isolation;
- version adapters can evolve independently;
- remote/worker execution becomes possible later.

Negative:

- IPC latency;
- two-language codebase;
- Agent deployment complexity;
- object handle registry required.

Mitigation: coarse domain calls, batch RPC, generated contracts and strict Agent scope.