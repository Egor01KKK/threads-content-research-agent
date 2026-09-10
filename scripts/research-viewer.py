#!/usr/bin/env python3
"""Serve the optional, read-only Threads research viewer.

The viewer receives an existing JSON export. It does not collect Threads data,
call an analysis provider, or expose the rest of the repository over HTTP.
"""

from __future__ import annotations

import argparse
import json
from http import HTTPStatus
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlparse


ROOT = Path(__file__).resolve().parents[1]
VIEWER = ROOT / "viewer"
DEFAULT_INPUT = ROOT / "examples" / "sample-export.json"
DATA_ROUTE = "/research/data/complete-readable.json"
PREP_ROUTE = "/research/data/video-prep.json"
STATIC_ROUTES = {
    "/research": VIEWER / "index.html",
    "/research/index.html": VIEWER / "index.html",
    "/research/app.js": VIEWER / "app.js",
    "/research/styles.css": VIEWER / "styles.css",
}


class ResearchViewerServer(ThreadingHTTPServer):
    """HTTP server carrying the two user-selected data paths."""

    allow_reuse_address = True

    def __init__(self, address, handler, input_path: Path, prep_path: Path | None):
        super().__init__(address, handler)
        self.input_path = input_path
        self.prep_path = prep_path


class ResearchViewerHandler(SimpleHTTPRequestHandler):
    """Serve only the viewer assets and the explicitly selected JSON files."""

    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=str(VIEWER), **kwargs)

    def _route(self) -> str:
        return urlparse(self.path).path.rstrip("/") or "/"

    def _send_bytes(self, body: bytes, content_type: str) -> None:
        self.send_response(HTTPStatus.OK)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(body)

    def _is_known_route(self, route: str) -> bool:
        return route in STATIC_ROUTES or route in {DATA_ROUTE, PREP_ROUTE}

    def translate_path(self, path: str) -> str:
        route = urlparse(path).path.rstrip("/") or "/"
        if route in STATIC_ROUTES:
            return str(STATIC_ROUTES[route])
        if route == DATA_ROUTE:
            return str(self.server.input_path)
        if route == PREP_ROUTE and self.server.prep_path is not None:
            return str(self.server.prep_path)
        # Keep SimpleHTTPRequestHandler's filesystem fallback inside a path
        # that does not exist, so unknown paths cannot turn into a repository
        # directory listing or file server.
        return str(VIEWER / "__not_found__")

    def do_GET(self) -> None:  # noqa: N802 - stdlib handler hook
        route = self._route()
        if not self._is_known_route(route):
            self.send_error(HTTPStatus.NOT_FOUND, "viewer route not found")
            return
        if route == PREP_ROUTE and self.server.prep_path is None:
            self._send_bytes(b'{"proof_threads":[]}', "application/json; charset=utf-8")
            return
        super().do_GET()

    def do_HEAD(self) -> None:  # noqa: N802 - stdlib handler hook
        route = self._route()
        if not self._is_known_route(route):
            self.send_error(HTTPStatus.NOT_FOUND, "viewer route not found")
            return
        if route == PREP_ROUTE and self.server.prep_path is None:
            self._send_bytes(b'{"proof_threads":[]}', "application/json; charset=utf-8")
            return
        super().do_HEAD()

    def end_headers(self) -> None:
        self.send_header("Cache-Control", "no-store")
        super().end_headers()

    def log_message(self, format: str, *args) -> None:
        print(f"[research-viewer] {self.address_string()} - {format % args}")


def resolve_file(raw: str, label: str, parser: argparse.ArgumentParser) -> Path:
    path = Path(raw).expanduser()
    if not path.is_absolute():
        path = Path.cwd() / path
    path = path.resolve()
    if not path.is_file():
        parser.error(f"{label} file not found: {path}")
    return path


def validate_json(path: Path, label: str, parser: argparse.ArgumentParser) -> None:
    try:
        with path.open("r", encoding="utf-8") as stream:
            value = json.load(stream)
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as exc:
        parser.error(f"could not read {label} JSON {path}: {exc}")
    if label == "dataset":
        if not isinstance(value, (dict, list)):
            parser.error(f"dataset JSON must contain an object or array: {path}")
        if isinstance(value, dict) and not (
            isinstance(value.get("corpus"), list) or isinstance(value.get("posts"), list)
        ):
            parser.error(
                f"dataset JSON must contain a top-level 'corpus' or 'posts' array: {path}"
            )
    if label == "prep" and not isinstance(value, dict):
        parser.error(f"prep JSON must contain an object: {path}")


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Serve an existing Threads JSON export in the read-only viewer"
    )
    parser.add_argument(
        "--input",
        default=None,
        help="dataset JSON path (default: examples/sample-export.json)",
    )
    parser.add_argument(
        "--prep",
        default=None,
        help="optional video-prep JSON path; omitted for a generic dataset view",
    )
    parser.add_argument(
        "--host", default="127.0.0.1", help="bind address (default: 127.0.0.1)"
    )
    parser.add_argument(
        "--port", type=int, default=4173, help="bind port (default: 4173)"
    )
    args = parser.parse_args()

    for path in (VIEWER / "index.html", VIEWER / "app.js", VIEWER / "styles.css"):
        if not path.is_file():
            parser.error(f"missing viewer source file: {path}")

    input_path = resolve_file(args.input or str(DEFAULT_INPUT), "dataset", parser)
    validate_json(input_path, "dataset", parser)

    prep_path = None
    if args.prep:
        prep_path = resolve_file(args.prep, "prep", parser)
        validate_json(prep_path, "prep", parser)

    try:
        server = ResearchViewerServer(
            (args.host, args.port), ResearchViewerHandler, input_path, prep_path
        )
    except OSError as exc:
        parser.error(f"could not listen on {args.host}:{args.port}: {exc}")

    print(f"Research viewer: http://{args.host}:{args.port}/research")
    print(f"Dataset: {input_path}")
    if prep_path:
        print(f"Prep package: {prep_path}")
    print("Press Ctrl-C to stop.")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nStopping research viewer.")
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
