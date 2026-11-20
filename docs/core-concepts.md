# Core concepts

## Expressions and interpretation

The [runnable external-package examples](../examples/basics/expression_test.go) cover construction, complete interpretation, partial folding and a dedicated double-negation rewriter over a shared subtree:

```sh
go test ./examples/... -run '^Example_' -v
```

- `expressions` provides LiteralExpression, ReferenceExpression, CallExpression, LogicalExpression, SpecialFormExpression (currently IN), and OpaqueExpression. Nodes retain their effective Type and children; sharing requires no parent fields. `ChildCount`/`ChildAt` read immutable children without copying; `Children()` returns a caller-owned slice for rebuilding. Use `expressions/utils.ValidateForOptimization` once to check imported graphs through each call's retained provider.
- `rewriter` supplies typed callbacks and one tree traversal. Embed `BaseExpressionRewriter` in a dedicated rule and override the relevant callbacks. An absent optional requests child traversal; a present replacement finishes that occurrence. `DefaultRewrite` skips the current callback. Replacements preserve the exact Type; the rule is responsible for semantic equivalence. Scheduling and fixpoint iteration belong to orchestration.
- `interpreter` evaluates expressions independently of rewrite rules. Supply an `interpreter/interfaces.Environment` that looks up Values by scoped ColumnID. An absent binding means unavailable; a present typed NullValue means SQL NULL. Complete evaluation requires executable inputs; partial evaluation yields a known Value or residual Expression. Configure an equality FunctionContext to evaluate scalar IN; CallExpressions retain their own paired contexts.
- [`folding_rewriter.FoldingRewriter`](../concrete_rewriters/folding_rewriter/folding_rewriter.go) connects partial interpretation to traversal. Its constructor explicitly selects whether semantic evaluation failures preserve the whole input subtree or propagate. Provider contract errors propagate. Unavailable execution remains residual.

[`expressions.PrettyPrint`](../expressions/pretty_print.go) renders a Unicode tree with child roles, literal payloads and complete Types. It expands shared subtrees at each occurrence, uses local diagnostic getters without function lookup or evaluation, and returns text without a trailing newline. Use `fmt.Println(expressions.PrettyPrint(root))` to display it. Existing `String()` diagnostics remain available.

The [pretty-print example](../examples/basics/pretty_print_test.go) displays nullable `x IN [3, 1, 3] AND x != 1` before extraction and its `x == 3` candidate after reconstruction. Both trees retain a nullable Boolean result; the printer only renders the structures. Its expected output is checked by Go's example test runner:

```sh
go test ./examples/... -run '^Example_prettyPrintReconstruction$' -v
```

## Scalar analysis

The [runnable external-package examples](../examples/scalar_analysis/scalar_analysis_test.go) use only public APIs to connect extraction, scoped reasoning, reconstruction and independent interpretation:

```sh
go test ./examples/... -run '^Example_scalar' -v
```

| Example | What it demonstrates |
| --- | --- |
| `Example_scalarStatusIntersection` | Intersect two VARCHAR status lists, reconstruct the singleton as equality, and compare interpretation with the source. |
| `Example_scalarNullableContradiction` | An empty non-null true set can still produce NULL; reconstruction retains the source, and a null-only input domain yields KnownTruth(Null). |
| `Example_scalarGlobalRange` | Summarize `x < 10 AND x != 3`, reconstruct its ranges, verify evaluation and report a larger candidate while retaining the compact source. |
| `Example_scalarRangeGuard` | Restrict inputs by a retained range guard and prove a nested comparison true, preserving the guard dependency and the reference's nullable declaration. |
| `Example_scalarUnsupportedInput` | Preserve an unsupported whole predicate while analyzing a supported child independently. |

`analysis/fact_extraction.ScalarExtractor` returns an Exact summary or an Unsupported result retaining the original expression. Valid immutable input is a precondition; encountered provider and operational failures return errors. `analysis/facts.PredicateSummary` contains an exact non-null true set, a separate null-input outcome and the original result Type. The finite/range representations live under `analysis/value_set`.

[Scoped reasoning](../analysis/scoped_reasoning/README.md) consumes summaries and explicit input domains. A known NULL outcome, an absent uniform proof and unreachable inputs remain distinct. Restrictions retain their condition premise; a conditional conclusion does not authorize deleting that premise or replacing the whole expression globally.

[Reconstruction](../analysis/reconstruction/README.md) consumes an exact summary and a target FunctionContext, returning an optional equivalent candidate. Unsupported representation/null/type cases leave the source available; provider failures remain errors. `MeasureExpression` reports node occurrences, literal occurrences and IN list entries, counting shared subtrees at each use. These counts help callers compare candidates without imposing an acceptance policy.

The examples share small construction helpers and the expression examples' binding environment. They panic on unexpected setup errors to keep the API flow visible; their `Output` comments are checked by `go test`. Production analysis packages do not import the testbed or concrete rewriters. Scoped reasoning and reconstruction consume facts independently of extraction and of each other.

Run the [touchstone scalar-proof tests](../testbed/touchstone/tests/scalar_analysis_test.go) over the original fixture tree:

```sh
go test ./testbed/touchstone/tests -run 'TestTouchstone.*ScalarProofs' -v
```

They prove tsa_04 disjointness and all four nested article-membership containments under the retained outer article guard. All conclusions retain that guard dependency and the original target occurrence. The nullable-article test obtains the same conditional proofs but demonstrates why they cannot justify the proposed whole-expression replacement: with NULL article_type and tag 8791004, the input returns Null and the proposed output returns False. The test applies no rewrite.

[Back to README](../README.md)
