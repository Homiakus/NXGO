# Quality attributes and engineering budgets

These are initial targets to force measurable design decisions. Values may change after prototypes, but changes should be documented.

## 1. Usability

- common open/query/export workflow: <= 10 meaningful Go statements excluding error handling;
- no Builder lifecycle in domain API;
- IDE autocomplete usable without an NX installation for high-level Go SDK;
- errors include operation and remediation context.

## 2. Compatibility

- Go domain API follows semantic versioning;
- protocol major changes are rare and migration documented;
- support at least two NX Continuous Release families during early stabilization;
- exact validated builds published in compatibility matrix.

## 3. Reliability

- no leaked Agent handles in a passing integration suite;
- request timeout cannot permanently block Go caller;
- worker fatal error produces classified failure and artifact bundle;
- dirty worker is recycled before subsequent mutating jobs by default.

## 4. Performance

Initial budgets to benchmark, not promises:

- local no-op protocol round trip p50 target < 5 ms on development workstation;
- bulk API should outperform equivalent N individual RPCs by a material margin;
- protocol overhead SHOULD remain small compared with typical NX operations;
- handle registry lookup effectively O(1);
- log streaming must not materially block NX execution.

## 5. Resource control

- bounded request queue;
- bounded stream buffers;
- configurable max handles/client;
- max request/message sizes;
- test worker memory-growth monitoring over repeated jobs.

## 6. Security

- zero network listeners in default local mode;
- current-user local IPC policy;
- dangerous dynamic execution off/restricted by default;
- explicit workspace restrictions in CI;
- no secrets in standard logs.

## 7. Observability

- 100% of remote operations have request/correlation ID;
- 100% of errors carry stable category;
- every NX-backed test records exact NX build;
- every worker crash is distinguishable from assertion failure.

## 8. Maintainability

- generated vs handwritten code physically separated;
- no NX-version branching scattered through Go domain code;
- release differences isolated in capability/adapters;
- architecture decisions captured in ADRs;
- cyclomatic complexity and package dependency checks introduced before feature expansion.

## 9. Testability

- domain planning/validation logic testable without NX;
- protocol fake Agent supplied for Go tests;
- real NX fixture suite available for critical domains;
- mutation testing used on validators and quality rules.