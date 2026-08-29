# Test data policy

This directory will contain source-controlled, redistributable fixtures and normalized expected manifests.

Rules:

- do not commit proprietary Siemens sample data unless redistribution rights are explicit;
- prefer minimal synthetic `.prt` fixtures created specifically for NXGO tests;
- source fixtures are treated as immutable and copied to a per-test workspace;
- expected results are semantic manifests, not binary `.prt` byte equality;
- every real-NX fixture records units, required capabilities/licenses, native/managed mode and supported NX builds;
- production/customer CAD data must not be committed.

Recommended case layout:

```text
testdata/nx/<case>/
  source/
  request.json
  expected-semantic.json
  tolerances.json
  environment.json
```
