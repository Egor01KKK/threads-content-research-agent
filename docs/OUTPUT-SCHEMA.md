# Output schema

The CLI has two related output families.

## Direct collection records

`profile`, `post`, `feed`, `replies`, and `search` emit source-shaped records.
Common fields include:

| Field | Meaning |
| --- | --- |
| `id` | Threads post or profile identifier |
| `shortcode` | Public post shortcode, when exposed |
| `text` | Original post text |
| `username` | Source author handle |
| `permalink` | Canonical Threads URL |
| `timestamp` | Published timestamp, when exposed |
| `like_count`, `reply_count`, `repost_count`, `quote_count` | Engagement counts |
| `view_count` | View/play count only when the source exposes it |
| `is_reply`, `is_quote_post` | Source relationship flags |
| `media_type`, `media_urls` | Media metadata |
| `fetched_at` / `searched_at` | Collection timestamps |

`search` uses nullable metric fields. A JSON `null` means the public source did
not expose that metric; it does not mean zero.

## Research export

A readable research export is an object containing a `corpus` array. A corpus
record normally has:

```json
{
  "post": {
    "id": "…",
    "url": "https://www.threads.com/@author/post/…",
    "text": "Original text is preserved here",
    "author_username": "author",
    "likes": 12,
    "replies": 3
  },
  "search_queries": ["query that found the post"],
  "performance": {
    "engagement": 18,
    "baseline_engagement": 9,
    "relative_performance": 2,
    "baseline_confidence": "usable",
    "metric_coverage": {"known": 4, "total": 5}
  },
  "provenance": [],
  "assessment": {}
}
```

Exact fields can grow with the research package. Use `jq 'keys'` and the
versioned export metadata instead of assuming every optional field is present.

## Raw versus derived

Raw/source fields are the original text, public URL, author handle, timestamps,
media, and engagement values parsed from Threads. Research adds query links,
profile snapshots, baseline observations, coverage, ranks, relative performance,
relevance labels, and business-evidence labels.

Derived labels and scores are deterministic heuristics. They are useful for
sorting and review, but they are not claims made by Threads and should be
checked against the preserved original text.

## SQLite

`th db` creates a legacy `posts` table for profile data. `th research` uses
`research_*` tables for runs, queries, canonical posts, query/post links,
authors, snapshots, baselines, and classifications. SQLite files are local
artifacts and should not be committed when they contain personal or collected
data.

## Safe consumers

Consumers should:

- treat `null`, omitted, and zero as different where availability flags exist;
- preserve the source URL and original text;
- tolerate unknown future fields and missing optional fields;
- show coverage before comparing rank or performance across posts;
- avoid calling a heuristic relevance or pain label ground truth.
