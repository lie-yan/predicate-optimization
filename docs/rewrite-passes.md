# Rewrite passes

Each pass is invoked explicitly. The caller controls pass order and repetition.

## Contextual rewriting and Boolean cleanup

Concrete rules have individual subpackages under `concrete_rewriters/`, each with its public API tests in a local `tests/` folder. The [scoped contextual rule](../concrete_rewriters/scoped_contextual_rewriter/README.md) publishes complete guarded reductions. The [Boolean cleanup rule](../concrete_rewriters/boolean_cleanup_rewriter/README.md) independently simplifies Boolean structure in post-order and can run afterward as an explicit second pass. Both reuse local cleanup and eligibility checks from `concrete_rewriters/internal/boolean_cleanup/`.

The [runnable scoped rewriting example](../examples/rewriting/scoped_contextual_rewriting_test.go) composes the public rules in two explicit passes:

```text
x IN [1, 2] AND NOT(x == 3)
  → x IN [1, 2] AND True       (contextual rewriting)
  → x IN [1, 2]                (Boolean cleanup)
```

Run it from the repository root:

```sh
go test ./examples/... -run '^Example_scopedContextualRewriting$' -v
```

Its checked output prints the input, accepted intermediate and cleaned trees, then independently evaluates all three for x=1, 2, 3 and 4. Each rule is wrapped in `ExpressionTreeRewriter[struct{}]`; each Rewrite call makes one traversal. A successful contextual replacement finishes its occurrence, and cleanup is scheduled by the caller. Repetition to a fixed point is a separate orchestration decision.

## Logical flattening

The [runnable example](../examples/rewriting/logical_flattening_test.go) flattens `(a AND b) AND (c OR False)` into `AND(a, b, OR(c, False))`, prints both trees, checks exact Type and second-pass identity, and independently evaluates complete bindings including known NULL:

```sh
go test ./examples/... -run '^Example_logicalFlattening$' -v
```

[`NewLogicalFlatteningRewriter`](../concrete_rewriters/logical_flattening_rewriter/logical_flattening_rewriter.go) takes no FunctionContext argument. Run it through `ExpressionTreeRewriter[struct{}]` as one explicitly scheduled traversal. It flattens matching AND/OR groups while preserving operand order, duplicates and opposite-operator boundaries. One pass suffices for this rule alone; an unchanged second pass returns the original instance. Subsequent rules may expose additional groups, so broader scheduling remains the caller's responsibility.

Schedule flattening before scoped attempts begin or after accepted results are published, because grouping changes occurrence paths. Ordinary reconstruction of an enclosing CallExpression can still query that call's own paired FunctionContext; flattening performs no evaluation. The [package README](../concrete_rewriters/logical_flattening_rewriter/README.md) explains the algorithm, verified compositions and measured costs.

## Array membership normalization

The [runnable example](../examples/rewriting/array_membership_normalization_test.go) converts `NOT array_contains_any(category_ids, [8792001])` to `NOT array_contains(category_ids, 8792001)` for a nullable array. It prints both trees with PrettyPrint, preserves Bool?, checks second-pass identity, and independently evaluates hit/miss/empty/NULL rows:

```sh
go test ./examples/... -run '^Example_arrayMembershipNormalization$' -v
go test -tags debug ./examples/... -run '^Example_arrayMembershipNormalization$' -v
```

[`NewArrayMembershipNormalizationRewriter`](../concrete_rewriters/array_membership_normalization_rewriter/array_membership_normalization_rewriter.go) takes no arguments. Use the public `ExpressionTreeRewriter[struct{}]`; source metadata, target lookup and checked construction all use each call's existing paired FunctionContext. Providers must supply stable, successful, effect-free contains/contains-any implementations with matching element equality and admitted null propagation. Metadata recognition alone cannot establish those execution guarantees. The standard testbed provides the example's Int64 overloads; the rule supports any admitted element Type.

Accept the canonical call form even though replacing an array literal with one scalar literal leaves node/literal/IN-entry counts unchanged. Rewriting performs no evaluation. Run each pass before scoped attempts start or after their results are published; scheduling and revisits remain explicit caller decisions. A second unchanged normalization pass retains root identity.

Candidate inspection uses `ArrayValue.ElementCount`/`ElementAt`, avoiding an element-slice copy and rejecting non-singleton arrays by count. Provider metadata, lookup and checked construction still contribute work in addition to expression traversal. The [package README](../concrete_rewriters/array_membership_normalization_rewriter/README.md) documents admission, errors, composition and touchstone coverage; tag-exclusion grouping remains separate.

## Array exclusion grouping

The [runnable example](../examples/rewriting/array_exclusion_grouping_test.go) groups two exclusions on nullable tag_ids into one NOT contains-any call. It prints before/after Unicode trees, retains the duplicate candidate, reduces expression nodes from 9 to 4, verifies Bool? and second-pass identity, and independently evaluates hit/miss/empty/NULL rows:

```sh
go test ./examples/... -run '^Example_arrayExclusionGrouping$' -v
go test -tags debug ./examples/... -run '^Example_arrayExclusionGrouping$' -v
```

[`NewArrayExclusionGroupingRewriter`](../concrete_rewriters/array_exclusion_grouping_rewriter/array_exclusion_grouping_rewriter.go) takes a caller-supplied `ValueFactory` and runs through `ExpressionTreeRewriter[struct{}]`. Members must use the same safely comparable FunctionContext and compatible full Types; sharing a handle or catalog alone is insufficient. Array operands are references with the same ColumnID, or the same immutable literal node. Providers supply matching contains/contains-any semantics, and array-reference lookup must be stable, successful and effect-free. Grouping eliminates repeated reads and invocations, so its execution contract is stronger than singleton normalization, which retains one input evaluation. Metadata recognition does not establish these runtime properties.

The output concatenates candidates in source order, preserving duplicates, and places each grouped exclusion at its first member. Direct AND children form the grouping boundary; explicit logical flattening can expose further opportunities. Schedule complete passes outside active scoped attempts. Both normalization → grouping and grouping → normalization are verified after contextual rewriting, Boolean cleanup and flattening; broader orchestration and revisits remain caller decisions.

Arbitrary array-producing calls and opaque arrays remain ungrouped. The [execution fixtures](../concrete_rewriters/array_exclusion_grouping_rewriter/tests/evaluation_behavior_test.go), especially `TestComputedAndOpaqueArraysRemainUngrouped`, demonstrate retained repeated producers and errors. The [package README](../concrete_rewriters/array_exclusion_grouping_rewriter/README.md) explains admission and the completed primary touchstone. Node reduction is not a general latency guarantee, and nonempty large-list workloads can still regress. Use the [grouping benchmarks](../concrete_rewriters/array_exclusion_grouping_rewriter/tests/grouping_benchmark_test.go) to measure your workload.

[Back to README](../README.md)
