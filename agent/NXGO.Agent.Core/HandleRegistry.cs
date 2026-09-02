namespace NXGO.Agent.Core;

/// <summary>
/// NX-independent opaque-handle identity. ObjectId is a bounded registry slot;
/// Generation changes every time a released slot is reused, making stale slot
/// reuse fail closed instead of resolving to a different live object.
/// </summary>
public sealed class ObjectHandleToken
{
    public string SessionId { get; init; } = string.Empty;
    public ulong Epoch { get; init; }
    public string ObjectId { get; init; } = string.Empty;
    public uint Generation { get; init; }
    public string Kind { get; init; } = string.Empty;
    public string LeaseScopeId { get; init; } = string.Empty;
}

public sealed class StaleObjectHandleException : InvalidOperationException
{
    public StaleObjectHandleException(string message) : base(message) { }
}

public sealed class HandleRegistryCapacityException : InvalidOperationException
{
    public HandleRegistryCapacityException(int capacity)
        : base($"object handle registry capacity reached ({capacity}); release handles or recycle the worker")
    {
        Capacity = capacity;
    }

    public int Capacity { get; }
}

/// <summary>
/// Thread-safe bounded registry used by NX-specific hosts to keep native object
/// instances behind opaque session/epoch/generation handles. The registry has
/// no Siemens dependency and is therefore covered by the canonical fast gate.
/// </summary>
public sealed class HandleRegistry<T> where T : class
{
    private sealed class Entry
    {
        public required int Slot { get; init; }
        public required ObjectHandleToken Token { get; init; }
        public required T Target { get; init; }
    }

    private readonly string _sessionId;
    private readonly ulong _epoch;
    private readonly int _capacity;
    private readonly Dictionary<string, Entry> _entries = new(StringComparer.Ordinal);
    private readonly Dictionary<int, uint> _slotGenerations = new();
    private readonly Queue<int> _freeSlots = new();
    private readonly object _sync = new();
    private int _nextSlot = 1;
    private int _highWatermark;

    public HandleRegistry(string sessionId, ulong epoch, int capacity = 4096)
    {
        if (string.IsNullOrWhiteSpace(sessionId)) throw new ArgumentException("session id is required", nameof(sessionId));
        if (epoch == 0) throw new ArgumentOutOfRangeException(nameof(epoch), "epoch must be non-zero");
        if (capacity <= 0) throw new ArgumentOutOfRangeException(nameof(capacity));
        _sessionId = sessionId;
        _epoch = epoch;
        _capacity = capacity;
    }

    public string SessionId => _sessionId;
    public ulong Epoch => _epoch;
    public int Capacity => _capacity;

    public int Count
    {
        get { lock (_sync) return _entries.Count; }
    }

    public int HighWatermark
    {
        get { lock (_sync) return _highWatermark; }
    }

    public ObjectHandleToken Register(T target, string kind, string leaseScopeId = "")
    {
        if (target is null) throw new ArgumentNullException(nameof(target));
        if (string.IsNullOrWhiteSpace(kind)) throw new ArgumentException("object kind is required", nameof(kind));

        lock (_sync)
        {
            if (_entries.Count >= _capacity)
            {
                throw new HandleRegistryCapacityException(_capacity);
            }

            int slot;
            if (_freeSlots.Count > 0)
            {
                slot = _freeSlots.Dequeue();
            }
            else
            {
                if (_nextSlot > _capacity)
                {
                    // This should only occur if bookkeeping was corrupted. Do
                    // not grow the namespace silently; fail closed.
                    throw new HandleRegistryCapacityException(_capacity);
                }
                slot = _nextSlot++;
            }

            _slotGenerations.TryGetValue(slot, out var previousGeneration);
            if (previousGeneration == uint.MaxValue)
            {
                throw new InvalidOperationException($"generation exhausted for object registry slot {slot}; worker must be recycled");
            }
            var generation = previousGeneration + 1;
            _slotGenerations[slot] = generation;

            var objectId = $"obj-{slot}";
            var token = new ObjectHandleToken
            {
                SessionId = _sessionId,
                Epoch = _epoch,
                ObjectId = objectId,
                Generation = generation,
                Kind = kind,
                LeaseScopeId = leaseScopeId ?? string.Empty,
            };
            _entries.Add(objectId, new Entry { Slot = slot, Token = token, Target = target });
            if (_entries.Count > _highWatermark) _highWatermark = _entries.Count;
            return token;
        }
    }

    public T Resolve(ObjectHandleToken token, params string[] expectedKinds)
    {
        if (token is null) throw new StaleObjectHandleException("object handle is null");

        lock (_sync)
        {
            ValidateIdentityLocked(token);
            var entry = _entries[token.ObjectId];
            if (expectedKinds is { Length: > 0 } && !expectedKinds.Any(k => string.Equals(k, entry.Token.Kind, StringComparison.OrdinalIgnoreCase)))
            {
                throw new StaleObjectHandleException($"wrong object kind for {token.ObjectId}: got {entry.Token.Kind}, expected one of [{string.Join(", ", expectedKinds)}]");
            }
            return entry.Target;
        }
    }

    public bool Release(ObjectHandleToken token)
    {
        if (token is null) throw new StaleObjectHandleException("object handle is null");

        lock (_sync)
        {
            ValidateIdentityLocked(token);
            var entry = _entries[token.ObjectId];
            if (!_entries.Remove(token.ObjectId)) return false;
            _freeSlots.Enqueue(entry.Slot);
            return true;
        }
    }

    public int ReleaseScope(string leaseScopeId)
    {
        if (string.IsNullOrWhiteSpace(leaseScopeId)) return 0;

        lock (_sync)
        {
            var doomed = _entries.Values
                .Where(e => string.Equals(e.Token.LeaseScopeId, leaseScopeId, StringComparison.Ordinal))
                .ToArray();
            foreach (var entry in doomed)
            {
                _entries.Remove(entry.Token.ObjectId);
                _freeSlots.Enqueue(entry.Slot);
            }
            return doomed.Length;
        }
    }

    private void ValidateIdentityLocked(ObjectHandleToken token)
    {
        if (!string.Equals(token.SessionId, _sessionId, StringComparison.Ordinal))
        {
            throw new StaleObjectHandleException($"stale session reference: got {token.SessionId}, expected {_sessionId}");
        }
        if (token.Epoch != _epoch)
        {
            throw new StaleObjectHandleException($"stale epoch reference: got {token.Epoch}, expected {_epoch}");
        }
        if (string.IsNullOrWhiteSpace(token.ObjectId))
        {
            throw new StaleObjectHandleException("object handle is missing object id");
        }
        if (token.Generation == 0)
        {
            throw new StaleObjectHandleException("object handle is missing generation");
        }
        if (!_entries.TryGetValue(token.ObjectId, out var entry))
        {
            throw new StaleObjectHandleException("object handle not found or expired: " + token.ObjectId);
        }
        if (token.Generation != entry.Token.Generation)
        {
            throw new StaleObjectHandleException($"stale object generation for {token.ObjectId}: got {token.Generation}, expected {entry.Token.Generation}");
        }
        if (!string.Equals(token.Kind, entry.Token.Kind, StringComparison.OrdinalIgnoreCase))
        {
            throw new StaleObjectHandleException($"object kind mismatch for {token.ObjectId}: got {token.Kind}, registered {entry.Token.Kind}");
        }
    }
}
