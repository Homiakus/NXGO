using System.Text;
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
}
