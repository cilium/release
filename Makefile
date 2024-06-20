all: tests local

.PHONY: docker-image
docker-image:
	docker build -t cilium/release:${VERSION} .

.PHONY: tests
tests:
	go test -mod=vendor ./...

.PHONY: release
release:
	CGO_ENABLED=0 go build -mod=vendor -a -installsuffix cgo -o $@ ./cmd/main.go

.PHONY: local
local: release
	strip release

.PHONY: clean
clean:
	rm -fr release
