using System;
using System.Collections.Generic;
using System.Globalization;
using System.Threading;
using System.Threading.Tasks;
using NXGO.Agent.Core;
using NXOpen;

namespace NXGO.Agent.NXHost;

public static partial class EntryPoint
{
    private static readonly UndoTransactionLedger<Session.UndoMarkId> Transactions =
        new UndoTransactionLedger<Session.UndoMarkId>(UndoTransactionLedger<Session.UndoMarkId>.DefaultCapacity);

    private static Task<byte[]> StartTransactionBegin(
        Session session,
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var requestedName = GetString(payload, "name", string.Empty);

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();

            // Capacity is a pure Core preflight. Do it before MarkStarted so a
            // full ledger is a cacheable failed-before-start request rather
            // than an ambiguous NX mutation.
            Transactions.EnsureCanBegin();
            Journal.MarkStarted(requestId);

            var name = string.IsNullOrWhiteSpace(requestedName)
                ? "NXGO_Tx_" + Guid.NewGuid().ToString("N").Substring(0, 8)
                : requestedName;
            var mark = session.SetUndoMark(Session.MarkVisibility.Visible, name);

            UndoTransaction<Session.UndoMarkId> tx;
            try
            {
                tx = Transactions.Add(mark, name);
            }
            catch
            {
                // The capacity preflight ran immediately before creating the
                // native mark on the same serialized NX thread. If insertion
                // still fails unexpectedly, best-effort remove the native mark
                // and let MapMutation quarantine the uncertain worker.
                try { session.DeleteUndoMark(mark, name); } catch { }
                throw;
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["tx_id"] = tx.TxId,
                ["mark_id"] = Convert.ToInt32(tx.Mark, CultureInfo.InvariantCulture),
            });
        }, token));
    }

    private static Task<byte[]> StartTransactionCommit(
        Session session,
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var txId = RequireTransactionId(payload);

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();

            // Take is NX-independent and atomic. It intentionally happens
            // before MarkStarted: an unknown/already-consumed TxID has not
            // touched NX and must be recorded as failed-before-start. Because
            // all queued work is drained on one NX thread, only one request can
            // successfully claim the transaction.
            var tx = Transactions.Take(txId);
            Journal.MarkStarted(requestId);

            try
            {
                session.DeleteUndoMark(tx.Mark, tx.Name);
            }
            catch (Exception ex)
            {
                // Commit means keep the model state. Deleting the undo mark is
                // cleanup; matching legacy semantics, failure is logged rather
                // than falsely rolling committed CAD state back.
                session.LogFile.WriteLine("[NXGO][WARN] commit undo-mark cleanup failed tx=" + txId + ": " + ex.Message);
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["committed"] = true,
                ["tx_id"] = txId,
            });
        }, token));
    }

    private static Task<byte[]> StartTransactionRollback(
        Session session,
        NxExecutor executor,
        string requestId,
        Dictionary<string, object> payload,
        CancellationToken token)
    {
        var txId = RequireTransactionId(payload);

        return MapMutation(requestId, executor.EnqueueTracked(() =>
        {
            Health.RequireReusable();

            // Claim before MarkStarted for the same reason as commit: missing
            // TxID is a deterministic pre-NX rejection, not outcome-unknown.
            var tx = Transactions.Take(txId);
            Journal.MarkStarted(requestId);

            // If UndoToMark throws, MapMutation classifies the started mutation
            // as OUTCOME_UNKNOWN and marks the worker lost. Reusing a session
            // after failed rollback would be less safe than recycling it.
            session.UndoToMark(tx.Mark, tx.Name);
            try
            {
                session.DeleteUndoMark(tx.Mark, tx.Name);
            }
            catch (Exception ex)
            {
                // Undo already succeeded, so mark deletion is cleanup only.
                session.LogFile.WriteLine("[NXGO][WARN] rollback undo-mark cleanup failed tx=" + txId + ": " + ex.Message);
            }

            return FormatResponse(requestId, new Dictionary<string, object>
            {
                ["rolled_back"] = true,
                ["tx_id"] = txId,
            });
        }, token));
    }

    private static string RequireTransactionId(Dictionary<string, object> payload)
    {
        var txId = GetString(payload, "tx_id", string.Empty);
        if (string.IsNullOrWhiteSpace(txId))
        {
            throw new ArgumentException("tx_id is required");
        }
        return txId;
    }
}
