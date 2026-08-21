# syntax=docker/dockerfile:1
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY . .
WORKDIR /src/integration
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -o /cc ./fabric/events/chaincode

FROM scratch
COPY --from=build /cc /cc
ENTRYPOINT ["/cc"]
