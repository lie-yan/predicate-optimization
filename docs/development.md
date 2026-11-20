# Development

## Build modes and tests

Required non-nil inputs use [`internal/assertions`](../internal/assertions/nil.go). With the `debug` build tag, `NotNilPointer` checks statically known pointers directly; `NotNil` checks interface inputs, rejecting both nil interfaces and typed nils with a uniform panic message. Without `debug`, both functions are no-ops; argument expressions are still evaluated. The build-time constant `assertions.Enabled` lets tests condition nil-precondition expectations on the selected build. These shallow checks neither validate construction state nor traverse composite data.

`assertions.IsNil` remains active in every build: validation paths use it to return errors. Package-specific construction checks also remain active. Disabling nil assertions does not make nil inputs supported; they may panic later or go undetected.

```sh
go build ./...             # Default: shared nil assertions disabled.
go build -tags debug ./... # Enable shared nil assertions.
make test                  # Test both configurations.
make test-release          # Test the default configuration only.
make test-debug            # Test with nil assertions enabled.
```

## Benchmarks

The [touchstone benchmark](../examples/rewriting/touchstone_optimization_benchmark_test.go) times the five passes together, entry validation plus the five passes, and each pass on its actual intermediate input:

```sh
GOTOOLCHAIN=go1.26.1 go test ./examples/rewriting -run '^$' -bench '^BenchmarkTouchstoneOptimization$' -benchmem -benchtime=500ms -count=5 -cpu=1
```

The benchmark uses public Value/payload/factory interfaces and bundled testbed implementations. Fixture construction and correctness checks are outside timing. Each operation rewrites the original input. Measure on your target hardware; structural reduction does not guarantee lower production latency.

## Formatting and lint checks

The prototype uses a focused Milvus-inspired [lint configuration](../.golangci.yml) and [tool version pin](../Makefile): `golangci-lint` v2.11.3, `revive` naming checks, `gofmt`, `gofumpt`, and `goimports`. `govet`, `ineffassign`, and `staticcheck` additionally check correctness. Milvus-specific dependency restrictions and execution settings are omitted.

Use mixed-capital names, with an initial uppercase letter for exported identifiers and lowercase for package-private identifiers. Preserve conventional initialisms such as `ID`. Boolean names need not start with `Is`. The naming linter does not mandate either prefix. This prototype uses `IsValid` for its public validation function and `isValid` for private validation helpers.

From the repository root:

```sh
make tools  # Install the pinned linter; put $(go env GOPATH)/bin on PATH.
make fmt    # Apply formatting and import organization.
make lint   # Check the default build without edits.
make lint-debug # Check the debug build.
make check  # Run lint and tests in both configurations.
```

The Makefile selects Go 1.26.1 for these commands to match the pinned linter's supported compiler format. Go may download that toolchain on first use. The module still declares Go 1.22 as its minimum language version. Override `GOTOOLCHAIN` only with a compatible compiler, and use `GOLANGCI_LINT=/path/to/golangci-lint` for a linter outside PATH. Installing development tools requires downloads; the prototype itself has no external dependencies. These checks are invoked explicitly; no Git hooks are installed.

[Back to README](../README.md)
