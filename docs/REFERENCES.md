# Research references

This file records external sources that informed the initial architecture. It is not a substitute for the Siemens documentation installed/licensed with the exact target NX release.

## Siemens

- Siemens NX Mach Series brochure — describes NX Open as an API/customization platform using C/C++, Visual Basic, C#, Java and Python and describes NX Open toolkit offerings: https://blogs.sw.siemens.com/wp-content/uploads/sites/2/2020/11/NX-Add-on-Module-Brochure.pdf
- Siemens Designcenter blog, *Improve productivity and usability with NX Core Architecture upgrades in latest release* — documents Continuous Release and NX Open Reporter tools for .NET, C++, Python and Java upgrade impact analysis: https://blogs.sw.siemens.com/designcenter/improve-productivity-and-usability-with-nx-core-architecture-upgrades-in-latest-release/
- Siemens openness/NX Open discussion: https://blogs.sw.siemens.com/podcasts/next-generation-design/siemens-openness-strategy-sdks/
- Siemens Simcenter example discussing journal recording and NX Open/Python automation: https://blogs.sw.siemens.com/simcenter/the-rocket-that-refused-to-behave-the-ultimate-guide-to-spider-element-automation-for-rocket-and-aerospace-structures/

## Microsoft / IPC

- `NamedPipeServerStream` API: https://learn.microsoft.com/en-us/dotnet/api/system.io.pipes.namedpipeserverstream
- Named pipes guidance: https://learn.microsoft.com/en-us/dotnet/standard/io/how-to-use-named-pipes-for-network-interprocess-communication
- `System.IO.Pipes` / access-control APIs: https://learn.microsoft.com/en-us/dotnet/api/system.io.pipes

## Protocol Buffers / gRPC

- Protocol Buffers documentation: https://protobuf.dev/
- gRPC documentation: https://grpc.io/docs/

## Research principles

1. Exact installed NX help/API metadata outranks internet examples for signatures and supported behavior.
2. Community examples are useful for patterns but do not define compatibility guarantees.
3. Every new NX release/build must be verified by scanner diff plus real NX tests.
4. Licensing requirements and redistribution restrictions must be confirmed against the organization's Siemens agreements before packaging production integrations.