all: local

docker-image:
	docker build -t cilium/release:${VERSION} .

tests:
	go test -mod=vendor ./...

release: tests
	CGO_ENABLED=0 GOOS=linux go build -mod=vendor -a -installsuffix cgo -o $@ ./cmd/...

local: release
	strip release

clean:
	rm -fr release
