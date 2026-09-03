using System.Text;
using System.IO;
using System;
using NXGO.Agent.Core;
using Xunit;

namespace NXGO.Agent.Core.Tests;

public sealed class RequestJournalTests
{
    [Fact]
    public void Committed_request_replays_cached_result_without_new_admission()
    {
        var journal = new RequestJournal();
        var payload = Encoding.UTF8.GetBytes("{\"length\":100}");

        var first = journal.Admit("req-1", "feature.create_block", payload);
        Assert.True(first.IsNew);
        journal.MarkStarted("req-1");
        var response = Encoding.UTF8.GetBytes("{\"status\":\"OK\"}");
        journal.MarkCommitted("req-1", response);

        var replay = journal.Admit("req-1", "feature.create_block", payload);
        Assert.False(replay.IsNew);
        Assert.Equal(RequestReplayDisposition.ReturnCommittedResult, replay.Disposition);
        Assert.Equal(RequestJournalState.Committed, replay.Record.State);
        Assert.Equal(response, replay.Record.ResultEnvelope);
        Assert.Equal(1, journal.Count);
    }

    [Fact]
    public void Same_request_id_with_different_payload_is_hard_conflict()
    {
        var journal = new RequestJournal();
        journal.Admit("req-1", "feature.create_block", Encoding.UTF8.GetBytes("A"));

        var ex = Assert.Throws<RequestIdentityConflictException>(() =>
            journal.Admit("req-1", "feature.create_block", Encoding.UTF8.GetBytes("B")));
        Assert.Equal("req-1", ex.RequestId);
    }

    [Fact]
    public void Same_request_id_with_different_operation_is_hard_conflict()
    {
        var journal = new RequestJournal();
        journal.Admit("req-1", "part.save", Array.Empty<byte>());

        Assert.Throws<RequestIdentityConflictException>(() =>
            journal.Admit("req-1", "part.close", Array.Empty<byte>()));
    }

    [Fact]
    public void Duplicate_request_while_started_is_reported_in_flight()
    {
        var journal = new RequestJournal();
        var payload = Encoding.UTF8.GetBytes("payload");
        journal.Admit("req-1", "part.save", payload);
        journal.MarkStarted("req-1");

        var duplicate = journal.Admit("req-1", "part.save", payload);
        Assert.Equal(RequestReplayDisposition.InFlight, duplicate.Disposition);
        Assert.Equal(RequestJournalState.Started, duplicate.Record.State);
    }

    [Fact]
    public void Outcome_unknown_is_terminal_and_never_replayed_as_success()
    {
        var journal = new RequestJournal();
        journal.Admit("req-1", "assembly.remove_component", Array.Empty<byte>());
        journal.MarkStarted("req-1");
        journal.MarkOutcomeUnknown("req-1", "transport lost after execution started");

        var replay = journal.Admit("req-1", "assembly.remove_component", Array.Empty<byte>());
        Assert.Equal(RequestReplayDisposition.OutcomeUnknown, replay.Disposition);
        Assert.Equal(RequestJournalState.OutcomeUnknown, replay.Record.State);
        Assert.Null(replay.Record.ResultEnvelope);
    }

    [Fact]
    public void Rolled_back_request_replays_rollback_result()
    {
        var journal = new RequestJournal();
        journal.Admit("req-1", "workflow.mutate", Array.Empty<byte>());
        journal.MarkStarted("req-1");
        var response = Encoding.UTF8.GetBytes("{\"rolled_back\":true}");
        journal.MarkRolledBack("req-1", response);

        var replay = journal.Admit("req-1", "workflow.mutate", Array.Empty<byte>());
        Assert.Equal(RequestReplayDisposition.ReturnRolledBackResult, replay.Disposition);
        Assert.Equal(response, replay.Record.ResultEnvelope);
    }

    [Fact]
    public void Capacity_is_fail_closed_and_does_not_evict_evidence()
    {
        var journal = new RequestJournal(capacity: 2);
        journal.Admit("req-1", "part.save", Array.Empty<byte>());
        journal.Admit("req-2", "part.save", Array.Empty<byte>());

        var ex = Assert.Throws<RequestJournalCapacityException>(() =>
            journal.Admit("req-3", "part.save", Array.Empty<byte>()));
        Assert.Equal(2, ex.Capacity);
        Assert.Equal(2, journal.Count);
        Assert.True(journal.TryGet("req-1", out _));
        Assert.True(journal.TryGet("req-2", out _));
    }

    [Fact]
    public void Snapshots_do_not_allow_result_buffer_mutation()
    {
        var journal = new RequestJournal();
        journal.Admit("req-1", "part.save", Array.Empty<byte>());
        journal.MarkStarted("req-1");
        var result = new byte[] { 1, 2, 3 };
        journal.MarkCommitted("req-1", result);

        result[0] = 9;
        Assert.True(journal.TryGet("req-1", out var first));
        Assert.NotNull(first);
        Assert.Equal((byte)1, first!.ResultEnvelope![0]);

        first.ResultEnvelope[0] = 8;
        Assert.True(journal.TryGet("req-1", out var second));
        Assert.Equal((byte)1, second!.ResultEnvelope![0]);
    }

    [Fact]
    public void Illegal_state_transitions_are_rejected()
    {
        var journal = new RequestJournal();
        journal.Admit("req-1", "part.save", Array.Empty<byte>());

        Assert.Throws<InvalidOperationException>(() =>
            journal.MarkCommitted("req-1", Array.Empty<byte>()));

        journal.MarkStarted("req-1");
        journal.MarkCommitted("req-1", Array.Empty<byte>());

        Assert.Throws<InvalidOperationException>(() =>
            journal.MarkOutcomeUnknown("req-1", "too late"));
    }

    [Fact]
    public void Snapshot_is_deterministic_and_does_not_close_destination()
    {
        var journal = new RequestJournal();
        journal.Admit("req-2", "part.save", Encoding.UTF8.GetBytes("B"));
        journal.Admit("req-1", "part.save", Encoding.UTF8.GetBytes("A"));
        using var first = new MemoryStream();
        using var second = new MemoryStream();
        journal.SaveSnapshot(first);
        journal.SaveSnapshot(second);
        Assert.Equal(first.ToArray(), second.ToArray());
        Assert.True(first.CanWrite);
    }

    [Fact]
    public void Snapshot_round_trips_terminal_result_and_rejects_truncation()
    {
        var journal = new RequestJournal();
        journal.Admit("req-1", "part.save", Array.Empty<byte>(), "corr-1", "tx-1");
        journal.MarkStarted("req-1");
        journal.MarkCommitted("req-1", Encoding.UTF8.GetBytes("ok"));
        using var snapshot = new MemoryStream();
        journal.SaveSnapshot(snapshot);
        snapshot.Position = 0;
        var restored = RequestJournal.LoadSnapshot(snapshot);
        Assert.True(restored.TryGet("req-1", out var restoredRecord));
        Assert.Equal("corr-1", restoredRecord!.CorrelationId);
        Assert.Equal("tx-1", restoredRecord.TransactionId);
        var replay = restored.Admit("req-1", "part.save", Array.Empty<byte>());
        Assert.Equal(RequestReplayDisposition.ReturnCommittedResult, replay.Disposition);
        Assert.Equal("ok", Encoding.UTF8.GetString(replay.Record.ResultEnvelope!));
        var bytes = snapshot.ToArray();
        using var truncated = new MemoryStream(bytes[..^1]);
        Assert.ThrowsAny<Exception>(() => RequestJournal.LoadSnapshot(truncated));
    }

    [Fact]
    public void Snapshot_rejects_terminal_record_without_result()
    {
        var journal = new RequestJournal();
        journal.Admit("req-1", "part.save", Array.Empty<byte>());
        journal.MarkStarted("req-1");
        journal.MarkCommitted("req-1", Array.Empty<byte>());
        using var snapshot = new MemoryStream();
        journal.SaveSnapshot(snapshot);
        var bytes = snapshot.ToArray();
        // The result-length field is the final four bytes for an empty result.
        for (var i = bytes.Length - 4; i < bytes.Length; i++) bytes[i] = 0xff;
        Assert.Throws<InvalidDataException>(() => RequestJournal.LoadSnapshot(new MemoryStream(bytes)));
    }

    [Fact]
    public void Snapshot_rejects_trailing_data()
    {
        var journal = new RequestJournal();
        journal.Admit("req-1", "part.save", Array.Empty<byte>());
        using var snapshot = new MemoryStream();
        journal.SaveSnapshot(snapshot);
        snapshot.WriteByte(0x7f);
        Assert.Throws<InvalidDataException>(() => RequestJournal.LoadSnapshot(new MemoryStream(snapshot.ToArray())));
    }

    [Fact]
    public void Snapshot_round_trips_correlation_and_transaction_ids()
    {
        var journal = new RequestJournal();
        journal.Admit("req-1", "part.save", Array.Empty<byte>(), "corr-1", "tx-1");
        using var snapshot = new MemoryStream();
        journal.SaveSnapshot(snapshot);

        var restored = RequestJournal.LoadSnapshot(new MemoryStream(snapshot.ToArray()));
        Assert.True(restored.TryGet("req-1", out var record));
        Assert.Equal("corr-1", record!.CorrelationId);
        Assert.Equal("tx-1", record.TransactionId);
    }

    [Fact]
    public void Atomic_store_replaces_snapshot_and_loads_it()
    {
        var path = System.IO.Path.Combine(System.IO.Path.GetTempPath(), "nxgo-journal-" + Guid.NewGuid().ToString("N"), "journal.bin");
        try
        {
            var journal = new RequestJournal();
            journal.Admit("req-1", "part.save", Array.Empty<byte>());
            var store = new AtomicRequestJournalStore(path);
            store.Save(journal);
            Assert.Equal(RequestReplayDisposition.InFlight, store.Load().Admit("req-1", "part.save", Array.Empty<byte>()).Disposition);

            journal.MarkStarted("req-1");
            store.Save(journal);
            var recovered = store.Load().Admit("req-1", "part.save", Array.Empty<byte>());
            Assert.Equal(RequestReplayDisposition.OutcomeUnknown, recovered.Disposition);
            Assert.Null(recovered.Record.ResultEnvelope);
            Assert.Contains("cannot prove outcome", recovered.Record.Failure);

            journal.MarkCommitted("req-1", Encoding.UTF8.GetBytes("saved"));
            store.Save(journal);
            var restored = store.Load();
            var replay = restored.Admit("req-1", "part.save", Array.Empty<byte>());
            Assert.Equal(RequestReplayDisposition.ReturnCommittedResult, replay.Disposition);
            Assert.Equal("saved", Encoding.UTF8.GetString(replay.Record.ResultEnvelope!));

            Assert.False(File.Exists(path + ".tmp"));
        }
        finally
        {
            var directory = System.IO.Path.GetDirectoryName(path);
            if (directory != null && Directory.Exists(directory)) Directory.Delete(directory, true);
        }
    }
}
