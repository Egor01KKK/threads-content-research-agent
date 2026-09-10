# Security policy

`th` defaults to a local, read-only client for publicly available Threads pages.
It does not log in, acquire credentials, post, rotate proxies, bypass CAPTCHAs,
or automate account actions. If you explicitly provide the existing
`--session`/`--csrf` compatibility values, the client forwards them; use that
mode only with an account and data you control, and treat the resulting data as
private.

## Do not disclose

Please do not include any of the following in an issue or pull request:

- API tokens, session cookies, CSRF values, or `.env` files;
- private research exports, databases, or raw account data;
- screenshots or logs containing personal filesystem paths or credentials.

The repository's ignore rules reduce accidental commits, but they cannot erase
secrets that were already committed. If a credential was exposed, revoke or
rotate it first, then report the incident without pasting the replacement.

## Reporting a vulnerability

For a security issue, use GitHub's private security reporting channel when it is
enabled for the repository. If it is not enabled, contact the maintainer
privately through the repository owner's verified GitHub profile before opening
a public issue. Include a short reproduction, affected version, impact, and a
safe mitigation if known. Do not test against accounts or data you do not own.

## Scope

Reports about Threads changing anonymous HTML, search availability, rate
limits, or login walls are service-compatibility reports rather than security
vulnerabilities. They are still useful as ordinary issues when they contain no
private data.
