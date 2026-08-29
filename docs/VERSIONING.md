# Versioning and capability negotiation

## 1. Independent versions

Track independently:

- NXGO Go SDK version;
- wire protocol version;
- Agent version;
- NX release/build;
- generated API manifest version;
- domain capability schema version.

Do not encode all compatibility into one version number.

## 2. NX release support

Support is build-specific, not merely family-specific. CI metadata should record exact release/build, for example `2512.x` or `2606.x` maintenance level.

A release/build is one of:

- `experimental`;
- `validated`;
- `deprecated`;
- `unsupported`.

## 3. Handshake capabilities

Example capabilities:

```text
parts.open
parts.save
modeling.extrude.v1
drawing.generate.v1
pmi.retrieve.v1
validation.checkmate
raw.reflection.v1
journal.execute
logs.syslog.follow
```

Capabilities may have parameters/limits, not just booleans.

## 4. Domain API behavior

A domain method MUST either:

- execute with the documented semantics;
- choose a tested compatibility implementation;
- return `ErrUnsupported` before mutating state.

Silent semantic degradation is prohibited.

## 5. Generated manifest

For each supported NX installation, scanner records:

- assembly identities and versions;
- public types;
- methods/properties/events;
- enums;
- signatures;
- inheritance;
- obsolete markers where visible;
- optional documentation IDs;
- selected UF metadata.

Manifest is normalized and deterministic so Git diffs are meaningful.

## 6. API diff

Upgrade report categories:

- added;
- removed;
- signature changed;
- type moved/renamed;
- enum changed;
- obsolete/deprecated;
- behavior unknown-requires-test.

The scanner diff is evidence, not proof of runtime compatibility.

## 7. Compatibility gates

A new NX build is `validated` only after:

1. Agent builds against it;
2. generated API compiles;
3. protocol contract tests pass;
4. smoke worker starts and handshakes;
5. domain integration suite passes;
6. drawing/export golden tests pass within approved tolerances;
7. crash/timeout/logging tests pass;
8. known API diff is reviewed.

## 8. CI matrix

Minimum intended matrix:

```text
Go unit/contract tests: every commit
NX stable baseline:     every relevant PR
NX current target:      every relevant PR
extended release set:   nightly/weekly
```

NX installations used by CI MUST be pinned and must not auto-update outside controlled upgrade work.