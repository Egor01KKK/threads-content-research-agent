---
title: "th"
description: "th turns threads.com into clean, scriptable, structured data: profiles, posts, replies, and search, read as an anonymous crawler."
heroTitle: "threads, from the command line"
heroLead: "A command line for Threads. One pure-Go binary, no API key, output that pipes into the rest of your tools."
heroPrimaryURL: "/getting-started/quick-start/"
heroPrimaryText: "Get started"
---

`th` turns threads.com into clean, scriptable, structured data. It resolves a
profile to a rich record, streams its posts and replies, pulls a single post's
whole thread, and searches, all from one pure-Go binary with no login and no
browser.

```bash
th profile zuck                  # a public profile's full record
th profile zuck --posts -n 20    # its twenty most recent posts
th post <url> --replies          # a post and its reply thread
```

`th` reads anonymously, as a web crawler: it asks Threads for the same
server-rendered pages a search engine gets and parses what comes back. Private
content stays private, and `th` is explicit about the wall rather than silently
returning nothing.

## Where to go next

- New here? Read the [introduction](/getting-started/introduction/), then the
  [quick start](/getting-started/quick-start/).
- Installing? See [installation](/getting-started/installation/).
- Need every flag? The [CLI reference](/reference/cli/) is the full surface.
- Want bounded topic research or the local export viewer? Start with the
  repository's [research guide](https://github.com/Egor01KKK/threads-content-research-agent/blob/main/docs/RESEARCH-MODE.md)
  and [viewer guide](https://github.com/Egor01KKK/threads-content-research-agent/blob/main/docs/VIEWER.md).
