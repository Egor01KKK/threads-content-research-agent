# Research Viewer

The viewer is an optional, read-only interface for an existing JSON export. It
does not collect new Threads data, call an analysis provider, or require a
database.

## Start with the safe fixture

From the repository root:

```sh
python3 scripts/research-viewer.py --input examples/sample-export.json
```

Open <http://127.0.0.1:4173/research>. The fixture is synthetic and includes a
few missing fields so the empty/missing-data states can be inspected. Its
placeholder Threads URLs are not real evidence.

## Open a real export

Pass any readable export containing a top-level `corpus` or `posts` array:

```sh
python3 scripts/research-viewer.py \
  --input path/to/complete-readable.json
```

An optional recording package adds the proof-post, comparison, QA, and CLI demo
views:

```sh
python3 scripts/research-viewer.py \
  --input path/to/complete-readable.json \
  --prep path/to/video-prep.json
```

`make viewer` starts the same server with the synthetic fixture. Override the
input with `make viewer VIEWER_INPUT=path/to/export.json`.

## What is shown

The viewer displays original text, author, public URL, available engagement
metrics, performance/baseline fields when present, provenance rows, and stored
derived assessment fields. Missing fields are shown as unavailable rather than
filled in.

The server binds to `127.0.0.1` by default and exposes only:

- `/research` and its static assets;
- the explicitly selected dataset at `/research/data/complete-readable.json`;
- the optional prep package at `/research/data/video-prep.json`.

It does not serve arbitrary repository paths or directory listings. Use
`--host 0.0.0.0` only when you intentionally want to expose the viewer to
other machines on the network.

## Troubleshooting

- `dataset file not found`: pass a path relative to the current directory or an absolute path to an existing JSON file.
- `could not read dataset JSON`: validate the file with `python3 -m json.tool path/to/export.json`.
- `Address already in use`: stop the old viewer or choose another port with `--port 4174`.
- Empty list: confirm the export has a top-level `corpus` or `posts` array; an empty array is valid and is displayed as an empty state.
