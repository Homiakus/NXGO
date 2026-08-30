# Pre-flight: 0007-assembly-operations-and-bom

## Task
Implement Phase 7 Assembly Domain API (Add Component, Component Tree Query, BOM Aggregation, Remove Component) in `agent/bundle/AgentWorker.cs`, protocol schemas in `internal/protocol/`, and typed Go client in `pkg/nxgo/` with verification against real Siemens NX 2512.

## Root Cause & Characterization
Complex CAD automation requires constructing and querying assembly hierarchies. Siemens NX manages assemblies through `ComponentAssembly` and `Component` trees. Components reference prototype part files with local positioning coordinates and orientation matrices. These components must be addressable via opaque `ObjectHandleWire` descriptors and allow structural navigation without exposing proprietary Siemens assembly structures.

## Invariants Maintained
- `NXGO-INV-COR-001`: Structural postcondition validation (tree component count, BOM quantities, and correct parent-child relations).
- `NXGO-INV-OBJ-001` / `002`: `Component` objects are registered in `ObjectRegistry` with opaque, epoch-bound `ObjectHandleWire` references.
- `NXGO-INV-MEM-001` / `002`: Unconditional cleanup and safe handle release on component removal or assembly closure.
- `NXGO-INV-EXEC-001` / `002`: All assembly mutations and tree queries run on the bound NX main thread.

## Protected Surfaces
- Pure-Go boundary: Go SDK uses typed structs (`AddComponentParams`, `ComponentNode`, `BOMItem`) with `CGO_ENABLED=0`.
- Siemens assembly isolation: NXOpen `ComponentAssembly` calls remain strictly inside `AgentWorker.cs`.

## Verification Ladder & Edge Space
1. Typed protocol schemas in `internal/protocol/messages.go`.
2. Go client methods in `pkg/nxgo/assembly.go`.
3. Static check: `go vet ./...` and `go run ./cmd/invariantcheck`.
4. Real NX 2512 integration tests in `tests/nx/assembly_test.go`:
   - `TestRealNXAssemblyComponentAddAndTreeQuery`:
     - Creates sub-parts `part_a.prt` (block) and `part_b.prt` (cylinder).
     - Creates assembly `top_assembly.prt`.
     - Adds `part_a` at `[0, 0, 0]` and `part_b` at `[50, 0, 0]`.
     - Queries component tree and BOM summary.
     - Removes `part_b` and verifies tree count updates.
     - Saves and closes cleanly.
