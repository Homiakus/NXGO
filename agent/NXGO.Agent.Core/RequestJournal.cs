using System.Security.Cryptography;
using System.Text;

namespace NXGO.Agent.Core;

public enum RequestJournalState
{
    Received = 0,
    Started = 1,
    Committed = 2,
    RolledBack = 3,
    Failed = 4,
    OutcomeUnknown = 5,
}

public enum RequestReplayDisposition
{
    New = 0,
    InFlight = 1,
    ReturnCommittedResult = 2,
    ReturnRolledBackResult = 3,
    ReturnFailure = 4,
    OutcomeUnknown = 5,
}

public sealed class RequestIdentityConflictException : InvalidOperationException
{
    public RequestIdentityConflictException(string requestId)
        : base($"request_id {requestId} was reused with a different operation or payload")
    {
        RequestId = requestId;
    }

    public string RequestId { get; }
}

public sealed class RequestJournalCapacityException : InvalidOperationException
{
    public RequestJournalCapacityException(int capacity)
        : base($"request journal capacity {capacity} reached; recycle the worker before accepting more mutations")
    {
        Capacity = capacity;
    }

    public int Capacity { get; }
}

public sealed class RequestJournalRecord
{
    internal RequestJournalRecord(
        string requestId,
        string operation,
        string payloadHash,
        DateTime createdAtUtc,
        string? correlationId = null,
        string? transactionId = null)
    {
        RequestId = requestId;
        Operation = operation;
        PayloadHash = payloadHash;
        CreatedAtUtc = createdAtUtc;
        CorrelationId = correlationId;
        TransactionId = transactionId;
        State = RequestJournalState.Received;
    }

    public string RequestId { get; }
    public string Operation { get; }
    public string PayloadHash { get; }
    public RequestJournalState State { get; internal set; }
    public byte[]? ResultEnvelope { get; internal set; }
    public string? Failure { get; internal set; }
    public DateTime CreatedAtUtc { get; }
    public string? CorrelationId { get; }
    public string? TransactionId { get; }
    public DateTime? CompletedAtUtc { get; internal set; }

    internal RequestJournalRecord Snapshot()
    {
        return new RequestJournalRecord(RequestId, Operation, PayloadHash, CreatedAtUtc, CorrelationId, TransactionId)
        {
            State = State,
            ResultEnvelope = ResultEnvelope is null ? null : (byte[])ResultEnvelope.Clone(),
            Failure = Failure,
            CompletedAtUtc = CompletedAtUtc,
        };
    }
}

public sealed class RequestAdmission
{
    internal RequestAdmission(RequestReplayDisposition disposition, RequestJournalRecord record)
    {
        Disposition = disposition;
        Record = record;
    }

    public RequestReplayDisposition Disposition { get; }
    public RequestJournalRecord Record { get; }
    public bool IsNew => Disposition == RequestReplayDisposition.New;
}

/// <summary>
/// Bounded per-worker journal for mutation identity and final outcomes.
///
/// The journal deliberately does not evict records automatically. Losing a
/// committed request record while the same NX session remains reusable would
/// weaken exactly-once replay semantics. Capacity exhaustion is therefore a
/// fail-closed signal to recycle the worker/session.
/// </summary>
public sealed class RequestJournal
{
    public const int DefaultCapacity = 4096;

    private readonly object _sync = new object();
    private readonly Dictionary<string, RequestJournalRecord> _records =
        new Dictionary<string, RequestJournalRecord>(StringComparer.Ordinal);
    private readonly int _capacity;

    public RequestJournal(int capacity = DefaultCapacity)
    {
        if (capacity <= 0) throw new ArgumentOutOfRangeException(nameof(capacity));
        _capacity = capacity;
    }

    public int Capacity => _capacity;

    public int Count
    {
        get
        {
            lock (_sync) return _records.Count;
        }
    }

    public RequestAdmission Admit(string requestId, string operation, byte[]? payload, string? correlationId = null, string? transactionId = null)
    {
        if (string.IsNullOrWhiteSpace(requestId)) throw new ArgumentException("request id is required", nameof(requestId));
        if (string.IsNullOrWhiteSpace(operation)) throw new ArgumentException("operation is required", nameof(operation));

        var hash = ComputePayloadHash(payload);
        lock (_sync)
        {
            if (_records.TryGetValue(requestId, out var existing))
            {
                if (!StringComparer.Ordinal.Equals(existing.Operation, operation) ||
                    !StringComparer.Ordinal.Equals(existing.PayloadHash, hash))
                {
                    throw new RequestIdentityConflictException(requestId);
                }

                return new RequestAdmission(DispositionFor(existing.State), existing.Snapshot());
            }

            if (_records.Count >= _capacity)
            {
                throw new RequestJournalCapacityException(_capacity);
            }

            var record = new RequestJournalRecord(requestId, operation, hash, DateTime.UtcNow, correlationId, transactionId);
            _records.Add(requestId, record);
            return new RequestAdmission(RequestReplayDisposition.New, record.Snapshot());
        }
    }

    public RequestJournalRecord MarkStarted(string requestId)
    {
        lock (_sync)
        {
            var record = RequireRecord(requestId);
            RequireState(record, RequestJournalState.Received);
            record.State = RequestJournalState.Started;
            return record.Snapshot();
        }
    }

    public RequestJournalRecord MarkCommitted(string requestId, byte[] resultEnvelope)
    {
        if (resultEnvelope is null) throw new ArgumentNullException(nameof(resultEnvelope));
        lock (_sync)
        {
            var record = RequireRecord(requestId);
            RequireState(record, RequestJournalState.Started);
            record.ResultEnvelope = (byte[])resultEnvelope.Clone();
            record.Failure = null;
            record.State = RequestJournalState.Committed;
            record.CompletedAtUtc = DateTime.UtcNow;
            return record.Snapshot();
        }
    }

    public RequestJournalRecord MarkRolledBack(string requestId, byte[] resultEnvelope)
    {
        if (resultEnvelope is null) throw new ArgumentNullException(nameof(resultEnvelope));
        lock (_sync)
        {
            var record = RequireRecord(requestId);
            RequireState(record, RequestJournalState.Started);
            record.ResultEnvelope = (byte[])resultEnvelope.Clone();
            record.Failure = null;
            record.State = RequestJournalState.RolledBack;
            record.CompletedAtUtc = DateTime.UtcNow;
            return record.Snapshot();
        }
    }

    public RequestJournalRecord MarkFailed(string requestId, string failure, byte[]? resultEnvelope = null)
    {
        if (string.IsNullOrWhiteSpace(failure)) throw new ArgumentException("failure is required", nameof(failure));
        lock (_sync)
        {
            var record = RequireRecord(requestId);
            if (record.State != RequestJournalState.Received && record.State != RequestJournalState.Started)
            {
                throw new InvalidOperationException($"cannot fail request {requestId} from state {record.State}");
            }
            record.ResultEnvelope = resultEnvelope is null ? null : (byte[])resultEnvelope.Clone();
            record.Failure = failure;
            record.State = RequestJournalState.Failed;
            record.CompletedAtUtc = DateTime.UtcNow;
            return record.Snapshot();
        }
    }

    public RequestJournalRecord MarkOutcomeUnknown(string requestId, string diagnostic)
    {
        if (string.IsNullOrWhiteSpace(diagnostic)) throw new ArgumentException("diagnostic is required", nameof(diagnostic));
        lock (_sync)
        {
            var record = RequireRecord(requestId);
            if (record.State != RequestJournalState.Started)
            {
                throw new InvalidOperationException(
                    $"only a started request can become outcome-unknown; {requestId} is {record.State}");
            }
            record.ResultEnvelope = null;
            record.Failure = diagnostic;
            record.State = RequestJournalState.OutcomeUnknown;
            record.CompletedAtUtc = DateTime.UtcNow;
            return record.Snapshot();
        }
    }

    public bool TryGet(string requestId, out RequestJournalRecord? record)
    {
        lock (_sync)
        {
            if (_records.TryGetValue(requestId, out var existing))
            {
                record = existing.Snapshot();
                return true;
            }
            record = null;
            return false;
        }
    }

    /// <summary>Writes a deterministic, length-delimited snapshot for crash recovery.</summary>
    public void SaveSnapshot(Stream destination)
    {
        if (destination is null) throw new ArgumentNullException(nameof(destination));
        lock (_sync)
        {
            using var writer = new BinaryWriter(destination, Encoding.UTF8, leaveOpen: true);
            writer.Write(2);
            writer.Write(_records.Count);
            foreach (var record in _records.Values.OrderBy(r => r.RequestId, StringComparer.Ordinal))
            {
                writer.Write(record.RequestId);
                writer.Write(record.Operation);
                writer.Write(record.PayloadHash);
                writer.Write(record.CorrelationId ?? string.Empty);
                writer.Write(record.TransactionId ?? string.Empty);
                writer.Write((int)record.State);
                writer.Write(record.CreatedAtUtc.Ticks);
                writer.Write(record.CompletedAtUtc?.Ticks ?? 0);
                writer.Write(record.Failure ?? string.Empty);
                writer.Write(record.ResultEnvelope?.Length ?? -1);
                if (record.ResultEnvelope is not null) writer.Write(record.ResultEnvelope);
            }
            writer.Flush();
        }
    }

    public static RequestJournal LoadSnapshot(Stream source, int capacity = DefaultCapacity)
    {
        if (source is null) throw new ArgumentNullException(nameof(source));
        var journal = new RequestJournal(capacity);
        using var reader = new BinaryReader(source, Encoding.UTF8, leaveOpen: true);
        var version = reader.ReadInt32();
        if (version != 1 && version != 2) throw new InvalidDataException("unsupported request journal snapshot version");
        var count = reader.ReadInt32();
        if (count < 0 || count > capacity) throw new InvalidDataException("request journal snapshot count exceeds capacity");
        for (var i = 0; i < count; i++)
        {
            var requestId = reader.ReadString();
            var operation = reader.ReadString();
            var payloadHash = reader.ReadString();
            var correlationId = version >= 2 ? reader.ReadString() : string.Empty;
            var transactionId = version >= 2 ? reader.ReadString() : string.Empty;
            var state = (RequestJournalState)reader.ReadInt32();
            var created = new DateTime(reader.ReadInt64(), DateTimeKind.Utc);
            var completedTicks = reader.ReadInt64();
            var failure = reader.ReadString();
            var resultLength = reader.ReadInt32();
            if (resultLength < -1 || resultLength > 64 * 1024 * 1024) throw new InvalidDataException("invalid journal result length");
            var result = resultLength < 0 ? null : reader.ReadBytes(resultLength);
            if (resultLength >= 0 && result!.Length != resultLength) throw new EndOfStreamException("truncated journal result");
            if (string.IsNullOrWhiteSpace(requestId) || string.IsNullOrWhiteSpace(operation) || payloadHash.Length != 64 || !Enum.IsDefined(typeof(RequestJournalState), state))
                throw new InvalidDataException("invalid request journal record");
            if ((state == RequestJournalState.Committed || state == RequestJournalState.RolledBack) && result == null)
                throw new InvalidDataException("terminal journal record is missing result envelope");
            if ((state == RequestJournalState.Failed || state == RequestJournalState.OutcomeUnknown) && string.IsNullOrEmpty(failure))
                throw new InvalidDataException("failed journal record is missing diagnostic");
            if ((state == RequestJournalState.Received || state == RequestJournalState.Started) && result != null)
                throw new InvalidDataException("non-terminal journal record contains a result envelope");
            if (journal._records.ContainsKey(requestId)) throw new InvalidDataException("duplicate request journal id");
            var restoredState = state == RequestJournalState.Started ? RequestJournalState.OutcomeUnknown : state;
            var restoredFailure = string.IsNullOrEmpty(failure) ? null : failure;
            if (state == RequestJournalState.Started)
            {
                restoredFailure = "process recovery cannot prove outcome after execution started";
            }
            journal._records.Add(requestId, new RequestJournalRecord(requestId, operation, payloadHash, created, EmptyToNull(correlationId), EmptyToNull(transactionId))
            {
                State = restoredState,
                Failure = restoredFailure,
                ResultEnvelope = result,
                CompletedAtUtc = completedTicks == 0 ? null : new DateTime(completedTicks, DateTimeKind.Utc),
            });
        }
        if (source.CanSeek && source.Position != source.Length)
            throw new InvalidDataException("request journal snapshot contains trailing data");
        return journal;
    }

    private static string? EmptyToNull(string value) => string.IsNullOrEmpty(value) ? null : value;

    public static string ComputePayloadHash(byte[]? payload)
    {
        payload ??= Array.Empty<byte>();
        using var sha = SHA256.Create();
        var hash = sha.ComputeHash(payload);
        var builder = new StringBuilder(hash.Length * 2);
        foreach (var b in hash)
        {
            builder.Append(b.ToString("x2"));
        }
        return builder.ToString();
    }

    private RequestJournalRecord RequireRecord(string requestId)
    {
        if (!_records.TryGetValue(requestId, out var record))
        {
            throw new KeyNotFoundException($"request journal record not found: {requestId}");
        }
        return record;
    }

    private static void RequireState(RequestJournalRecord record, RequestJournalState required)
    {
        if (record.State != required)
        {
            throw new InvalidOperationException(
                $"request {record.RequestId} must be {required}, current state is {record.State}");
        }
    }

    private static RequestReplayDisposition DispositionFor(RequestJournalState state)
    {
        switch (state)
        {
            case RequestJournalState.Received:
            case RequestJournalState.Started:
                return RequestReplayDisposition.InFlight;
            case RequestJournalState.Committed:
                return RequestReplayDisposition.ReturnCommittedResult;
            case RequestJournalState.RolledBack:
                return RequestReplayDisposition.ReturnRolledBackResult;
            case RequestJournalState.Failed:
                return RequestReplayDisposition.ReturnFailure;
            case RequestJournalState.OutcomeUnknown:
                return RequestReplayDisposition.OutcomeUnknown;
            default:
                throw new ArgumentOutOfRangeException(nameof(state), state, "unknown journal state");
        }
    }
}
