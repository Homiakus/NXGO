#!/usr/bin/env python3
from pathlib import Path

path = Path("agent/NXGO.Agent.NXHost/EntryPoint.cs")
text = path.read_text(encoding="utf-8")


def replace_once(old: str, new: str, label: str) -> None:
    global text
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{label}: expected exactly one source snippet, got {count}")
    text = text.replace(old, new, 1)


replace_once(
    '''                case "geometry.query_bounding_box":\n                    return StartBoundingBox(executor, requestId, requestPayload, token);\n\n                default:\n''',
    '''                case "geometry.query_bounding_box":\n                    return StartBoundingBox(executor, requestId, requestPayload, token);\n                case "transaction.begin":\n                    return StartTransactionBegin(session, executor, requestId, requestPayload, token);\n                case "transaction.commit":\n                    return StartTransactionCommit(session, executor, requestId, requestPayload, token);\n                case "transaction.rollback":\n                    return StartTransactionRollback(session, executor, requestId, requestPayload, token);\n\n                default:\n''',
    "transaction routing",
)

replace_once(
    '''        catch (StaleObjectHandleException ex)\n        {\n            return Task.FromResult(FormatError(requestId, "INVALID_ARGUMENT", ex.Message, false));\n        }\n''',
    '''        catch (StaleObjectHandleException ex)\n        {\n            var response = FormatError(requestId, "INVALID_ARGUMENT", ex.Message, false);\n            if (IsJournaledMutation(operation))\n            {\n                TryMarkFailedBeforeStart(requestId, ex.Message, response);\n            }\n            return Task.FromResult(response);\n        }\n''',
    "stale-handle journal terminalization",
)

replace_once(
    '''        catch (Exception ex)\n        {\n            RequestJournalRecord? record;\n            if (Journal.TryGet(requestId, out record) && record != null && record.State == RequestJournalState.Started)\n            {\n                Journal.MarkOutcomeUnknown(requestId, ex.GetType().Name + ": " + ex.Message);\n                Health.MarkLost();\n                return FormatError(requestId, "OUTCOME_UNKNOWN", "NX mutation faulted after execution started; worker is quarantined: " + ex.Message, false);\n            }\n\n            var response = FormatError(requestId, "INTERNAL", ex.GetType().Name + ": " + ex.Message, false);\n            TryMarkFailedBeforeStart(requestId, ex.Message, response);\n            Health.MarkSuspect();\n            return response;\n        }\n''',
    '''        catch (Exception ex)\n        {\n            RequestJournalRecord? record;\n            if (Journal.TryGet(requestId, out record) && record != null)\n            {\n                if (record.State == RequestJournalState.Started)\n                {\n                    Journal.MarkOutcomeUnknown(requestId, ex.GetType().Name + ": " + ex.Message);\n                    Health.MarkLost();\n                    return FormatError(requestId, "OUTCOME_UNKNOWN", "NX mutation faulted after execution started; worker is quarantined: " + ex.Message, false);\n                }\n                if (record.State == RequestJournalState.Received)\n                {\n                    var category = PreStartErrorCategory(ex);\n                    var response = FormatError(requestId, category, ex.GetType().Name + ": " + ex.Message, category == "CAPACITY");\n                    TryMarkFailedBeforeStart(requestId, ex.Message, response);\n                    return response;\n                }\n            }\n\n            Health.MarkSuspect();\n            return FormatError(requestId, "INTERNAL", ex.GetType().Name + ": " + ex.Message, false);\n        }\n''',
    "pre-start mutation classification",
)

replace_once(
    '''    private static void TryMarkFailedBeforeStart(string requestId, string diagnostic, byte[] response)\n''',
    '''    private static string PreStartErrorCategory(Exception ex)\n    {\n        if (ex is ArgumentException || ex is KeyNotFoundException || ex is StaleObjectHandleException)\n        {\n            return "INVALID_ARGUMENT";\n        }\n        if (ex is UndoTransactionCapacityException ||\n            ex is HandleRegistryCapacityException ||\n            ex is HandleScopeCapacityException)\n        {\n            return "CAPACITY";\n        }\n        return "INTERNAL";\n    }\n\n    private static void TryMarkFailedBeforeStart(string requestId, string diagnostic, byte[] response)\n''',
    "pre-start error categorizer",
)

replace_once(
    '''            case "feature.create_block":\n            case "feature.create_cylinder":\n                return true;\n''',
    '''            case "feature.create_block":\n            case "feature.create_cylinder":\n            case "transaction.begin":\n            case "transaction.commit":\n            case "transaction.rollback":\n                return true;\n''',
    "transaction journal classification",
)

replace_once(
    '''                "geometry.query_mass_properties",\n                "geometry.query_bounding_box",\n                "shutdown",\n''',
    '''                "geometry.query_mass_properties",\n                "geometry.query_bounding_box",\n                "transaction.begin",\n                "transaction.commit",\n                "transaction.rollback",\n                "shutdown",\n''',
    "transaction capabilities",
)

path.write_text(text, encoding="utf-8")
print("canonical NXHost transaction wiring applied")
