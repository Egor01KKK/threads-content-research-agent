# Releasing `th`

Releases use Semantic Versioning and are published only from tags named
`vMAJOR.MINOR.PATCH`. The version shown by `th version` is injected from that
tag by GoReleaser; local builds show the git description or `dev`.

## Before tagging

1. Confirm repository ownership, the module path, Apache-2.0 attribution, and
   the target GitHub repository.
2. Review `git status`, `git diff`, and the files that will enter the archive.
   Do not include databases, collected exports, credentials, local recordings,
   or generated validation output.
3. Run:

   ```sh
   make check
   make build
   ./bin/th version
   ./bin/th --help
   ```

4. Run the stranger simulation in `PUBLIC-RELEASE-TEST.md` and inspect the
   synthetic viewer fixture.
5. Update release notes and the version-specific user-facing documentation.

## Tag and verify

After the repository owner has reviewed the changes:

```sh
git tag vX.Y.Z
git push origin vX.Y.Z
```

The tag workflow builds these archives:

- macOS arm64 and amd64;
- Linux arm64 and amd64;
- Windows amd64.

It also publishes `checksums.txt` with SHA256 digests. Verify the downloaded
archive against that file before installing it. The workflow does not publish
package-manager entries, container images, or signatures.

## Rollback

If an artifact is wrong, stop linking to the release, mark it as a draft or
remove it in GitHub, fix the source, and publish the next patch version. Do not
silently replace a tagged archive without documenting the change.
