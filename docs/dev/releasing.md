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

You need write access to the repository (see [MAINTAINERS.md](../../MAINTAINERS.md)), a
clean checkout of the commit to release, and Docker, which generating the storage fixtures uses
to run Postgres. The examples release `v0.28.0` after `v0.27.0`.

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

3. **Open the release PR.** It adds everything the release records, so the tag contains it:

   - **The release data and notes.** `bump` prints the version to release; the other
     commands write the data file and the notes, and `check` confirms the data file is what
     the tree generates:

     ```bash
     go run ./scripts/release bump        # v0.28.0
     go run ./scripts/release/refnode version --json > report.json
     go run ./scripts/release data --version v0.28.0 --report report.json \
       > docs/upgrade/releases/v0.28.0.json
     gh api repos/hyperledger-labs/fabric-smart-client/releases/generate-notes \
       -f tag_name=v0.28.0 -f previous_tag_name=v0.27.0 -f target_commitish=main \
       --jq .body > changelog.md
     go run ./scripts/release notes --data docs/upgrade/releases/v0.28.0.json \
       --changelog changelog.md > docs/upgrade/releases/v0.28.0.md
     go run ./scripts/release check --report report.json
     ```

     Read `docs/upgrade/releases/v0.28.0.md`: it links to the upgrade guide and the data
     file, lists the tested versions and ends with the changelog. What each file holds is in
     [the release data README](../upgrade/releases/README.md).
   - **The storage fixtures.** Generate them from this checkout, then add `"v0.28.0"` to
     `fixtureVersions` in `platform/fabric/services/db/driver/sql/fixtures_test.go` before
     running the fixture tests, so they open the new fixture (`TestFixturesSQLite/v0.28.0` and
     `TestPostgresFixtures/v0.28.0`):

     ```bash
     platform/fabric/services/db/driver/sql/testdata/generate.sh --local v0.28.0
     # add "v0.28.0" to fixtureVersions in fixtures_test.go
     go test -run Fixtures ./platform/fabric/services/db/driver/sql/
     ```

     `--local` builds the fixture writer against the checkout, which is the commit the release
     PR's merge tags; see
     [the fixture README](../../platform/fabric/services/db/driver/sql/testdata/README.md).

   Commit `docs/upgrade/releases/v0.28.0.json`, `docs/upgrade/releases/v0.28.0.md`, the new
   `platform/fabric/services/db/driver/sql/testdata/v0.28.0/` directory and the change to
   `fixtures_test.go`, and nothing else: `report.json` and `changelog.md` from the preview are
   scratch files. Open the PR titled `build(release): v0.28.0`: the PR-title check labels a
   `build` PR `skip-changelog`, so the release PR stays out of the next changelog (see
   [PR Labels](workflow.md#pr-labels)). Its CI runs the fixture tests on the new fixture.

   The release PR adds only data and fixtures, so the code it releases is the head of `main`
   it starts from. Run the `Mixed versions` workflow on `main` before you open it, which runs
   nodes of `v0.27.0` next to nodes of that code, and check that its specs ran rather than
   skipped:

   ```bash
   gh workflow run mixedversion.yml -R hyperledger-labs/fabric-smart-client --ref main
   ```

   `data` writes `upgrade.mode` `stop-all` for every release; marking one `rolling` is a
   change to `scripts/release` (see [the release data README](../upgrade/releases/README.md)). Once the release PR
   merges, check out the commit it merged as, which is the commit to tag:

   ```bash
   git fetch upstream
   git checkout "$(gh pr view <release PR number> -R hyperledger-labs/fabric-smart-client \
     --json mergeCommit --jq .mergeCommit.oid)"
   git log --oneline -1   # build(release): v0.28.0
   ```

   Tag that commit, not a later head of `main`: the notes and the fixtures describe it.

4. **Create the tags.** Dry-run first; it lists one tag per module and refuses a version that
   is not `vMAJOR.MINOR.PATCH` or a tag that exists:

   ```bash
   make tag-release VERSION=v0.28.0 DRY=1
   make tag-release VERSION=v0.28.0
   ```

5. **Push the tags and create the GitHub release.** Push the new tags only; `git push --tags`
   would push every local tag you have. The release takes the notes as its body. Once a tag is
   pushed it is the release; if something is wrong, cut the next version:

   ```bash
   git push upstream $(git tag --points-at HEAD | grep -E '(^|/)v0\.28\.0$')
   gh release create v0.28.0 -R hyperledger-labs/fabric-smart-client --verify-tag \
     --title v0.28.0 --notes-file docs/upgrade/releases/v0.28.0.md
   ```

6. **Announce the release** in
   [#fabric-smart-client](https://discord.com/channels/905194001349627914/945691888348967012) on
   the LFDT Discord with a link to the GitHub release.

Remove the worktree with `git worktree remove ../fsc-release`.
