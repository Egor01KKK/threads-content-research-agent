# Contributing

Thanks for helping improve `th`.

## Local setup

Install Go 1.26 or newer, then run:

```sh
make build
make check
```

The viewer is optional and uses Python 3.10 or newer:

```sh
make viewer
```

Do not add live Threads calls to deterministic unit tests. Use small fixtures
and keep public-source assumptions isolated in the `threads` package.

## Pull requests

- Keep changes focused and explain user-visible behavior.
- Add or update deterministic tests for parser, storage, output, or CLI changes.
- Keep collection bounded and public-data-only.
- Do not commit credentials, cookies, databases, research exports, or generated
  output.
- Run `make check` before opening a pull request.

Live smoke checks are useful during development but are not a substitute for
fixture-based tests; public Threads responses can change or be unavailable.
