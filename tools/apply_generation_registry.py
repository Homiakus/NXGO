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

# Existing SDK tests use hand-built valid handles. Give those handles a real
# generation so the tests continue to exercise their intended guard.
test = pathlib.Path("pkg/nxgo/objectref_test.go")
t = test.read_text(encoding="utf-8")
t2, count = re.subn(
    r'(?m)^(\s*)Kind:\s+"(Part|Body)",$',
    lambda m: f'{m.group(1)}Generation: 1,\n{m.group(1)}Kind:      "{m.group(2)}",',
    t,
)
if count < 1:
    raise SystemExit("objectref tests: no handles patched")
test.write_text(t2, encoding="utf-8")

agent = pathlib.Path("agent/bundle/AgentWorker.cs")
a = agent.read_text(encoding="utf-8")

old_registry = '''public sealed class RegisteredObject\n{\n    public TaggedObject Target { get; set; }\n    public string Kind { get; set; }\n    public string LeaseScopeId { get; set; }\n    public uint NativeTag { get; set; }\n    public DateTime RegisteredAt { get; set; }\n}\n\npublic sealed class ObjectRegistry\n{\n    private readonly string _sessionId;\n    private ulong _epoch;\n    private long _handleCounter;\n    private readonly Dictionary<string, RegisteredObject> _objects = new Dictionary<string, RegisteredObject>();\n    private readonly object _lock = new object();\n\n    public ObjectRegistry(string sessionId, ulong epoch)\n    {\n        _sessionId = sessionId;\n        _epoch = epoch;\n    }\n\n    public ulong Epoch { get { return _epoch; } }\n    public string SessionId { get { return _sessionId; } }\n\n    public string Register(TaggedObject obj, string kind, string leaseScopeId, out uint nativeTag)\n    {\n        if (obj == null) throw new ArgumentNullException("obj");\n        lock (_lock)\n        {\n            var id = "obj-" + Interlocked.Increment(ref _handleCounter);\n            nativeTag = 0;\n            try\n            {\n                nativeTag = (uint)obj.Tag;\n            }\n            catch {}\n\n            _objects[id] = new RegisteredObject\n            {\n                Target = obj,\n                Kind = kind ?? "TaggedObject",\n                LeaseScopeId = leaseScopeId ?? "",\n                NativeTag = nativeTag,\n                RegisteredAt = DateTime.UtcNow\n            };\n            return id;\n        }\n    }\n\n    public string FormatHandleJson(string objectId, string kind, uint nativeTag, string leaseScopeId)\n    {\n        return string.Format(\n            "{{\\\"session_id\\\":\\\"{0}\\\",\\\"epoch\\\":{1},\\\"object_id\\\":\\\"{2}\\\",\\\"kind\\\":\\\"{3}\\\",\\\"native_tag\\\":{4},\\\"lease_scope_id\\\":\\\"{5}\\\"}}",\n            _sessionId, _epoch, objectId, kind, nativeTag, leaseScopeId ?? ""\n        );\n    }\n\n    public T Resolve<T>(string objectId, ulong epoch, string sessionId) where T : TaggedObject\n    {\n        if (sessionId != _sessionId)\n        {\n            throw new InvalidOperationException(string.Format("stale session reference: got {0}, expected {1}", sessionId, _sessionId));\n        }\n        if (epoch != _epoch)\n        {\n            throw new InvalidOperationException(string.Format("stale epoch reference: got {0}, expected {1}", epoch, _epoch));\n        }\n\n        lock (_lock)\n        {\n            RegisteredObject reg;\n            if (!_objects.TryGetValue(objectId, out reg))\n            {\n                throw new KeyNotFoundException("object handle not found or expired: " + objectId);\n            }\n            if (reg.Target == null)\n            {\n                throw new InvalidOperationException("object target is null for handle: " + objectId);\n            }\n            if (!(reg.Target is T))\n            {\n                throw new InvalidOperationException(string.Format("object kind/type mismatch for handle {0}: registered={1}, requested={2}", objectId, reg.Kind, typeof(T).Name));\n            }\n            return (T)reg.Target;\n        }\n    }\n'''
new_registry = '''public sealed class RegisteredObject\n{\n    public TaggedObject Target { get; set; }\n    public string Kind { get; set; }\n    public string LeaseScopeId { get; set; }\n    public uint NativeTag { get; set; }\n    public uint Generation { get; set; }\n    public DateTime RegisteredAt { get; set; }\n}\n\npublic sealed class ObjectRegistry\n{\n    private readonly string _sessionId;\n    private readonly int _capacity;\n    private ulong _epoch;\n    private long _handleCounter;\n    private long _generationCounter;\n    private int _highWatermark;\n    private readonly Dictionary<string, RegisteredObject> _objects = new Dictionary<string, RegisteredObject>();\n    private readonly object _lock = new object();\n\n    public ObjectRegistry(string sessionId, ulong epoch, int capacity)\n    {\n        if (capacity <= 0) throw new ArgumentOutOfRangeException("capacity");\n        _sessionId = sessionId;\n        _epoch = epoch;\n        _capacity = capacity;\n    }\n\n    public ulong Epoch { get { return _epoch; } }\n    public string SessionId { get { return _sessionId; } }\n    public int Capacity { get { return _capacity; } }\n    public int Count { get { lock (_lock) return _objects.Count; } }\n    public int HighWatermark { get { lock (_lock) return _highWatermark; } }\n\n    public string Register(TaggedObject obj, string kind, string leaseScopeId, out uint nativeTag)\n    {\n        if (obj == null) throw new ArgumentNullException("obj");\n        lock (_lock)\n        {\n            if (_objects.Count >= _capacity)\n            {\n                throw new InvalidOperationException("object registry capacity reached; release handles or recycle worker");\n            }\n            var id = "obj-" + Interlocked.Increment(ref _handleCounter);\n            var generationValue = Interlocked.Increment(ref _generationCounter);\n            if (generationValue <= 0 || generationValue > uint.MaxValue)\n            {\n                throw new InvalidOperationException("object generation space exhausted; recycle worker");\n            }\n            uint generation = (uint)generationValue;\n            nativeTag = 0;\n            try\n            {\n                nativeTag = (uint)obj.Tag;\n            }\n            catch {}\n\n            _objects[id] = new RegisteredObject\n            {\n                Target = obj,\n                Kind = kind ?? "TaggedObject",\n                LeaseScopeId = leaseScopeId ?? "",\n                NativeTag = nativeTag,\n                Generation = generation,\n                RegisteredAt = DateTime.UtcNow\n            };\n            if (_objects.Count > _highWatermark) _highWatermark = _objects.Count;\n            return id;\n        }\n    }\n\n    public string FormatHandleJson(string objectId, string kind, uint nativeTag, string leaseScopeId)\n    {\n        lock (_lock)\n        {\n            RegisteredObject reg;\n            if (!_objects.TryGetValue(objectId, out reg))\n            {\n                throw new KeyNotFoundException("cannot format released/unknown object handle: " + objectId);\n            }\n            return string.Format(\n                "{{\\\"session_id\\\":\\\"{0}\\\",\\\"epoch\\\":{1},\\\"object_id\\\":\\\"{2}\\\",\\\"generation\\\":{3},\\\"kind\\\":\\\"{4}\\\",\\\"native_tag\\\":{5},\\\"lease_scope_id\\\":\\\"{6}\\\"}}",\n                _sessionId, _epoch, objectId, reg.Generation, kind, nativeTag, leaseScopeId ?? ""\n            );\n        }\n    }\n\n    public T Resolve<T>(string objectId, ulong epoch, string sessionId, uint generation) where T : TaggedObject\n    {\n        if (sessionId != _sessionId)\n        {\n            throw new InvalidOperationException(string.Format("stale session reference: got {0}, expected {1}", sessionId, _sessionId));\n        }\n        if (epoch != _epoch)\n        {\n            throw new InvalidOperationException(string.Format("stale epoch reference: got {0}, expected {1}", epoch, _epoch));\n        }\n        if (generation == 0)\n        {\n            throw new InvalidOperationException("object reference generation must be non-zero");\n        }\n\n        lock (_lock)\n        {\n            RegisteredObject reg;\n            if (!_objects.TryGetValue(objectId, out reg))\n            {\n                throw new KeyNotFoundException("object handle not found or expired: " + objectId);\n            }\n            if (reg.Generation != generation)\n            {\n                throw new InvalidOperationException(string.Format("stale object generation for {0}: got {1}, expected {2}", objectId, generation, reg.Generation));\n            }\n            if (reg.Target == null)\n            {\n                throw new InvalidOperationException("object target is null for handle: " + objectId);\n            }\n            if (!(reg.Target is T))\n            {\n                throw new InvalidOperationException(string.Format("object kind/type mismatch for handle {0}: registered={1}, requested={2}", objectId, reg.Kind, typeof(T).Name));\n            }\n            return (T)reg.Target;\n        }\n    }\n'''
if a.count(old_registry) != 1:
    raise SystemExit("production registry source snippet not found exactly once")
a = a.replace(old_registry, new_registry, 1)
a = a.replace('new ObjectRegistry(_sessionId, 1);', 'new ObjectRegistry(_sessionId, 1, 4096);', 1)

old_resolve = '''        if (!HasJsonKey(handleJson, "epoch")) throw new InvalidOperationException(expectedKind + " reference is missing epoch");\n        if (string.IsNullOrEmpty(kind)) throw new InvalidOperationException(expectedKind + " reference is missing kind");\n'''
new_resolve = '''        if (!HasJsonKey(handleJson, "epoch")) throw new InvalidOperationException(expectedKind + " reference is missing epoch");\n        if (!HasJsonKey(handleJson, "generation")) throw new InvalidOperationException(expectedKind + " reference is missing generation");\n        if (string.IsNullOrEmpty(kind)) throw new InvalidOperationException(expectedKind + " reference is missing kind");\n'''
if a.count(old_resolve) != 1:
    raise SystemExit("strict resolver generation insertion source not found")
a = a.replace(old_resolve, new_resolve, 1)
old_call = '''        ulong epoch = ExtractJsonUlong(handleJson, "epoch", 0);\n        return Registry.Resolve<T>(objectId, epoch, sessionId);'''
new_call = '''        ulong epoch = ExtractJsonUlong(handleJson, "epoch", 0);\n        ulong generationValue = ExtractJsonUlong(handleJson, "generation", 0);\n        if (generationValue == 0 || generationValue > uint.MaxValue) throw new InvalidOperationException(expectedKind + " reference has invalid generation");\n        return Registry.Resolve<T>(objectId, epoch, sessionId, (uint)generationValue);'''
if a.count(old_call) != 1:
    raise SystemExit("strict resolver call source not found")
a = a.replace(old_call, new_call, 1)
agent.write_text(a, encoding="utf-8")

hardening = pathlib.Path("internal/agentbundle/hardening_test.go")
h = hardening.read_text(encoding="utf-8")
needle = '''\t\t"object kind/type mismatch",\n'''
replacement = '''\t\t"object kind/type mismatch",\n\t\t"reference is missing generation",\n\t\t"stale object generation",\n\t\t"new ObjectRegistry(_sessionId, 1, 4096)",\n\t\t"object registry capacity reached",\n\t\t"HighWatermark",\n'''
if h.count(needle) != 1:
    raise SystemExit("hardening test insertion source not found")
hardening.write_text(h.replace(needle, replacement, 1), encoding="utf-8")

print("generation-aware ObjectRef and bounded production registry patched")
