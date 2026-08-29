# ADR-0003: Separate workflow, domain and generated raw APIs

- Status: Accepted
- Date: 2026-08-29

## Context

A 1:1 NXOpen wrapper preserves coverage but also preserves NXOpen complexity and produces chatty IPC. A purely high-level facade is pleasant but inevitably lags new/rare NX functions.

## Decision

Provide three primary layers plus controlled fallbacks:

1. workflow API for coarse automation;
2. handwritten idiomatic Go domain API;
3. generated typed raw NXOpen API;
4. UF/dynamic raw escape hatches;
5. optional journal/library fallback.

Generated code and handwritten code are physically separated and have different compatibility guarantees.

## Consequences

The project can optimize common workflows without trapping advanced users. New NX versions can gain broad raw coverage quickly via scanning/codegen while high-level APIs evolve deliberately.