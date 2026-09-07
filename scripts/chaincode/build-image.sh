#!/bin/bash
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
# build-image.sh <image> <module> <pkg>
#
# Builds one chaincode container image tagged <image>:latest: compiles with the
# host toolchain, then packages the static binary into a scratch image.
#
# If a Dockerfile sits next to the chaincode source it is used instead, with the
# repo root as the build context -- the escape hatch for a chaincode that needs
# more than a static binary.
set -euo pipefail

if [ "$#" -ne 3 ]; then
    echo "usage: $0 <image> <module> <pkg>" >&2
    exit 2
fi

IMAGE="$1"
MODULE="$2"
PKG="$3"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

OWN_DOCKERFILE="${ROOT}/${MODULE}/${PKG#./}/Dockerfile"
if [ -f "${OWN_DOCKERFILE}" ]; then
    echo "==> building ${IMAGE}:latest from ${OWN_DOCKERFILE#"${ROOT}/"}"
    DOCKER_BUILDKIT=1 docker build \
        -t "${IMAGE}:latest" \
        -f "${OWN_DOCKERFILE}" \
        "${ROOT}"
    exit 0
fi

# Nothing in a scratch image pins its platform, so label it to match the binary.
ARCH="$(go env GOARCH)"

BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "${BUILD_DIR}"' EXIT

echo "==> building ${IMAGE}:latest from ${MODULE}/${PKG} (linux/${ARCH})"
CGO_ENABLED=0 GOOS=linux go build \
    -C "${ROOT}/${MODULE}" \
    -buildvcs=false \
    -o "${BUILD_DIR}/cc" \
    "${PKG}"

docker build \
    --platform "linux/${ARCH}" \
    -t "${IMAGE}:latest" \
    -f "${SCRIPT_DIR}/Dockerfile" \
    "${BUILD_DIR}"
