# Bundled values

This package implements the contracts in
[`values/interfaces`](../../values/interfaces). Import it as
`testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"` when choosing the bundled
representation.

- `doc.go`: package documentation.
- `value_factory.go`: the stateless factory and its interface conformance check.
- `*_value.go`: each concrete value, checked constructor, payload accessors,
  nullability adaptation, equality and interface conformance check.
- `non_null.go`: the shared non-null marker.
- `tests/`: construction, ownership, nullability, equality, allocation and
  touchstone tests, plus equality benchmarks. Shared test helpers live in
  `helpers_test.go`.

Cross-implementation contract tests and reusable external-value fixtures live
in [`values/interfaces/tests`](../../values/interfaces/tests).
