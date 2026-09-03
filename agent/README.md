# NXGO Agent implementation

This directory contains the .NET side of NXGO.

## Projects

- `NXGO.Agent.Core` — NX-independent `netstandard2.0` safety/runtime primitives. It contains no Siemens references and is tested on ordinary CI.
- `NXGO.Agent.Core.Tests` — contract tests for executor, builder lifetime, session health and named-pipe framing/transport.
- `NXGO.Agent.NXHost` — .NET 8 dedicated-worker executable launched by Siemens' `managed_core/run_dotnet_core_nxopen.exe`; its NXOpen references come from `NXGO_NX_MANAGED`.

## Critical execution rule

`NxExecutor` does **not** invent a background thread and call NXOpen from it. Transport threads only enqueue work. The NX host binds the executor on a supported NX execution thread and drains the queue there.

The current NX host is explicitly a **dedicated worker** implementation. Its entry point remains on the NX execution thread and pumps queued work. It is not an interactive-attach solution; interactive NX requires a separate Siemens-supported main-thread pump/callback mechanism before it can be enabled.

## Build without NX

```text
dotnet test agent/NXGO.Agent.Core.Tests/NXGO.Agent.Core.Tests.csproj -c Release
```

## Build NX host

On an authorized Windows/NX developer machine:

```powershell
$env:NXGO_NX_MANAGED = 'C:\path\to\NX\NXBIN\managed_core'
./scripts/build-agent.ps1
```

No Siemens binary is committed to this repository.

## Bootstrap wire protocol

The managed_core runner launches `NXGO.Agent.NXHost.dll` directly. `Program.Main` calls `EntryPoint.Run` without reflection; the shared 4-byte little-endian frame and temporary bootstrap payload (`ping`, `nx.ping`, `shutdown`) remain intentionally **not** the stable public protocol.
