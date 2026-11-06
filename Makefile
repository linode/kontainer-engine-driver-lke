TEST_TIMEOUT := 25m

DOCKER_TAG ?= dev

GOLANGCILINT      := golangci-lint
GOLANGCILINT_ARGS := run

test:
	go test $(TEST_ARGS) -timeout $(TEST_TIMEOUT)

build:
	CGO_ENABLED=1 go build -o kontainer-engine-driver-lke

docker-build:
	docker build -t linode/kontainer-engine-driver-lke:$(DOCKER_TAG) -f package/Dockerfile .

lint:
	go vet ./...
	$(GOLANGCILINT) $(GOLANGCILINT_ARGS)

fmt:
	gofumpt -l -w .

test-ci:
	./scripts/test

build-ci:
	./scripts/build

.DEFAULT_GOAL := build
