# NXGO protocol

## 1. Purpose

The protocol is an internal stable boundary between Pure-Go clients/supervisors and the in-process NX Agent.

Initial recommendation: Protocol Buffers for schemas and generated Go/C# DTOs. Transport is pluggable; Windows named pipes are the default local carrier.

## 2. Handshake

Client sends:

- protocol major/minor;
- SDK version;
- requested mode;
- requested features;
- client PID/user identity hints;
- random connection nonce.

Agent returns:

- protocol major/minor;
- Agent version;
- NX release/build;
- NX process ID;
- session ID;
- capability set;
- server limits;
- security policy summary.

Major mismatch fails closed. Minor mismatch is accepted only according to compatibility rules.

## 3. Request envelope

Every request contains:

- request ID;
- correlation/run ID;
- optional test ID;
- operation name/ID;
- deadline or remaining timeout;
- optional transaction ID;
- payload;
- trace metadata.

Every response contains:

- request ID;
- status;
- typed result or error;
- warnings;
- timing data;
- produced object handles;
- artifact references where applicable.

## 4. Error envelope

Fields:

- stable NXGO category;
- NX native error code when available;
- safe message;
- operation;
- recoverability;
- session health (`healthy`, `dirty`, `lost`);
- diagnostic correlation IDs.

Stack traces are diagnostic fields and MUST NOT be required for program logic.

## 5. Object references

Wire representation:

```text
session_id
object_id
kind
optional_native_tag
lease_scope_id
```

The client MUST treat IDs as opaque. Native tags are diagnostic/optimization metadata, not identity contracts.

## 6. Batching

Protocol supports:

- domain-specific bulk requests;
- generic batch request with explicit ordering;
- optional stop-on-first-error semantics;
- one transaction envelope around a batch.

Batch size limits are negotiated in handshake.

## 7. Streaming

Server streams include:

- logs;
- events;
- operation progress;
- optional large result chunks.

Each stream defines bounded buffering and overflow behavior. Diagnostic logs MAY drop low-severity entries under pressure but MUST emit a loss marker. State-change events MUST NOT silently disappear.

## 8. Cancellation

Go context cancellation sends a cancel request. Cancellation is cooperative:

- queued request: MUST be removed where possible;
- cancellable NX operation: SHOULD request cancellation;
- non-cancellable NX operation: response is suppressed/marked cancelled while supervisor timeout policy remains active.

Cancellation does not imply rollback unless the operation is inside an NXGO transaction with rollback policy.

## 9. Security

Default local transport:

- named pipe scoped to current user/session;
- unguessable instance suffix/nonce;
- ACL restricted to approved SID(s);
- no TCP listener;
- handshake validates expected Agent/session metadata.

Remote transport is a separate feature and MUST require TLS plus explicit authentication/authorization.

## 10. Protocol evolution

- never reuse field numbers;
- additive optional fields are preferred;
- enums reserve `UNSPECIFIED=0`;
- unknown fields tolerated;
- removal occurs only across major versions;
- capability flags gate semantic differences not representable by syntax alone.

## 11. Observability

Agent records per-call:

- queue wait;
- NX execution time;
- serialization time;
- total latency;
- result category;
- cancellation/timeout status.

This enables finding IPC chatter and slow NX operations separately.