# End-to-end example

[Example_touchstoneOptimization](../examples/rewriting/touchstone_optimization_test.go) derives the complete primary touchstone result with five explicit public passes: contextual rewriting, Boolean cleanup, logical flattening, array normalization and array exclusion grouping. It prints stage sizes (111 → 65 → 64 → 63 → 63 → 39 nodes), the generated final Unicode tree, equality with the independently assembled Expected and preservation of the exact result Type.

Run from the repository root:

```sh
go test ./examples/... -run '^Example_touchstoneOptimization$' -v
go test -tags debug ./examples/... -run '^Example_touchstoneOptimization$' -v
```

Go checks the example's `Output` comment against its printed result; successful example output is suppressed by the test runner even with `-v`. Read the final tree directly in that comment.

This example uses non-null article_type and nullable remaining fields with the standard testbed provider. For nullable article_type, conditional exclusions must remain; the [composition tests](../concrete_rewriters/array_exclusion_grouping_rewriter/tests/touchstone_test.go) verify that variant, both array-pass orders and independent per-stage evaluation. The explicit schedule illustrates this five-pass pipeline; general orchestration and work budgets remain open.

[Touchstone fixtures](../testbed/touchstone/touchstone.go) construct the original and proposed output independently. Their [evaluation tests](../testbed/touchstone/tests/evaluation_test.go) compare explicit expected results, including the nullable-article counterexample. The five-pass schedule in this example derives the primary output from the original fixture. General orchestration remains separate from this completed integration. These checks are fixture evidence, not an exhaustive proof or live Milvus validation.

[Back to README](../README.md)
