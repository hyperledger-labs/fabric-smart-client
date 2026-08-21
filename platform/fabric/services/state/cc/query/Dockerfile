# syntax=docker/dockerfile:1
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY . .
WORKDIR /src/platform/fabric/services/state/cc/query
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -o /cc .

FROM scratch
COPY --from=build /cc /cc
ENTRYPOINT ["/cc"]
