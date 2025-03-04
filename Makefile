GO ?= go
GO_BUILD_FLAGS ?=
DOCKER_BUILD_FLAGS ?=
IMAGE_TAG ?= latest

all: tests bump-readme release

.PHONY: bump-readme
bump-readme:
	CGO_ENABLED=0 $(GO) build $(GO_BUILD_FLAGS) \
		    -mod=vendor \
		    -o ./bump-readme ./tools/bump-readme/main.go

.PHONY: docker-image
docker-image:
	docker build $(DOCKER_BUILD_FLAGS) -t cilium/release-tool:${IMAGE_TAG} .

.PHONY: generate-golden
generate-golden:
	$(MAKE) $(patsubst %.input,%.golden,$(shell find ./testdata/checklist/ -name "*.input"))

%.golden: %.input
	$(GO) run ./cmd/ checklist open --dry-run \
		--target-version "v1.10.0-pre.0" \
		--template $< \
		> $@ \

.PHONY: tests
tests:
	$(GO) test -mod=vendor ./...

.PHONY: release
release:
	CGO_ENABLED=0 $(GO) build $(GO_BUILD_FLAGS) -mod=vendor -o ./release ./cmd/main.go

.PHONY: clean
clean:
	rm -fr release bump-readme
