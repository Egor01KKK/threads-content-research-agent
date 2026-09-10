# Public-release sanitization

This repository is prepared as software for public distribution. Collected
research and local recording material remain user-owned working data.

## Removed from the public surface

- Internal planning snapshots under `.planning/codebase/`.
- The local research/MVP audit in `docs/THREADS_RESEARCH_AGENT_AUDIT.md`.
- The repository-local `Dockerfile`, because the public release no longer ships
  a container image.
- The Cloudflare Pages deployment helper and its deployment workflow. The docs
  workflow now builds a downloadable artifact only.
- The non-functional `--token`/`THREADS_TOKEN` compatibility surface, which was
  misleading for this public, anonymous collector.

## Excluded by default

`.gitignore` excludes local databases, JSONL/log output, validation results,
video-prep packages, analysis corpora, virtual environments, editor files, and
build products. The full research corpus is not a product fixture and is not
served by the viewer unless a user explicitly passes an input path.

## Public replacements

- `examples/sample-export.json` is a small synthetic fixture. Its URLs are
  placeholders and are not collected evidence.
- The relevance-classifier fixture under `research/testdata/` uses fictional
  authors, placeholder URLs, and synthetic text while retaining the test's
  label distribution.
- The viewer accepts `--input` and optionally `--prep`; it binds to localhost by
  default and serves only the selected JSON files and viewer assets.
- The default deep-research export location is `research-output/`, with an
  explicit `--export-dir` override.
- README, CLI, research, schema, viewer, architecture, troubleshooting,
  contribution, security, and release instructions describe supported versus
  experimental behavior.
- Release automation is tag-only and creates the documented cross-platform
  archives plus SHA256 checksums. It does not publish packages, containers, or
  signatures.

## Deliberate boundaries

The core tool remains a bounded, read-only collector for public Threads pages.
It does not add authentication bypass, private APIs, CAPTCHA bypass, proxy
rotation, posting, or aggressive collection. The existing user-supplied session
compatibility path is optional and is not required by anonymous collection or
the viewer.

The public `main` history was created as a clean orphan root and contains none
of the internal planning/audit commit. The old `research-mvp`, `master`, and
`backup/pre-public-release` refs remain local only so the previous state is
recoverable; none was pushed to the public repository. See
`PUBLIC-RELEASE-TEST.md` for the verification results and remaining warnings.
