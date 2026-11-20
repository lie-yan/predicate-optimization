# Runnable examples

Examples use public APIs and check their printed output with Go's example tests.
From the repository root, run all examples with:

```sh
go test ./examples/... -run '^Example_' -v
```

| Folder | Contents |
| --- | --- |
| [basics](basics) | Expression construction, interpretation, folding, shared-subtree rewriting and pretty printing |
| [rewriting](rewriting) | Contextual rewriting, logical flattening, array normalization/grouping and the complete touchstone pipeline |
| [scalar_analysis](scalar_analysis) | Scalar extraction, set operations, reconstruction and unsupported-input handling |
| [customvalues](customvalues) | An independent value implementation and factory, their tests and a complete custom-value workflow |
| [validation](validation) | Validation integration tests, query-count checks and cost benchmarks |
| [internal/fixtures](internal/fixtures) | Shared setup helpers, bindings and scalar-expression builders for these examples |

Run one group or the touchstone benchmark with:

```sh
go test ./examples/rewriting -run '^Example_' -v
go test ./examples/rewriting -run '^$' -bench '^BenchmarkTouchstoneOptimization$' -benchmem
```

The custom-value implementation depends only on the public value contracts,
foundation and types. Its workflow test composes it with the optimizer and
testbed function provider. Shared fixtures are internal to the examples and
are not part of the library API.
