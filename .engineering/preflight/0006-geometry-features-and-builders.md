# Pre-flight: 0006-geometry-features-and-builders

## Task
Implement Phase 7 Geometry & Modeling API (Block feature, Cylinder feature, Mass properties, Bounding box, Body query) with `BuilderScope<T>` recipes in `agent/bundle/AgentWorker.cs`, protocol schemas in `internal/protocol/`, and typed Go client in `pkg/nxgo/` verified against real Siemens NX 2512.

## Root Cause & Characterization
CAD automation requires procedural feature generation (primitives, sweeps, booleans) with mathematical and semantic correctness verification. In Siemens NX, creating features requires NXOpen Builder instances (e.g. `BlockFeatureBuilder`, `CylinderBuilder`). These builders allocate native C++ resources that must be destroyed via `b.Destroy()` immediately upon commit or failure to avoid native memory leaks. Furthermore, CAD operations require semantic postcondition checks (e.g. volume, bounding box, body counts) to fulfill invariant `NXGO-INV-COR-001`.

## Invariants Maintained
- `NXGO-INV-COR-001`: Semantic CAD postcondition validation (verifying computed volume, bounding box dimensions, body/feature count against expected analytical geometry).
- `NXGO-INV-MUT-001` / `002`: `BuilderScope<T>` wraps every NX builder, enforces single-attempt `CommitOnce`, and unconditionally executes `Destroy()` on success and failure paths.
- `NXGO-INV-OBJ-001` / `002`: Created features and solid bodies are registered in `ObjectRegistry` with opaque, epoch-bound `ObjectHandleWire` references.
- `NXGO-INV-MEM-001` / `002`: Zero native builder leaks, automatic handle invalidation on part close.
- `NXGO-INV-EXEC-001` / `002`: All builder creation, updates, and measurements run on the bound NX main execution thread.

## Protected Surfaces
- Pure-Go boundary: Go SDK uses typed structs (`BlockParams`, `MassProperties`, `BoundingBox`) with `CGO_ENABLED=0`.
- Siemens assembly isolation: NXOpen builders and UF native calls remain strictly inside `AgentWorker.cs`.

## Verification Ladder & Edge Space
1. Typed protocol schemas in `internal/protocol/messages.go`.
2. Go client methods in `pkg/nxgo/geometry.go`.
3. Static check: `go vet ./...` and `go run ./cmd/invariantcheck`.
4. Unit tests in `pkg/nxgo/geometry_test.go`.
5. Real NX 2512 integration tests in `tests/nx/geometry_test.go`:
   - `TestRealNXGeometryCreationAndMassProperties`:
     - Creates 100x50x25 mm block -> verifies volume == 125,000 mm³ and bounding box == [100, 50, 25].
     - Creates d=20 mm, h=30 mm cylinder -> verifies cylinder volume == π*10²*30 ≈ 9424.78 mm³.
   - `TestRealNXTransactionRollbackOnFeatureCreation`:
     - Creates feature inside an undo mark -> rolls back -> verifies body count returns to 0.
