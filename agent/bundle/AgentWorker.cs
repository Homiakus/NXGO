using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;
using NXOpen;
using NXOpen.Assemblies;
using Thread = System.Threading.Thread;

public enum SessionHealth
{
    Healthy = 0,
    Dirty = 1,
    Poisoned = 2,
    Lost = 3
}

public sealed class SessionHealthState
{
    private int _value;
    public SessionHealth Value { get { return (SessionHealth)Thread.VolatileRead(ref _value); } }

    public void Set(SessionHealth next)
    {
        Thread.VolatileWrite(ref _value, (int)next);
    }

    public void RequireReusable()
    {
        var current = Value;
        if (current != SessionHealth.Healthy)
        {
            throw new InvalidOperationException("session is not healthy: " + current);
        }
    }
}

public static class FrameCodec
{
    public const int HeaderSize = 4;
    public const int DefaultMaxPayloadBytes = 4 * 1024 * 1024;

    public static byte[] Encode(byte[] payload)
    {
        if (payload == null) payload = new byte[0];
        if (payload.Length > DefaultMaxPayloadBytes)
        {
            throw new ArgumentException("payload exceeds max allowed frame size", "payload");
        }
        var frame = new byte[HeaderSize + payload.Length];
        frame[0] = (byte)(payload.Length & 0xFF);
        frame[1] = (byte)((payload.Length >> 8) & 0xFF);
        frame[2] = (byte)((payload.Length >> 16) & 0xFF);
        frame[3] = (byte)((payload.Length >> 24) & 0xFF);
        Array.Copy(payload, 0, frame, HeaderSize, payload.Length);
        return frame;
    }
}

public sealed class NxExecutor
{
    private readonly Queue<Action> _queue = new Queue<Action>();
    private readonly object _sync = new object();
    private int _boundThreadId;

    public int BoundThreadId { get { return _boundThreadId; } }

    public void BindToCurrentThread()
    {
        _boundThreadId = Thread.CurrentThread.ManagedThreadId;
    }

    public int DrainUntilEmpty(int maxPerBatch)
    {
        if (_boundThreadId != 0 && Thread.CurrentThread.ManagedThreadId != _boundThreadId)
        {
            throw new InvalidOperationException("drain must occur on the bound NX execution thread");
        }

        var drained = 0;
        while (drained < maxPerBatch)
        {
            Action action = null;
            lock (_sync)
            {
                if (_queue.Count > 0)
                {
                    action = _queue.Dequeue();
                }
            }
            if (action == null) break;
            action();
            drained++;
        }
        return drained;
    }

    public byte[] EnqueueSync(Func<byte[]> work, int timeoutMs)
    {
        var mre = new ManualResetEvent(false);
        byte[] result = null;
        Exception capturedEx = null;

        Action item = delegate
        {
            try
            {
                result = work();
            }
            catch (Exception ex)
            {
                capturedEx = ex;
            }
            finally
            {
                mre.Set();
            }
        };

        lock (_sync)
        {
            _queue.Enqueue(item);
        }

        if (!mre.WaitOne(timeoutMs > 0 ? timeoutMs : 30000, false))
        {
            throw new TimeoutException("operation timed out waiting for NX execution thread");
        }

        if (capturedEx != null) throw capturedEx;
        return result;
    }
}

public sealed class BuilderScope<TBuilder> : IDisposable where TBuilder : class
{
    private readonly Action<TBuilder> _destroy;
    private bool _commitAttempted;
    private bool _disposed;

    public BuilderScope(TBuilder builder, Action<TBuilder> destroy)
    {
        if (builder == null) throw new ArgumentNullException("builder");
        if (destroy == null) throw new ArgumentNullException("destroy");
        Builder = builder;
        _destroy = destroy;
    }

    public TBuilder Builder { get; private set; }

    public TResult CommitOnce<TResult>(Func<TBuilder, TResult> commit)
    {
        if (commit == null) throw new ArgumentNullException("commit");
        if (_disposed) throw new ObjectDisposedException(typeof(BuilderScope<TBuilder>).Name);
        if (_commitAttempted)
        {
            throw new InvalidOperationException("Builder commit already attempted; create a fresh builder for retry.");
        }
        _commitAttempted = true;
        return commit(Builder);
    }

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        _destroy(Builder);
    }
}

public sealed class RegisteredObject
{
    public TaggedObject Target { get; set; }
    public string Kind { get; set; }
    public string LeaseScopeId { get; set; }
    public uint NativeTag { get; set; }
    public DateTime RegisteredAt { get; set; }
}

public sealed class ObjectRegistry
{
    private readonly string _sessionId;
    private ulong _epoch;
    private long _handleCounter;
    private readonly Dictionary<string, RegisteredObject> _objects = new Dictionary<string, RegisteredObject>();
    private readonly object _lock = new object();

    public ObjectRegistry(string sessionId, ulong epoch)
    {
        _sessionId = sessionId;
        _epoch = epoch;
    }

    public ulong Epoch { get { return _epoch; } }
    public string SessionId { get { return _sessionId; } }

    public string Register(TaggedObject obj, string kind, string leaseScopeId, out uint nativeTag)
    {
        if (obj == null) throw new ArgumentNullException("obj");
        lock (_lock)
        {
            var id = "obj-" + Interlocked.Increment(ref _handleCounter);
            nativeTag = 0;
            try
            {
                nativeTag = (uint)obj.Tag;
            }
            catch {}

            _objects[id] = new RegisteredObject
            {
                Target = obj,
                Kind = kind ?? "TaggedObject",
                LeaseScopeId = leaseScopeId ?? "",
                NativeTag = nativeTag,
                RegisteredAt = DateTime.UtcNow
            };
            return id;
        }
    }

    public string FormatHandleJson(string objectId, string kind, uint nativeTag, string leaseScopeId)
    {
        return string.Format(
            "{{\"session_id\":\"{0}\",\"epoch\":{1},\"object_id\":\"{2}\",\"kind\":\"{3}\",\"native_tag\":{4},\"lease_scope_id\":\"{5}\"}}",
            _sessionId, _epoch, objectId, kind, nativeTag, leaseScopeId ?? ""
        );
    }

    public T Resolve<T>(string objectId, ulong epoch, string sessionId) where T : TaggedObject
    {
        if (sessionId != _sessionId)
        {
            throw new InvalidOperationException(string.Format("stale session reference: got {0}, expected {1}", sessionId, _sessionId));
        }
        if (epoch != _epoch)
        {
            throw new InvalidOperationException(string.Format("stale epoch reference: got {0}, expected {1}", epoch, _epoch));
        }

        lock (_lock)
        {
            RegisteredObject reg;
            if (!_objects.TryGetValue(objectId, out reg))
            {
                throw new KeyNotFoundException("object handle not found or expired: " + objectId);
            }
            if (reg.Target == null)
            {
                throw new InvalidOperationException("object target is null for handle: " + objectId);
            }
            return (T)reg.Target;
        }
    }

    public bool Release(string objectId)
    {
        lock (_lock)
        {
            return _objects.Remove(objectId);
        }
    }

    public int ReleaseMany(IEnumerable<string> objectIds)
    {
        var count = 0;
        lock (_lock)
        {
            foreach (var id in objectIds)
            {
                if (_objects.Remove(id)) count++;
            }
        }
        return count;
    }

    public void InvalidateAll()
    {
        lock (_lock)
        {
            _epoch++;
            _objects.Clear();
        }
    }
}

public sealed class TransactionInfo
{
    public string TxId { get; set; }
    public Session.UndoMarkId MarkId { get; set; }
    public string Name { get; set; }
    public DateTime CreatedAt { get; set; }
}

public sealed class TransactionManager
{
    private readonly Dictionary<string, TransactionInfo> _active = new Dictionary<string, TransactionInfo>();
    private readonly object _lock = new object();

    public TransactionInfo Begin(Session session, string name)
    {
        if (string.IsNullOrEmpty(name)) name = "NXGO_Tx_" + Guid.NewGuid().ToString("N").Substring(0, 8);
        var mark = session.SetUndoMark(Session.MarkVisibility.Visible, name);
        var tx = new TransactionInfo
        {
            TxId = "tx-" + Guid.NewGuid().ToString("N"),
            MarkId = mark,
            Name = name,
            CreatedAt = DateTime.UtcNow
        };
        lock (_lock)
        {
            _active[tx.TxId] = tx;
        }
        return tx;
    }

    public bool Commit(Session session, string txId)
    {
        TransactionInfo tx;
        lock (_lock)
        {
            if (!_active.TryGetValue(txId, out tx))
            {
                throw new KeyNotFoundException("transaction not found: " + txId);
            }
            _active.Remove(txId);
        }
        try
        {
            session.DeleteUndoMark(tx.MarkId, tx.Name);
        }
        catch
        {
            // best-effort cleanup
        }
        return true;
    }

    public bool Rollback(Session session, string txId, SessionHealthState health)
    {
        TransactionInfo tx;
        lock (_lock)
        {
            if (!_active.TryGetValue(txId, out tx))
            {
                throw new KeyNotFoundException("transaction not found: " + txId);
            }
            _active.Remove(txId);
        }

        try
        {
            session.UndoToMark(tx.MarkId, tx.Name);
            return true;
        }
        catch (Exception ex)
        {
            session.LogFile.WriteLine("[NXGO][ERROR] rollback failed for tx=" + txId + ": " + ex.Message);
            health.Set(SessionHealth.Dirty);
            throw;
        }
    }
}

public sealed class Win32PipeServer : IDisposable
{
    private const uint PIPE_ACCESS_DUPLEX = 0x00000003;
    private const uint PIPE_TYPE_BYTE = 0x00000000;
    private const uint PIPE_READMODE_BYTE = 0x00000000;
    private const uint PIPE_WAIT = 0x00000000;
    private const uint BUFFER_SIZE = 65536;
    private static readonly IntPtr INVALID_HANDLE_VALUE = new IntPtr(-1);

    [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Auto)]
    private static extern IntPtr CreateNamedPipe(
        string lpName,
        uint dwOpenMode,
        uint dwPipeMode,
        uint nMaxInstances,
        uint nOutBufferSize,
        uint nInBufferSize,
        uint nDefaultTimeOut,
        IntPtr lpSecurityAttributes);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool ConnectNamedPipe(IntPtr hNamedPipe, IntPtr lpOverlapped);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool DisconnectNamedPipe(IntPtr hNamedPipe);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool ReadFile(IntPtr hFile, byte[] lpBuffer, uint nNumberOfBytesToRead, out uint lpNumberOfBytesRead, IntPtr lpOverlapped);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool WriteFile(IntPtr hFile, byte[] lpBuffer, uint nNumberOfBytesToWrite, out uint lpNumberOfBytesWritten, IntPtr lpOverlapped);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool CloseHandle(IntPtr hObject);

    private readonly string _pipePath;
    private readonly Func<byte[], byte[]> _handler;
    private readonly Thread _workerThread;
    private volatile bool _stopped;
    private IntPtr _hPipe = INVALID_HANDLE_VALUE;

    public Win32PipeServer(string pipeName, Func<byte[], byte[]> handler)
    {
        _pipePath = @"\\.\pipe\" + pipeName;
        _handler = handler;
        _workerThread = new Thread(RunLoop);
        _workerThread.IsBackground = true;
    }

    public void Start()
    {
        _workerThread.Start();
    }

    public void Dispose()
    {
        _stopped = true;
        if (_hPipe != INVALID_HANDLE_VALUE)
        {
            CloseHandle(_hPipe);
            _hPipe = INVALID_HANDLE_VALUE;
        }
    }

    private void RunLoop()
    {
        while (!_stopped)
        {
            _hPipe = CreateNamedPipe(
                _pipePath,
                PIPE_ACCESS_DUPLEX,
                PIPE_TYPE_BYTE | PIPE_READMODE_BYTE | PIPE_WAIT,
                1,
                BUFFER_SIZE,
                BUFFER_SIZE,
                0,
                IntPtr.Zero);

            if (_hPipe == INVALID_HANDLE_VALUE)
            {
                if (_stopped) break;
                Thread.Sleep(100);
                continue;
            }

            bool connected = ConnectNamedPipe(_hPipe, IntPtr.Zero) ? true : (Marshal.GetLastWin32Error() == 535); // ERROR_PIPE_CONNECTED = 535
            if (connected && !_stopped)
            {
                ServeClient(_hPipe);
            }

            DisconnectNamedPipe(_hPipe);
            CloseHandle(_hPipe);
            _hPipe = INVALID_HANDLE_VALUE;
        }
    }

    private void ServeClient(IntPtr hPipe)
    {
        while (!_stopped)
        {
            var header = new byte[FrameCodec.HeaderSize];
            if (!ReadExact(hPipe, header, (uint)header.Length)) return;

            int length = header[0] | (header[1] << 8) | (header[2] << 16) | (header[3] << 24);
            if (length < 0 || length > FrameCodec.DefaultMaxPayloadBytes) return;

            var payload = new byte[length];
            if (length > 0 && !ReadExact(hPipe, payload, (uint)length)) return;

            var responsePayload = _handler(payload);
            var responseFrame = FrameCodec.Encode(responsePayload);
            if (!WriteExact(hPipe, responseFrame, (uint)responseFrame.Length)) return;
        }
    }

    private static bool ReadExact(IntPtr hPipe, byte[] buffer, uint count)
    {
        uint total = 0;
        while (total < count)
        {
            uint read;
            var subBuffer = new byte[count - total];
            if (!ReadFile(hPipe, subBuffer, (uint)subBuffer.Length, out read, IntPtr.Zero) || read == 0)
            {
                return false;
            }
            Array.Copy(subBuffer, 0, buffer, (int)total, (int)read);
            total += read;
        }
        return true;
    }

    private static bool WriteExact(IntPtr hPipe, byte[] buffer, uint count)
    {
        uint total = 0;
        while (total < count)
        {
            uint written;
            var subBuffer = new byte[count - total];
            Array.Copy(buffer, (int)total, subBuffer, 0, (int)(count - total));
            if (!WriteFile(hPipe, subBuffer, (uint)subBuffer.Length, out written, IntPtr.Zero) || written == 0)
            {
                return false;
            }
            total += written;
        }
        return true;
    }
}

public class Program
{
    private static readonly SessionHealthState Health = new SessionHealthState();
    private static volatile bool _shutdownRequested;
    private static readonly string _sessionId = "nx-sess-" + Guid.NewGuid().ToString("N");
    private static readonly ObjectRegistry Registry = new ObjectRegistry(_sessionId, 1);
    private static readonly TransactionManager Transactions = new TransactionManager();

    public static void Main(string[] args)
    {
        var session = Session.GetSession();
        var executor = new NxExecutor();
        executor.BindToCurrentThread();

        var pipeName = Environment.GetEnvironmentVariable("NXGO_PIPE_NAME");
        if (string.IsNullOrEmpty(pipeName))
        {
            pipeName = "nxgo-worker-" + Process.GetCurrentProcess().Id;
        }

        var server = new Win32PipeServer(pipeName, delegate(byte[] payload)
        {
            return HandleRequest(session, executor, payload);
        });

        session.LogFile.WriteLine("[NXGO] worker agent starting on pipe=" + pipeName);
        server.Start();

        while (!_shutdownRequested && Health.Value != SessionHealth.Lost)
        {
            executor.DrainUntilEmpty(64);
            Thread.Sleep(5);
        }

        session.LogFile.WriteLine("[NXGO] worker agent exiting cleanly");
        server.Dispose();
        Process.GetCurrentProcess().Kill();
    }

    public static int GetUnloadOption(string dummy)
    {
        return (int)Session.LibraryUnloadOption.AtTermination;
    }

    private static byte[] HandleRequest(Session session, NxExecutor executor, byte[] payload)
    {
        var text = Encoding.UTF8.GetString(payload);

        // 1. Handshake
        if (text.Contains("\"protocol_version\"") && text.Contains("\"nonce\""))
        {
            var ugiiVer = session.GetEnvironmentVariableValue("UGII_VERSION");
            if (string.IsNullOrEmpty(ugiiVer)) ugiiVer = "2512";

            var hsRespJson = string.Format(
                "{{\"protocol_version\":{{\"major\":1,\"minor\":0}},\"agent_version\":\"v0.1.0-realnx\",\"nx_release\":\"{0}\",\"nx_build\":\"{0}.real\",\"nx_pid\":{1},\"session_id\":\"{2}\",\"epoch\":{3},\"capabilities\":[\"nx.ping\",\"session.info\",\"transaction.begin\",\"transaction.commit\",\"transaction.rollback\",\"part.new\",\"part.open\",\"part.save\",\"part.close\",\"part.query_summary\",\"object.release\"],\"max_payload_bytes\":4194304,\"security_policy\":\"local_pipe_only\"}}",
                ugiiVer,
                Process.GetCurrentProcess().Id,
                Registry.SessionId,
                Registry.Epoch
            );
            return Encoding.UTF8.GetBytes(hsRespJson);
        }

        // 2. RequestEnvelope
        if (text.Contains("\"request_id\"") && text.Contains("\"op\""))
        {
            string reqId = ExtractJsonString(text, "request_id");
            string op = ExtractJsonString(text, "op");
            string payloadRaw = ExtractJsonObjectOrSection(text, "payload");

            if (op == "shutdown")
            {
                _shutdownRequested = true;
                var respJson = string.Format("{{\"request_id\":\"{0}\",\"status\":\"OK\",\"payload\":{{\"shutdown\":true}}}}", reqId);
                return Encoding.UTF8.GetBytes(respJson);
            }

            try
            {
                return executor.EnqueueSync(delegate
                {
                    Health.RequireReusable();

                    if (op == "nx.ping")
                    {
                        session.LogFile.WriteLine("[NXGO] nx.ping on bound thread " + Thread.CurrentThread.ManagedThreadId);
                        return FormatResponse(reqId, "{\"ping\":\"pong\"}");
                    }

                    if (op == "session.info")
                    {
                        var ugiiVer = session.GetEnvironmentVariableValue("UGII_VERSION");
                        var baseDir = session.GetEnvironmentVariableValue("UGII_BASE_DIR");
                        string activePartName = session.Parts.Work != null ? session.Parts.Work.Name : "";
                        var respJson = string.Format(
                            "{{\"release\":\"{0}\",\"base_dir\":\"{1}\",\"thread_id\":{2},\"work_part\":\"{3}\",\"epoch\":{4},\"session_id\":\"{5}\"}}",
                            ugiiVer,
                            baseDir != null ? baseDir.Replace('\\', '/') : "",
                            Thread.CurrentThread.ManagedThreadId,
                            activePartName,
                            Registry.Epoch,
                            Registry.SessionId
                        );
                        return FormatResponse(reqId, respJson);
                    }

                    // Transaction operations (Phase 5)
                    if (op == "transaction.begin")
                    {
                        string txName = ExtractJsonString(payloadRaw, "name");
                        var tx = Transactions.Begin(session, txName);
                        var txJson = string.Format("{{\"tx_id\":\"{0}\",\"mark_id\":{1}}}", tx.TxId, (int)tx.MarkId);
                        return FormatResponse(reqId, txJson);
                    }

                    if (op == "transaction.commit")
                    {
                        string txId = ExtractJsonString(payloadRaw, "tx_id");
                        Transactions.Commit(session, txId);
                        var txJson = string.Format("{{\"committed\":true,\"tx_id\":\"{0}\"}}", txId);
                        return FormatResponse(reqId, txJson);
                    }

                    if (op == "transaction.rollback")
                    {
                        string txId = ExtractJsonString(payloadRaw, "tx_id");
                        Transactions.Rollback(session, txId, Health);
                        var txJson = string.Format("{{\"rolled_back\":true,\"tx_id\":\"{0}\"}}", txId);
                        return FormatResponse(reqId, txJson);
                    }

                    // Part operations (Phase 7)
                    if (op == "part.new")
                    {
                        string partName = ExtractJsonString(payloadRaw, "name");
                        if (string.IsNullOrEmpty(partName)) partName = "model_" + Guid.NewGuid().ToString("N").Substring(0, 6) + ".prt";
                        string unitsStr = ExtractJsonString(payloadRaw, "units").ToLowerInvariant();
                        var units = (unitsStr == "in" || unitsStr == "inches") ? Part.Units.Inches : Part.Units.Millimeters;

                        Part part = session.Parts.NewDisplay(partName, units);
                        uint nativeTag;
                        string objId = Registry.Register(part, "Part", "", out nativeTag);
                        string handleJson = Registry.FormatHandleJson(objId, "Part", nativeTag, "");

                        var respJson = string.Format(
                            "{{\"part_ref\":{0},\"name\":\"{1}\",\"units\":\"{2}\"}}",
                            handleJson,
                            part.Name,
                            part.PartUnits.ToString()
                        );
                        return FormatResponse(reqId, respJson);
                    }

                    if (op == "part.open")
                    {
                        string filePath = ExtractJsonString(payloadRaw, "path");
                        if (string.IsNullOrEmpty(filePath))
                        {
                            throw new ArgumentException("missing part path for part.open");
                        }
                        PartLoadStatus loadStatus;
                        Part part = session.Parts.OpenDisplay(filePath, out loadStatus);
                        if (loadStatus != null) loadStatus.Dispose();

                        uint nativeTag;
                        string objId = Registry.Register(part, "Part", "", out nativeTag);
                        string handleJson = Registry.FormatHandleJson(objId, "Part", nativeTag, "");

                        var respJson = string.Format(
                            "{{\"part_ref\":{0},\"name\":\"{1}\",\"units\":\"{2}\"}}",
                            handleJson,
                            part.Name,
                            part.PartUnits.ToString()
                        );
                        return FormatResponse(reqId, respJson);
                    }

                    if (op == "part.save")
                    {
                        Part part = ResolvePartFromPayload(session, payloadRaw);
                        part.Save(BasePart.SaveComponents.True, BasePart.CloseAfterSave.False);
                        var respJson = string.Format(
                            "{{\"saved\":true,\"name\":\"{0}\",\"full_path\":\"{1}\"}}",
                            part.Name,
                            part.FullPath.Replace('\\', '/')
                        );
                        return FormatResponse(reqId, respJson);
                    }

                    if (op == "part.close")
                    {
                        string objId = ExtractJsonString(payloadRaw, "object_id");
                        Part part = ResolvePartFromPayload(session, payloadRaw);
                        string partName = part.Name;
                        bool save = ExtractJsonBool(payloadRaw, "save", false);
                        if (save)
                        {
                            try { part.Save(BasePart.SaveComponents.True, BasePart.CloseAfterSave.False); } catch {}
                        }
                        part.Close(BasePart.CloseWholeTree.False, BasePart.CloseModified.CloseModified, null);
                        if (!string.IsNullOrEmpty(objId))
                        {
                            Registry.Release(objId);
                        }

                        var respJson = string.Format("{{\"closed\":true,\"name\":\"{0}\"}}", partName);
                        return FormatResponse(reqId, respJson);
                    }

                    if (op == "part.query_summary")
                    {
                        Part part = ResolvePartFromPayload(session, payloadRaw);
                        int bodyCount = 0;
                        foreach (Body b in part.Bodies) bodyCount++;
                        int featCount = 0;
                        foreach (NXOpen.Features.Feature f in part.Features) featCount++;
                        int compCount = 0;
                        if (part.ComponentAssembly != null && part.ComponentAssembly.RootComponent != null)
                        {
                            compCount = part.ComponentAssembly.RootComponent.GetChildren().Length;
                        }

                        uint nativeTag = 0;
                        try { nativeTag = (uint)part.Tag; } catch {}

                        var respJson = string.Format(
                            "{{\"name\":\"{0}\",\"units\":\"{1}\",\"body_count\":{2},\"feature_count\":{3},\"component_count\":{4},\"native_tag\":{5}}}",
                            part.Name,
                            part.PartUnits.ToString(),
                            bodyCount,
                            featCount,
                            compCount,
                            nativeTag
                        );
                        return FormatResponse(reqId, respJson);
                    }

                    if (op == "object.release")
                    {
                        var ids = ExtractStringArray(payloadRaw, "object_id");
                        int released = Registry.ReleaseMany(ids);
                        var respJson = string.Format("{{\"released_count\":{0}}}", released);
                        return FormatResponse(reqId, respJson);
                    }

                    // Unknown op
                    return FormatError(reqId, "INVALID_ARGUMENT", "unsupported operation: " + op, 0, Health.Value.ToString().ToLowerInvariant(), true);
                }, 30000);
            }
            catch (NXException nxEx)
            {
                session.LogFile.WriteLine("[NXGO][NXException] op=" + op + " code=" + nxEx.ErrorCode + " msg=" + nxEx.Message);
                return FormatError(reqId, "NX_EXCEPTION", nxEx.Message, nxEx.ErrorCode, Health.Value.ToString().ToLowerInvariant(), true);
            }
            catch (Exception ex)
            {
                session.LogFile.WriteLine("[NXGO][Exception] op=" + op + " msg=" + ex.Message);
                return FormatError(reqId, "INTERNAL", ex.Message, 0, Health.Value.ToString().ToLowerInvariant(), true);
            }
        }

        // Fallbacks
        if (text == "ping") return Encoding.UTF8.GetBytes("ok|pong");
        if (text == "shutdown")
        {
            _shutdownRequested = true;
            return Encoding.UTF8.GetBytes("ok|shutdown");
        }

        return Encoding.UTF8.GetBytes("error|unknown_format");
    }

    private static Part ResolvePartFromPayload(Session session, string payloadJson)
    {
        string objId = ExtractJsonString(payloadJson, "object_id");
        if (!string.IsNullOrEmpty(objId))
        {
            ulong epoch = ExtractJsonUlong(payloadJson, "epoch", Registry.Epoch);
            string sessId = ExtractJsonString(payloadJson, "session_id");
            if (string.IsNullOrEmpty(sessId)) sessId = Registry.SessionId;
            return Registry.Resolve<Part>(objId, epoch, sessId);
        }
        if (session.Parts.Work != null) return session.Parts.Work;
        if (session.Parts.Display != null) return session.Parts.Display;
        throw new InvalidOperationException("no active work or display part in session");
    }

    private static byte[] FormatResponse(string reqId, string payloadJson)
    {
        var resp = string.Format(
            "{{\"request_id\":\"{0}\",\"status\":\"OK\",\"payload\":{1},\"timing\":{{\"execution_ms\":1}}}}",
            reqId,
            payloadJson
        );
        return Encoding.UTF8.GetBytes(resp);
    }

    private static byte[] FormatError(string reqId, string category, string message, int nxCode, string health, bool recoverable)
    {
        var escMsg = (message ?? "").Replace("\\", "\\\\").Replace("\"", "\\\"").Replace("\r", "").Replace("\n", " ");
        var err = string.Format(
            "{{\"request_id\":\"{0}\",\"status\":\"ERROR\",\"error\":{{\"category\":\"{1}\",\"message\":\"{2}\",\"nx_error_code\":{3},\"session_health\":\"{4}\",\"recoverable\":{5}}}}}",
            reqId,
            category,
            escMsg,
            nxCode,
            health,
            recoverable ? "true" : "false"
        );
        return Encoding.UTF8.GetBytes(err);
    }

    private static string ExtractJsonString(string json, string key)
    {
        if (string.IsNullOrEmpty(json)) return "";
        var search = "\"" + key + "\":\"";
        var idx = json.IndexOf(search, StringComparison.Ordinal);
        if (idx < 0) return "";
        var start = idx + search.Length;
        var end = json.IndexOf("\"", start, StringComparison.Ordinal);
        if (end < 0) return "";
        return json.Substring(start, end - start);
    }

    private static ulong ExtractJsonUlong(string json, string key, ulong defaultVal)
    {
        if (string.IsNullOrEmpty(json)) return defaultVal;
        var search = "\"" + key + "\":";
        var idx = json.IndexOf(search, StringComparison.Ordinal);
        if (idx < 0) return defaultVal;
        var start = idx + search.Length;
        while (start < json.Length && char.IsWhiteSpace(json[start])) start++;
        var end = start;
        while (end < json.Length && char.IsDigit(json[end])) end++;
        if (end <= start) return defaultVal;
        ulong val;
        return ulong.TryParse(json.Substring(start, end - start), out val) ? val : defaultVal;
    }

    private static bool ExtractJsonBool(string json, string key, bool defaultVal)
    {
        if (string.IsNullOrEmpty(json)) return defaultVal;
        var search = "\"" + key + "\":";
        var idx = json.IndexOf(search, StringComparison.Ordinal);
        if (idx < 0) return defaultVal;
        var start = idx + search.Length;
        while (start < json.Length && char.IsWhiteSpace(json[start])) start++;
        if (json.Length >= start + 4 && json.Substring(start, 4).ToLowerInvariant() == "true") return true;
        if (json.Length >= start + 5 && json.Substring(start, 5).ToLowerInvariant() == "false") return false;
        return defaultVal;
    }

    private static string ExtractJsonObjectOrSection(string json, string key)
    {
        if (string.IsNullOrEmpty(json)) return "";
        var search = "\"" + key + "\":";
        var idx = json.IndexOf(search, StringComparison.Ordinal);
        if (idx < 0) return "";
        var start = idx + search.Length;
        while (start < json.Length && char.IsWhiteSpace(json[start])) start++;
        if (start >= json.Length) return "";

        if (json[start] == '{')
        {
            int depth = 0;
            for (int i = start; i < json.Length; i++)
            {
                if (json[i] == '{') depth++;
                else if (json[i] == '}')
                {
                    depth--;
                    if (depth == 0) return json.Substring(start, i - start + 1);
                }
            }
        }
        return json.Substring(start);
    }

    private static List<string> ExtractStringArray(string json, string key)
    {
        var list = new List<string>();
        if (string.IsNullOrEmpty(json)) return list;
        int idx = 0;
        var search = "\"" + key + "\":\"";
        while ((idx = json.IndexOf(search, idx, StringComparison.Ordinal)) >= 0)
        {
            var start = idx + search.Length;
            var end = json.IndexOf("\"", start, StringComparison.Ordinal);
            if (end > start)
            {
                list.Add(json.Substring(start, end - start));
                idx = end + 1;
            }
            else break;
        }
        return list;
    }
}
