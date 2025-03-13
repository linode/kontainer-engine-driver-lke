TEST_TIMEOUT := 25m

DOCKER_TAG ?= dev
SKIP_DOCKER ?= 0

GOLANGCILINT      := golangci-lint
GOLANGCILINT_IMG  := golangci/golangci-lint:v1.64.7
GOLANGCILINT_ARGS := run

test:
	go test $(TEST_ARGS) -timeout $(TEST_TIMEOUT)

build:
	CGO_ENABLED=1 go build -o kontainer-engine-driver-lke

docker-build:
	docker build -t linode/kontainer-engine-driver-lke:$(DOCKER_TAG) -f package/Dockerfile .

lint:
	go vet ./...

ifeq ($(SKIP_DOCKER), 1)
	$(GOLANGCILINT) $(GOLANGCILINT_ARGS)
else
	docker run --rm -v $(shell pwd):/app -w /app $(GOLANGCILINT_IMG) $(GOLANGCILINT) $(GOLANGCILINT_ARGS)
endif

fmt:
	gofumpt -l -w .

test-ci:
	./scripts/test

build-ci:
	./scripts/build

.DEFAULT_GOAL := build
