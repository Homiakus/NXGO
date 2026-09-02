using System;
using System.Collections.Generic;

namespace NXGO.Agent.Core;

public sealed class UndoTransaction<TMark>
{
    public string TxId { get; set; } = string.Empty;
    public string Name { get; set; } = string.Empty;
    public TMark Mark { get; set; } = default!;
    public DateTime CreatedAtUtc { get; set; }
}

public sealed class UndoTransactionCapacityException : InvalidOperationException
{
    public UndoTransactionCapacityException(int capacity)
        : base($"active undo transaction capacity reached ({capacity}); commit/rollback existing transactions or recycle the worker")
    {
        Capacity = capacity;
    }

    public int Capacity { get; }
}

/// <summary>
/// NX-independent bounded ledger for native undo marks. NXHost creates/deletes
/// the Siemens mark; this class only owns transaction identity/lifetime.
/// Atomic Take prevents two concurrent commit/rollback requests from consuming
/// the same native mark.
/// </summary>
public sealed class UndoTransactionLedger<TMark>
{
    public const int DefaultCapacity = 128;

    private readonly int _capacity;
    private readonly Dictionary<string, UndoTransaction<TMark>> _active =
        new Dictionary<string, UndoTransaction<TMark>>(StringComparer.Ordinal);
    private readonly object _sync = new object();
    private int _highWatermark;

    public UndoTransactionLedger(int capacity = DefaultCapacity)
    {
        if (capacity <= 0) throw new ArgumentOutOfRangeException(nameof(capacity));
        _capacity = capacity;
    }

    public int Capacity { get { return _capacity; } }

    public int Count
    {
        get { lock (_sync) return _active.Count; }
    }

    public int HighWatermark
    {
        get { lock (_sync) return _highWatermark; }
    }

    /// <summary>
    /// Call immediately before creating a native undo mark. NXHost invokes this
    /// on its serialized NX execution thread, so a successful preflight cannot
    /// race another begin before Add is called.
    /// </summary>
    public void EnsureCanBegin()
    {
        lock (_sync)
        {
            if (_active.Count >= _capacity)
            {
                throw new UndoTransactionCapacityException(_capacity);
            }
        }
    }

    public UndoTransaction<TMark> Add(TMark mark, string name, string txId = "")
    {
        if (string.IsNullOrWhiteSpace(name)) throw new ArgumentException("transaction name is required", nameof(name));
        if (string.IsNullOrWhiteSpace(txId)) txId = "tx-" + Guid.NewGuid().ToString("N");

        lock (_sync)
        {
            if (_active.Count >= _capacity)
            {
                throw new UndoTransactionCapacityException(_capacity);
            }
            if (_active.ContainsKey(txId))
            {
                throw new InvalidOperationException("transaction id already exists: " + txId);
            }

            var tx = new UndoTransaction<TMark>
            {
                TxId = txId,
                Name = name,
                Mark = mark,
                CreatedAtUtc = DateTime.UtcNow,
            };
            _active.Add(txId, tx);
            if (_active.Count > _highWatermark) _highWatermark = _active.Count;
            return tx;
        }
    }

    public UndoTransaction<TMark> Peek(string txId)
    {
        if (string.IsNullOrWhiteSpace(txId)) throw new ArgumentException("transaction id is required", nameof(txId));
        lock (_sync)
        {
            UndoTransaction<TMark> tx;
            if (!_active.TryGetValue(txId, out tx))
            {
                throw new KeyNotFoundException("transaction not found or already completed: " + txId);
            }
            return tx;
        }
    }

    /// <summary>
    /// Atomically consumes an active transaction. Only the caller that obtains
    /// the record may commit/rollback the corresponding native undo mark.
    /// </summary>
    public UndoTransaction<TMark> Take(string txId)
    {
        if (string.IsNullOrWhiteSpace(txId)) throw new ArgumentException("transaction id is required", nameof(txId));
        lock (_sync)
        {
            UndoTransaction<TMark> tx;
            if (!_active.TryGetValue(txId, out tx))
            {
                throw new KeyNotFoundException("transaction not found or already completed: " + txId);
            }
            _active.Remove(txId);
            return tx;
        }
    }

    public void Restore(UndoTransaction<TMark> tx)
    {
        if (tx == null) throw new ArgumentNullException(nameof(tx));
        if (string.IsNullOrWhiteSpace(tx.TxId)) throw new ArgumentException("transaction id is required", nameof(tx));
        lock (_sync)
        {
            if (_active.ContainsKey(tx.TxId))
            {
                throw new InvalidOperationException("transaction id already active: " + tx.TxId);
            }
            if (_active.Count >= _capacity)
            {
                throw new UndoTransactionCapacityException(_capacity);
            }
            _active.Add(tx.TxId, tx);
            if (_active.Count > _highWatermark) _highWatermark = _active.Count;
        }
    }

    public int Clear()
    {
        lock (_sync)
        {
            var count = _active.Count;
            _active.Clear();
            return count;
        }
    }
}
