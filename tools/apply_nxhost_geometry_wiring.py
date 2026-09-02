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
    "public static class EntryPoint\n{",
    "public static partial class EntryPoint\n{",
    "partial entry point",
)

replace_once(
    '''                case "part.query_summary":\n                    return StartPartSummary(executor, requestId, requestPayload, token);\n\n                default:\n''',
    '''                case "part.query_summary":\n                    return StartPartSummary(executor, requestId, requestPayload, token);\n                case "object.release":\n                    return StartObjectRelease(executor, requestId, requestPayload, token);\n                case "feature.create_block":\n                    return StartCreateBlock(executor, requestId, requestPayload, token);\n                case "feature.create_cylinder":\n                    return StartCreateCylinder(executor, requestId, requestPayload, token);\n                case "part.query_bodies":\n                    return StartQueryBodies(executor, requestId, requestPayload, token);\n                case "geometry.query_mass_properties":\n                    return StartMassProperties(executor, requestId, requestPayload, token);\n                case "geometry.query_bounding_box":\n                    return StartBoundingBox(executor, requestId, requestPayload, token);\n\n                default:\n''',
    "geometry routing",
)

replace_once(
    '''            case "part.save":\n            case "part.close":\n                return true;\n''',
    '''            case "part.save":\n            case "part.close":\n            case "object.release":\n            case "feature.create_block":\n            case "feature.create_cylinder":\n                return true;\n''',
    "geometry journal classification",
)

replace_once(
    '''                "part.close",\n                "part.query_summary",\n                "shutdown",\n''',
    '''                "part.close",\n                "part.query_summary",\n                "object.release",\n                "feature.create_block",\n                "feature.create_cylinder",\n                "part.query_bodies",\n                "geometry.query_mass_properties",\n                "geometry.query_bounding_box",\n                "shutdown",\n''',
    "geometry capabilities",
)

path.write_text(text, encoding="utf-8")
print("canonical NXHost geometry wiring applied")
