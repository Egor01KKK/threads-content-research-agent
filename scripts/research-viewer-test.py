#!/usr/bin/env python3
"""Deterministic tests for the read-only viewer route boundary."""

from __future__ import annotations

import importlib.util
import json
import threading
import urllib.error
import urllib.request
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
MODULE_PATH = ROOT / "scripts" / "research-viewer.py"
SPEC = importlib.util.spec_from_file_location("research_viewer", MODULE_PATH)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError(f"could not import {MODULE_PATH}")
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def request(base: str, route: str) -> tuple[int, bytes]:
    try:
        with urllib.request.urlopen(base + route, timeout=3) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read()


def main() -> None:
    dataset = ROOT / "examples" / "sample-export.json"
    server = MODULE.ResearchViewerServer(
        ("127.0.0.1", 0), MODULE.ResearchViewerHandler, dataset, None
    )
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    base = f"http://127.0.0.1:{server.server_address[1]}"
    try:
        status, body = request(base, "/research")
        assert status == 200 and b"Threads Research" in body

        status, body = request(base, "/research/data/complete-readable.json")
        assert status == 200
        payload = json.loads(body)
        assert len(payload["corpus"]) == 3

        status, body = request(base, "/research/data/video-prep.json")
        assert status == 200 and json.loads(body) == {"proof_threads": []}

        for route in ("/", "/research/../README.md", "/.env", "/research/unknown"):
            status, _ = request(base, route)
            assert status == 404, f"{route} unexpectedly returned HTTP {status}"
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=3)

    print("research viewer route tests passed")


if __name__ == "__main__":
    main()
