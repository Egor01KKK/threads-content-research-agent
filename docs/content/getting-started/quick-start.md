---
title: "Quick start"
description: "Run your first th command."
weight: 30
---

Once `th` is on your `PATH`:

```bash
th profile <handle>                # a public profile's full record
th profile <handle> --posts -n 20  # its twenty most recent posts
th post <url> --replies            # a post and its reply thread
th id <anything>                   # classify any Threads handle, id, or URL
```

`th` reads Threads anonymously, as a web crawler, with no login. Ask it how it
is connecting any time:

```bash
th whoami
```

## Get clean data out

Every command renders through one formatter. On a terminal you get an aligned
table; piped, you get JSON Lines, so it flows straight into `jq`:

```bash
th profile <handle> --posts -n 50 -o jsonl | jq '.like_count'
```

Pick any format explicitly with `-o` (`table`, `json`, `jsonl`, `csv`, `tsv`,
`yaml`, `url`), narrow the columns with `--fields`, or template each row:

```bash
th profile <handle> --posts --fields permalink,like_count,reply_count -o table
th profile <handle> --posts --template '{{.Permalink}} {{.LikeCount}}'
```

## Build a dataset

Crawl a profile's posts into SQLite, then query the local file:

```bash
th db build <handle> --posts -n 200 --db output/profile.db
th db query --db output/profile.db "select username, sum(like_count) from posts group by 1"
```

When a target is private or behind a login wall, `th` exits `4` with a one-line
hint, so a script can tell that apart from a real error. See the
[CLI reference](/reference/cli/) for every command and flag.
