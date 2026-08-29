# Observability and continuous NX diagnostics

## 1. Goals

A failed automation run must answer:

- which NX build/process/session ran it;
- which NXGO operation was executing;
- what NX reported before failure;
- whether state was rolled back;
- whether the process crashed/hung;
- where resulting artifacts are stored.

## 2. Correlation model

Identifiers:

- `run_id` — one end-to-end automation invocation;
- `session_id` — one NX process Agent session;
- `request_id` — one RPC;
- `transaction_id` — one logical mutation unit;
- `test_id` — optional test identity.

These identifiers MUST appear in structured Agent/Go logs and artifact metadata.

## 3. Sources

NXGO collects/normalizes:

- Go SDK logs;
- supervisor/process logs;
- Agent structured logs;
- NX system log/syslog where accessible;
- stdout/stderr of worker launch paths;
- NX exceptions/native codes;
- journal diagnostics when explicitly enabled;
- test framework events;
- crash dumps/artifacts where policy permits.

Journal recording is a development/reproduction tool, not the primary runtime log.

## 4. Structured event format

Recommended JSONL fields:

```json
{
  "time":"...",
  "level":"info",
  "run_id":"...",
  "session_id":"...",
  "request_id":"...",
  "component":"agent",
  "operation":"drawing.generate",
  "phase":"create_base_view",
  "duration_ms":382,
  "status":"ok"
}
```

Secrets, credentials and user-sensitive content MUST be redacted by policy.

## 5. NX syslog following

Supervisor discovers the active NX log path from controlled environment/configuration and/or Agent-reported session log metadata. It follows appended content continuously and tags emitted lines with session/run context.

Requirements:

- tolerate log rotation/recreation;
- preserve original timestamp/text where available;
- detect file truncation;
- bounded buffering;
- expose a `nxctl logs --follow` stream;
- save complete per-run copies for CI failures.

## 6. Agent markers

Where supported, Agent SHOULD write compact correlation markers to NX system log:

```text
[NXGO run=<id> req=<id> op=drawing.generate] START
[NXGO run=<id> req=<id> op=drawing.generate] END status=ok
```

This makes native NX messages between markers attributable to a specific operation.

## 7. Metrics

Track at minimum:

- request count by operation/result;
- queue depth/wait;
- NX execution duration;
- total request duration;
- worker startup duration;
- worker crash count;
- timeout count;
- rollback failures;
- handle registry size/leaks;
- bytes/messages per RPC;
- log drops;
- export/generation durations.

## 8. Failure artifact bundle

Each failed integration test SHOULD produce:

```text
run.json
sdk.jsonl
agent.jsonl
nx-syslog.txt
process.txt
error.json
input-manifest.json
output-manifest.json
screenshots-or-pdf/   (when applicable)
crash/                (when available)
```

Do not copy proprietary/customer CAD files into CI artifacts unless fixture policy explicitly allows it.

## 9. Developer experience

`nxctl logs --follow --session <id>` presents merged live output. `nxctl diagnose --run <id>` summarizes the failure and points to raw artifacts.