#!/usr/bin/env bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "Usage: $0 <input-coverage-file> <output-file>"
  exit 1
fi

INPUT="$1"
OUTPUT="$2"

if [ ! -f "$INPUT" ]; then
  echo "Error: Input file '$INPUT' does not exist."
  exit 1
fi

tmp="$(mktemp)"

# Ensure temp file is cleaned up on failure
trap 'rm -f "$tmp"' EXIT

# We filter some of the files for test coverage reporting.
awk '
  # Preserve mode line
  /^mode:/ { print; next }

  # Exclude main.go
  /main\.go/ { next }

  # Exclude generated protobuf files (*.pb.go)
  /\.pb\.go/ { next }

  # Exclude mock and fake packages
  /\/mocks?\// { next }      # Matches both /mock/ and /mocks/
  /\/fakes?\// { next }      # Matches both /fake/ and /fakes/
  /[^\/]*mock[^\/]*\.go:/ { next }  # Matches any file with mock in filename
  /[^\/]*fake[^\/]*\.go:/ { next }  # Matches any file with fake in filename

  # Exclude integration
  /\/integration\// { next }

  # Exclude tools and docs
  /\/tools\// { next }
  /\/docs\// { next }

  # Exclude shared test helpers.
  #
  # Helpers imported by other packages cannot live in _test.go files, so
  # conformance suites, benchmark bodies and node harnesses sit in plain .go
  # files named *test_utils*.go. They are test scaffolding, not shipped code, so
  # -- like the mocks and fakes above -- they stay out of the denominator.
  #
  # Naming is the whole mechanism: call a new shared helper *_test_utils.go and
  # it is filtered. A helper that is named otherwise is counted as production
  # code, so keep the convention when adding one.
  /[^\/]*test_utils[^\/]*\.go:/ { next }

  # Keep everything else
  { print }
' "$INPUT" > "$tmp"

# Atomically replace output
mv "$tmp" "$OUTPUT"

# Disable trap since mv succeeded
trap - EXIT

echo "Filtered coverage written to $OUTPUT"
