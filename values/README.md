# Value contracts

`interfaces/` defines `Value`, its domain payload interfaces and `ValueFactory`.
Consumers import `github.com/lie-yan/predicate-optimization/values/interfaces`, conventionally as
`valueinterfaces`. There is no root `values` Go package.

```text
interfaces/
  value.go
  payload.go
  value_factory.go
  tests/                 Contract and cross-implementation conformance tests
    independent/         Isolated tests without bundled implementation imports
    testutil/            Shared recording factories and external value fixtures
```

Bundled implementations and their implementation-specific tests live in
[`../testbed/values`](../testbed/values). A complete caller-owned implementation
lives in [`../examples/customvalues`](../examples/customvalues).
