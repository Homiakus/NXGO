# Pre-flight: 0010-api-scanner-and-nxctl-commands

## Task
Implement Phase 6 API Scanner and `nxctl api` command suite (`scan`, `find`, `inspect`, `diff`) in `internal/apiscanner/` and `cmd/nxctl/` to extract, query, and diff NXOpen assembly metadata without checking in proprietary Siemens binaries.

## Root Cause & Characterization
Siemens NX evolves across releases (e.g. 2512, 2606), introducing new methods, deprecating old builders, and altering signatures. To ensure forward compatibility and detect breaking changes without storing proprietary Siemens DLLs in the git repository, NXGO requires a deterministic scanner that inspects the local machine's NX installation and outputs normalized, machine-readable JSON API manifests and diff reports.

## Invariants Maintained
- `NXGO-INV-SRC-001`: No Siemens proprietary binary files (.dll, .exe) are committed to git; metadata is extracted on-demand into plain JSON.
- `NXGO-INV-COR-001`: Deterministic output sorting (namespaces, types, methods, and parameters are lexicographically sorted for stable git diffs).
- `NXGO-INV-TEST-001`: Strict discovery and doctor diagnostics for NX managed assemblies.

## Verification Ladder & Edge Space
1. Implementation of `internal/apiscanner/` (Scanner, Manifest schema, Search, Diff).
2. Integration into `cmd/nxctl/main.go` (`api scan`, `api find`, `api inspect`, `api diff`).
3. Unit tests in `internal/apiscanner/scanner_test.go`.
4. Real NX 2512 scan test (`go run ./cmd/nxctl api scan` against local NX 2512 managed directory).
