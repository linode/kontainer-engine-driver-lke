TARGETS := $(shell ls scripts)

TEST_TIMEOUT := 25m

test:
	go test $(TEST_ARGS) -timeout $(TEST_TIMEOUT)

.DEFAULT_GOAL := ci

.PHONY: $(TARGETS)
