# Execution invariants

## NXGO-INV-EXEC-001 — no arbitrary-thread NXOpen

**MUST NOT:** call NXOpen/NXOpen.UF from transport callbacks, Go-facing RPC worker threads, timers or arbitrary .NET thread-pool work.

**MUST:** convert every NX-affecting request to an internal command and execute it through the NX-safe executor.

**Why:** NXOpen contains APIs that require the NX main thread / valid NX application context; community failures include `Function may only be called from the main thread`.

**Enforcement:**
- Siemens references are private to Agent adapter/executor packages;
- RPC handler packages have no direct access to `Session`, `UFSession` or builders;
- architecture test rejects NXOpen references outside allowed assemblies/namespaces.

**Tests:** issue concurrent RPC requests and prove all actual NX calls execute on the allowed executor context.

## NXGO-INV-EXEC-002 — serialization must handle reentrancy

**MUST NOT:** implement safety as a naive non-reentrant mutex around NXOpen.

**MUST:** track execution context/call depth and define callback reentrancy policy. NX callbacks that arrive during an active operation must either run in explicitly allowed nested context, enqueue safely, or fail with a typed state error.

**Why:** NX calls may trigger NX callbacks. Callback code that tries to acquire the same naive lock can deadlock the session.

**Tests:** synthetic callback during an active command; verify no deadlock and deterministic ordering.

## NXGO-INV-EXEC-003 — Go concurrency is not NX concurrency

**MUST NOT:** map goroutines/parallel client requests to parallel mutation of one NX session.

**MUST:** permit concurrency outside NX (planning, file hashing, DB/network work, multiple isolated NX workers) while preserving a serialized mutation lane per NX session.

**Allowed:** parallelism across independent NX worker processes after licensing/resource policy permits it.

**Review smell:** `Task.Run`, `Parallel.ForEach`, thread pool, or multiple executor lanes around live NX objects without a specific proven-safe ADR.