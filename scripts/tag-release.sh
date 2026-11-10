#!/usr/bin/env bash
set -euo pipefail

usage() {
    echo "Usage: $0 [--dry] [--push=<remote>] <version>"
    echo "  --dry             Print tags that would be created without creating them"
    echo "  --push=<remote>   Push the created tags to <remote>, all or none"
    echo "  version           Git tag version, e.g. v0.13.0"
    exit 1
}

DRY=false
REMOTE=""
VERSION=""
for arg in "$@"; do
    case "$arg" in
        --dry) DRY=true ;;
        --push=?*) REMOTE="${arg#--push=}" ;;
        -*) echo "Error: unknown flag $arg" >&2; usage ;;
        *) [[ -n "$VERSION" ]] && usage; VERSION="$arg" ;;
    esac
done
[[ -z "$VERSION" ]] && usage

if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: version must match vMAJOR.MINOR.PATCH (got: $VERSION)" >&2
    exit 1
fi

REPO_ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
HEAD="$(git -C "$REPO_ROOT" rev-parse HEAD)"

# The release data is part of the tagged commit, so a check of the tag sees it.
DATA="docs/upgrade/releases/${VERSION}.json"
if ! git -C "$REPO_ROOT" cat-file -e "${HEAD}:${DATA}" 2>/dev/null; then
    echo "Error: ${DATA} is not in the commit to tag; merge the release PR first" >&2
    exit 1
fi

# Collect all module paths: root first, then submodules excluding tools.
#
# Modules are discovered from the tree of the commit being tagged, not from the
# working tree: that is exactly what a consumer sees at `go get <module>@<tag>`,
# and it is immune to go.mod files that are untracked, ignored, vendored, or
# sitting in a nested git worktree checked out under the repo (e.g. .claude/).
MODULES=(".")
while IFS= read -r gomod; do
    dir="${gomod%/go.mod}"
    [[ "$dir" == "go.mod" || "$dir" == "tools" ]] && continue
    MODULES+=("$dir")
done < <(git -C "$REPO_ROOT" ls-tree -r --name-only "$HEAD" | grep -E '(^|/)go\.mod$' | sort)

# Build the list of tags to create
declare -a TAGS
for module in "${MODULES[@]}"; do
    if [[ "$module" == "." ]]; then
        TAGS+=("$VERSION")
    else
        TAGS+=("${module}/${VERSION}")
    fi
done

# Check for conflicts before creating anything
CONFLICTS=()
for tag in "${TAGS[@]}"; do
    if git -C "$REPO_ROOT" tag --list "$tag" | grep -q .; then
        CONFLICTS+=("$tag")
    fi
done

if [[ ${#CONFLICTS[@]} -gt 0 ]]; then
    echo "Warning: the following tags already exist — aborting without creating any tags:" >&2
    for tag in "${CONFLICTS[@]}"; do
        echo "  $tag" >&2
    done
    exit 1
fi

if $DRY; then
    echo "Dry run — would create ${#TAGS[@]} tag(s) at $HEAD:"
    for tag in "${TAGS[@]}"; do
        echo "  $tag"
    done
    exit 0
fi

echo "Tagging ${#TAGS[@]} module(s) at $HEAD"
for tag in "${TAGS[@]}"; do
    git -C "$REPO_ROOT" tag "$tag"
    echo "  created $tag"
done

if [[ -n "$REMOTE" ]]; then
    # Exactly the new tags (--tags would push every local tag), all or none.
    if ! git -C "$REPO_ROOT" push --atomic "$REMOTE" "${TAGS[@]}"; then
        # Nothing reached the remote; remove the local tags so that a rerun starts over.
        git -C "$REPO_ROOT" tag -d "${TAGS[@]}" >/dev/null
        echo "Error: pushing the tags to $REMOTE failed; no tag was created" >&2
        exit 1
    fi
    exit 0
fi

echo "Done. To publish, push exactly these tags to the main repository (docs/dev/releasing.md, Without the scripts):"
echo "  git push upstream ${TAGS[*]}"
