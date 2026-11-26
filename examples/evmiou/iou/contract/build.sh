#!/usr/bin/env bash
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

# Regenerates IOU.bin from IOU.sol using solc in Docker, so nobody needs Solidity tooling
# installed locally and CI never has to compile Solidity. Run this script and commit the result
# whenever IOU.sol changes.
set -euo pipefail

dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
solc_image="ethereum/solc:0.8.26"

rm -rf "${dir}/out"
docker run --rm -v "${dir}:/src" "${solc_image}" \
    --evm-version paris --optimize --bin -o /src/out /src/IOU.sol

cp "${dir}/out/IOU.bin" "${dir}/IOU.bin"
rm -rf "${dir}/out"

echo "wrote ${dir}/IOU.bin"
