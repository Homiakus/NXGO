"""Minimal real-NX smoke journal used by scripts/nx-real-smoke.ps1.

This file intentionally exercises only stable session/log access. It must fail if
it is not actually executed inside the Siemens NX Python/NXOpen runtime.
"""
import json
import os
import time

import NXOpen


def main():
    session = NXOpen.Session.GetSession()
    if session is None:
        raise RuntimeError("NXOpen.Session.GetSession() returned None")

    marker = os.environ.get("NXGO_SMOKE_MARKER")
    if not marker:
        raise RuntimeError("NXGO_SMOKE_MARKER is not set")

    payload = {
        "kind": "nxgo-real-nx-smoke",
        "status": "pass",
        "pid": os.getpid(),
        "unix_time": time.time(),
    }
    session.LogFile.WriteLine("[NXGO][SMOKE] real NX smoke passed")
    with open(marker, "w", encoding="utf-8") as f:
        json.dump(payload, f, sort_keys=True)


if __name__ == "__main__":
    main()
