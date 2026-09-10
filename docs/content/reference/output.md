---
title: "Output formats"
description: "The output contract every command shares: formats, fields, and templates."
weight: 30
---

Every command renders through one formatter, so the same flags work everywhere.
Pick a format with `-o`, or let `th` choose: a table when writing to a terminal,
JSONL when piped.

## Formats

```bash
th <command> -o table   # aligned columns for reading
th <command> -o jsonl   # one JSON object per line, for piping
th <command> -o json    # a single JSON array
th <command> -o csv     # spreadsheet friendly
th <command> -o tsv     # tab-separated
th <command> -o url     # just the URL column
th <command> -o raw     # the underlying bytes, unformatted
```

| Format | Best for |
|---|---|
| `table` | Reading on a terminal |
| `jsonl` | Piping into another tool, one object at a time |
| `json` | Loading a whole result as an array |
| `csv` / `tsv` | Spreadsheets and quick column math |
| `url` | Feeding URLs into other commands |
| `raw` | The unformatted bytes (response bodies, file contents) |

## Narrowing columns

Keep only the fields you want, in the order you list them:

```bash
th profile zuck --posts --fields permalink,likes,replies
```

A field name resolves the same three ways a template does (see below): the short
column alias (`likes`), the JSON key (`like_count`), or the Go struct field name
(`LikeCount`). That also lets you pull a field that has no column alias, such as
`user_id` or `quote_count`.

`--no-header` drops the header row in `table` and `csv` output, which helps when
a downstream tool expects bare rows.

## Templating rows

For full control over each line, apply a Go text/template. You can name a field
three ways, whichever you find easier to remember:

- the Go struct field name, like `{{.LikeCount}}`
- the JSON key, like `{{.like_count}}`
- the short column alias from `--fields` and the table header, like `{{.likes}}`

```bash
th profile zuck --posts --template '{{.permalink}} {{.likes}}'
```

JSON keys keep their native types, so a numeric field still compares as a number
(use a float literal, as in `{{if gt .like_count 100.0}}...{{end}}`).

## Why auto-detection helps

Because the default adapts to the destination, the same command reads well by
hand and parses cleanly in a pipe:

```bash
th <command>            # a table, because this is a terminal
th <command> | wc -l    # JSONL, because this is a pipe
```

You only reach for `-o` when you want something other than that default.
