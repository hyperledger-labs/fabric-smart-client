# Releasing

This guide is for maintainers. It lists the steps to cut a release of Fabric Smart Client
(FSC).

## How FSC is released

- Every release is cut from `main`. There are no release branches, and fixes are not
  backported to an earlier release.
- The maintainer who cuts the release picks the version, `vX.Y.Z`; `go run ./scripts/release
  bump` prints the next minor. `scripts/release` generates release data for a minor only.
- A release tags the root module and every submodule (`integration`,
  `platform/fabric/services/state/cc/query`, `platform/view/services/comm/host/libp2p`) with
  the same version: `v0.28.0`, `integration/v0.28.0`, and so on. `scripts/tag-release.sh`
  finds the modules in the commit it tags. The submodules `replace` the root module with a
  relative path, so no `go.mod` changes before tagging.
- The Go module proxy keeps the first content it sees for a version. Never move, delete or
  re-push a tag after pushing it; if a release is broken, cut the next version.

## Before you start

You need write access to the repository (see [MAINTAINERS.md](../../MAINTAINERS.md)) and a
clean checkout of the commit to release. The examples release `v0.28.0` after `v0.27.0`.

The commands assume that the git remote `upstream` points to
`https://github.com/hyperledger-labs/fabric-smart-client.git`, not to your fork: release tags
exist only there, and tags pushed anywhere else are not a release. If you work from a fork, add
the remote as [Rebasing](rebasing.md) shows.

## Cutting a release

1. **Check out the commit to release** in a clean worktree. Usually it is the head of `main`:

   ```bash
   git fetch upstream --tags
   git worktree add ../fsc-release upstream/main
   cd ../fsc-release
   ```

2. **Check the upgrade guide.** `docs/upgrade/v0.27-to-v0.28.md` describes what changes for
   operators and developers since `v0.27.0`. PRs that break something add to it as they
   merge. Read it, and fix gaps in a PR before tagging. To list the breaking commits:

   ```bash
   git log --format='%h %s' v0.27.0..HEAD | grep -E '^[0-9a-f]+ [a-z]+(\([^)]*\))?!:'
   ```

3. **Commit the release data and notes.** Generate both files as
   [Previewing a release](../upgrade/releases/README.md#previewing-a-release) shows, for
   `v0.28.0` with previous tag `v0.27.0`, read the notes (they link to the upgrade guide and
   the data file, list the tested versions and end with the changelog), and merge the two files
   to `main` in a PR titled `chore(release): v0.28.0` and labelled `skip-changelog`, so the
   release PR stays out of the next changelog. The tag then contains both files, so a later
   `check` compares them with the tag. Check out the merge commit before tagging.

4. **Create the tags.** Dry-run first; it lists one tag per module and refuses a version that
   is not `vMAJOR.MINOR.PATCH` or a tag that exists:

   ```bash
   make tag-release VERSION=v0.28.0 DRY=1
   make tag-release VERSION=v0.28.0
   ```

5. **Push the tags and create the GitHub release.** Push the new tags only; `git push --tags`
   would push every local tag you have. The release takes the notes as its body:

   ```bash
   git push upstream $(git tag --points-at HEAD | grep -E '(^|/)v0\.28\.0$')
   gh release create v0.28.0 -R hyperledger-labs/fabric-smart-client --verify-tag \
     --title v0.28.0 --notes-file docs/upgrade/releases/v0.28.0.md
   ```

6. **Add the storage fixtures.** `make tag-release` prints this step. The generator fetches
   the release from the module proxy, so it runs after the tags are public. In a branch off
   `main`:

   ```bash
   platform/fabric/services/db/driver/sql/testdata/generate.sh v0.28.0
   ```

   Add `"v0.28.0"` to `fixtureVersions` in
   `platform/fabric/services/db/driver/sql/fixtures_test.go` and open a PR. See
   [the fixture README](../../platform/fabric/services/db/driver/sql/testdata/README.md).

7. **Announce the release** in
   [#fabric-smart-client](https://discord.com/channels/905194001349627914/945691888348967012) on
   the LFDT Discord with a link to the GitHub release.

Remove the worktree with `git worktree remove ../fsc-release`.
