#!/usr/bin/env python3
from __future__ import annotations

import pathlib

PATH = pathlib.Path("agent/bundle/AgentWorker.cs")
text = PATH.read_text(encoding="utf-8")
original = text


def replace_once(old: str, new: str, label: str) -> None:
    global text
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{label}: expected one source snippet, got {count}")
    text = text.replace(old, new, 1)


replace_once(
    "using System.Runtime.InteropServices;\n",
    "using System.Runtime.InteropServices;\nusing System.Security.Cryptography;\n",
    "add crypto import",
)

journal_types = r'''public enum MutationJournalState
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

'''
replace_once("public class Program\n{\n", journal_types + "public class Program\n{\n", "insert mutation journal types")

replace_once(
    '''    private static readonly TransactionManager Transactions = new TransactionManager();\n''',
    '''    private static readonly TransactionManager Transactions = new TransactionManager();\n    private static readonly MutationJournal Journal = new MutationJournal(4096);\n''',
    "add mutation journal singleton",
)

admission = r'''            bool journaled = IsJournaledOperation(op);
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

'''
replace_once(
    '''            if (op == "shutdown")\n            {\n                _shutdownRequested = true;\n                var respJson = string.Format("{{\\\"request_id\\\":\\\"{0}\\\",\\\"status\\\":\\\"OK\\\",\\\"payload\\\":{{\\\"shutdown\\\":true}}}}", reqId);\n                return Encoding.UTF8.GetBytes(respJson);\n            }\n\n            try\n''',
    '''            if (op == "shutdown")\n            {\n                _shutdownRequested = true;\n                var respJson = string.Format("{{\\\"request_id\\\":\\\"{0}\\\",\\\"status\\\":\\\"OK\\\",\\\"payload\\\":{{\\\"shutdown\\\":true}}}}", reqId);\n                return Encoding.UTF8.GetBytes(respJson);\n            }\n\n''' + admission + '''            try\n''',
    "insert journal admission",
)

replace_once(
    '''                return executor.EnqueueSync(delegate\n                {\n                    Health.RequireReusable();''',
    '''                byte[] executionResult = executor.EnqueueSync(delegate\n                {\n                    if (journaled) Journal.MarkStarted(reqId);\n                    Health.RequireReusable();''',
    "mark journal started",
)

replace_once(
    '''                    // Unknown op\n                    return FormatError(reqId, "INVALID_ARGUMENT", "unsupported operation: " + op, 0, Health.Value.ToString().ToLowerInvariant(), true);\n                }, 60000);\n            }\n            catch (OutcomeUnknownException outcomeEx)''',
    '''                    // Unknown op\n                    return FormatError(reqId, "INVALID_ARGUMENT", "unsupported operation: " + op, 0, Health.Value.ToString().ToLowerInvariant(), true);\n                }, 60000);\n                if (journaled) Journal.MarkCommitted(reqId, executionResult);\n                return executionResult;\n            }\n            catch (TimeoutException timeoutEx)\n            {\n                var timeoutResponse = FormatError(reqId, "CANCELLED_BEFORE_START", timeoutEx.Message, 0, Health.Value.ToString().ToLowerInvariant(), true);\n                if (journaled && Journal.GetState(reqId) == MutationJournalState.Received)\n                {\n                    Journal.MarkFailedBeforeStart(reqId, timeoutResponse, timeoutEx.Message);\n                }\n                return timeoutResponse;\n            }\n            catch (OutcomeUnknownException outcomeEx)''',
    "commit journal result and handle pre-start timeout",
)

replace_once(
    '''            catch (OutcomeUnknownException outcomeEx)\n            {\n                Health.Set(SessionHealth.Lost);''',
    '''            catch (OutcomeUnknownException outcomeEx)\n            {\n                if (journaled && Journal.GetState(reqId) == MutationJournalState.Started)\n                {\n                    Journal.MarkOutcomeUnknown(reqId, outcomeEx.Message);\n                }\n                Health.Set(SessionHealth.Lost);''',
    "journal explicit outcome unknown",
)

# Any exception from a journaled operation after execution has started is
# conservatively outcome-unknown. This may quarantine more often than strictly
# necessary, but it never authorizes an unsafe automatic replay.
replace_once(
    '''            catch (NXException nxEx)\n            {\n                session.LogFile.WriteLine("[NXGO][NXException] op=" + op + " code=" + nxEx.ErrorCode + " msg=" + nxEx.Message);''',
    '''            catch (NXException nxEx)\n            {\n                if (journaled && Journal.GetState(reqId) == MutationJournalState.Started)\n                {\n                    Journal.MarkOutcomeUnknown(reqId, nxEx.Message);\n                    Health.Set(SessionHealth.Dirty);\n                }\n                session.LogFile.WriteLine("[NXGO][NXException] op=" + op + " code=" + nxEx.ErrorCode + " msg=" + nxEx.Message);''',
    "journal NX exception",
)
replace_once(
    '''            catch (Exception ex)\n            {\n                session.LogFile.WriteLine("[NXGO][Exception] op=" + op + " msg=" + ex.Message);''',
    '''            catch (Exception ex)\n            {\n                if (journaled && Journal.GetState(reqId) == MutationJournalState.Started)\n                {\n                    Journal.MarkOutcomeUnknown(reqId, ex.Message);\n                    Health.Set(SessionHealth.Dirty);\n                }\n                session.LogFile.WriteLine("[NXGO][Exception] op=" + op + " msg=" + ex.Message);''',
    "journal generic exception",
)

classification = r'''    private static bool IsJournaledOperation(string op)
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

'''
replace_once(
    '''    private static bool HasJsonKey(string json, string key)\n''',
    classification + '''    private static bool HasJsonKey(string json, string key)\n''',
    "insert mutation classification",
)

if text == original:
    raise SystemExit("journal patch produced no changes")
PATH.write_text(text, encoding="utf-8")
print(f"patched production mutation journal: {len(original)} -> {len(text)} bytes")
