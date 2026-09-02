#!/usr/bin/env python3
from pathlib import Path

path = Path("agent/NXGO.Agent.NXHost/EntryPoint.cs")
text = path.read_text(encoding="utf-8")
old = "            Registry.Release(handle);\n"
new = "            Registry.ReleaseWithDependents(handle);\n"
count = text.count(old)
if count != 1:
    raise SystemExit(f"part-close release source count={count}")
path.write_text(text.replace(old, new, 1), encoding="utf-8")
print("canonical part-close dependent invalidation applied")
