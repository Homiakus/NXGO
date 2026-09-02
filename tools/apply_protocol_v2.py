#!/usr/bin/env python3
from __future__ import annotations

import pathlib


def replace_once(path: pathlib.Path, old: str, new: str, label: str) -> None:
    text = path.read_text(encoding="utf-8")
    if text.count(old) != 1:
        raise SystemExit(f"{label}: source snippet count={text.count(old)} in {path}")
    path.write_text(text.replace(old, new, 1), encoding="utf-8")

messages = pathlib.Path("internal/protocol/messages.go")
replace_once(
    messages,
    '''const (\n\tCurrentProtocolMajor = 1\n\tCurrentProtocolMinor = 0\n)''',
    '''const (\n\t// v2 makes ObjectHandleWire.Generation mandatory. This is intentionally a\n\t// major-version boundary: v1 peers cannot safely reason about handle reuse.\n\tCurrentProtocolMajor = 2\n\tCurrentProtocolMinor = 0\n)''',
    "bump protocol major",
)

agent = pathlib.Path("agent/bundle/AgentWorker.cs")
a = agent.read_text(encoding="utf-8")
old = '''{\\\"protocol_version\\\":{\\\"major\\\":1,\\\"minor\\\":0},\\\"agent_version\\\":\\\"v0.1.0-realnx\\\"'''
new = '''{\\\"protocol_version\\\":{\\\"major\\\":2,\\\"minor\\\":0},\\\"agent_version\\\":\\\"v0.2.0-realnx\\\"'''
if a.count(old) != 1:
    raise SystemExit(f"production handshake source count={a.count(old)}")
agent.write_text(a.replace(old, new, 1), encoding="utf-8")

contract_test = pathlib.Path("internal/protocol/version2_contract_test.go")
contract_test.write_text('''package protocol\n\nimport (\n    "encoding/json"\n    "testing"\n)\n\nfunc TestProtocolV2RequiresGenerationAwareObjectHandles(t *testing.T) {\n    if CurrentProtocolMajor != 2 {\n        t.Fatalf("generation-aware handles require protocol major 2, got %d", CurrentProtocolMajor)\n    }\n\n    original := ObjectHandleWire{\n        SessionID: "sess-v2",\n        Epoch: 3,\n        ObjectID: "obj-7",\n        Generation: 11,\n        Kind: "Body",\n    }\n    data, err := json.Marshal(original)\n    if err != nil { t.Fatal(err) }\n    var decoded ObjectHandleWire\n    if err := json.Unmarshal(data, &decoded); err != nil { t.Fatal(err) }\n    if decoded.Generation != original.Generation {\n        t.Fatalf("generation lost across wire roundtrip: got %d want %d", decoded.Generation, original.Generation)\n    }\n}\n\nfunc TestProtocolV1IsRejectedByCurrentV2Negotiation(t *testing.T) {\n    current := Version{Major: CurrentProtocolMajor, Minor: CurrentProtocolMinor}\n    if err := NegotiateVersion(Version{Major: 1, Minor: 0}, current); err == nil {\n        t.Fatal("expected v1 client to be rejected by v2 server contract")\n    }\n}\n''', encoding="utf-8")

print("protocol v2 generation boundary patched")
