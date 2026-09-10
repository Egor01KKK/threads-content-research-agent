# Architecture

The repository is a small Go CLI with an optional static viewer.

```text
cmd/th
  -> cli/                 Cobra commands, flags, output formatting
  -> threads/              HTTP client, cache, SSR parser, GraphQL fallback
  -> pkg/thid/             offline identifier classifier
  -> research/             bounded collection, SQLite, ranking, reports
  -> scripts/research-viewer.py + viewer/
                            read-only export browser
```

## Collection layer

The `threads` package makes paced HTTPS requests to public Threads pages, parses
server-rendered JSON, and preserves typed profiles, posts, replies, metrics,
media, and permalinks. It uses a best-effort URL cache under the platform's XDG
cache location. Limited persisted-query pagination is a compatibility fallback,
not a stable API contract.

The default client is anonymous. No login flow, cookie acquisition, posting, or
automation is implemented. Existing credential flags are opt-in compatibility
inputs: when supplied by the user, they are forwarded as-is and are not needed
for the public path.

## CLI layer

`cli/` builds the `th` command tree and sends all structured records through one
formatter. `cmd/th` adds signal handling, styled help, and documented exit-code
mapping. `pkg/thid` is independent of network and storage so identifier parsing
can be tested deterministically.

## Research layer

`research/` adapts the collector behind small interfaces. Bounded mode expands a
topic deterministically, searches each query, deduplicates posts, stores query
relationships, enriches a bounded author set, samples author baselines, and
calculates transparent ranking/performance fields. Deep mode is a separate
profile-first Russian small-business workflow with hard collection ceilings and
provenance exports.

SQLite keeps source observations, run/query relationships, author snapshots,
baseline posts, and optional classifications separate. Missing source metrics
remain nullable in research records. Report rendering is local and
evidence-linked.

## Viewer boundary

The Python server is intentionally separate from Go. It accepts an existing
JSON path, validates it before serving, and whitelists the viewer/data routes.
The browser code normalizes the supported export shapes for display, escapes
source text, validates Threads links before opening them, and tolerates missing
optional fields. It never performs collection.

## Deliberate non-goals

This repository does not include a hosted service, authentication system,
scheduler, crawler evasion, proxy rotation, CAPTCHA bypass, posting automation,
or an LLM requirement. Public Threads availability remains an external,
time-varying dependency.
