# Deployment and environment model

## 1. Artifacts

NXGO releases are expected to contain only project-owned artifacts:

- Go module/source;
- `nxctl.exe`;
- NXGO Agent binaries built for supported NX runtime targets;
- protocol/generated code;
- configuration templates;
- documentation.

Do not redistribute Siemens NX binaries.

## 2. Installation modes

### Developer workstation
NX installed normally, NX Programming Tools/API components available, Agent deployed to a user/dev customization location, `nxctl doctor` verifies setup.

### Interactive production workstation
Agent deployed under managed configuration. Restricted operation profile recommended.

### CI worker
Dedicated Windows machine/VM with pinned NX build, valid license access, isolated workspaces and disabled uncontrolled auto-update.

## 3. NX discovery

`nxctl` should discover installations from configured paths plus safe platform-specific installation metadata. Discovery returns exact executable, release/build and API assembly locations.

No API path should be hardcoded in the Go SDK.

## 4. Agent packaging

Because NX releases can differ in supported .NET/runtime details, Agent may ship release-targeted builds/adapters. `nxctl` selects a compatible Agent based on discovery and manifest.

## 5. Configuration

Suggested files:

```text
nxgo.yaml             project settings
%APPDATA%/NXGO/config.yaml  user settings
```

Settings include:

- preferred NX versions;
- workspace/log roots;
- security profile;
- worker timeout/recycle policy;
- Agent path policy;
- artifact retention;
- logging level.

Secrets use external secret/environment mechanisms.

## 6. Environment isolation

Worker launcher constructs a controlled environment and records it in sanitized form. NX-specific variables are set only for the child process when possible rather than globally modifying the machine.

## 7. Multiple NX versions

Side-by-side validated versions are first-class. Every worker run binds to one exact installation. Cache/config keys include NX build identity.

## 8. Update workflow

NX upgrade is deliberate:

1. install new build side-by-side;
2. `nxctl api scan`;
3. generate/diff bindings;
4. build Agent;
5. run compatibility suite;
6. review failures/API changes;
7. mark validated;
8. only then change default.

CI machines never silently redefine `stable` because an NX updater ran.

## 9. Retention

Successful runs retain minimal metrics/manifests. Failed runs retain richer logs/artifacts according to configurable policy and confidentiality rules.