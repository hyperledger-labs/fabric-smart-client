#!/usr/bin/env bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
# generate.sh VERSION writes the storage fixtures of FSC release VERSION into
# VERSION/: it builds generate.go in a temporary module that requires that
# release, runs it (Postgres runs in a Docker container), and writes
# SHA256SUMS. See README.md.
set -euo pipefail

version=${1:?usage: generate.sh VERSION, e.g. v0.26.0}
here=$(cd "$(dirname "$0")" && pwd)
out="$here/$version"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

mkdir -p "$out"
cp "$here/generate.go" "$work/main.go"
sed -i.bak '1,2d' "$work/main.go" && rm "$work/main.go.bak" # drop the go:build ignore line
(
  cd "$work"
  go mod init fixturegen >/dev/null 2>&1
  go get "github.com/hyperledger-labs/fabric-smart-client@$version" >/dev/null
  go mod tidy >/dev/null
  go run . "$out"
)
(cd "$out" && shasum -a 256 fsc.sqlite fsc.postgres.json > SHA256SUMS)
echo "fixtures for $version written to $out"
