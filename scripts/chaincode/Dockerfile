# syntax=docker/dockerfile:1
#
# The one recipe for building an in-tree Go chaincode into a CCaaS image.
# MODULE is the module directory relative to the repo root; PKG is the package
# within that module. A chaincode needing something else drops its own
# Dockerfile next to its source and build-image.sh picks that up instead.
ARG GO_VERSION=1.26
FROM golang:${GO_VERSION}-bookworm AS build
ARG MODULE=.
ARG PKG=.
WORKDIR /src
COPY . .
WORKDIR /src/${MODULE}
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -o /cc ${PKG}

FROM scratch
COPY --from=build /cc /cc
ENTRYPOINT ["/cc"]
