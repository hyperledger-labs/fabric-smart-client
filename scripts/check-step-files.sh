#!/usr/bin/env bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

# Fails if a schema step released in the latest release tag reachable from HEAD (or
# in the tag given as the first argument) was changed or removed: every step file of
# that tag must be a line prefix of the file at HEAD. A step file that the tag does
# not have is new and skipped. Needs the tags fetched. On a release branch the tag
# is the branch's latest patch, since a newer minor's steps are not on the branch;
# a patch adds no step.

set -euo pipefail

# The latest final release: tags with a suffix, such as v1.2.0-rc.1, are skipped.
tag="${1:-$(git tag --list 'v*' --merged HEAD --sort=-v:refname | grep -v -- - | head -n1)}"
if [ -z "$tag" ]; then
  echo "no release tag found; fetch tags first" >&2
  exit 1
fi

rc=0
checked=0
while IFS= read -r f; do
  checked=$((checked + 1))
  released=$(git show "$tag:$f")
  [ -z "$released" ] && continue
  lines=$(printf '%s\n' "$released" | wc -l)
  current=$(head -n "$lines" "$f" 2>/dev/null || true)
  if [ "$current" != "$released" ]; then
    echo "$f: steps released in $tag were changed or removed; add a new step instead" >&2
    diff <(echo "$released") <(echo "$current") >&2 || true
    rc=1
  fi
done < <(git ls-tree -r --name-only "$tag" -- platform | grep '/testdata/steps/.*\.steps$' || true)

echo "checked $checked step file(s) against $tag"
exit $rc
