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
    '''                case "assembly.remove_component":\n                    return StartAssemblyRemoveComponent(executor, requestId, requestPayload, token);\n\n                default:\n''',
    '''                case "assembly.remove_component":\n                    return StartAssemblyRemoveComponent(executor, requestId, requestPayload, token);\n                case "drafting.create_sheet":\n                    return StartDraftingCreateSheet(executor, requestId, requestPayload, token);\n                case "drafting.query_sheets":\n                    return StartDraftingQuerySheets(executor, requestId, requestPayload, token);\n                case "drafting.export_pdf":\n                    return StartDraftingExportPdf(executor, requestId, requestPayload, token);\n\n                default:\n''',
    "drafting routing",
)

replace_once(
    '''            case "assembly.add_component":\n            case "assembly.remove_component":\n                return true;\n''',
    '''            case "assembly.add_component":\n            case "assembly.remove_component":\n            case "drafting.create_sheet":\n            case "drafting.export_pdf":\n                return true;\n''',
    "drafting journal classification",
)

replace_once(
    '''                "assembly.query_bom",\n                "assembly.remove_component",\n                "shutdown",\n''',
    '''                "assembly.query_bom",\n                "assembly.remove_component",\n                "drafting.create_sheet",\n                "drafting.query_sheets",\n                "drafting.export_pdf",\n                "shutdown",\n''',
    "drafting capabilities",
)

path.write_text(text, encoding="utf-8")
print("canonical NXHost Drafting wiring applied")
