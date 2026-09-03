using System;
using System.Collections.Generic;
using System.Linq;

namespace NXGO.Agent.Core;

/// <summary>
/// NX-independent opaque-handle identity. ObjectId is a bounded registry slot;
/// Generation changes every time a released slot is reused, making stale slot
/// reuse fail closed instead of resolving to a different live object.
/// </summary>
public sealed class ObjectHandleToken
{
    public string SessionId { get; set; } = string.Empty;
    public ulong Epoch { get; set; }
    public string ObjectId { get; set; } = string.Empty;
    public uint Generation { get; set; }
    public string Kind { get; set; } = string.Empty;
    public string LeaseScopeId { get; set; } = string.Empty;
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

public sealed class HandleScopeCapacityException : InvalidOperationException
{
    public HandleScopeCapacityException(string scopeId, int capacity)
        : base($"object handle scope {scopeId} reached its capacity ({capacity}); request must be rejected or paged")
    {
        ScopeId = scopeId;
        Capacity = capacity;
    }

    public string ScopeId { get; }
    public int Capacity { get; }
}

public sealed class HandleRegistryDiagnostics
{
    public int Count { get; set; }
    public int Capacity { get; set; }
    public int HighWatermark { get; set; }
}

/// <summary>
/// Thread-safe bounded registry used by NX-specific hosts to keep native object
/// instances behind opaque session/epoch/generation handles. The registry has
/// no Siemens dependency and is therefore covered by the canonical fast gate.
///
/// Handles may optionally belong to a lease scope and/or an owning handle.
/// Scope budgets prevent one request from exhausting the worker-wide registry;
/// ownership lets a host invalidate every Feature/Body/Component handle when
/// its owning Part is closed, before stale native objects can be touched again.
/// </summary>
public sealed class HandleRegistry<T> where T : class
{
    private sealed class Entry
    {
        public int Slot { get; set; }
        public ObjectHandleToken Token { get; set; } = new ObjectHandleToken();
        public T Target { get; set; } = default!;
        public string OwnerObjectId { get; set; } = string.Empty;
    }

    private readonly string _sessionId;
    private readonly ulong _epoch;
    private readonly int _capacity;
    private readonly Dictionary<string, Entry> _entries = new Dictionary<string, Entry>(StringComparer.Ordinal);
    private readonly Dictionary<int, uint> _slotGenerations = new Dictionary<int, uint>();
    private readonly Queue<int> _freeSlots = new Queue<int>();
    private readonly object _sync = new object();
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

    public string SessionId { get { return _sessionId; } }
    public ulong Epoch { get { return _epoch; } }
    public int Capacity { get { return _capacity; } }

    public int Count
    {
        get { lock (_sync) return _entries.Count; }
    }

    public int HighWatermark
    {
        get { lock (_sync) return _highWatermark; }
    }

    public HandleRegistryDiagnostics GetDiagnostics()
    {
        lock (_sync)
        {
            return new HandleRegistryDiagnostics
            {
                Count = _entries.Count,
                Capacity = _capacity,
                HighWatermark = _highWatermark,
            };
        }
    }

    public int CountScope(string leaseScopeId)
    {
        if (string.IsNullOrWhiteSpace(leaseScopeId)) return 0;
        lock (_sync)
        {
            return _entries.Values.Count(e => string.Equals(e.Token.LeaseScopeId, leaseScopeId, StringComparison.Ordinal));
        }
    }

    public ObjectHandleToken Register(
        T target,
        string kind,
        string leaseScopeId = "",
        string ownerObjectId = "",
        int leaseScopeLimit = 0)
    {
        if (target == null) throw new ArgumentNullException(nameof(target));
        if (string.IsNullOrWhiteSpace(kind)) throw new ArgumentException("object kind is required", nameof(kind));
        if (leaseScopeLimit < 0) throw new ArgumentOutOfRangeException(nameof(leaseScopeLimit));
        if (leaseScopeLimit > 0 && string.IsNullOrWhiteSpace(leaseScopeId))
        {
            throw new ArgumentException("a non-empty lease scope is required when a scope limit is enforced", nameof(leaseScopeId));
        }

        lock (_sync)
        {
            if (_entries.Count >= _capacity)
            {
                throw new HandleRegistryCapacityException(_capacity);
            }
            if (!string.IsNullOrWhiteSpace(ownerObjectId) && !_entries.ContainsKey(ownerObjectId))
            {
                throw new StaleObjectHandleException("owner handle not found or expired: " + ownerObjectId);
            }
            if (leaseScopeLimit > 0)
            {
                var scopeCount = _entries.Values.Count(e => string.Equals(e.Token.LeaseScopeId, leaseScopeId, StringComparison.Ordinal));
                if (scopeCount >= leaseScopeLimit)
                {
                    throw new HandleScopeCapacityException(leaseScopeId, leaseScopeLimit);
                }
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

            uint previousGeneration;
            _slotGenerations.TryGetValue(slot, out previousGeneration);
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
            _entries.Add(objectId, new Entry
            {
                Slot = slot,
                Token = token,
                Target = target,
                OwnerObjectId = ownerObjectId ?? string.Empty,
            });
            if (_entries.Count > _highWatermark) _highWatermark = _entries.Count;
            return token;
        }
    }

    public T Resolve(ObjectHandleToken token, params string[] expectedKinds)
    {
        if (token == null) throw new StaleObjectHandleException("object handle is null");

        lock (_sync)
        {
            ValidateIdentityLocked(token);
            var entry = _entries[token.ObjectId];
            if (expectedKinds != null && expectedKinds.Length > 0 &&
                !expectedKinds.Any(k => string.Equals(k, entry.Token.Kind, StringComparison.OrdinalIgnoreCase)))
            {
                throw new StaleObjectHandleException($"wrong object kind for {token.ObjectId}: got {entry.Token.Kind}, expected one of [{string.Join(", ", expectedKinds)}]");
            }
            return entry.Target;
        }
    }

    /// <summary>
    /// Resolves a child only when its canonical registry owner matches the
    /// supplied live owner handle. This lets hosts validate cross-object
    /// relationships without touching native NX objects on transport threads.
    /// </summary>
    public T ResolveOwned(ObjectHandleToken token, ObjectHandleToken owner, params string[] expectedKinds)
    {
        if (token == null) throw new StaleObjectHandleException("object handle is null");
        if (owner == null) throw new StaleObjectHandleException("owner handle is null");

        lock (_sync)
        {
            ValidateIdentityLocked(owner);
            ValidateIdentityLocked(token);
            var entry = _entries[token.ObjectId];
            if (!string.Equals(entry.OwnerObjectId, owner.ObjectId, StringComparison.Ordinal))
            {
                throw new StaleObjectHandleException(
                    $"object {token.ObjectId} is not owned by {owner.ObjectId}");
            }
            if (expectedKinds != null && expectedKinds.Length > 0 &&
                !expectedKinds.Any(k => string.Equals(k, entry.Token.Kind, StringComparison.OrdinalIgnoreCase)))
            {
                throw new StaleObjectHandleException(
                    $"wrong object kind for {token.ObjectId}: got {entry.Token.Kind}, expected one of [{string.Join(", ", expectedKinds)}]");
            }
            return entry.Target;
        }
    }

    public bool Release(ObjectHandleToken token)
    {
        if (token == null) throw new StaleObjectHandleException("object handle is null");

        lock (_sync)
        {
            ValidateIdentityLocked(token);
            return RemoveEntryLocked(token.ObjectId);
        }
    }

    /// <summary>
    /// Invalidates every descendant owned directly or indirectly by owner, but
    /// leaves the owner itself live. Useful when native child objects are
    /// recreated while their owning Part remains open.
    /// </summary>
    public int ReleaseDependents(ObjectHandleToken owner)
    {
        if (owner == null) throw new StaleObjectHandleException("owner handle is null");
        lock (_sync)
        {
            ValidateIdentityLocked(owner);
            var ids = CollectDependentIdsLocked(owner.ObjectId);
            foreach (var id in ids) RemoveEntryLocked(id);
            return ids.Count;
        }
    }

    /// <summary>
    /// Atomically invalidates all descendants and the owner handle. A Part host
    /// should call this after native close succeeds so no Feature/Body handle
    /// can later resolve to an object owned by the closed Part.
    /// </summary>
    public int ReleaseWithDependents(ObjectHandleToken owner)
    {
        if (owner == null) throw new StaleObjectHandleException("owner handle is null");
        lock (_sync)
        {
            ValidateIdentityLocked(owner);
            var ids = CollectDependentIdsLocked(owner.ObjectId);
            foreach (var id in ids) RemoveEntryLocked(id);
            if (RemoveEntryLocked(owner.ObjectId)) return ids.Count + 1;
            return ids.Count;
        }
    }

    public int ReleaseScope(string leaseScopeId)
    {
        if (string.IsNullOrWhiteSpace(leaseScopeId)) return 0;

        lock (_sync)
        {
            var doomed = _entries.Values
                .Where(e => string.Equals(e.Token.LeaseScopeId, leaseScopeId, StringComparison.Ordinal))
                .Select(e => e.Token.ObjectId)
                .ToArray();
            foreach (var id in doomed) RemoveEntryLocked(id);
            return doomed.Length;
        }
    }

    private List<string> CollectDependentIdsLocked(string ownerObjectId)
    {
        var result = new List<string>();
        var frontier = new Queue<string>();
        frontier.Enqueue(ownerObjectId);
        while (frontier.Count > 0)
        {
            var ownerId = frontier.Dequeue();
            var children = _entries.Values
                .Where(e => string.Equals(e.OwnerObjectId, ownerId, StringComparison.Ordinal))
                .Select(e => e.Token.ObjectId)
                .ToArray();
            foreach (var childId in children)
            {
                result.Add(childId);
                frontier.Enqueue(childId);
            }
        }
        // Children appear before no required ordering contract, but releasing
        // deepest descendants first makes ownership debugging less surprising.
        result.Reverse();
        return result;
    }

    private bool RemoveEntryLocked(string objectId)
    {
        Entry entry;
        if (!_entries.TryGetValue(objectId, out entry)) return false;
        _entries.Remove(objectId);
        _freeSlots.Enqueue(entry.Slot);
        return true;
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
        Entry entry;
        if (!_entries.TryGetValue(token.ObjectId, out entry))
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
        if (!string.Equals(token.LeaseScopeId ?? string.Empty, entry.Token.LeaseScopeId ?? string.Empty, StringComparison.Ordinal))
        {
            throw new StaleObjectHandleException($"object lease scope mismatch for {token.ObjectId}");
        }
    }
}
