# Match the version pinned by the inspected Milvus Makefile.
GOLANGCI_LINT_VERSION := v2.11.3
GOLANGCI_LINT ?= golangci-lint
# The pinned linter supports this toolchain; newer compiler formats may not.
export GOTOOLCHAIN ?= go1.26.1

.PHONY: tools lint lint-debug fmt test test-release test-debug check

tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

# run checks enabled formatters as well as linters, without modifying files.
lint:
	$(GOLANGCI_LINT) run --config .golangci.yml ./...

lint-debug:
	$(GOLANGCI_LINT) run --config .golangci.yml --build-tags debug ./...

fmt:
	$(GOLANGCI_LINT) fmt --config .golangci.yml

test: test-release test-debug

test-release:
	go test ./...

test-debug:
	go test -tags debug ./...

check: lint lint-debug test
