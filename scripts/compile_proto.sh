#!/usr/bin/env bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

set -euo pipefail

# Find all proto dirs to be processed
PROTO_DIRS=$(find . -name '*.proto' -print0 | \
  xargs -0 -n 1 dirname | \
  sort -u | grep -v testdata)

echo "Found proto dirs:"
echo "$PROTO_DIRS"
echo

for dir in $PROTO_DIRS; do
  echo "Compiling: $dir"
  protoc \
    --proto_path=. \
    --go_out=paths=source_relative:. \
    --go-grpc_out=paths=source_relative:. \
    "$dir"/*.proto
done
