#!/bin/bash
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
# build-image.sh <image> <module> <pkg>
#
# Builds one chaincode container image tagged <image>:latest. The build context
# is always the repo root, so a chaincode may import from sibling modules.
# If a Dockerfile sits next to the chaincode source it is used; otherwise the
# shared recipe in this directory is, parameterised by MODULE and PKG.
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

# Track the repo's Go version rather than pinning a second copy in the
# Dockerfile: "go 1.26.5" -> "1.26".
GO_VERSION="$(awk '/^go /{split($2,v,"."); print v[1]"."v[2]; exit}' "${ROOT}/go.mod")"
if [ -z "${GO_VERSION}" ]; then
    echo "==> could not read the go version from ${ROOT}/go.mod" >&2
    exit 1
fi

OWN_DOCKERFILE="${ROOT}/${MODULE}/${PKG#./}/Dockerfile"
if [ -f "${OWN_DOCKERFILE}" ]; then
    echo "==> building ${IMAGE}:latest from ${OWN_DOCKERFILE#"${ROOT}/"}"
    DOCKER_BUILDKIT=1 docker build \
        -t "${IMAGE}:latest" \
        -f "${OWN_DOCKERFILE}" \
        "${ROOT}"
    exit 0
fi

echo "==> building ${IMAGE}:latest from ${MODULE}/${PKG} (go ${GO_VERSION})"
DOCKER_BUILDKIT=1 docker build \
    -t "${IMAGE}:latest" \
    -f "${SCRIPT_DIR}/Dockerfile" \
    --build-arg "GO_VERSION=${GO_VERSION}" \
    --build-arg "MODULE=${MODULE}" \
    --build-arg "PKG=${PKG}" \
    "${ROOT}"
