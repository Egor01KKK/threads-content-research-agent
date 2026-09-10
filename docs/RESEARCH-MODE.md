# Research modes

`th research` is a bounded collection-and-ranking workflow built on the public
Threads collector. It is not a general-purpose market-research truth engine.
Every report keeps source URLs, collection coverage, metric coverage, and
warnings visible.

## Bounded mode

The default mode:

```sh
mkdir -p output
./bin/th research "finding clients for freelancers" \
  --queries 5 --per-query 10 --profile-limit 10 \
  --db output/research.db --output jsonl > output/research.jsonl
```

The pipeline expands the topic with deterministic intent-diverse queries,
searches each query, deduplicates posts by ID, stores query-to-post links,
fetches a bounded author set, and samples independent recent author posts for
baseline comparisons. Baseline posts are kept separate from topic evidence.

Default bounded limits are:

- 10 search queries;
- 10 results per query;
- 50 candidate author profiles;
- 20 baseline authors;
- 30 baseline posts per baseline author;
- 3 usable baseline posts before outperformance is called reliable.

Change these explicitly with `research --help`. The defaults are conservative,
but user-supplied limits can still create a larger network and SQLite workload.

## Deep mode

Deep mode is a separate, experimental Russian small-business workflow. It uses
a fixed bank of short anchors, verifies candidate owner/operator profiles before
fetching recent feeds, records provenance, and applies hard ceilings:

```sh
mkdir -p output
./bin/th research "проблемы малого бизнеса которые можно решить автоматизацией и IT" \
  --mode deep --lang ru-RU --db output/deep.db \
  --export-dir output/deep-research --output jsonl > output/deep-report.jsonl
```

It is intentionally not a generic semantic-search mode. The topic must belong
to the supported Russian small-business pain domain. Default maximums are 1,000
candidate seed profiles, 300 verified profiles, 50 recent posts per profile,
and 15,000 total profile posts. Reply/profile delays default to two seconds.

Deep export files include a readable `complete-readable.json`, `corpus.jsonl`,
owner and rejection lists, query diagnostics, pain-signal summaries, a run
summary, and a SQLite copy. An interrupted run can be resumed with `--resume`
and the same database.

## Analysis boundary

The collection, ranking, baseline, and deterministic business-evidence layers
do not call an LLM. The existing `--analyze` flag is an opt-in experimental
provider path. If enabled, it can send selected post text to a provider chosen
by the user; credentials, costs, provider availability, and output quality are
outside the core workflow. Deep mode rejects `--analyze`.

## Evidence rules

The report distinguishes:

- raw public observations from derived fields;
- search-discovered topic posts from author baseline posts;
- known metrics from unavailable metrics;
- a completed run from a completed run with warnings;
- deterministic heuristic labels from source truth.

Collection is bounded and provider-dependent. A small or empty result is not
evidence that a topic has no conversation on Threads.
