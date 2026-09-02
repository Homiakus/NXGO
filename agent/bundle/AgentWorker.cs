using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Runtime.InteropServices;
using System.Security.Cryptography;
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

public sealed class OutcomeUnknownException : Exception
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
    public uint Generation { get; set; }
    public DateTime RegisteredAt { get; set; }
}

public sealed class ObjectRegistry
{
    private readonly string _sessionId;
    private readonly int _capacity;
    private ulong _epoch;
    private long _handleCounter;
    private long _generationCounter;
    private int _highWatermark;
    private readonly Dictionary<string, RegisteredObject> _objects = new Dictionary<string, RegisteredObject>();
    private readonly object _lock = new object();

    public ObjectRegistry(string sessionId, ulong epoch, int capacity)
    {
        if (capacity <= 0) throw new ArgumentOutOfRangeException("capacity");
        _sessionId = sessionId;
        _epoch = epoch;
        _capacity = capacity;
    }

    public ulong Epoch { get { return _epoch; } }
    public string SessionId { get { return _sessionId; } }
    public int Capacity { get { return _capacity; } }
    public int Count { get { lock (_lock) return _objects.Count; } }
    public int HighWatermark { get { lock (_lock) return _highWatermark; } }

    public string Register(TaggedObject obj, string kind, string leaseScopeId, out uint nativeTag)
    {
        if (obj == null) throw new ArgumentNullException("obj");
        lock (_lock)
        {
            if (_objects.Count >= _capacity)
            {
                throw new InvalidOperationException("object registry capacity reached; release handles or recycle worker");
            }
            var id = "obj-" + Interlocked.Increment(ref _handleCounter);
            var generationValue = Interlocked.Increment(ref _generationCounter);
            if (generationValue <= 0 || generationValue > uint.MaxValue)
            {
                throw new InvalidOperationException("object generation space exhausted; recycle worker");
            }
            uint generation = (uint)generationValue;
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
                Generation = generation,
                RegisteredAt = DateTime.UtcNow
            };
            if (_objects.Count > _highWatermark) _highWatermark = _objects.Count;
            return id;
        }
    }

    public string FormatHandleJson(string objectId, string kind, uint nativeTag, string leaseScopeId)
    {
        lock (_lock)
        {
            RegisteredObject reg;
            if (!_objects.TryGetValue(objectId, out reg))
            {
                throw new KeyNotFoundException("cannot format released/unknown object handle: " + objectId);
            }
            return string.Format(
                "{{\"session_id\":\"{0}\",\"epoch\":{1},\"object_id\":\"{2}\",\"generation\":{3},\"kind\":\"{4}\",\"native_tag\":{5},\"lease_scope_id\":\"{6}\"}}",
                _sessionId, _epoch, objectId, reg.Generation, kind, nativeTag, leaseScopeId ?? ""
            );
        }
    }

    public T Resolve<T>(string objectId, ulong epoch, string sessionId, uint generation) where T : TaggedObject
    {
        if (sessionId != _sessionId)
        {
            throw new InvalidOperationException(string.Format("stale session reference: got {0}, expected {1}", sessionId, _sessionId));
        }
        if (epoch != _epoch)
        {
            throw new InvalidOperationException(string.Format("stale epoch reference: got {0}, expected {1}", epoch, _epoch));
        }
        if (generation == 0)
        {
            throw new InvalidOperationException("object reference generation must be non-zero");
        }

        lock (_lock)
        {
            RegisteredObject reg;
            if (!_objects.TryGetValue(objectId, out reg))
            {
                throw new KeyNotFoundException("object handle not found or expired: " + objectId);
            }
            if (reg.Generation != generation)
            {
                throw new InvalidOperationException(string.Format("stale object generation for {0}: got {1}, expected {2}", objectId, generation, reg.Generation));
            }
            if (reg.Target == null)
            {
                throw new InvalidOperationException("object target is null for handle: " + objectId);
            }
            if (!(reg.Target is T))
            {
                throw new InvalidOperationException(string.Format("object kind/type mismatch for handle {0}: registered={1}, requested={2}", objectId, reg.Kind, typeof(T).Name));
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

public enum MutationJournalState
{
    Received = 0,
    Started = 1,
    Committed = 2,
    FailedBeforeStart = 3,
    OutcomeUnknown = 4
}

public enum MutationReplayDisposition
{
    New = 0,
    InFlight = 1,
    ReturnCommitted = 2,
    ReturnFailure = 3,
    OutcomeUnknown = 4
}

public sealed class MutationJournalAdmission
{
    public MutationReplayDisposition Disposition { get; set; }
    public byte[] CachedResponse { get; set; }
}

public sealed class MutationJournalRecord
{
    public string RequestId { get; set; }
    public string Operation { get; set; }
    public string PayloadHash { get; set; }
    public MutationJournalState State { get; set; }
    public byte[] Response { get; set; }
    public string Diagnostic { get; set; }
}

public sealed class MutationJournal
{
    private readonly int _capacity;
    private readonly Dictionary<string, MutationJournalRecord> _records = new Dictionary<string, MutationJournalRecord>(StringComparer.Ordinal);
    private readonly object _sync = new object();

    public MutationJournal(int capacity)
    {
        if (capacity <= 0) throw new ArgumentOutOfRangeException("capacity");
        _capacity = capacity;
    }

    public MutationJournalAdmission Admit(string requestId, string operation, byte[] payload)
    {
        if (string.IsNullOrEmpty(requestId)) throw new ArgumentException("request_id is required");
        if (string.IsNullOrEmpty(operation)) throw new ArgumentException("operation is required");
        string hash = ComputeHash(payload ?? new byte[0]);

        lock (_sync)
        {
            MutationJournalRecord existing;
            if (_records.TryGetValue(requestId, out existing))
            {
                if (!string.Equals(existing.Operation, operation, StringComparison.Ordinal) || !string.Equals(existing.PayloadHash, hash, StringComparison.Ordinal))
                {
                    throw new InvalidOperationException("request_id reused with different operation or payload: " + requestId);
                }

                if (existing.State == MutationJournalState.Committed)
                {
                    return new MutationJournalAdmission { Disposition = MutationReplayDisposition.ReturnCommitted, CachedResponse = Clone(existing.Response) };
                }
                if (existing.State == MutationJournalState.FailedBeforeStart)
                {
                    return new MutationJournalAdmission { Disposition = MutationReplayDisposition.ReturnFailure, CachedResponse = Clone(existing.Response) };
                }
                if (existing.State == MutationJournalState.OutcomeUnknown)
                {
                    return new MutationJournalAdmission { Disposition = MutationReplayDisposition.OutcomeUnknown };
                }
                return new MutationJournalAdmission { Disposition = MutationReplayDisposition.InFlight };
            }

            if (_records.Count >= _capacity)
            {
                throw new InvalidOperationException("mutation journal capacity reached; worker must be recycled");
            }

            _records.Add(requestId, new MutationJournalRecord
            {
                RequestId = requestId,
                Operation = operation,
                PayloadHash = hash,
                State = MutationJournalState.Received
            });
            return new MutationJournalAdmission { Disposition = MutationReplayDisposition.New };
        }
    }

    public void MarkStarted(string requestId)
    {
        lock (_sync)
        {
            MutationJournalRecord record = Require(requestId);
            if (record.State != MutationJournalState.Received)
            {
                throw new InvalidOperationException("request cannot start from journal state " + record.State);
            }
            record.State = MutationJournalState.Started;
        }
    }

    public void MarkCommitted(string requestId, byte[] response)
    {
        lock (_sync)
        {
            MutationJournalRecord record = Require(requestId);
            if (record.State != MutationJournalState.Started)
            {
                throw new InvalidOperationException("request cannot commit from journal state " + record.State);
            }
            record.Response = Clone(response);
            record.Diagnostic = "";
            record.State = MutationJournalState.Committed;
        }
    }

    public void MarkFailedBeforeStart(string requestId, byte[] response, string diagnostic)
    {
        lock (_sync)
        {
            MutationJournalRecord record = Require(requestId);
            if (record.State != MutationJournalState.Received)
            {
                throw new InvalidOperationException("only an unstarted request can be marked failed-before-start");
            }
            record.Response = Clone(response);
            record.Diagnostic = diagnostic ?? "";
            record.State = MutationJournalState.FailedBeforeStart;
        }
    }

    public void MarkOutcomeUnknown(string requestId, string diagnostic)
    {
        lock (_sync)
        {
            MutationJournalRecord record = Require(requestId);
            if (record.State != MutationJournalState.Started)
            {
                throw new InvalidOperationException("only a started request can become outcome-unknown");
            }
            record.Response = null;
            record.Diagnostic = diagnostic ?? "outcome unknown";
            record.State = MutationJournalState.OutcomeUnknown;
        }
    }

    public MutationJournalState GetState(string requestId)
    {
        lock (_sync) return Require(requestId).State;
    }

    private MutationJournalRecord Require(string requestId)
    {
        MutationJournalRecord record;
        if (!_records.TryGetValue(requestId, out record))
        {
            throw new KeyNotFoundException("mutation journal request not found: " + requestId);
        }
        return record;
    }

    private static byte[] Clone(byte[] value)
    {
        if (value == null) return null;
        var clone = new byte[value.Length];
        Array.Copy(value, clone, value.Length);
        return clone;
    }

    private static string ComputeHash(byte[] payload)
    {
        using (var sha = SHA256.Create())
        {
            byte[] hash = sha.ComputeHash(payload);
            var sb = new StringBuilder(hash.Length * 2);
            foreach (byte b in hash) sb.Append(b.ToString("x2"));
            return sb.ToString();
        }
    }
}

public class Program
{
    private static readonly SessionHealthState Health = new SessionHealthState();
    private static volatile bool _shutdownRequested;
    private static readonly string _sessionId = "nx-sess-" + Guid.NewGuid().ToString("N");
    private static readonly ObjectRegistry Registry = new ObjectRegistry(_sessionId, 1, 4096);
    private static readonly TransactionManager Transactions = new TransactionManager();
    private static readonly MutationJournal Journal = new MutationJournal(4096);

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

            bool journaled = IsJournaledOperation(op);
            if (journaled)
            {
                try
                {
                    var admission = Journal.Admit(reqId, op, Encoding.UTF8.GetBytes(payloadRaw ?? ""));
                    if (admission.Disposition == MutationReplayDisposition.ReturnCommitted ||
                        admission.Disposition == MutationReplayDisposition.ReturnFailure)
                    {
                        return admission.CachedResponse;
                    }
                    if (admission.Disposition == MutationReplayDisposition.InFlight)
                    {
                        return FormatError(reqId, "REQUEST_IN_FLIGHT", "request with this request_id is already executing", 0, Health.Value.ToString().ToLowerInvariant(), true);
                    }
                    if (admission.Disposition == MutationReplayDisposition.OutcomeUnknown)
                    {
                        return FormatError(reqId, "OUTCOME_UNKNOWN", "previous execution outcome is unknown; request must not be replayed", 0, Health.Value.ToString().ToLowerInvariant(), false);
                    }
                }
                catch (Exception journalEx)
                {
                    Health.Set(SessionHealth.Dirty);
                    return FormatError(reqId, "JOURNAL_ERROR", journalEx.Message, 0, "dirty", false);
                }
            }

            try
            {
                byte[] executionResult = executor.EnqueueSync(delegate
                {
                    if (journaled) Journal.MarkStarted(reqId);
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
                        string syslogPath = "";
                        try { syslogPath = session.LogFile.FileName; } catch {}
                        var respJson = string.Format(
                            "{{\"release\":\"{0}\",\"base_dir\":\"{1}\",\"thread_id\":{2},\"work_part\":\"{3}\",\"epoch\":{4},\"session_id\":\"{5}\",\"syslog_path\":\"{6}\"}}",
                            ugiiVer,
                            baseDir != null ? baseDir.Replace('\\', '/') : "",
                            Thread.CurrentThread.ManagedThreadId,
                            activePartName,
                            Registry.Epoch,
                            Registry.SessionId,
                            syslogPath != null ? syslogPath.Replace('\\', '/') : ""
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
                        string objId = ExtractHandleObjectId(payloadRaw, "part_ref", "assembly_part_ref");
                        Part part = ResolvePartFromPayload(session, payloadRaw);
                        string partName = part.Name;
                        bool save = ExtractJsonBool(payloadRaw, "save", false);
                        if (save)
                        {
                            part.Save(BasePart.SaveComponents.True, BasePart.CloseAfterSave.False);
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

                    // Feature & Geometry operations (Phase 7)
                    if (op == "feature.create_block")
                    {
                        Part part = ResolvePartFromPayload(session, payloadRaw);
                        RequireCreateOnlyFeatureOptions(payloadRaw);
                        double[] origin = ExtractJsonDoubleArray3(payloadRaw, "origin");
                        double length = ExtractJsonDouble(payloadRaw, "length", 100.0);
                        double width = ExtractJsonDouble(payloadRaw, "width", 100.0);
                        double height = ExtractJsonDouble(payloadRaw, "height", 100.0);

                        using (var scope = new BuilderScope<NXOpen.Features.BlockFeatureBuilder>(
                            part.Features.CreateBlockFeatureBuilder(null),
                            delegate(NXOpen.Features.BlockFeatureBuilder b) { try { b.Destroy(); } catch {} }))
                        {
                            var b = scope.Builder;
                            b.Type = NXOpen.Features.BlockFeatureBuilder.Types.OriginAndEdgeLengths;
                            b.SetOriginAndLengths(
                                new Point3d(origin[0], origin[1], origin[2]),
                                length.ToString("G", System.Globalization.CultureInfo.InvariantCulture),
                                width.ToString("G", System.Globalization.CultureInfo.InvariantCulture),
                                height.ToString("G", System.Globalization.CultureInfo.InvariantCulture)
                            );
                            b.BooleanOption.Type = NXOpen.GeometricUtilities.BooleanOperation.BooleanType.Create;

                            NXOpen.Features.BodyFeature feat = scope.CommitOnce(delegate(NXOpen.Features.BlockFeatureBuilder builder)
                            {
                                return (NXOpen.Features.BodyFeature)builder.CommitFeature();
                            });

                            Body[] bodies = feat.GetBodies();
                            Body body = bodies != null && bodies.Length > 0 ? bodies[0] : null;

                            uint featNativeTag, bodyNativeTag = 0;
                            string featObjId = Registry.Register(feat, "Feature", "", out featNativeTag);
                            string featHandleJson = Registry.FormatHandleJson(featObjId, "Feature", featNativeTag, "");

                            string bodyHandleJson = "{}";
                            if (body != null)
                            {
                                string bodyObjId = Registry.Register(body, "Body", "", out bodyNativeTag);
                                bodyHandleJson = Registry.FormatHandleJson(bodyObjId, "Body", bodyNativeTag, "");
                            }

                            var respJson = string.Format(
                                "{{\"feature_ref\":{0},\"body_ref\":{1},\"feature_name\":\"{2}\",\"feature_type\":\"{3}\"}}",
                                featHandleJson,
                                bodyHandleJson,
                                feat.GetFeatureName(),
                                feat.FeatureType
                            );
                            return FormatResponse(reqId, respJson);
                        }
                    }

                    if (op == "feature.create_cylinder")
                    {
                        Part part = ResolvePartFromPayload(session, payloadRaw);
                        RequireCreateOnlyFeatureOptions(payloadRaw);
                        double[] origin = ExtractJsonDoubleArray3(payloadRaw, "origin");
                        double[] dir = ExtractJsonDoubleArray3(payloadRaw, "direction");
                        if (dir[0] == 0 && dir[1] == 0 && dir[2] == 0) { dir[2] = 1.0; }
                        double diameter = ExtractJsonDouble(payloadRaw, "diameter", 50.0);
                        double height = ExtractJsonDouble(payloadRaw, "height", 100.0);

                        using (var scope = new BuilderScope<NXOpen.Features.CylinderBuilder>(
                            part.Features.CreateCylinderBuilder(null),
                            delegate(NXOpen.Features.CylinderBuilder b) { try { b.Destroy(); } catch {} }))
                        {
                            var b = scope.Builder;
                            b.Type = NXOpen.Features.CylinderBuilder.Types.AxisDiameterAndHeight;
                            b.Diameter.RightHandSide = diameter.ToString("G", System.Globalization.CultureInfo.InvariantCulture);
                            b.Height.RightHandSide = height.ToString("G", System.Globalization.CultureInfo.InvariantCulture);
                            b.Origin = new Point3d(origin[0], origin[1], origin[2]);
                            b.Direction = new Vector3d(dir[0], dir[1], dir[2]);
                            b.BooleanOption.Type = NXOpen.GeometricUtilities.BooleanOperation.BooleanType.Create;

                            NXOpen.Features.BodyFeature feat = scope.CommitOnce(delegate(NXOpen.Features.CylinderBuilder builder)
                            {
                                return (NXOpen.Features.BodyFeature)builder.CommitFeature();
                            });

                            Body[] bodies = feat.GetBodies();
                            Body body = bodies != null && bodies.Length > 0 ? bodies[0] : null;

                            uint featNativeTag, bodyNativeTag = 0;
                            string featObjId = Registry.Register(feat, "Feature", "", out featNativeTag);
                            string featHandleJson = Registry.FormatHandleJson(featObjId, "Feature", featNativeTag, "");

                            string bodyHandleJson = "{}";
                            if (body != null)
                            {
                                string bodyObjId = Registry.Register(body, "Body", "", out bodyNativeTag);
                                bodyHandleJson = Registry.FormatHandleJson(bodyObjId, "Body", bodyNativeTag, "");
                            }

                            var respJson = string.Format(
                                "{{\"feature_ref\":{0},\"body_ref\":{1},\"feature_name\":\"{2}\"}}",
                                featHandleJson,
                                bodyHandleJson,
                                feat.GetFeatureName()
                            );
                            return FormatResponse(reqId, respJson);
                        }
                    }

                    if (op == "geometry.query_mass_properties")
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

                    if (op == "geometry.query_bounding_box")
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

                    if (op == "part.query_bodies")
                    {
                        Part part = ResolvePartFromPayload(session, payloadRaw);
                        var bodyList = new List<string>();
                        foreach (Body b in part.Bodies)
                        {
                            uint bTag = 0;
                            string bId = Registry.Register(b, "Body", "", out bTag);
                            string bHandleJson = Registry.FormatHandleJson(bId, "Body", bTag, "");
                            int fCount = 0;
                            foreach (Face f in b.GetFaces()) fCount++;
                            int eCount = 0;
                            foreach (Edge e in b.GetEdges()) eCount++;

                            bodyList.Add(string.Format(
                                "{{\"body_ref\":{0},\"name\":\"{1}\",\"solid_type\":\"{2}\",\"face_count\":{3},\"edge_count\":{4},\"native_tag\":{5}}}",
                                bHandleJson,
                                b.Name,
                                b.IsSolidBody ? "solid" : "sheet",
                                fCount,
                                eCount,
                                bTag
                            ));
                        }

                        var respJson = string.Format("{{\"bodies\":[{0}]}}", string.Join(",", bodyList.ToArray()));
                        return FormatResponse(reqId, respJson);
                    }

                    // Assembly operations (Phase 7)
                    if (op == "assembly.add_component")
                    {
                        Part part = ResolvePartFromPayload(session, payloadRaw);
                        string partPath = ExtractJsonString(payloadRaw, "part_path");
                        if (string.IsNullOrEmpty(partPath))
                        {
                            throw new ArgumentException("missing part_path for assembly.add_component");
                        }
                        string compName = ExtractJsonString(payloadRaw, "component_name");
                        if (string.IsNullOrEmpty(compName)) compName = "comp_" + Guid.NewGuid().ToString("N").Substring(0, 6);
                        double[] origin = ExtractJsonDoubleArray3(payloadRaw, "origin");
                        double[] orient = ExtractJsonDoubleArray9(payloadRaw, "orientation");
                        int layer = (int)ExtractJsonUlong(payloadRaw, "layer", 1);

                        var matrix = new Matrix3x3
                        {
                            Xx = orient[0], Xy = orient[1], Xz = orient[2],
                            Yx = orient[3], Yy = orient[4], Yz = orient[5],
                            Zx = orient[6], Zy = orient[7], Zz = orient[8]
                        };

                        PartLoadStatus loadStatus;
                        Component comp = part.ComponentAssembly.AddComponent(
                            partPath,
                            "MODEL",
                            compName,
                            new Point3d(origin[0], origin[1], origin[2]),
                            matrix,
                            layer,
                            out loadStatus
                        );
                        if (loadStatus != null) loadStatus.Dispose();

                        uint compTag = 0;
                        string compObjId = Registry.Register(comp, "Component", "", out compTag);
                        string compHandleJson = Registry.FormatHandleJson(compObjId, "Component", compTag, "");

                        var respJson = string.Format(
                            "{{\"component_ref\":{0},\"component_name\":\"{1}\",\"part_path\":\"{2}\",\"native_tag\":{3}}}",
                            compHandleJson,
                            comp.DisplayName,
                            partPath.Replace('\\', '/'),
                            compTag
                        );
                        return FormatResponse(reqId, respJson);
                    }

                    if (op == "assembly.query_tree")
                    {
                        Part part = ResolvePartFromPayload(session, payloadRaw);
                        Component root = part.ComponentAssembly.RootComponent;
                        string treeJson = SerializeComponentNode(root);
                        var respJson = string.Format("{{\"root\":{0}}}", treeJson);
                        return FormatResponse(reqId, respJson);
                    }

                    if (op == "assembly.query_bom")
                    {
                        Part part = ResolvePartFromPayload(session, payloadRaw);
                        Component root = part.ComponentAssembly.RootComponent;
                        var items = new Dictionary<string, List<string>>();
                        if (root != null)
                        {
                            CollectBOMItems(root, items);
                        }

                        var bomList = new List<string>();
                        foreach (var kvp in items)
                        {
                            var names = string.Join(",", kvp.Value.ConvertAll(n => "\"" + n + "\"").ToArray());
                            var path = kvp.Key.Replace('\\', '/');
                            var leaf = Path.GetFileName(kvp.Key);
                            bomList.Add(string.Format(
                                "{{\"part_name\":\"{0}\",\"part_path\":\"{1}\",\"quantity\":{2},\"component_names\":[{3}]}}",
                                leaf, path, kvp.Value.Count, names
                            ));
                        }

                        var respJson = string.Format("{{\"items\":[{0}]}}", string.Join(",", bomList.ToArray()));
                        return FormatResponse(reqId, respJson);
                    }

                    if (op == "assembly.remove_component")
                    {
                        Part part = ResolvePartFromPayload(session, payloadRaw);
                        Component comp = ResolveComponentFromPayload(session, payloadRaw);
                        string objId = ExtractJsonString(payloadRaw, "object_id");
                        string compRefJson = ExtractJsonObjectOrSection(payloadRaw, "component_ref");
                        if (!string.IsNullOrEmpty(compRefJson))
                        {
                            string subObjId = ExtractJsonString(compRefJson, "object_id");
                            if (!string.IsNullOrEmpty(subObjId)) objId = subObjId;
                        }

                        part.ComponentAssembly.RemoveComponent(comp);
                        if (!string.IsNullOrEmpty(objId))
                        {
                            Registry.Release(objId);
                        }

                        return FormatResponse(reqId, "{\"removed\":true}");
                    }

                    // Drafting & Export operations (Phase 8)
                    if (op == "drafting.create_sheet")
                    {
                        Part part = ResolvePartFromPayload(session, payloadRaw);
                        string sheetName = ExtractJsonString(payloadRaw, "sheet_name");
                        if (string.IsNullOrEmpty(sheetName)) sheetName = "Sheet_1";
                        double height = ExtractJsonDouble(payloadRaw, "height", 297.0); // A3 default height mm
                        double length = ExtractJsonDouble(payloadRaw, "length", 420.0); // A3 default length mm
                        double num = ExtractJsonDouble(payloadRaw, "scale_numerator", 1.0);
                        double den = ExtractJsonDouble(payloadRaw, "scale_denominator", 1.0);
                        string units = ExtractJsonString(payloadRaw, "units");

                        var sheetUnit = units == "inch" ? NXOpen.Drawings.DrawingSheet.Unit.Inches : NXOpen.Drawings.DrawingSheet.Unit.Millimeters;

                        NXOpen.Drawings.DrawingSheet sheet = part.DrawingSheets.InsertSheet(
                            sheetName,
                            sheetUnit,
                            height,
                            length,
                            num,
                            den,
                            NXOpen.Drawings.DrawingSheet.ProjectionAngleType.FirstAngle
                        );

                        uint tag = 0;
                        string objId = Registry.Register(sheet, "DrawingSheet", "", out tag);
                        string handleJson = Registry.FormatHandleJson(objId, "DrawingSheet", tag, "");

                        var respJson = string.Format(
                            System.Globalization.CultureInfo.InvariantCulture,
                            "{{\"sheet_ref\":{0},\"sheet_name\":\"{1}\",\"height\":{2:F2},\"length\":{3:F2},\"native_tag\":{4}}}",
                            handleJson,
                            sheet.Name,
                            height,
                            length,
                            tag
                        );
                        return FormatResponse(reqId, respJson);
                    }

                    if (op == "drafting.export_pdf")
                    {
                        Part part = ResolvePartFromPayload(session, payloadRaw);
                        string pdfPath = ExtractJsonString(payloadRaw, "output_pdf_path");
                        if (string.IsNullOrEmpty(pdfPath))
                        {
                            throw new ArgumentException("missing output_pdf_path");
                        }
                        string colorMode = ExtractJsonString(payloadRaw, "color_mode");

                        using (var scope = new BuilderScope<NXOpen.PrintPDFBuilder>(
                            part.PlotManager.CreatePrintPdfbuilder(),
                            delegate(NXOpen.PrintPDFBuilder b) { try { b.Destroy(); } catch {} }))
                        {
                            var b = scope.Builder;
                            b.Action = NXOpen.PrintPDFBuilder.ActionOption.Native;
                            b.Filename = pdfPath;
                            b.Colors = NXOpen.PrintPDFBuilder.Color.BlackOnWhite;

                            var sheetList = new List<NXOpen.NXObject>();
                            foreach (NXOpen.Drawings.DrawingSheet s in part.DrawingSheets)
                            {
                                sheetList.Add(s);
                            }
                            if (sheetList.Count > 0)
                            {
                                b.SourceBuilder.SetSheets(sheetList.ToArray());
                            }

                            scope.CommitOnce(delegate(NXOpen.PrintPDFBuilder builder)
                            {
                                return builder.Commit();
                            });
                        }

                        long fileSizeBytes = 0;
                        try
                        {
                            if (File.Exists(pdfPath))
                            {
                                fileSizeBytes = new FileInfo(pdfPath).Length;
                            }
                        }
                        catch {}

                        var respJson = string.Format(
                            "{{\"exported_path\":\"{0}\",\"file_size_bytes\":{1}}}",
                            pdfPath.Replace('\\', '/'),
                            fileSizeBytes
                        );
                        return FormatResponse(reqId, respJson);
                    }

                    if (op == "drafting.query_sheets")
                    {
                        Part part = ResolvePartFromPayload(session, payloadRaw);
                        var list = new List<string>();
                        foreach (NXOpen.Drawings.DrawingSheet s in part.DrawingSheets)
                        {
                            uint tag = 0;
                            string objId = Registry.Register(s, "DrawingSheet", "", out tag);
                            string handleJson = Registry.FormatHandleJson(objId, "DrawingSheet", tag, "");
                            double sNum = 1.0, sDen = 1.0;
                            try { s.GetScale(out sNum, out sDen); } catch {}

                            list.Add(string.Format(
                                System.Globalization.CultureInfo.InvariantCulture,
                                "{{\"sheet_ref\":{0},\"name\":\"{1}\",\"height\":{2:F2},\"length\":{3:F2},\"numerator\":{4:F2},\"denominator\":{5:F2},\"native_tag\":{6}}}",
                                handleJson,
                                s.Name,
                                s.Height,
                                s.Length,
                                sNum,
                                sDen,
                                tag
                            ));
                        }

                        var respJson = string.Format("{{\"sheets\":[{0}]}}", string.Join(",", list.ToArray()));
                        return FormatResponse(reqId, respJson);
                    }

                    // Unknown op
                    return FormatError(reqId, "INVALID_ARGUMENT", "unsupported operation: " + op, 0, Health.Value.ToString().ToLowerInvariant(), true);
                }, 60000);
                if (journaled) Journal.MarkCommitted(reqId, executionResult);
                return executionResult;
            }
            catch (TimeoutException timeoutEx)
            {
                var timeoutResponse = FormatError(reqId, "CANCELLED_BEFORE_START", timeoutEx.Message, 0, Health.Value.ToString().ToLowerInvariant(), true);
                if (journaled && Journal.GetState(reqId) == MutationJournalState.Received)
                {
                    Journal.MarkFailedBeforeStart(reqId, timeoutResponse, timeoutEx.Message);
                }
                return timeoutResponse;
            }
            catch (OutcomeUnknownException outcomeEx)
            {
                if (journaled && Journal.GetState(reqId) == MutationJournalState.Started)
                {
                    Journal.MarkOutcomeUnknown(reqId, outcomeEx.Message);
                }
                Health.Set(SessionHealth.Lost);
                session.LogFile.WriteLine("[NXGO][OUTCOME_UNKNOWN] op=" + op + " msg=" + outcomeEx.Message);
                return FormatError(reqId, "OUTCOME_UNKNOWN", outcomeEx.Message, 0, "lost", false);
            }
            catch (NXException nxEx)
            {
                if (journaled && Journal.GetState(reqId) == MutationJournalState.Started)
                {
                    Journal.MarkOutcomeUnknown(reqId, nxEx.Message);
                    Health.Set(SessionHealth.Dirty);
                }
                session.LogFile.WriteLine("[NXGO][NXException] op=" + op + " code=" + nxEx.ErrorCode + " msg=" + nxEx.Message);
                return FormatError(reqId, "NX_EXCEPTION", nxEx.Message, nxEx.ErrorCode, Health.Value.ToString().ToLowerInvariant(), true);
            }
            catch (Exception ex)
            {
                if (journaled && Journal.GetState(reqId) == MutationJournalState.Started)
                {
                    Journal.MarkOutcomeUnknown(reqId, ex.Message);
                    Health.Set(SessionHealth.Dirty);
                }
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

    private static T ResolveRegisteredHandle<T>(string handleJson, string expectedKind) where T : TaggedObject
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
        if (!HasJsonKey(handleJson, "generation")) throw new InvalidOperationException(expectedKind + " reference is missing generation");
        if (string.IsNullOrEmpty(kind)) throw new InvalidOperationException(expectedKind + " reference is missing kind");
        if (!string.Equals(kind, expectedKind, StringComparison.OrdinalIgnoreCase))
        {
            throw new InvalidOperationException(string.Format("wrong object kind: got {0}, expected {1}", kind, expectedKind));
        }

        ulong epoch = ExtractJsonUlong(handleJson, "epoch", 0);
        ulong generationValue = ExtractJsonUlong(handleJson, "generation", 0);
        if (generationValue == 0 || generationValue > uint.MaxValue) throw new InvalidOperationException(expectedKind + " reference has invalid generation");
        return Registry.Resolve<T>(objectId, epoch, sessionId, (uint)generationValue);
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

    private static bool IsJournaledOperation(string op)
    {
        return op != "nx.ping" &&
               op != "session.info" &&
               op != "part.query_summary" &&
               op != "part.query_bodies" &&
               op != "geometry.query_mass_properties" &&
               op != "geometry.query_bounding_box" &&
               op != "assembly.query_tree" &&
               op != "assembly.query_bom" &&
               op != "drafting.query_sheets";
    }

    private static bool HasJsonKey(string json, string key)
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

    private static double ExtractJsonDouble(string json, string key, double defaultVal)
    {
        if (string.IsNullOrEmpty(json)) return defaultVal;
        var search = "\"" + key + "\":";
        var idx = json.IndexOf(search, StringComparison.Ordinal);
        if (idx < 0) return defaultVal;
        var start = idx + search.Length;
        while (start < json.Length && char.IsWhiteSpace(json[start])) start++;
        var end = start;
        while (end < json.Length && (char.IsDigit(json[end]) || json[end] == '.' || json[end] == '-' || json[end] == 'e' || json[end] == 'E' || json[end] == '+')) end++;
        if (end <= start) return defaultVal;
        double val;
        return double.TryParse(json.Substring(start, end - start), System.Globalization.NumberStyles.Float, System.Globalization.CultureInfo.InvariantCulture, out val) ? val : defaultVal;
    }

    private static double[] ExtractJsonDoubleArray3(string json, string key)
    {
        var res = new double[] { 0, 0, 0 };
        if (string.IsNullOrEmpty(json)) return res;
        var search = "\"" + key + "\":";
        var idx = json.IndexOf(search, StringComparison.Ordinal);
        if (idx < 0) return res;
        var start = json.IndexOf("[", idx, StringComparison.Ordinal);
        if (start < 0) return res;
        var end = json.IndexOf("]", start, StringComparison.Ordinal);
        if (end < 0) return res;
        var inner = json.Substring(start + 1, end - start - 1);
        var parts = inner.Split(',');
        for (int i = 0; i < parts.Length && i < 3; i++)
        {
            double val;
            if (double.TryParse(parts[i].Trim(), System.Globalization.NumberStyles.Float, System.Globalization.CultureInfo.InvariantCulture, out val))
            {
                res[i] = val;
            }
        }
        return res;
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

    private static Component ResolveComponentFromPayload(Session session, string payloadJson)
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

    private static string SerializeComponentNode(Component comp)
    {
        if (comp == null) return "{}";
        uint tag = 0;
        string objId = Registry.Register(comp, "Component", "", out tag);
        string handleJson = Registry.FormatHandleJson(objId, "Component", tag, "");

        Point3d pos;
        Matrix3x3 orient;
        try { comp.GetPosition(out pos, out orient); } catch { pos = new Point3d(0, 0, 0); }

        string protoPath = "";
        try
        {
            if (comp.Prototype != null && comp.Prototype.OwningPart != null)
            {
                protoPath = comp.Prototype.OwningPart.FullPath.Replace('\\', '/');
            }
        }
        catch {}

        var childList = new List<string>();
        try
        {
            foreach (Component child in comp.GetChildren())
            {
                childList.Add(SerializeComponentNode(child));
            }
        }
        catch {}

        return string.Format(
            System.Globalization.CultureInfo.InvariantCulture,
            "{{\"component_ref\":{0},\"name\":\"{1}\",\"display_name\":\"{2}\",\"prototype_path\":\"{3}\",\"position\":[{4:F6},{5:F6},{6:F6}],\"children\":[{7}]}}",
            handleJson,
            comp.Name ?? "",
            comp.DisplayName ?? "",
            protoPath,
            pos.X, pos.Y, pos.Z,
            string.Join(",", childList.ToArray())
        );
    }

    private static void CollectBOMItems(Component comp, Dictionary<string, List<string>> items)
    {
        if (comp == null) return;
        Component[] children = null;
        try { children = comp.GetChildren(); } catch {}

        if (children != null && children.Length > 0)
        {
            foreach (var c in children)
            {
                CollectBOMItems(c, items);
            }
        }
        else
        {
            string path = "";
            try
            {
                if (comp.Prototype != null && comp.Prototype.OwningPart != null)
                {
                    path = comp.Prototype.OwningPart.FullPath;
                }
            }
            catch {}
            if (string.IsNullOrEmpty(path)) path = comp.DisplayName;

            List<string> list;
            if (!items.TryGetValue(path, out list))
            {
                list = new List<string>();
                items[path] = list;
            }
            list.Add(comp.DisplayName);
        }
    }

    private static double[] ExtractJsonDoubleArray9(string json, string key)
    {
        var res = new double[] { 1, 0, 0, 0, 1, 0, 0, 0, 1 };
        if (string.IsNullOrEmpty(json)) return res;
        var search = "\"" + key + "\":";
        var idx = json.IndexOf(search, StringComparison.Ordinal);
        if (idx < 0) return res;
        var start = json.IndexOf("[", idx, StringComparison.Ordinal);
        if (start < 0) return res;
        var end = json.IndexOf("]", start, StringComparison.Ordinal);
        if (end < 0) return res;
        var inner = json.Substring(start + 1, end - start - 1);
        var parts = inner.Split(',');
        for (int i = 0; i < parts.Length && i < 9; i++)
        {
            double val;
            if (double.TryParse(parts[i].Trim(), System.Globalization.NumberStyles.Float, System.Globalization.CultureInfo.InvariantCulture, out val))
            {
                res[i] = val;
            }
        }
        return res;
    }
}
