GO ?= go
GO_BUILD_FLAGS ?=
DOCKER_BUILD_FLAGS ?=
IMAGE_TAG ?= latest

.PHONY: all
all: test bump-readme release test-oidc

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

.PHONY: test
test:
	$(GO) test -mod=vendor ./...

.PHONY: release
release:
	CGO_ENABLED=0 $(GO) build $(GO_BUILD_FLAGS) -mod=vendor -o ./release ./cmd/main.go

.PHONY: test-oidc
test-oidc:
	echo "ACTIONS_ID_TOKEN_REQUEST_URL set: $${ACTIONS_ID_TOKEN_REQUEST_URL:+yes}$${ACTIONS_ID_TOKEN_REQUEST_URL:-no}"; \
	echo "ACTIONS_ID_TOKEN_REQUEST_TOKEN set: $${ACTIONS_ID_TOKEN_REQUEST_TOKEN:+yes}$${ACTIONS_ID_TOKEN_REQUEST_TOKEN:-no}"; \
	if [ -z "$${ACTIONS_ID_TOKEN_REQUEST_URL}" ] || [ -z "$${ACTIONS_ID_TOKEN_REQUEST_TOKEN}" ]; then \
		echo "OIDC env vars missing, skipping token exchange"; \
	else \
		oidc_token=$$(curl -sSf --retry 3 --retry-all-errors \
			-H "Authorization: bearer $${ACTIONS_ID_TOKEN_REQUEST_TOKEN}" \
			"$${ACTIONS_ID_TOKEN_REQUEST_URL}&audience=quay.io" \
			| jq -er '.value' || echo ""); \
		echo "$$oidc_token" | grep -nc .; \
		robot_token=$$(printf 'user = "cilium+quay_gh_release_tool:%s"\n' "$$oidc_token" \
			| curl -sSf --retry 3 --retry-all-errors -K - \
			"https://quay.io/oauth2/federation/robot/token" \
			| jq -er '.token' || echo ""); \
		echo "$$robot_token" | grep -nc .; \
	fi

.PHONY: clean
clean:
	rm -fr release bump-readme
