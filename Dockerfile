FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.22.5 AS builder
# TARGETOS is an automatic platform ARG enabled by Docker BuildKit.
ARG TARGETOS
# TARGETARCH is an automatic platform ARG enabled by Docker BuildKit.
ARG TARGETARCH

COPY . /go/src/github.com/cilium/release
WORKDIR /go/src/github.com/cilium/release

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    make GOOS=$TARGETOS GOARCH=$TARGETARCH GO_BUILD_FLAGS='-ldflags="-s -w"' release

FROM gcr.io/distroless/static
LABEL maintainer="maintainer@cilium.io"
COPY --from=builder /go/src/github.com/cilium/release/release /usr/bin/release
ENTRYPOINT ["/usr/bin/release"]
