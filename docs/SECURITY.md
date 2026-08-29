# Security model

## 1. Threat model

The Agent can mutate valuable CAD data and potentially invoke broad NX APIs. Treat it as a privileged local automation endpoint.

Threats include:

- untrusted local process connecting to Agent;
- exposed TCP endpoint;
- arbitrary reflection invocation;
- malicious file paths;
- unsafe journal/library loading;
- denial of service through huge requests/handle leaks;
- secrets/customer data leaking into logs/artifacts;
- remote worker credential theft.

## 2. Default posture

Secure by default:

- local named pipe only;
- current-user ACL;
- random per-session endpoint/token material;
- no unauthenticated TCP;
- dynamic execution restricted;
- external assembly/journal loading disabled unless enabled;
- request and handle limits;
- filesystem policy boundaries;
- fail closed on protocol/auth mismatch.

.NET provides named-pipe IPC and Windows access-control primitives; NXGO should use explicit pipe security rather than relying on obscurity.

## 3. Authorization profiles

Suggested profiles:

### `interactive-standard`
Domain API, approved raw calls, user-owned filesystem.

### `worker-ci`
Known workspace only, no arbitrary journals, no UI automation, deterministic exports.

### `developer`
Enables reflection inspection, journals and richer diagnostics; explicit warning.

### `hardened`
Only allowlisted domain operations; raw/dynamic execution disabled.

## 4. Filesystem safety

Requests carrying paths are canonicalized and checked against policy. Worker mode SHOULD use a dedicated workspace root. Avoid following unsafe symlink/reparse-point escapes where relevant.

Output overwrite policy MUST be explicit.

## 5. Dynamic invocation

Reflection/raw dynamic call layer MUST:

- restrict assembly/type namespace;
- reject arbitrary assembly load/path execution;
- enforce argument size/depth limits;
- maintain method denylist for dangerous unsupported surfaces;
- produce audit events.

## 6. Journals and external libraries

Treat as code execution. Require explicit enablement and path allowlisting/signing policy if used in controlled production.

## 7. Remote mode

If later supported:

- mutually authenticated TLS or equivalent strong identity;
- explicit service authorization;
- no direct exposure of the in-process Agent to the network; remote gateway belongs in a separate process;
- rate limits;
- audit logs;
- short-lived credentials;
- network segmentation.

## 8. Secrets

NXGO configuration MUST support secret references rather than embedding secrets in source. Logs redact tokens/passwords/credentials. CAD file content and metadata are potentially confidential.

## 9. Supply chain

- pin Go/.NET dependencies;
- generate SBOM for releases;
- sign release artifacts where feasible;
- do not redistribute proprietary Siemens binaries;
- verify generated API manifests originate from approved NX installations.

## 10. Security tests

Required tests include:

- unauthorized pipe connection denied;
- cross-session token rejected;
- stale/replayed session metadata rejected;
- path traversal blocked;
- oversized request rejected;
- handle quota enforced;
- dynamic invocation policy enforced;
- logs redact configured sensitive fields.