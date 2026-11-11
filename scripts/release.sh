#!/usr/bin/env bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
# release.sh prepare|pr|publish cuts a release of FSC in three steps: a minor vX.Y.0
# from main, or a patch vX.Y.Z from its release branch release-X.Y.
# docs/dev/releasing.md describes the steps, a release from a fork, and what to do
# by hand when one of them stops.
#
#   prepare  generate the release files on release/vX.Y.Z, staged
#   pr       commit them, push the branch, open or update the release PR
#   publish  check the merged release commit, tag it, push the tags, create the
#            GitHub release and, for a minor, the backport labels
#
# Settings, all optional:
#   RELEASE_REPO  the GitHub repository to release in, owner/name; by default the
#                 main repository. A fork releases on its own: its branch, PR,
#                 tags and GitHub release, and notes that link to it.
#   REMOTE        the git remote of RELEASE_REPO; upstream for the main repository,
#                 origin for a fork.
#   VERSION       the version prepare picks, vX.Y.Z; by default the next minor
#                 after the main repository's latest release. publish takes it
#                 when more than one release is untagged.
#   DRY_RUN=1     do every local step and check, and print what would be written to
#                 GitHub instead of writing it.
#   YES=1         answer every question with yes.
# Every step asks before it changes anything outside the checkout, and stops at the
# first error.
set -euo pipefail

MAIN_REPO=hyperledger-labs/fabric-smart-client
REPO=${RELEASE_REPO:-$MAIN_REPO}
if [[ "$REPO" == "$MAIN_REPO" ]]; then REMOTE=${REMOTE:-upstream}; else REMOTE=${REMOTE:-origin}; fi
# The URL the remote must have; the tests set it to a local repository.
REPO_URL=${RELEASE_REPO_URL:-github\.com[:/]${REPO}(\.git)?$}
# Where the release tags of FSC are: the previous release and the next minor
# come from there.
MAIN_URL=${RELEASE_MAIN_URL:-https://github.com/${MAIN_REPO}.git}
RELEASES=docs/upgrade/releases
MERGIFY=.mergify.yml
FIXTURES=platform/fabric/services/db/driver/sql/testdata
SEMVER='^v[0-9]+\.[0-9]+\.[0-9]+$'

die() { echo "release: $*" >&2; exit 1; }
say() { echo "==> $*"; }

# write runs a command that writes to GitHub, or with DRY_RUN=1 prints it.
write() {
    if [[ "${DRY_RUN:-}" == 1 ]]; then
        echo "dry run, would run: $*"
    else
        "$@"
    fi
}

# confirm asks a yes/no question; any answer but y or yes stops the step.
confirm() {
    [[ "${YES:-}" == 1 ]] && return 0
    local answer
    read -r -p "$1 [y/N] " answer || true
    [[ "$answer" == y || "$answer" == yes ]] || die "stopped; nothing more was done"
}

cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

# check_remote fails unless $REMOTE is the repository the release goes to. A release
# of the main repository through any other remote would put its tags on a fork.
check_remote() {
    local url
    url=$(git remote get-url "$REMOTE" 2>/dev/null) ||
        die "no remote $REMOTE; add it as docs/dev/rebasing.md shows"
    [[ "$url" =~ $REPO_URL ]] || die "remote $REMOTE is $url, not $REPO; set REMOTE or RELEASE_REPO"
    [[ "$REPO" == "$MAIN_REPO" ]] || say "releasing in the fork $REPO, not in $MAIN_REPO"
    [[ "${DRY_RUN:-}" != 1 ]] || say "dry run: nothing is written to GitHub"
}

check_clean() { [[ -z "$(git status --porcelain)" ]] || die "the working tree is not clean"; }

# base prints the branch release $1 is cut from: main for a minor, release-X.Y for a patch.
base() {
    [[ "$1" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]] || die "invalid version $1"
    if [[ "${BASH_REMATCH[3]}" == 0 ]]; then echo main; else echo "release-${BASH_REMATCH[1]}.${BASH_REMATCH[2]}"; fi
}

# fetch_main_repo fetches the release tags, main and the release branches of the main
# repository, for a release in a fork: they hold the previous release, and GitHub
# generates the changelog there.
fetch_main_repo() {
    git fetch -q --tags "$MAIN_URL" "+main:refs/release/main" "+refs/heads/release-*:refs/release/release-*"
}

# fetch fetches main and the release branches from $REMOTE, and the main repository
# too when the release goes to a fork.
fetch() {
    say "fetching $REMOTE"
    git fetch -q --tags "$REMOTE" "+refs/heads/main:refs/remotes/$REMOTE/main" \
        "+refs/heads/release-*:refs/remotes/$REMOTE/release-*"
    if [[ "$REPO" != "$MAIN_REPO" ]]; then
        say "fetching the release tags, main and the release branches of $MAIN_REPO"
        fetch_main_repo
    fi
}

# tagged tells whether release $1 is tagged in the repository the release goes to. It asks
# the remote: a local tag may never have been pushed, and a fork's local tags include the
# main repository's.
tagged() { git ls-remote --exit-code -q --tags "$REMOTE" "refs/tags/$1" >/dev/null; }

# previous_release prints the release $1 follows: vX.Y.(Z-1) for a patch, vX.(Y-1).0 for a
# minor.
previous_release() { go run ./scripts/release previous --version "$1"; }

# report writes the reference node's report to $TMP/report.json.
report() { go run ./scripts/release/refnode version --json > "$TMP/report.json"; }

# notes checks the release data against $TMP/report.json and writes to $2 the notes of
# release $1 whose release commit has parent $3, with the changelog GitHub generates now
# for the PRs merged up to $3. prepare, publish and the Release check workflow all pass
# the commit the release PR is based on.
notes() {
    local previous target=$3
    previous=$(previous_release "$1")
    go run ./scripts/release check --report "$TMP/report.json"
    # GitHub resolves the previous tag in the repository it generates the notes in, and only
    # the main repository has the release tags. A fork's notes list the PRs merged into the
    # main repository up to the commit the fork's release is based on.
    if [[ "$REPO" != "$MAIN_REPO" ]]; then
        local b
        b=$(base "$1")
        target=$(git merge-base "$3" "refs/release/$b") || die "${3:0:9} shares no commit with $MAIN_REPO $b"
    fi
    gh api "repos/$MAIN_REPO/releases/generate-notes" -f tag_name="$1" \
        -f previous_tag_name="$previous" -f target_commitish="$target" --jq .body > "$TMP/changelog.md"
    go run ./scripts/release notes --data "$RELEASES/$1.json" --changelog "$TMP/changelog.md" --repo "$REPO" > "$2"
}

# check_notes fails unless the committed notes of release $1 are what notes generates now for
# base commit $2, after checking the release data against the tree. publish, the dry run and
# the Release check workflow share it.
check_notes() {
    report
    notes "$1" "$TMP/notes.md" "$2"
    diff -u "$RELEASES/$1.md" "$TMP/notes.md" ||
        die "the notes of $1 are not what GitHub generates for ${2:0:9} now; prepare the release again"
}

prepare() {
    [[ -z "$(git status --porcelain)" ]] ||
        die "the working tree is not clean; to prepare again, remove the staged release files as docs/dev/releasing.md, Stopping or abandoning a release, shows"
    check_remote
    gh auth status >/dev/null 2>&1 || die "gh is not logged in; run gh auth login"
    fetch
    local version previous b
    version=${VERSION:-$(go run ./scripts/release bump)}
    b=$(base "$version" 2>/dev/null) || die "a release is vX.Y.Z (got: $version)"
    # A minor writes storage fixtures, which run Postgres; a patch has none.
    [[ "$b" != main ]] || docker info >/dev/null 2>&1 || die "Docker is not running; the storage fixtures need it"
    git rev-parse -q --verify "$REMOTE/$b^{commit}" >/dev/null ||
        die "$REMOTE has no branch $b; create it as docs/dev/releasing.md, Creating a release branch, shows"
    ! tagged "$version" || die "$version is tagged already"
    # One release at a time: the release PR of another version would be tagged later
    # with notes that miss this one.
    local others
    others=$(gh pr list -R "$REPO" --state open --search "head:release/v" --json headRefName \
        --jq "[.[].headRefName | select(startswith(\"release/v\") and . != \"release/$version\")] | join(\" \")")
    [[ -z "$others" ]] || die "release PR $others is open; merge or close it first"
    previous=$(previous_release "$version")
    if [[ "$b" == main ]]; then
        local guide
        guide=$(git ls-tree --name-only "$REMOTE/main" docs/upgrade/ | grep -- "-to-${version%.0}\.md$" | head -n1 || true)
        [[ -n "$guide" ]] || die "$REMOTE/main has no upgrade guide docs/upgrade/v…-to-${version%.0}.md"
        say "the upgrade guide of $version is $guide"
        confirm "Is $guide complete: every breaking change has a section that says what to do, and the checklist lists every step?"
    else
        # A patch carries fixes only: a feature or a breaking change waits for the next minor.
        # The release steps and CI run from the branch, so it must come from a minor cut with
        # them: one whose Makefile has the release targets and whose Tests run on release-*.
        if ! git grep -q '^release-pr:' "$REMOTE/$b" -- Makefile ||
            ! git grep -q "release-\*" "$REMOTE/$b" -- .github/workflows/tests.yml; then
            die "$b predates patch releases; only a minor cut with them gets a release branch (docs/dev/releasing.md, Creating a release branch)"
        fi
        local unfit
        unfit=$( (git log --format='%h %s' "$previous..$REMOTE/$b" | grep -E '^[0-9a-f]+ (feat[(:!]|[a-z]+(\([^)]*\))?!:)'
            git log -E --grep='^BREAKING[ -]CHANGE:' --format='%h %s' "$previous..$REMOTE/$b") |
            sort -u || true)
        [[ -z "$unfit" ]] || die "a patch carries fixes only, but $b has:"$'\n'"$unfit"
        say "$version has these commits since $previous:"
        git log --format='  %h %s' "$previous..$REMOTE/$b"
    fi
    confirm "Prepare $version (after $previous) on release/$version from $REMOTE/$b?"

    git switch -q -C "release/$version" "$REMOTE/$b"
    say "release data, backport rules and notes"
    report
    go run ./scripts/release data --version "$version" --report "$TMP/report.json" > "$RELEASES/$version.json"
    # A minor changes the supported minors, and so the backport rules; a patch leaves them.
    [[ "$b" != main ]] || go run ./scripts/release mergify > "$MERGIFY"
    notes "$version" "$RELEASES/$version.md" "$(git rev-parse HEAD)"
    git add "$RELEASES/$version.json" "$RELEASES/$version.md" "$MERGIFY"
    if [[ "$b" == main ]]; then
        say "storage fixtures"
        make pull-images-database >/dev/null
        "$FIXTURES/generate.sh" --local "$version"
        go test -count=1 -run Fixtures ./platform/fabric/services/db/driver/sql/
        git add "$FIXTURES/$version"
    fi
    echo
    git status --short
    echo
    echo "Review the staged files, above all the notes in $RELEASES/$version.md, then run: make release-pr"
}

pr() {
    check_remote
    local branch version b
    branch=$(git branch --show-current)
    [[ "$branch" =~ ^release/(v[0-9]+\.[0-9]+\.[0-9]+)$ ]] || die "check out release/vX.Y.Z first (on ${branch:-a detached HEAD})"
    version=${BASH_REMATCH[1]}
    b=$(base "$version")
    [[ -z "$(git status --porcelain | grep -v '^[AM] ')" ]] ||
        die "the tree has changes that are not staged; the release PR adds only what make release-prepare staged"
    # The release files: the data and the notes, and for a minor its storage fixtures and
    # backport rules.
    local other files="${RELEASES}/${version}\.(json|md)"
    [[ "$b" != main ]] || files+="|${FIXTURES}/${version}/.+|\.mergify\.yml"
    other=$(git diff --cached --name-only | grep -v -E "^(${files})$" || true)
    [[ -z "$other" ]] || die "staged files that are not part of the release: $other"
    if git diff --cached --quiet; then
        [[ "$(git log -1 --format=%s)" == "build(release): $version" ]] || die "nothing staged; run make release-prepare"
    fi
    confirm "Commit, push $branch to $REMOTE and open or update its PR in $REPO?"
    if [[ "${DRY_RUN:-}" == 1 ]]; then
        echo "dry run, would commit (signed off and signed) \"build(release): $version\":"
        git diff --cached --name-only | sed 's/^/  /'
    else
        git diff --cached --quiet || git commit -q -s -S -m "build(release): $version"
    fi
    write git push -q --force "$REMOTE" "$branch"
    local open
    open=$(gh pr list -R "$REPO" --head "$branch" --state open --json url --jq '.[0].url // ""')
    if [[ -n "$open" ]]; then
        echo "Updated the open release PR $open."
    else
        write gh pr create -R "$REPO" --base "$b" --head "$branch" --title "build(release): $version" \
            --body "The release files of $version, from \`make release-prepare\`. Read the notes as GitHub will show them. Once it is merged, run \`make release-publish\`; see docs/dev/releasing.md."
    fi
    echo "Once the PR is approved and merged, run: make release-publish"
}

# ours tells whether release $1 can be a release of $REPO: always for the main repository,
# and for a fork only when the main repository has not released it. A fork's main holds the
# main repository's release data, whose tags are only in the main repository.
ours() { [[ "$REPO" == "$MAIN_REPO" ]] || ! git rev-parse -q --verify "refs/tags/$1" >/dev/null; }

# released prints, lowest first, every release of $REPO with a data file on main or on a
# release branch of $REMOTE.
released() {
    local ref v
    for ref in $(git for-each-ref --format='%(refname)' "refs/remotes/$REMOTE/main" "refs/remotes/$REMOTE/release-*"); do
        git ls-tree --name-only "$ref" "$RELEASES/" | sed -n 's|.*/\(v[0-9.]*\)\.json$|\1|p'
    done | { grep -E "$SEMVER" || true; } | sort -u -V | while read -r v; do
        if ours "$v"; then echo "$v"; fi
    done
}

# untagged prints the one release of released that has no tag; $VERSION picks one of
# several, such as a minor on main and a patch on a release branch.
untagged() {
    # A string, not an array: bash 3.2 (macOS) fails on an empty array under set -u.
    local found="" v
    for v in $(released); do
        tagged "$v" || found+=" $v"
    done
    found=${found# }
    if [[ -n "${VERSION:-}" ]]; then
        [[ " $found " == *" $VERSION "* ]] || return 0
        echo "$VERSION"
        return
    fi
    [[ "$found" != *" "* ]] || die "more than one untagged release: $found; set VERSION"
    echo "$found"
}

publish() {
    local version commit subject base
    if [[ "${DRY_RUN:-}" == 1 && "$(git branch --show-current)" =~ ^release/(v[0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
        # Before the merge: check the release branch, staged or committed, as if it had
        # been squash-merged onto the commit it starts from.
        version=${BASH_REMATCH[1]}
        check_remote
        fetch
        if [[ "$(git log -1 --format=%s)" == "build(release): $version" ]]; then
            base=$(git rev-parse HEAD^)
            check_clean
        else
            base=$(git rev-parse HEAD)
            git diff --cached --quiet && die "nothing staged or committed on release/$version; run make release-prepare"
        fi
        say "dry run: checking release/$version as if it were merged"
        check_notes "$version" "$base"
        say "dry run: not checking Tests, which needs the merged PR"
        # tag-release.sh checks the commit to tag, which staged files are not part of yet.
        [[ "$base" == "$(git rev-parse HEAD)" ]] && say "dry run: tagging needs the release commit" ||
            ./scripts/tag-release.sh --dry "$version"
        echo "dry run, would run: ./scripts/tag-release.sh --push=$REMOTE $version"
        release "$version" "$RELEASES/$version.md" "$MERGIFY"
        return
    fi
    check_clean
    check_remote
    fetch
    version=$(untagged)
    if [[ -z "$version" ]]; then
        # A run that stopped after pushing the tags left the GitHub release to create:
        # the latest release with a data file on $REMOTE and a tag in this repository.
        # A patch can be such a release after a newer minor, so VERSION picks it.
        version=${VERSION:-$(released | tail -n1)}
        # Not grep -q: it would stop reading, and pipefail would fail released.
        released | grep -xF -- "$version" >/dev/null || die "no release ${VERSION:-of $REPO} on $REMOTE; merge the release PR first"
        gh release view "$version" -R "$REPO" >/dev/null 2>&1 &&
            die "no untagged release on $REMOTE; merge the release PR first, or set VERSION to a release whose tags are pushed but which has no GitHub release"
        say "$version is tagged but has no GitHub release"
        confirm "Create the GitHub release of $version?"
        git show "$version:$RELEASES/$version.md" > "$TMP/notes.md"
        [[ "$version" != *.0 ]] || git show "$version:$MERGIFY" > "$TMP/mergify.yml"
        release "$version" "$TMP/notes.md" "$TMP/mergify.yml"
        return
    fi

    # The squash merge of the release PR added the data file. That commit is the
    # release, whatever the head of its branch is now.
    # The latest release commit of the version: a release opened again after the merge
    # updates its files, maybe only the notes.
    local ref
    ref="$REMOTE/$(base "$version")"
    commit=$(git log --format=%H -n1 --grep="^build(release): $version\( (#[0-9]*)\)\?$" "$ref" -- \
        "$RELEASES/$version.json" "$RELEASES/$version.md" "$FIXTURES/$version")
    [[ -n "$commit" ]] || commit=$(git log --format=%H -n1 "$ref" -- "$RELEASES/$version.json")
    subject=$(git log -1 --format=%s "$commit")
    [[ "$subject" == "build(release): $version"* ]] ||
        die "$RELEASES/$version.json was added by ${commit:0:9} \"$subject\", not a release PR"
    say "release $version is $(git log -1 --format='%h "%s"' "$commit")"
    git switch -q --detach "$commit"

    say "checking the release data and notes against this commit"
    check_notes "$version" "$(git rev-parse "$commit^")"
    tests_passed "$commit"

    ./scripts/tag-release.sh --dry "$version"
    confirm "Push these tags to $REMOTE ($(git remote get-url "$REMOTE"))? This cannot be undone."
    if [[ "${DRY_RUN:-}" == 1 ]]; then
        echo "dry run, would run: ./scripts/tag-release.sh --push=$REMOTE $version"
    else
        ./scripts/tag-release.sh --push="$REMOTE" "$version"
        # A fork's tags stay in the fork: left in this repository, the next minor and the
        # tag conflict check of a release of the main repository would see them.
        if [[ "$REPO" != "$MAIN_REPO" ]]; then
            git tag -d $(git tag --list "$version" "*/$version") >/dev/null
        fi
    fi
    release "$version" "$RELEASES/$version.md" "$MERGIFY"
    echo "The checkout is on the release commit, detached; switch back with: git switch main"
}

# release creates, for a minor, the backport labels of Mergify configuration $3, and then the
# GitHub release $1 with notes $2: a run stopped before the GitHub release is run again. A
# patch is never marked Latest: a newer minor may be out.
release() {
    local latest=true
    [[ "$1" =~ \.0$ ]] || latest=false
    [[ "$latest" == false ]] || backport_labels "$1" "$3"
    write gh release create "$1" -R "$REPO" --verify-tag --title "$1" --notes-file "$2" --latest="$latest"
    [[ "$REPO" == "$MAIN_REPO" ]] && echo "Announce the release in #fabric-smart-client on the LFDT Discord."
    return 0
}

# backport_labels creates the label backport/X.Y of every rule in Mergify configuration $2
# of minor $1 and deletes every other backport/X.Y label, so only a supported minor can be
# picked. A label exists before its release branch; Mergify fails a backport to a branch that
# does not exist. An older minor than the latest released one changes no label.
backport_labels() {
    local want have create="" drop="" l
    if [[ "$( (echo "$1"; released | grep '\.0$') | sort -V | tail -n1)" != "$1" ]]; then
        say "a minor newer than $1 is released; the backport labels stay"
        return 0
    fi
    want=$(sed -n 's|^ *- label=\(backport/[0-9]*\.[0-9]*\)$|\1|p' "$2")
    have=$(gh label list -R "$REPO" --search backport/ --limit 100 --json name \
        --jq '.[].name | select(test("^backport/[0-9]+\\.[0-9]+$"))')
    for l in $want; do grep -qxF -- "$l" <<<"$have" || create+=" $l"; done
    for l in $have; do grep -qxF -- "$l" <<<"$want" || drop+=" $l"; done
    [[ -n "$create$drop" ]] || return 0
    confirm "In $REPO, create the labels:${create:- none}; delete:${drop:- none}?"
    for l in $create; do
        write gh label create "$l" -R "$REPO" \
            --description "Backport to release-${l#backport/} once merged to main"
    done
    for l in $drop; do write gh label delete "$l" -R "$REPO" --yes; done
}

# tests_passed fails unless the Tests workflow passed on commit $1 itself: tests.yml runs on
# every push to main that touches the release data, so the tree that gets tagged was tested,
# whatever main looked like when the PR merged.
tests_passed() {
    local conclusion
    conclusion=$(gh run list -R "$REPO" --workflow tests.yml --commit "$1" --event push --limit 1 \
        --json conclusion,status --jq '.[0] | if . == null then "" elif .status != "completed" then .status else .conclusion end')
    [[ "$conclusion" == success ]] ||
        die "Tests did not pass on the release commit ${1:0:9}: ${conclusion:-no run}; wait for it or rerun it"
    say "Tests passed on the release commit"
}

case "${1:-}" in
    prepare) prepare ;;
    pr) pr ;;
    publish) publish ;;
    # check-notes VERSION BASE: what the Release check workflow runs on a release PR.
    check-notes)
        [[ $# -eq 3 ]] || die "usage: $0 check-notes vX.Y.Z <base commit>"
        [[ "$REPO" == "$MAIN_REPO" ]] || fetch_main_repo
        check_notes "$2" "$3"
        ;;
    *) echo "usage: $0 prepare|pr|publish" >&2; exit 2 ;;
esac
