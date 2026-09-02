#!/usr/bin/env python3
from __future__ import annotations

import pathlib
import re


def replace_once(path: pathlib.Path, old: str, new: str, label: str) -> None:
    text = path.read_text(encoding="utf-8")
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{label}: expected one source snippet in {path}, got {count}")
    path.write_text(text.replace(old, new, 1), encoding="utf-8")


messages = pathlib.Path("internal/protocol/messages.go")
replace_once(
    messages,
    '''type ObjectHandleWire struct {\n\tSessionID    string `json:"session_id"`\n\tEpoch        uint64 `json:"epoch"`\n\tObjectID     string `json:"object_id"`\n\tKind         string `json:"kind"`\n''',
    '''type ObjectHandleWire struct {\n\tSessionID    string `json:"session_id"`\n\tEpoch        uint64 `json:"epoch"`\n\tObjectID     string `json:"object_id"`\n\tGeneration   uint32 `json:"generation"`\n\tKind         string `json:"kind"`\n''',
    "add wire generation",
)

obj = pathlib.Path("pkg/nxgo/objectref.go")
replace_once(
    obj,
    '''\tif ref.SessionID == "" || ref.SessionID != s.sessionID || ref.Epoch != s.epoch {\n\t\treturn fmt.Errorf(\n\t\t\t"%w: handle(session=%q epoch=%d object=%q) current(session=%q epoch=%d)",\n\t\t\tErrStaleObjectRef,\n\t\t\tref.SessionID,\n\t\t\tref.Epoch,\n\t\t\tref.ObjectID,\n\t\t\ts.sessionID,\n\t\t\ts.epoch,\n\t\t)\n\t}\n\n\tif len(expectedKinds) == 0 {\n''',
    '''\tif ref.SessionID == "" || ref.SessionID != s.sessionID || ref.Epoch != s.epoch {\n\t\treturn fmt.Errorf(\n\t\t\t"%w: handle(session=%q epoch=%d object=%q) current(session=%q epoch=%d)",\n\t\t\tErrStaleObjectRef,\n\t\t\tref.SessionID,\n\t\t\tref.Epoch,\n\t\t\tref.ObjectID,\n\t\t\ts.sessionID,\n\t\t\ts.epoch,\n\t\t)\n\t}\n\tif ref.Generation == 0 {\n\t\treturn fmt.Errorf("%w: object %q has missing/zero generation", ErrStaleObjectRef, ref.ObjectID)\n\t}\n\n\tif len(expectedKinds) == 0 {\n''',
    "enforce SDK generation",
)

# Existing SDK unit tests construct otherwise-valid handles manually. Give
# them a real generation so each test continues to target the intended guard.
test = pathlib.Path("pkg/nxgo/objectref_test.go")
t = test.read_text(encoding="utf-8")
t2 = re.sub(r'(\n\s*ObjectID:\s+"[^"]+",\n)(\s*Kind:\s+"[^"]+",)', r'\1\t\t\tGeneration: 1,\n\2', t)
# Some literals are indented two tabs rather than three.
t2 = re.sub(r'(\n\s*ObjectID:\s+"[^"]+",\n)(\s*Kind:\s+"[^"]+",)', lambda m: m.group(1) + re.match(r'\s*', m.group(2)).group(0) + 'Generation: 1,\n' + m.group(2), t2)
# Normalize accidental duplicate insertion if the first substitution matched.
t2 = t2.replace('Generation: 1,\n\t\t\tGeneration: 1,\n', 'Generation: 1,\n')
if t2 == t:
    raise SystemExit("objectref tests: no handles patched")
test.write_text(t2, encoding="utf-8")

agent = pathlib.Path("agent/bundle/AgentWorker.cs")
a = agent.read_text(encoding="utf-8")n
