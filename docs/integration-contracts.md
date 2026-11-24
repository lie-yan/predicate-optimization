# Integration contracts

## Valid input and preservation

Callers establish valid immutable input with `expressions/utils.ValidateForOptimization` once at entry, or controlled construction that also preserves graph-wide identity/Type consistency. Each rewriter returns a valid equivalent result or an error; unsupported simplification may retain the original input. Passes do not repeat whole-graph validation. New or changed calls use checked construction, while semantic recognition, omission eligibility and exact candidate-Type acceptance stay local to their consumers.

ColumnID and OpaqueID require canonical `Key() string` values consistent with EqualsTo. Validation indexes keys in separate maps to detect conflicting full Types. `Validate` remains structural-only; `ValidateWithContext` is the legacy single-provider API. Input validity does not establish executable availability or the correctness of future runtime Values.

The contextual rewriting and Boolean cleanup constructors require a caller-supplied ValueFactory and a non-nil FunctionContext. Supported comparisons, Boolean NOT and array-membership operations must be correct, stable, effect-free and successful over their complete valid effective input domains, including null behavior. Keep call handles paired with their contexts and expressions/providers immutable during rewriting; complete reference bindings must be stable, successful and effect-free. The [scoped rewriting example](../examples/rewriting/scoped_contextual_rewriting_test.go) uses the standard testbed provider and only public APIs; callers supply no guard paths or proof records. Known truth does not narrow the declared result Type or authorize omission of arbitrary failing operations.

The examples use testbed identities and functions; callers can implement ColumnID, OpaqueID, FunctionHandle, FunctionContext and Environment themselves. Types, Values and expression nodes are immutable and shareable. Traversal visits each occurrence of a shared subtree; it does not memoize context-dependent rewrites. Execution assumes benign performance and correctness behavior. Cancellation/deadline support is deferred; semantic scopes, FunctionContexts and environments remain explicit. Budgets and scheduling are separate orchestration concerns.

Scalar IN currently requires a nonempty list of non-null literal candidates in supported domains. Unsupported membership forms and opaque regions remain residual in partial mode. Known Boolean values use three-valued logic; unavailable values are distinct from SQL NULL.

## Value implementations

Public contracts live in `values/interfaces`. The bundled implementations and checked constructors live in `testbed/values` (import alias `testbedvalues`); `testbedvalues.ValueFactory{}` implements `valueinterfaces.ValueFactory`. Core optimizer components accept the supplied factory and public payload interfaces. Applications can supply their own implementations without importing the testbed. Import the contracts directly; there is no root `values` Go package or compatibility alias layer.

Run the independent [custom value workflow](../examples/customvalues/custom_values_test.go) with `go test ./examples/... -run Example_customValueWorkflow -v`. Its [value/factory package](../examples/customvalues/values.go) imports no testbed values, and its results interoperate with a provider using another representation. It covers extraction, reconstruction, cleanup, grouping and interpretation. See the [contract layout](../values/README.md), [bundled implementation](../testbed/values/README.md) and [conformance tests](../values/interfaces/tests/) for ownership and interoperability checks.

`Value.EqualsTo` belongs to each implementation and must compare complete typed content across representations. Payload access requires a non-null value and the interface for its declared Type. `ArrayValue.Elements()` and `StructValue.Fields()` return owned slices; `ArrayValue.ElementCount`/`ElementAt` provide indexed reads without copying the element slice. Every array implementation supplies all three accessors.

Supply a `ValueFactory` to consumers that construct values, including interpretation, scalar extraction/reconstruction, contextual rewriting, Boolean cleanup and array exclusion grouping. Logical flattening and singleton array-membership normalization do not require a factory. The factory is supplied separately; it is not stored in the `Value` interface.

## Function providers and testbed

[`testbed/functions.NewStandardContext`](../testbed/functions/standard.go) creates a matching context and catalog for development. The [runnable external-package example](../testbed/functions/tests/example_test.go) resolves Boolean NOT, inspects its metadata, binds an effective signature, and evaluates both a concrete nullable Boolean and a typed null:

```sh
go test ./testbed/functions/tests -run '^ExampleNewStandardContext$' -v
```

Pass the returned context to consumers as `functions/interfaces.FunctionContext`. Obtain handles from that same context using `LookupBuiltInFunction`, `ComparisonFunction` or `ArithmeticFunction`. Read metadata and call `BindFunctionSignature(metadata, argumentTypes)` using the actual expression argument types, then call `Evaluate(handle, signature, arguments)`. The handle describes implementation types; binding derives the effective result nullability. Unavailable analysis values remain expressions, not nil `Value` arguments.

Each `NewStandardContext` call has its own handle identity domain. Keep its handles paired with that context throughout a session; another independently constructed context rejects them. Metadata, Types and Values are immutable and shareable. Standard evaluators are stateless and support concurrent use; evaluation and array scans execute synchronously.

Arithmetic and comparisons also have typed built-in names, such as `functions.BuiltInInt64Add` (`int64.add`) and `functions.BuiltInVarCharEqual` (`varchar.equal`). Named lookup and operator lookup return equal handles for the same signature; operator metadata remains available for reasoning. Providers may supply additional non-empty names, while absent names or overloads return `ErrUnsupportedLookup`.

The current catalog includes Boolean NOT and equality, six comparisons for Int64, Float32, Float64 and unbounded VARCHAR, Int64-array contains/contains-any, and checked Int64 arithmetic. There are no implicit width or VARCHAR-bound conversions. Array candidates and elements must be non-null; input arrays may be nullable. Arithmetic overflow outside the verified domain returns `ErrEvaluationUnavailable`.

Use standard-library `errors.Is` to distinguish [error categories](../functions/errors.go). Unavailable execution leaves the expression for later evaluation; semantic errors must not become null or false. The example panics on unexpected errors only to keep its setup concise.

`FunctionSignature.ArgumentCount`/`ArgumentTypeAt` and `FunctionMetadata.NullPolicyAt` support read-only inspection without slice copies. Their bulk getters retain caller-owned slice semantics.

When only admission matters, [`CheckFunctionBinding`](../functions/signature_binding.go) performs binding checks without creating an effective signature or copying its argument slice. [`ProbeFunctionBinding`](../functions/binding_check.go) returns a compact `BindingCheck`; call `Supported`/`Is` while inspecting candidates, and `Err` only when a diagnostic must escape. Result-nullability adaptation can still allocate.

The shared [`errors`](../errors/coded_error.go) submodule supplies error codes and a `CodedError` contract, initially used for signature-binding failures. [`functions.BindingError`](../functions/binding_error.go) formats messages on demand and exposes argument/arity details. Use standard-library `errors.As` for details and `errors.Is` for behavioral classification; a code alone does not describe all wrapped causes. Import `errors` with an alias such as `opterrors` alongside the standard library.

For controlled fixtures, `NewCatalog` and `NewContext` provide inspection and lookup without concrete evaluators; `NewContextWithEvaluators` adds testbed-specific callbacks and policy. Shared contracts expose no registration API. Downstream components depend on `functions` and `functions/interfaces`; their test setup may import `testbed/functions`. A caller can supply its own paired handle/context implementation, as demonstrated by the [testbed handle](../testbed/functions/handle.go) and [context](../testbed/functions/context.go) implementations, without modifying the shared package.

[Back to README](../README.md)
