# Pre-flight: 0003-supervisor-discovery-and-doctor

## Task
Implement NX installation discovery, version selection, ownership manifest, and supervisor diagnostic status (`internal/supervisor`), and wire them into `cmd/nxctl` (`installations`, `doctor`, `status`).

## Root Cause & Characterization
Running real-NX workflows or diagnosing local environments currently requires manually setting environment variables without validation. NXGO needs:
1. `Discovery`: Finding installed Siemens NX installations on Windows (via `UGII_BASE_DIR`, `NXGO_NX_HOME`, standard Program Files paths `C:\Program Files\Siemens\NX*`, registry / environment hints).
2. `Validation`: Validating NXOpen assemblies (`NXOpen.dll`, `NXOpen.UF.dll`, `run_journal.exe`), detecting NX release/version from folder naming or version manifests.
3. `Ownership & Manifest`: Generating and tracking supervisor worker process manifests (PID, NX home, pipe endpoint, owner, started time).
4. `Doctor & Status`: Validating developer environment health, Agent availability, temp directory permissions, protocol version compatibility, and outputting both human and `--json` formats.

## Invariants Maintained
- `NXGO-INV-TEST-001`: Environment discovery explicitly records detected paths, builds, and tools.
- `NXGO-INV-VER-001` & `NXGO-INV-VER-003`: Validates that installed NX versions and assemblies meet supported baselines without guessing.
- `NXGO-INV-OBS-001` & `NXGO-INV-OBS-002`: Manifest tracks diagnostic artifacts and log locations.

## Protected Surfaces
- Pure-Go boundary: Discovery and supervisor logic must execute cleanly without cgo on any platform (with mockable / filesystem inspection hooks).
- `cmd/nxctl`: Maintain clean subcommands, returning structured error exit codes (10: NX not found, 11: Agent unavailable, etc.).

## Verification Ladder & Edge Space
1. Static analysis: `go vet ./...` and `go run ./cmd/invariantcheck`.
2. Unit tests (`internal/supervisor/discovery_test.go`, `internal/supervisor/manifest_test.go`):
   - Finding mock NX directories with valid/invalid assemblies.
   - Version parsing and priority resolution (CLI flag > Env > standard paths).
   - Ownership manifest serialization and validation.
   - Doctor checks on pristine vs missing installation environments.
3. Integration with `nxctl`:
   - `nxctl installations --json`
   - `nxctl doctor --json`
   - `nxctl status --json`
