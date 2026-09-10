# Threads research viewer

This is a read-only local interface for browsing an existing Threads research
export. It accepts any compatible dataset path and an optional recording package;
it does not collect new Threads data or call an analysis provider.

From the repository root:

```bash
python3 scripts/research-viewer.py --input examples/sample-export.json
```

Open [http://127.0.0.1:4173/research](http://127.0.0.1:4173/research).
