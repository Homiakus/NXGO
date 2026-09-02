#!/usr/bin/env python3
from __future__ import annotations

import pathlib
import re
import sys

PATH = pathlib.Path("agent/bundle/AgentWorker.cs")
text = PATH.read_text(encoding="utf-8")
original = text


def sub_once(pattern: str, replacement: str, label: str) -> None:
    global text
    text, count = re.subn(pattern, replacement, text, count=1, flags=re.S)
    if count != 1:
        raise SystemExit(f"{label}: expected exactly one replacement, got {count}")


# H1: a timed-out queue item must not execute later if it never started. If it
# already started, surface OUTCOME_UNKNOWN and poison the worker/session.
executor = r'''public sealed class OutcomeUnknownException : Exception
{
    public OutcomeUnknownException(string message) : base(message) {}
}

public sealed class NxExecutor
{
    private sealed class WorkItem
    {
        private const int Queued = 0;
        private const int Started = 1;
        private const int Completed = 2;
        private const int CancelledBeforeStart = 3;

        private readonly Func<byte[]> _work;
        private int _state;

        public WorkItem(Func<byte[]> work)
        {
            if (work == null) throw new ArgumentNullException("work");
            _work = work;
            Done = new ManualResetEvent(false);
        }

        public ManualResetEvent Done { get; private set; }
        public byte[] Result { get; private set; }
        public Exception Error { get; private set; }
        public int State { get { return Thread.VolatileRead(ref _state); } }

        public bool TryCancelBeforeStart()
        {
            if (Interlocked.CompareExchange(ref _state, CancelledBeforeStart, Queued) != Queued)
            {
                return false;
            }
            Done.Set();
            return true;
        }

        public void Execute()
        {
            if (Interlocked.CompareExchange(ref _state, Started, Queued) != Queued)
            {
                // Cancelled-before-start items remain harmless queue tombstones.
                return;
            }

            try
            {
                Result = _work();
            }
            catch (Exception ex)
            {
                Error = ex;
            }
            finally
            {
                Thread.VolatileWrite(ref _state, Completed);
                Done.Set();
            }
        }
    }

    private readonly Queue<WorkItem> _queue = new Queue<WorkItem>();
    private readonly object _sync = new object();
    private int _boundThreadId;

    public int BoundThreadId { get { return _boundThreadId; } }

    public void BindToCurrentThread()
    {
        var current = Thread.CurrentThread.ManagedThreadId;
        if (_boundThreadId != 0 && _boundThreadId != current)
        {
            throw new InvalidOperationException("NX executor is already bound to another thread");
        }
        _boundThreadId = current;
    }

    public int DrainUntilEmpty(int maxPerBatch)
    {
        if (_boundThreadId == 0)
        {
            throw new InvalidOperationException("NX executor is not bound to an execution thread");
        }
        if (Thread.CurrentThread.ManagedThreadId != _boundThreadId)
        {
            throw new InvalidOperationException("drain must occur on the bound NX execution thread");
        }

        var drained = 0;
        while (drained < maxPerBatch)
        {
            WorkItem item = null;
            lock (_sync)
            {
                if (_queue.Count > 0)
                {
                    item = _queue.Dequeue();
                }
            }
            if (item == null) break;
            item.Execute();
            drained++;
        }
        return drained;
    }

    public byte[] EnqueueSync(Func<byte[]> work, int timeoutMs)
    {
        var item = new WorkItem(work);
        lock (_sync)
        {
            _queue.Enqueue(item);
        }

        var timeout = timeoutMs > 0 ? timeoutMs : 30000;
        if (!item.Done.WaitOne(timeout, false))
        {
            if (item.TryCancelBeforeStart())
            {
                throw new TimeoutException("operation timed out and was cancelled before NX execution started");
            }

            // A completion racing the timeout is still a known final outcome.
            if (!item.Done.WaitOne(0, false))
            {
                throw new OutcomeUnknownException("operation timed out after NX execution started; outcome is unknown and worker must be quarantined");
            }
        }

        if (item.Error != null) throw item.Error;
        return item.Result;
    }
}
'''
sub_once(
    r'public sealed class NxExecutor\n\{.*?\n\}\n\npublic sealed class BuilderScope',
    executor + '\npublic sealed class BuilderScope',
    'replace NxExecutor',
)

# H3: make registry type mismatch explicit instead of relying on an unchecked cast.
old = '''            if (reg.Target == null)\n            {\n                throw new InvalidOperationException("object target is null for handle: " + objectId);\n            }\n            return (T)reg.Target;'''
new = '''            if (reg.Target == null)\n            {\n                throw new InvalidOperationException("object target is null for handle: " + objectId);\n            }\n            if (!(reg.Target is T))\n            {\n                throw new InvalidOperationException(string.Format("object kind/type mismatch for handle {0}: registered={1}, requested={2}", objectId, reg.Kind, typeof(T).Name));\n            }\n            return (T)reg.Target;'''
if old not in text:
    raise SystemExit('registry type check: source snippet not found')
text = text.replace(old, new, 1)

# H6/A-009: save failure must abort close; never swallow it.
old = '''                        string objId = ExtractJsonString(payloadRaw, "object_id");\n                        Part part = ResolvePartFromPayload(session, payloadRaw);\n                        string partName = part.Name;\n                        bool save = ExtractJsonBool(payloadRaw, "save", false);\n                        if (save)\n                        {\n                            try { part.Save(BasePart.SaveComponents.True, BasePart.CloseAfterSave.False); } catch {}\n                        }'''
new = '''                        string objId = ExtractHandleObjectId(payloadRaw, "part_ref", "assembly_part_ref");\n                        Part part = ResolvePartFromPayload(session, payloadRaw);\n                        string partName = part.Name;\n                        bool save = ExtractJsonBool(payloadRaw, "save", false);\n                        if (save)\n                        {\n                            part.Save(BasePart.SaveComponents.True, BasePart.CloseAfterSave.False);\n                        }'''
if old not in text:
    raise SystemExit('part.close save hardening: source snippet not found')
text = text.replace(old, new, 1)

# A-008: direct protocol callers also fail closed for unsupported feature options.
text = text.replace(
    '''                    if (op == "feature.create_block")\n                    {\n                        Part part = ResolvePartFromPayload(session, payloadRaw);''',
    '''                    if (op == "feature.create_block")\n                    {\n                        Part part = ResolvePartFromPayload(session, payloadRaw);\n                        RequireCreateOnlyFeatureOptions(payloadRaw);''',
    1,
)
text = text.replace(
    '''                    if (op == "feature.create_cylinder")\n                    {\n                        Part part = ResolvePartFromPayload(session, payloadRaw);''',
    '''                    if (op == "feature.create_cylinder")\n                    {\n                        Part part = ResolvePartFromPayload(session, payloadRaw);\n                        RequireCreateOnlyFeatureOptions(payloadRaw);''',
    1,
)

# H5/A-006: normalize UF mass properties exactly once according to owning part units.
mass_block = r'''                    if (op == "geometry.query_mass_properties")
                    {
                        Body body = ResolveBodyFromPayload(session, payloadRaw);
                        var uf = NXOpen.UF.UFSession.GetUFSession();
                        var bodyTags = new NXOpen.Tag[] { body.Tag };
                        double density = 1.0;
                        bool imperial = body.OwningPart != null && body.OwningPart.PartUnits == Part.Units.Inches;
                        int units = imperial ? 1 : 4; // UF: 1=lb/in, 4=kg/m
                        int mode = 1;
                        int accuracy = 1;
                        double[] accValues = new double[11];
                        double[] massProps = new double[47];
                        double[] statistics = new double[13];
                        uf.Modl.AskMassProps3d(bodyTags, 1, mode, units, density, accuracy, accValues, massProps, statistics);

                        double lengthScale = imperial ? 1.0 : 1000.0;
                        double areaScale = imperial ? 1.0 : 1000000.0;
                        double volumeScale = imperial ? 1.0 : 1000000000.0;
                        double area = massProps[0] * areaScale;
                        double vol = massProps[1] * volumeScale;
                        double mass = massProps[2];
                        double centX = massProps[3] * lengthScale;
                        double centY = massProps[4] * lengthScale;
                        double centZ = massProps[5] * lengthScale;

                        var respJson = string.Format(
                            System.Globalization.CultureInfo.InvariantCulture,
                            "{{\"volume\":{0:F6},\"area\":{1:F6},\"mass\":{2:F6},\"centroid\":[{3:F6},{4:F6},{5:F6}],\"solid_type\":\"solid\"}}",
                            vol, area, mass, centX, centY, centZ
                        );
                        return FormatResponse(reqId, respJson);
                    }
'''
sub_once(
    r'                    if \(op == "geometry\.query_mass_properties"\)\n                    \{.*?\n                    \}\n\n                    if \(op == "geometry\.query_bounding_box"\)',
    mass_block + '\n                    if (op == "geometry.query_bounding_box")',
    'replace mass-properties units',
)

bbox_block = r'''                    if (op == "geometry.query_bounding_box")
                    {
                        Body body = ResolveBodyFromPayload(session, payloadRaw);
                        var uf = NXOpen.UF.UFSession.GetUFSession();
                        double[] minMax = new double[6];
                        uf.Modl.AskBoundingBox(body.Tag, minMax);

                        // UF_MODL_ask_bounding_box already returns owning-part length units.
                        double minX = minMax[0], minY = minMax[1], minZ = minMax[2];
                        double maxX = minMax[3], maxY = minMax[4], maxZ = minMax[5];
                        double dx = maxX - minX, dy = maxY - minY, dz = maxZ - minZ;

                        var respJson = string.Format(
                            System.Globalization.CultureInfo.InvariantCulture,
                            "{{\"min_corner\":[{0:F6},{1:F6},{2:F6}],\"max_corner\":[{3:F6},{4:F6},{5:F6}],\"dimensions\":[{6:F6},{7:F6},{8:F6}]}}",
                            minX, minY, minZ, maxX, maxY, maxZ, dx, dy, dz
                        );
                        return FormatResponse(reqId, respJson);
                    }
'''
sub_once(
    r'                    if \(op == "geometry\.query_bounding_box"\)\n                    \{.*?\n                    \}\n\n                    if \(op == "part\.query_bodies"\)',
    bbox_block + '\n                    if (op == "part.query_bodies")',
    'replace bounding-box units',
)

# H1: distinguish execution-started timeout from ordinary internal errors.
needle = '''            catch (NXException nxEx)\n            {'''
insert = '''            catch (OutcomeUnknownException outcomeEx)\n            {\n                Health.Set(SessionHealth.Lost);\n                session.LogFile.WriteLine("[NXGO][OUTCOME_UNKNOWN] op=" + op + " msg=" + outcomeEx.Message);\n                return FormatError(reqId, "OUTCOME_UNKNOWN", outcomeEx.Message, 0, "lost", false);\n            }\n            catch (NXException nxEx)\n            {'''
if needle not in text:
    raise SystemExit('outcome unknown catch: source snippet not found')
text = text.replace(needle, insert, 1)

# H3: remove every supplied-handle fallback to current work/display part or first body.
resolvers = r'''    private static T ResolveRegisteredHandle<T>(string handleJson, string expectedKind) where T : TaggedObject
    {
        if (string.IsNullOrEmpty(handleJson) || !handleJson.StartsWith("{"))
        {
            throw new InvalidOperationException("missing " + expectedKind + " reference object");
        }

        string objectId = ExtractJsonString(handleJson, "object_id");
        string sessionId = ExtractJsonString(handleJson, "session_id");
        string kind = ExtractJsonString(handleJson, "kind");
        if (string.IsNullOrEmpty(objectId)) throw new InvalidOperationException(expectedKind + " reference is missing object_id");
        if (string.IsNullOrEmpty(sessionId)) throw new InvalidOperationException(expectedKind + " reference is missing session_id");
        if (!HasJsonKey(handleJson, "epoch")) throw new InvalidOperationException(expectedKind + " reference is missing epoch");
        if (string.IsNullOrEmpty(kind)) throw new InvalidOperationException(expectedKind + " reference is missing kind");
        if (!string.Equals(kind, expectedKind, StringComparison.OrdinalIgnoreCase))
        {
            throw new InvalidOperationException(string.Format("wrong object kind: got {0}, expected {1}", kind, expectedKind));
        }

        ulong epoch = ExtractJsonUlong(handleJson, "epoch", 0);
        return Registry.Resolve<T>(objectId, epoch, sessionId);
    }

    private static Part ResolvePartFromPayload(Session session, string payloadJson)
    {
        string partRefJson = ExtractJsonObjectOrSection(payloadJson, "assembly_part_ref");
        if (string.IsNullOrEmpty(partRefJson))
        {
            partRefJson = ExtractJsonObjectOrSection(payloadJson, "part_ref");
        }
        if (!string.IsNullOrEmpty(partRefJson))
        {
            return ResolveRegisteredHandle<Part>(partRefJson, "Part");
        }
        if (HasJsonKey(payloadJson, "object_id"))
        {
            return ResolveRegisteredHandle<Part>(payloadJson, "Part");
        }
        throw new InvalidOperationException("missing part reference in payload; implicit work/display fallback is forbidden");
    }

    private static Body ResolveBodyFromPayload(Session session, string payloadJson)
    {
        string bodyRefJson = ExtractJsonObjectOrSection(payloadJson, "body_ref");
        if (!string.IsNullOrEmpty(bodyRefJson))
        {
            return ResolveRegisteredHandle<Body>(bodyRefJson, "Body");
        }
        if (HasJsonKey(payloadJson, "object_id"))
        {
            return ResolveRegisteredHandle<Body>(payloadJson, "Body");
        }
        throw new InvalidOperationException("missing body reference in payload; implicit first-body fallback is forbidden");
    }
'''
sub_once(
    r'    private static Part ResolvePartFromPayload\(Session session, string payloadJson\)\n    \{.*?\n    \}\n\n    private static Body ResolveBodyFromPayload\(Session session, string payloadJson\)\n    \{.*?\n    \}\n',
    resolvers,
    'replace part/body resolvers',
)

component = r'''    private static Component ResolveComponentFromPayload(Session session, string payloadJson)
    {
        string compRefJson = ExtractJsonObjectOrSection(payloadJson, "component_ref");
        if (!string.IsNullOrEmpty(compRefJson))
        {
            return ResolveRegisteredHandle<Component>(compRefJson, "Component");
        }
        if (HasJsonKey(payloadJson, "object_id"))
        {
            return ResolveRegisteredHandle<Component>(payloadJson, "Component");
        }
        throw new InvalidOperationException("missing component reference in payload");
    }
'''
sub_once(
    r'    private static Component ResolveComponentFromPayload\(Session session, string payloadJson\)\n    \{.*?\n    \}\n',
    component,
    'replace component resolver',
)

# Helpers used by strict resolvers and feature validation.
helper_anchor = '    private static double ExtractJsonDouble(string json, string key, double defaultVal)\n'
helpers = r'''    private static bool HasJsonKey(string json, string key)
    {
        if (string.IsNullOrEmpty(json)) return false;
        return json.IndexOf("\"" + key + "\"", StringComparison.Ordinal) >= 0;
    }

    private static string ExtractHandleObjectId(string payloadJson, params string[] referenceKeys)
    {
        foreach (var key in referenceKeys)
        {
            string handleJson = ExtractJsonObjectOrSection(payloadJson, key);
            if (!string.IsNullOrEmpty(handleJson))
            {
                string objectId = ExtractJsonString(handleJson, "object_id");
                if (!string.IsNullOrEmpty(objectId)) return objectId;
            }
        }
        return ExtractJsonString(payloadJson, "object_id");
    }

    private static void RequireCreateOnlyFeatureOptions(string payloadJson)
    {
        string booleanOp = ExtractJsonString(payloadJson, "boolean_op");
        if (!string.IsNullOrEmpty(booleanOp) && !string.Equals(booleanOp, "create", StringComparison.OrdinalIgnoreCase))
        {
            throw new NotSupportedException("boolean operation is not implemented by production Agent: " + booleanOp);
        }
        string target = ExtractJsonObjectOrSection(payloadJson, "target_body_ref");
        if (!string.IsNullOrEmpty(target) && target != "{}")
        {
            throw new NotSupportedException("target_body_ref is not implemented by production Agent");
        }
    }

'''
if helper_anchor not in text:
    raise SystemExit('helper insertion anchor not found')
text = text.replace(helper_anchor, helpers + helper_anchor, 1)

if text == original:
    raise SystemExit("patch produced no changes")

PATH.write_text(text, encoding="utf-8")
print(f"patched {PATH}: {len(original)} -> {len(text)} bytes")
