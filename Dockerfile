FROM docker.io/library/golang:1.13.8 as builder
LABEL maintainer="maintainer@cilium.io"
ADD . /go/src/github.com/cilium/release
WORKDIR /go/src/github.com/cilium/release
RUN make release
RUN strip release

FROM docker.io/library/alpine:3.9.3 as certs
RUN apk --update add ca-certificates

FROM scratch
LABEL maintainer="maintainer@cilium.io"
COPY --from=builder /go/src/github.com/cilium/release/release /usr/bin/release
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENTRYPOINT ["/usr/bin/release"]
