# Array exclusion grouping rewriter

The public rule groups compatible direct array exclusions in post-order, preserving exact Types and operand order.

## Current transformation

The rule combines compatible direct exclusions within an AND:

```sql
NOT array_contains(tags, 8791003)
AND NOT array_contains(tags, 8791004)
→ NOT array_contains_any(tags, [8791003, 8791004])
```

The matcher also accepts nonempty literal contains-any exclusions. Candidate lists concatenate in source order, preserving duplicates. The grouped exclusion occupies its first member's position; unrelated operands retain their relative order. A separately scheduled flattening pass can expose additional direct AND operands.

The rule is generic over admitted non-null element Types. Int64 provides standard-testbed examples. Exact nested constraints, matching membership semantics and paired providers remain necessary for every supported element Type.

## Current public use

```go
rule := array_exclusion_grouping_rewriter.NewArrayExclusionGroupingRewriter(factory)
tree := rewriter.NewExpressionTreeRewriter[struct{}](rule)
result, err := tree.Rewrite(input, struct{}{})
```

The [runnable public-API example](../../examples/rewriting/array_exclusion_grouping_test.go) prints Unicode trees for nullable exclusions, preserves `[8791003, 8791004, 8791003]` including its duplicate, and checks Bool?, node reduction, second-pass identity and hit/miss/empty/NULL evaluation. From the repository root:

```sh
go test ./examples/... -run '^Example_arrayExclusionGrouping$' -v
go test -tags debug ./examples/... -run '^Example_arrayExclusionGrouping$' -v
```

The example uses one paired provider and stable bindings. The [retained-producer fixture](tests/evaluation_behavior_test.go), `TestComputedAndOpaqueArraysRemainUngrouped`, demonstrates the boundary for arbitrary producers and opaque arrays. Measured limitations include candidate-container copying and nonempty large-list evaluation costs.

The constructor retains no FunctionContext or mutable attempt state and performs no provider queries. Each logical callback first uses DefaultRewrite once, then considers the rebuilt AND's direct exclusions. Other variants inherit ordinary traversal. Calls retain their paired contexts for recognition, lookup and reconstruction; rewriting performs no evaluation.

Callers establish valid immutable input through controlled construction preserving graph-wide identity/Type consistency, or expressions/utils.ValidateForOptimization once at entry. Required objects are non-nil. The rule preserves valid equivalent output or returns an error under its admitted contracts. Recognition trusts stored-call validity while new calls use checked construction. Unsupported regions retain their rebuilt form, including accepted descendant changes.

## Grouping's execution contract

[Singleton normalization](../array_membership_normalization_rewriter/README.md) retains one evaluation of its array input. Grouping replaces several reads and membership invocations with one. That reduction needs stable, successful, effect-free membership implementations and reference lookup on complete valid inputs, including the specified null behavior. Matching metadata identifies the intended family but cannot prove execution availability or safety.

The initial grouping design admits ReferenceExpression array operands and shared LiteralExpression instances. Arbitrary array-producing calls and opaque arrays remain outside grouping admission because structural equality or sharing does not establish safe elimination of repeated computation. Compatible members must also use the same admitted provider context. These conditions govern local grouping admission.

Nullable arrays remain in scope: separate and grouped exclusions both produce NULL for NULL input and retain Bool?. Missing bindings during partial evaluation stay indeterminate. Unrelated computations keep their relative order and failure behavior under the admitted contract. The rule performs no evaluation during rewriting.

Schedule grouping as a completed pass outside active scoped attempts. General pipeline control, revisits and work budgets remain separate responsibilities. The final touchstone is verified under both explicit pass orders below; singleton normalization remains independently tested.

## Direct exclusion recognition

[Private admission](admission.go) now inspects direct AND children and returns ordered occurrence records. For example:

```text
AND(flag, NOT contains(A, 1), AND(...), NOT contains_any(A, [2, 1, 2]))
  occurrence index 1: array A, candidates [1]
  occurrence index 3: array A, candidates [2, 1, 2]
```

The collector does not descend into the nested AND. Public tree traversal visits descendants separately before collecting the rebuilt region. Repeated shared exclusions produce separate occurrence records. Each record retains its original NOT/call, array and paired context; its candidate slice owns the container and shares immutable Values.

Shape checks admit reference/literal arrays with non-null element Types and non-null literal candidates. For contains, a candidate has the exact element Type E; E itself may be an array, so even an empty array-valued element is one valid candidate. Contains-any instead requires a nonempty literal Array<E!>!. Nullable outer input arrays and known-NULL array literals remain eligible. Shared-literal identity and compatibility between occurrences are checked by the grouping helpers.

A shape miss makes no provider queries. A recognized shape reads metadata once through its call's context, then checks the matching built-in family with the shared helper. Unsupported families remain absent; metadata errors propagate with causes and invalidate the whole collected result. Recognition performs neither lookup nor evaluation and trusts established stored-call validity. Its [private-stage tests](admission_test.go) retain focused helper-level coverage without testing exports; public tests now exercise the assembled rule.

## Compatible occurrence groups

[Private grouping](grouping.go) indexes admitted occurrences by safely comparable provider identity and canonical column key or literal-node identity. Ordered partition indices preserve first-seen order, and each matching bucket still checks full Type and operand identity. For example, with a nonmember at position 2:

```text
0: NOT contains(x, 2)
1: NOT contains(y, 9)
2: unrelated operand
3: NOT contains_any(x_alias, [1, 2])
4: NOT contains(y, 7)
5: NOT contains(z, 4)
6: the same exclusion instance as position 0

returned groups:
  x: positions [0, 3, 6], candidates in member order [2, 1, 2, 2]
  y: positions [1, 4],    candidates in member order [9, 7]
```

Here x_alias has the same ColumnID and full Type as x. Diagnostic names do not control grouping. The z singleton remains outside returned replacement groups. Candidate Values are neither compared nor deduplicated, and candidate slices stay on each member until replacement construction builds one aggregate. The first member supplies the replacement position, array and paired context.

Contexts must compare equal after both dynamic values pass reflect.Value.Comparable. A struct with an interface field holding a slice is not safely comparable even if reflect.Type.Comparable reports true. Such contexts remain ungrouped. Different wrappers sharing a provider stay distinct; handles may be non-comparable because grouping never uses them as keys or Go equality operands.

Array references use ColumnID.EqualsTo plus full Type equality, including nested constraints. Literal arrays require the same LiteralExpression node; sharing a Value between two literal nodes does not establish a match. Grouping performs no provider queries, owns its member slices and preserves input expressions. The [private tests](grouping_test.go) cover these boundaries without adding testing exports. Checked replacement construction and public publication are described below.

## Checked group replacement

[buildGroupReplacement](replacement.go) now constructs a complete negated contains-any candidate for one compatible group. For the x group above, the candidate is:

```sql
NOT array_contains_any(x, [2, 1, 2, 2])
```

The builder first sums member candidate counts and concatenates their ordered Values into one aggregate buffer. ArrayValue and LiteralExpression construction use the existing public APIs; the array container is copied while immutable element Values and the anchor's array expression remain shared. Composite array-valued candidates retain their nesting rather than being flattened.

The anchor's paired context resolves contains-any using the actual input-array Type and non-null candidate-array Type. One metadata read recognizes the target family; NewCallExpression performs the sole binding check with its normal second metadata read. Logical NOT is then constructed, and the result must have the exact Type of every member exclusion. No source call is rebound or evaluated.

Only explicit unsupported target lookup is a whole-group miss. Unsupported target metadata also declines locally, while lookup failures with stronger categories, metadata errors and checked-construction errors propagate with causes intact. A wrong-width overload can have valid contains-any metadata yet fail the new invocation's binding; that failure is not treated as a lookup miss. Foreign target handles likewise remain errors. No partial candidate escapes, and the immutable source group remains available for retry.

The [private tests](replacement_test.go) cover order, multiplicity, copied container ownership, nullable references/literals, known NULL, nested elements, exact Types and phase-specific failure/retry behavior. This helper returns a candidate only; region publication applies the additional acceptance checks below.

## Ordered publication and acceptance

[RewriteLogical](array_exclusion_grouping_rewriter.go) processes descendants once, then [publication](publication.go) prepares local replacements before assembling a result. Successful groups replace their first occurrence; their later occurrences disappear. For the x/y example above, the resulting operand order is:

```text
AND(grouped_x, grouped_y, unrelated_operand, NOT contains(z, 4))
```

An unsupported group keeps all its rebuilt members in place while independent groups can succeed. A propagated error aborts the public result even after earlier groups were prepared. A local miss returns the rebuilt region, preserving accepted descendant rewrites and avoiding a second traversal.

Candidate-position accounting compares the source member counts with the generated aggregate counts for successful groups. Other subtrees are retained, so this establishes region-wide payload-position non-growth without metadata queries on unrelated expressions. The check includes an Elements() slice copy of each new aggregate. Whole-region acceptance also checks exact Type and expression-node non-growth. If only one operand survives, it replaces the AND only when its full Type matches. No flattening or deduplication is performed.

The [public tests](tests/publication_test.go) verify exact independently assembled trees, one visit per reference occurrence, ordering, shared inputs, nullable Types, graph validity, reduced node counts and unchanged second-pass identity. They also cover child grouping beneath an enclosing call or a region without a local match, unsupported groups beside successful ones, and failure/retry after tentative work. Independent semantic and execution evidence is described below.

## Public admission and provider boundaries

The [admission tests](tests/admission_test.go) validate each source graph before running the public rule. A valid call need not belong to the recognized membership family: empty/dynamic/NULL/nullable candidate payloads, differing element Types, nonstandard metadata and unsupported expression shapes are retained. Each case also appears beside an independent supported group, demonstrating local preservation while other work succeeds. Shape misses make no provider queries; recognized shapes that fail metadata admission make exactly one metadata query per occurrence. No evaluation or source overload lookup occurs.

A non-Boolean call cannot be the direct operand of logical NOT, so its test uses a valid IN wrapper. This preserves the distinction between malformed input and a valid expression outside the rule's scope. Generic grouping tests use metadata-only providers across primitive and composite element Types; execution is not an admission test.

The [pairing tests](tests/pairing_test.go) check equal comparable context values, distinct contexts sharing a provider, genuinely non-comparable contexts, and comparable structs whose interface fields contain non-comparable values. Non-comparable handles remain supported. Reference aliases can group; matching diagnostic names alone cannot. Literals group only when they are the same expression node, including known-NULL arrays. An empty array-valued element is one contains candidate, not an empty contains-any list.

For the exact/case-insensitive providers below, grouping across contexts would change this expression's result:

```sql
-- A = ["TAG"]
NOT exact.array_contains(A, "tag")
AND NOT case_insensitive.array_contains(A, "tag") -- FALSE

-- Incorrectly aggregating into the first context:
NOT exact.array_contains_any(A, ["tag", "tag"]) -- TRUE
```

The public rule retains the two original exclusions even though both contexts share handles and metadata. The fixture evaluates this counterexample separately from rewriting.

## Provider errors, rollback and retry

The [public provider-boundary tests](tests/provider_boundaries_test.go) inject failures after source construction and entry validation. Successful metadata stays immutable; only operation success or the handle returned by target lookup changes.

| Phase | Failure behavior |
| --- | --- |
| Source metadata | Propagate the error; publish no tree. |
| Explicit contains-any lookup | Retain the group for UnsupportedLookup alone, including wrapped errors and joined diagnostic details. A joined stronger category propagates. |
| Target metadata | Propagate the error, including UnsupportedLookup. Unsupported metadata family is a local miss. |
| Checked replacement construction | Propagate metadata/binding errors, including UnsupportedLookup from an incompatible returned overload. |
| Enclosing call reconstruction | Propagate the error even after its child was successfully grouped. |

For a two-member contains group, the observable provider sequence is:

```text
source metadata × 2
  → target lookup
  → target metadata
  → checked construction metadata
  → enclosing call metadata (if a parent call must be rebuilt)
```

The tests assert the precise prefix at each failure and the complete sequence after retry. Evaluation-related error categories supplied by a provider are preserved like other errors; the rewriter never invokes Evaluate to probe eligibility.

For interleaved groups `AND(a1, b1, a2, b2)`, preparing `group_a` does not publish it. A later failure while preparing `group_b` returns an error and no tree. If b's explicit lookup is merely unsupported, publication instead yields `AND(group_a, b1, b2)`. Removing the fault and rewriting the original input yields `AND(group_a, group_b)` with no retained attempt state.

Foreign target handles fail metadata lookup, and wrong-width overloads fail checked binding. A nullable implementation result is already outside the recognized target family; it declines before replacement construction. With valid stable metadata, admitted source and target signatures derive the same effective result nullability, so tests exercise that reachable boundary without fabricating a Type-inconsistent source. Exact-Type checks remain in replacement construction and publication.

## Paired caller-side fixtures

The [fixture builders](tests/fixtures_test.go) construct source exclusions and expected groups independently. For example, the matrix evaluates each contains/contains-any call, the source below, and its proposed group against expected truth and full Types:

```sql
NOT array_contains(A, v) AND NOT array_contains_any(A, [v, v])
-- Independently constructed expected group:
NOT array_contains_any(A, [v, v, v])
```

These fixtures establish executable contracts independently of rewriting; the public publication tests separately exercise actual grouping. Candidate duplicates are retained.

The 72 fixtures cover both array nullabilities and three provider/handle representations, using all integer widths, Boolean, bounded VARCHAR, float32/64, NaN, signed zero, nested arrays and structs. Int64 uses the standard provider. Alternative evaluators use IEEE equality for float elements and typed structural equality otherwise; checked argument/result binding and NULL propagation remain in the testbed context. These declared relations are fixture semantics, not live Milvus compatibility evidence.

[Provider fixtures](tests/provider_fixtures_test.go) record lookup, metadata and evaluation operations, inject operation failures without changing successful metadata, and support clean retry. A slice-bearing handle delegates semantic equality through its paired owner; a slice-bearing context remains genuinely non-comparable in stored calls. Environments log ColumnID lookups and distinguish absent bindings, present typed NULL and errors.

The shared-catalog test makes the provider distinction concrete:

| Context | Catalog/handles/metadata | Matching relation | Contains(["TAG"], "tag") |
| --- | --- | --- | --- |
| Exact | Shared | Exact text equality | False |
| Case-insensitive | Shared | Case-insensitive text equality | True |

Both membership families use the selected relation consistently, and both contexts accept the same handles. Grouping across these contexts would require evidence beyond handle or metadata identity. Foreign owner-wrapped handles are rejected separately.

## Independent semantic evidence

The [semantic tests](tests/semantics_test.go) run the public rewriter and compare both source and generated expressions with independently determined truth. Integer expectations use raw int64 slices and a separate T/F/NULL truth table, without invoking the interpreter, function evaluator or production TruthValue operations to compute the answer. There are 700 integer row scenarios covering bounded exhaustive arrays, empty/hit/miss/duplicate inputs, negative/zero/extreme candidates, mixed scalar/list exclusions and nullable siblings.

For example:

```sql
NOT array_contains(A, 0)
AND flag
AND NOT array_contains_any(A, [-1, 1, 1])
AND NOT array_contains(A, 0)
-- Grouped:
NOT array_contains_any(A, [0, -1, 1, 1, 0]) AND flag
```

| A | flag | Expected truth for both forms |
| --- | --- | --- |
| [] | TRUE | TRUE |
| [42, 42] | NULL | NULL |
| [1] | NULL | FALSE |
| NULL | FALSE | FALSE |
| NULL | TRUE | NULL |

When A is nullable, or the nullable flag is present, both expression and evaluated Value have Bool? Type even when a particular row produces TRUE or FALSE. With non-nullable A and no sibling, they retain Bool!.

A further 154 scenarios use explicit expected truth under the generic fixtures' declared relations. They cover primitive elements, nested arrays/structs, an empty array-valued element, and an array of structs containing arrays. Under the IEEE float fixture, NaN never matches and signed zeros match. The generated candidate sequence nevertheless retains all NaNs and keeps `[-0, +0, -0]` in order. These fixtures establish caller-side contracts; they do not assert live Milvus composite membership semantics.

Every tested group reduces expression-node occurrences while preserving ordered candidate Value identities and their count, the full candidate-array Type, the original array operand, and any unrelated sibling. Checking identities separately from semantic truth catches accidental deduplication, sorting or flattening of composite elements.

## Execution changes and retained failures

The [execution tests](tests/evaluation_behavior_test.go) share a trace between the function provider and reference environment. Normalization and grouping perform no FunctionContext.Evaluate or Environment.Lookup. They may query metadata and resolve target overloads. A provider with metadata but no implementation can therefore support rewriting while later interpretation still reports evaluation unavailable; metadata is not an execution-eligibility probe.

With stable, successful reference lookup and membership evaluation, three exclusions perform three array reads and three membership invocations before grouping, then one of each afterward. Shared literals require no reference lookup. Known-NULL arrays preserve their NULL results; the trace counts calls to the provider's Evaluate boundary, including those handled by null propagation.

For two interleaved groups with unrelated calls, the trace changes as follows (each `a` or `b` includes the corresponding reference read and membership call):

```text
Before: a1 → first → b1 → a2 → second → b2 → third → outer
After:  group_a → first → group_b → second → third → outer
```

Each aggregate occupies its first member's position. The unrelated calls keep their relative order, and an enclosing call executes after its arguments. Tests make successive unrelated calls fail, including multiple pending failures, and require the same first error and trace prefix for both forms. The prototype interpreter evaluates logical operands eagerly: even a preceding FALSE does not hide a later failure. This is evidence for the prototype's execution policy, not a claim that every upstream engine uses it.

Array-producing calls remain outside admission, even when shared or structurally equal. Their repeated executions, changing values and first/second failures remain intact. Opaque arrays likewise remain unchanged and retain unavailable-evaluation errors.

The tests also make the stable/successful reference precondition concrete:

```sql
NOT array_contains(A, 1) AND NOT array_contains(A, 2)
-- Grouped: NOT array_contains_any(A, [1, 2])
```

- If successive reads return `[2]` then `[1]`, the source yields TRUE; the grouped expression reads `[2]` once and yields FALSE.
- If the first read returns `[]` and the second fails, the source fails; the grouped expression reads once and yields TRUE.

These are deliberate violations of the caller contract. Graph validity and reference identity do not prove stable, successful execution. The rewriter relies on that contract rather than executing the input to infer it.

## Residual completion, traversal and sharing

The [partial-evaluation tests](tests/partial_evaluation_test.go) distinguish missing bindings from known SQL NULL. With A missing, both the original exclusions and grouped contains-any remain residual expressions even when A is declared non-nullable. A known Boolean sibling can become a literal while A stays unresolved; the interpreter does not erase that residual merely because the sibling is FALSE. Completing the original/generated residuals with empty, matching, nonmatching, repeated and permitted NULL rows preserves the same oracle truth and exact Bool! or Bool? Type. Residual syntax need not match.

| Available input | Partial result |
| --- | --- |
| A missing | Array reference remains in a residual expression. |
| A missing, flag known | Flag folds independently; array reference remains. |
| A is known NULL, no missing sibling | A known typed NULL result. |
| A is known NULL, flag missing | Array computation folds to NULL; flag remains residual. |

The [traversal tests](tests/traversal_test.go) observe every callback occurrence in order, including repeated edges to shared subtrees. Accepted child groups survive enclosing call, IN and logical reconstruction. Fresh equal-ID references group while unrelated nodes, the retained array operand and candidate Values keep their identities. Source snapshots and mutations of returned child/element slices verify immutable storage; no testing exports are needed.

Grouping itself keeps nested AND boundaries:

```text
AND(exclusion_1, AND(exclusion_2, flag))
  → grouping: unchanged
  → explicit flattening: AND(exclusion_1, exclusion_2, flag)
  → grouping: AND(grouped_exclusion, flag)
```

A second grouping pass preserves root identity. Subsequent singleton normalization also leaves grouped lists intact, including `[1, 1, 1]`; repeated normalization/grouping produces no cycle. Candidate deduplication remains outside the current grouping rule.

## Final touchstone composition

The [composition tests](tests/composition_test.go) schedule completed public passes explicitly:

```text
contextual rewriting → Boolean cleanup → logical flattening
  → singleton normalization → array exclusion grouping

contextual rewriting → Boolean cleanup → logical flattening
  → array exclusion grouping → singleton normalization
```

Each pass finishes before the next begins. No optimizer driver or active scoped-attempt mutation is added. Every stage retains exact Type, passes graph validation, avoids expression-node growth and leaves earlier trees unchanged. The [touchstone tests](tests/touchstone_test.go) verify both schedules against independently built expected expressions.

| article_type | Final nodes | Literal occurrences | IN entries | Tag grouping |
| --- | ---: | ---: | ---: | --- |
| Non-nullable | 39 | 22 | 19 | All seven exclusions, matching Touchstone.Expected exactly. |
| Nullable | 102 | 55 | 46 | Only the three unconditional exclusions; conditional clauses remain. |

For the primary schema, category 8792001 and mall 8793001 use scalar contains calls. The final tag exclusion is:

```sql
NOT array_contains_any(tag_ids,
    [8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009])
```

For nullable article_type, the independent expected tree retains the guarded clauses and groups only `[8791003, 8791006, 8791009]`. With article_type NULL and tag 8791004 present, the actual result stays NULL; the primary proposed output would be FALSE. Grouping does not broaden scalar guard inference.

An independent oracle evaluates the compact input using ordinary strings/integers and its own three-valued operations. Across 10,316 row/schedule scenarios, it agrees with the source, every generated stage and the appropriate expected tree. Rows cover both nullability choices for article and other fields, every exclusion, review/brand restrictions, duplicate/empty arrays and combinations of nullable fields. Final call-family and ordered-payload assertions establish that the intended transformations occurred, beyond merely comparing counts.

## Measured work and limitations

[Grouping benchmarks](tests/grouping_benchmark_test.go) measure rewrite costs, source and grouped evaluation, and candidate distributions. Run from the repository root:

```sh
go test ./concrete_rewriters/array_exclusion_grouping_rewriter/tests -run '^$' -bench 'BenchmarkArray(Grouping|ContainsAny)' -benchmem
```

[Index regression tests](tests/index_test.go) count identity comparisons independently of timing. Ordered output, full-Type checks, and safely comparable context identity remain required.

Fewer nodes or calls do not guarantee faster evaluation. Candidate copying and membership costs depend on list sizes and duplicates. Measure both rewriting and evaluation on representative inputs. The acceptance contract is equivalence, exact Type, and no structural or candidate growth; it is not a latency guarantee. Cost-based acceptance, evaluator preparation, and reduced copying remain open.

## Current evidence and files

| File | Responsibility |
| --- | --- |
| [array_exclusion_grouping_rewriter.go](array_exclusion_grouping_rewriter.go) | Public constructor, interface conformance and post-order RewriteLogical callback. |
| [admission.go](admission.go) | Direct-AND collection and semantic recognition with ordered occurrence records. |
| [grouping.go](grouping.go) | Ordered private partitioning by provider, full Type and array identity; singleton partitions remain untouched. |
| [replacement.go](replacement.go) | One ordered aggregate, paired target lookup, checked call/NOT construction and exact member-Type acceptance. |
| [Private replacement tests](replacement_test.go) | Payload ownership, query sequence, composite elements, unsupported targets, wrong overloads and error/retry boundaries. |
| [Private grouping tests](grouping_test.go) | Interleaved occurrences, duplicates, reference/literal identity, nested Types, safe context comparability and non-comparable handles. |
| [Private admission tests](admission_test.go) | Shape-query boundaries, occurrence identity, composite candidates, metadata failures and retry. |
| [Unchanged-tree tests](tests/array_exclusion_grouping_rewriter_test.go) | Thirty shape-miss cases across both array nullabilities and representative variants, including positive membership groups, nesting and shared occurrences. |
| [publication.go](publication.go) | Whole-region ordered assembly, exact Types and node/candidate-position acceptance. |
| [Public publication tests](tests/publication_test.go) | Actual grouping, interleaved groups, one traversal per occurrence, local misses, failures and retry. |
| [Public admission tests](tests/admission_test.go) | Valid unsupported inputs, independent supported groups, exact query boundaries and generic element grouping. |
| [Public pairing tests](tests/pairing_test.go) | Context comparability, shared-catalog counterexample, non-comparable handles and reference/literal identity. |
| [Provider-boundary tests](tests/provider_boundaries_test.go) | Phase-specific wrapped/joined errors, returned overloads, enclosing reconstruction, rollback and retry. |
| [Semantic tests](tests/semantics_test.go) | Independent integer/three-valued oracle, declared generic truth, exact expression/Value Types and candidate/node counts. |
| [Execution tests](tests/evaluation_behavior_test.go) | Provider/read traces, multiple groups, retained unrelated and producer failures, opaque inputs and negative reference contracts. |
| [Partial-evaluation tests](tests/partial_evaluation_test.go) | Missing versus known NULL, independent sibling folding, residual completion and exact Types. |
| [Traversal tests](tests/traversal_test.go) | Ordered occurrence visits, immutable sharing, nested boundaries, explicit flattening and normalization stability. |
| [Composition tests](tests/composition_test.go) | Explicit public pass orders, per-stage validation/immutability and final stability. |
| [Touchstone tests](tests/touchstone_test.go) | Exact final trees, independent row oracle, payload/count checks and nullable-article counterexample. |
| [Index tests](tests/index_test.go) | Deterministic comparison counts, first-member identity and ordered payloads. |
| [Benchmarks](tests/grouping_benchmark_test.go) | Separate rewrite/evaluation work, allocations, queries and candidate payload sizes. |
| [Runnable example](../../examples/rewriting/array_exclusion_grouping_test.go) | Nullable before/after trees, duplicate preservation, independent row evaluation, exact Types and second-pass identity. |
| [Typed fixtures](tests/fixtures_test.go) | Literal/reference/exclusion builders, independently assembled expected groups, complete/missing/NULL environments and executable generic catalogs. |
| [Provider fixtures](tests/provider_fixtures_test.go) | Recording/faulting contexts, non-comparable handles and non-comparable contexts. |
| [Fixture contract tests](tests/fixtures_contract_test.go) | Generic membership truth, partial inputs, shared-catalog execution differences, failure/retry and foreign-handle rejection. |
| [Shared membership recognition](../internal/array_membership/metadata.go) | Generic metadata-family helper, used by normalization and grouping admission. |
| [Shared lookup classification](../internal/array_membership/lookup_error.go) | Explicit-target miss classification with stronger-error precedence. |

Unchanged-tree tests construct and validate shape-miss fixtures while their providers are available, then disable all provider methods before constructing the rule and traversing. Root identity, exact Types, child slices, diagnostic content, shared expressions/Values and second-pass identity remain unchanged. These tests establish the shape-miss boundary; publication tests permit metadata/lookup while forbidding evaluation.

From the repository root:

```sh
go test ./concrete_rewriters/array_exclusion_grouping_rewriter/...
go test -tags debug ./concrete_rewriters/array_exclusion_grouping_rewriter/...
```

General scheduling, revisits, configurable bounded-work fallback, broader array rules, and live Milvus validation remain open.
