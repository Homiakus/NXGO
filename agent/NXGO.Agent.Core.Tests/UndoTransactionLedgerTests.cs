using System.Collections.Generic;
using NXGO.Agent.Core;
using Xunit;

namespace NXGO.Agent.Core.Tests;

public sealed class UndoTransactionLedgerTests
{
    [Fact]
    public void Add_and_take_are_atomic_and_single_consumer()
    {
        var ledger = new UndoTransactionLedger<int>(capacity: 2);
        var tx = ledger.Add(42, "create bracket", "tx-fixed");

        Assert.Equal("tx-fixed", tx.TxId);
        Assert.Equal(42, ledger.Peek(tx.TxId).Mark);
        Assert.Equal(1, ledger.Count);

        var taken = ledger.Take(tx.TxId);
        Assert.Equal(42, taken.Mark);
        Assert.Equal(0, ledger.Count);
        Assert.Throws<KeyNotFoundException>(() => ledger.Take(tx.TxId));
    }

    [Fact]
    public void Capacity_preflight_fails_before_native_mark_should_be_created()
    {
        var ledger = new UndoTransactionLedger<int>(capacity: 1);
        ledger.EnsureCanBegin();
        ledger.Add(1, "first", "tx-1");

        var ex = Assert.Throws<UndoTransactionCapacityException>(() => ledger.EnsureCanBegin());
        Assert.Equal(1, ex.Capacity);
        Assert.Equal(1, ledger.Count);
        Assert.Equal(1, ledger.HighWatermark);
    }

    [Fact]
    public void Restore_is_available_only_for_provably_unconsumed_native_operation()
    {
        var ledger = new UndoTransactionLedger<int>(capacity: 2);
        var tx = ledger.Add(7, "rollback candidate", "tx-7");
        var claimed = ledger.Take(tx.TxId);
        Assert.Equal(0, ledger.Count);

        ledger.Restore(claimed);
        Assert.Equal(1, ledger.Count);
        Assert.Equal(7, ledger.Peek(tx.TxId).Mark);
        Assert.Throws<InvalidOperationException>(() => ledger.Restore(claimed));
    }

    [Fact]
    public void Duplicate_ids_and_unknown_ids_fail_closed()
    {
        var ledger = new UndoTransactionLedger<int>();
        ledger.Add(1, "one", "tx-1");
        Assert.Throws<InvalidOperationException>(() => ledger.Add(2, "two", "tx-1"));
        Assert.Throws<KeyNotFoundException>(() => ledger.Peek("tx-missing"));
        Assert.Throws<KeyNotFoundException>(() => ledger.Take("tx-missing"));
    }

    [Fact]
    public void Clear_returns_number_of_abandoned_transactions()
    {
        var ledger = new UndoTransactionLedger<int>();
        ledger.Add(1, "one", "tx-1");
        ledger.Add(2, "two", "tx-2");

        Assert.Equal(2, ledger.Clear());
        Assert.Equal(0, ledger.Count);
        Assert.Equal(2, ledger.HighWatermark);
    }
}
