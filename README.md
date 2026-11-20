# Predicate Optimization

Predicate Optimization is an experimental Go framework for semantics-preserving expression optimization, initially focused on predicate evaluation in Milvus. It also supports general scalar expressions, such as simplifying `x + (1 + 2)` to `x + 3`.

The framework provides immutable types and values, typed expression trees, caller-supplied function contracts, tree rewriting, complete and partial interpretation, constant folding, and scalar analysis. Five explicit rewrite passes produce the primary touchstone result: scoped contextual rewriting, Boolean cleanup, logical flattening, array membership normalization, and array exclusion grouping.

The framework is experimental. Live Milvus equivalence has not been established. General scheduling, fixpoint iteration, and bounded-work fallback remain future work. Read the [integration contracts](docs/integration-contracts.md) before supplying custom values or function providers.

## Quick start

Use the module path declared in [go.mod](go.mod) when importing packages. The module declares Go 1.22 and has no external Go dependencies; current checks and measurements use Go 1.26.1.

Run the executable examples and tests from the repository root:

```sh
GOTOOLCHAIN=go1.26.1 go test ./examples/... -run '^Example_' -v
make test
```

See the [example index](examples/README.md) for available workflows, or start with the [end-to-end example](docs/end-to-end-example.md), which reduces the primary touchstone expression from 111 to 39 nodes using five explicit passes.

## Documentation

- [End-to-end example](docs/end-to-end-example.md): the five-pass pipeline, touchstone fixtures, and validation limits.
- [Core concepts](docs/core-concepts.md): expressions, interpretation, rewriting, scalar analysis, and reconstruction.
- [Rewrite passes](docs/rewrite-passes.md): pass APIs, examples, ordering, and execution requirements.
- [Integration contracts](docs/integration-contracts.md): valid inputs, custom values, function providers, and the testbed.
- [Development](docs/development.md): build modes, tests, benchmarks, formatting, and lint checks.
