#!/usr/bin/env bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
# generate.sh [--local] VERSION [DIR] writes the storage fixtures of FSC release
# VERSION into DIR, by default VERSION/ next to this script: it builds
# generate.go in a temporary module that requires that release, runs it
# (Postgres runs in a Docker container), and writes SHA256SUMS. See README.md.
#
# Without --local the module requires VERSION from the Go module proxy, so the
# release must be tagged and public. With --local it builds against the
# repository checkout this script is in instead, which is how a release PR adds
# the fixtures of the release it is about to tag.
set -euo pipefail

usage="usage: generate.sh [--local] VERSION [DIR], e.g. v0.26.0"
local=false
if [[ "${1:-}" == "--local" ]]; then
  local=true
  shift
fi
version=${1:?$usage}
here=$(cd "$(dirname "$0")" && pwd)
out=${2:-$here/$version}
mkdir -p "$out"
out=$(cd "$out" && pwd) # absolute: go run below runs in another directory
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

module=github.com/hyperledger-labs/fabric-smart-client
cp "$here/generate.go" "$work/main.go"
sed -i.bak '1,2d' "$work/main.go" && rm "$work/main.go.bak" # drop the go:build ignore line
(
  cd "$work"
  go mod init fixturegen >/dev/null 2>&1
  if $local; then
    root=$(git -C "$here" rev-parse --show-toplevel)
    go mod edit -require="$module@v0.0.0" -replace="$module=$root"
  else
    go get "$module@$version" >/dev/null
  fi
  go mod tidy >/dev/null
  go run . "$out"
)
(cd "$out" && shasum -a 256 fsc.sqlite fsc.postgres.json > SHA256SUMS)
echo "fixtures for $version written to $out"
