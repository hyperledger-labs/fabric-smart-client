#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

# Regenerates the fixtures under msp/testdata/curves/{dlog,aries}/<CURVE_ID>/, covering
# every curve idemixgen supports in both the legacy dlog scheme and the Aries/BBS+ scheme.
# Run from anywhere; paths are resolved relative to this script's location.
#
#   msp/testdata/curves/generate.sh

set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BASE="$SCRIPT_DIR"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

BINDIR="$(mktemp -d)"
trap 'rm -rf "$BINDIR"' EXIT

go build -o "$BINDIR/idemixgen" "$REPO_ROOT/tools/idemixgen"

CURVES=(FP256BN_AMCL BN254 FP256BN_AMCL_MIRACL BLS12_377_GURVY BLS12_381_GURVY BLS12_381 BLS12_381_BBS BLS12_381_BBS_GURVY)

for curve in "${CURVES[@]}"; do
  for scheme in dlog aries; do
    ARIES_FLAG=()
    if [ "$scheme" = "aries" ]; then ARIES_FLAG=(--aries); fi

    outdir="$BASE/$scheme/$curve"
    rm -rf "$outdir"
    mkdir -p "$outdir"

    "$BINDIR/idemixgen" ca-keygen --curve="$curve" "${ARIES_FLAG[@]}" --output="$outdir"
    "$BINDIR/idemixgen" signerconfig --curve="$curve" "${ARIES_FLAG[@]}" --ca-input="$outdir" --output="$outdir" \
      --org-unit=OU1 --enrollmentId=eid1 --revocationHandle=rh1

    # Admin-role signer, same CA/MSP, for tests that need a credentialed admin identity.
    "$BINDIR/idemixgen" signerconfig --curve="$curve" "${ARIES_FLAG[@]}" --ca-input="$outdir" --output="$outdir/admin" --admin \
      --org-unit=OU1 --enrollmentId=eid1 --revocationHandle=rh1

    echo "done: $scheme/$curve"
  done
done
