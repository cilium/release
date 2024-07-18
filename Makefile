GO ?= go
GO_BUILD_FLAGS ?=
DOCKER_BUILD_FLAGS ?=
IMAGE_TAG ?= latest

all: tests release

.PHONY: docker-image
docker-image:
	docker build $(DOCKER_BUILD_FLAGS) -t cilium/release-tool:${IMAGE_TAG} .

.PHONY: tests
tests:
	$(GO) test -mod=vendor ./...

.PHONY: release
release:
	CGO_ENABLED=0 $(GO) build $(GO_BUILD_FLAGS) -mod=vendor -o ./release ./cmd/main.go

.PHONY: clean
clean:
	rm -fr release
