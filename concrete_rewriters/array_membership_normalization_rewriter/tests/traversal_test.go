package array_membership_normalization_rewriter_test

import (
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Observe public callbacks while retaining the real rule's traversal decisions.
// Embedding supplies other typed callbacks; calls explicitly delegate to the rule.
type observingNormalizer struct {
	*array_membership_normalization_rewriter.ArrayMembershipNormalizationRewriter
	references []*expressions.ReferenceExpression
	opaques    []*expressions.OpaqueExpression
	completed  []*expressions.CallExpression
}

func (r *observingNormalizer) RewriteReference(node *expressions.ReferenceExpression, _ struct{}, _ rewriter.ExpressionTreeRewriter[struct{}]) (foundation.Optional[expressions.Expression], error) {
	r.references = append(r.references, node)
	return foundation.Optional[expressions.Expression]{}, nil
}

func (r *observingNormalizer) RewriteOpaque(node *expressions.OpaqueExpression, _ struct{}, _ rewriter.ExpressionTreeRewriter[struct{}]) (foundation.Optional[expressions.Expression], error) {
	r.opaques = append(r.opaques, node)
	return foundation.Optional[expressions.Expression]{}, nil
}

func (r *observingNormalizer) RewriteCall(node *expressions.CallExpression, scope struct{}, tree rewriter.ExpressionTreeRewriter[struct{}]) (foundation.Optional[expressions.Expression], error) {
	result, err := r.ArrayMembershipNormalizationRewriter.RewriteCall(node, scope, tree)
	if err == nil {
		r.completed = append(r.completed, node)
	}
	return result, err
}

type traversalSnapshot struct {
	node                    expressions.Expression
	children, savedChildren []expressions.Expression
	array                   *testbedvalues.ArrayValue
	elements, savedElements []valueinterfaces.Value
}

// Save caller-visible copies as well as node contents to catch mutation of either.
func captureTraversal(source expressions.Expression) []traversalSnapshot {
	children := source.Children()
	snapshot := traversalSnapshot{node: source, children: children, savedChildren: slices.Clone(children)}
	if literal, ok := source.(*expressions.LiteralExpression); ok {
		if array, ok := literal.Value().(*testbedvalues.ArrayValue); ok {
			snapshot.array = array
			snapshot.elements = array.Elements()
			snapshot.savedElements = slices.Clone(snapshot.elements)
		}
	}
	result := []traversalSnapshot{snapshot}
	for _, child := range children {
		result = append(result, captureTraversal(child)...)
	}
	return result
}

func assertTraversalUnchanged(t *testing.T, snapshots []traversalSnapshot) {
	t.Helper()
	for _, snapshot := range snapshots {
		if !slices.Equal(snapshot.children, snapshot.savedChildren) || !slices.Equal(snapshot.node.Children(), snapshot.savedChildren) {
			t.Fatal("source or captured child slice mutated")
		}
		if snapshot.array != nil && (!slices.Equal(snapshot.elements, snapshot.savedElements) || !slices.Equal(snapshot.array.Elements(), snapshot.savedElements)) {
			t.Fatal("source or captured literal element slice mutated")
		}
	}
}

// Metadata inspection here is outside the observed traversal and its query checks.
func singletonContainsAnyCount(node expressions.Expression) int {
	result := 0
	if call, ok := node.(*expressions.CallExpression); ok {
		name, present := must(call.Context().Metadata(call.Handle())).BuiltInName().Get()
		if present && name == functions.BuiltInArrayContainsAny {
			arguments := call.Arguments()
			if literal, ok := arguments[1].(*expressions.LiteralExpression); ok {
				if array, ok := literal.Value().(*testbedvalues.ArrayValue); ok && len(array.Elements()) == 1 {
					result++
				}
			}
		}
	}
	for _, child := range node.Children() {
		result += singletonContainsAnyCount(child)
	}
	return result
}

type traversalCase struct {
	source, expected expressions.Expression
	references       []*expressions.ReferenceExpression
	opaques          []*expressions.OpaqueExpression
	completed        []*expressions.CallExpression
	contexts         []*recordingContext
	singletonCount   int
}

func verifyNormalizationTraversal(t *testing.T, tc traversalCase) expressions.Expression {
	t.Helper()
	if err := utils.ValidateForOptimization(tc.source); err != nil {
		t.Fatal(err)
	}
	before := expressions.PrettyPrint(tc.source)
	snapshots := captureTraversal(tc.source)
	size := reconstruction.MeasureExpression(tc.source)
	if singletonContainsAnyCount(tc.source) != tc.singletonCount {
		t.Fatal("unexpected source singleton count")
	}
	for _, context := range tc.contexts {
		context.events = nil
	}
	observer := &observingNormalizer{ArrayMembershipNormalizationRewriter: array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter()}
	tree := rewriter.NewExpressionTreeRewriter[struct{}](observer)
	result := must(tree.Rewrite(tc.source, struct{}{}))
	if !result.EqualsTo(tc.expected) || !result.Type().EqualsTo(tc.source.Type()) {
		t.Fatal("unexpected tree or full Type")
	}
	if !slices.Equal(observer.references, tc.references) || !slices.Equal(observer.opaques, tc.opaques) {
		t.Fatal("leaves were skipped, reordered or traversed twice")
	}
	if !slices.Equal(observer.completed, tc.completed) {
		t.Fatal("calls did not complete in expected post-order")
	}
	if reconstruction.MeasureExpression(result) != size {
		t.Fatal("singleton normalization changed structural counts")
	}
	assertNoBoundaryEvaluation(t, tc.contexts...)
	assertTraversalUnchanged(t, snapshots)
	if expressions.PrettyPrint(tc.source) != before {
		t.Fatal("source content changed")
	}
	if singletonContainsAnyCount(result) != 0 {
		t.Fatal("eligible singleton remained")
	}
	if err := utils.ValidateForOptimization(result); err != nil {
		t.Fatal(err)
	}
	// The second pass still visits each occurrence, but neither rebuilds nodes
	// nor queries providers: every contains-any singleton was normalized already.
	observer.references, observer.opaques, observer.completed = nil, nil, nil
	for _, context := range tc.contexts {
		context.events = nil
	}
	if again := must(tree.Rewrite(result, struct{}{})); again != result {
		t.Fatal("second pass did not preserve root identity")
	}
	if !slices.Equal(observer.references, tc.references) || !slices.Equal(observer.opaques, tc.opaques) {
		t.Fatal("second-pass leaf visits changed")
	}
	for _, context := range tc.contexts {
		if len(context.events) != 0 {
			t.Fatal("unchanged second pass queried a provider")
		}
	}
	assertTraversalUnchanged(t, snapshots)
	t.Logf("size preserved: %+v; singleton contains-any occurrences: %d -> 0", size, tc.singletonCount)
	return result
}

func TestNormalizationTraversalLogicalGroupsAndSharedOccurrences(t *testing.T) {
	f := newNormalizationFixture(must(testbed.NewStandardContext()), foundation.MayNull)
	boolean := must(types.NewBoolType(foundation.MayNull))
	flag := must(expressions.NewReferenceExpression(must(ids.NewColumnID("traversal", "flag")), boolean, "flag"))
	opaqueBoolean := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("traversal", "boolean")), boolean, "opaque bool"))
	opaqueArray := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("traversal", "array")), f.input.Type(), "opaque array"))
	candidates := f.candidates(8792001)
	item := candidates.Value().(*testbedvalues.ArrayValue).Elements()[0]
	shared := f.call(functions.BuiltInArrayContainsAny, f.input, candidates)
	opaqueCandidates := f.candidates(8793001)
	opaqueItem := opaqueCandidates.Value().(*testbedvalues.ArrayValue).Elements()[0]
	opaqueCall := f.call(functions.BuiltInArrayContainsAny, opaqueArray, opaqueCandidates)
	logical := func(operator foundation.LogicalOperator, children ...expressions.Expression) *expressions.LogicalExpression {
		return must(expressions.NewLogicalExpression(operator, children))
	}
	truth := must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(must(types.NewBoolType(foundation.NonNull)), true))))
	membership := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{flag, truth}))
	body := logical(foundation.And, shared, logical(foundation.Or, shared, opaqueBoolean), logical(foundation.Not, opaqueCall), membership)
	notHandle := must(f.context.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{body.Type()}))
	source := must(expressions.NewCallExpression(f.context, notHandle, []expressions.Expression{body}))
	target := f.call(functions.BuiltInArrayContains, f.input, must(expressions.NewLiteralExpression(item)))
	opaqueTarget := f.call(functions.BuiltInArrayContains, opaqueArray, must(expressions.NewLiteralExpression(opaqueItem)))
	expectedBody := logical(foundation.And, target, logical(foundation.Or, target, opaqueBoolean), logical(foundation.Not, opaqueTarget), membership)
	expected := must(expressions.NewCallExpression(f.context, notHandle, []expressions.Expression{expectedBody}))
	result := verifyNormalizationTraversal(t, traversalCase{
		source: source, expected: expected,
		references: []*expressions.ReferenceExpression{f.input, f.input, flag},
		opaques:    []*expressions.OpaqueExpression{opaqueBoolean, opaqueArray},
		completed:  []*expressions.CallExpression{shared, shared, opaqueCall, source},
		contexts:   []*recordingContext{f.context}, singletonCount: 3,
	})
	children := result.Children()[0].Children()
	// Both uses retain the same input and Value. Replacement call instances may
	// be shared or distinct; occurrence-based rewriting does not require either.
	for _, node := range []expressions.Expression{children[0], children[1].Children()[0]} {
		args := node.(*expressions.CallExpression).Arguments()
		if args[0] != f.input || args[1].(*expressions.LiteralExpression).Value() != item {
			t.Fatal("shared occurrence lost its input or element Value")
		}
	}
	args := children[2].Children()[0].(*expressions.CallExpression).Arguments()
	if args[0] != opaqueArray || args[1].(*expressions.LiteralExpression).Value() != opaqueItem || children[3] != membership {
		t.Fatal("retained subtrees or Values changed identity")
	}
}

func TestNormalizationTraversalThroughArrayProducer(t *testing.T) {
	f := newNormalizationFixture(must(testbed.NewStandardContext()), foundation.MayNull)
	boolean := must(types.NewBoolType(foundation.MayNull))
	flag := must(expressions.NewReferenceExpression(must(ids.NewColumnID("traversal", "flag")), boolean, "flag"))
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("traversal", "producer argument")), f.input.Type(), "opaque array"))
	innerCandidates := f.candidates(8792001)
	innerItem := innerCandidates.Value().(*testbedvalues.ArrayValue).Elements()[0]
	inner := f.call(functions.BuiltInArrayContainsAny, f.input, innerCandidates)
	// This unclassified, metadata-only function is not eligible for local
	// normalization. Its retained arguments can still be rewritten structurally.
	signature := must(functions.NewFunctionSignature([]types.Type{boolean, f.input.Type(), boolean}, f.input.Type()))
	metadata := must(functions.NewFunctionMetadata(signature, "array producer", foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, []functions.NullPolicy{functions.PassNull, functions.PassNull, functions.PassNull}))
	catalog := must(testbed.NewCatalog([]testbed.Definition{{Namespace: "traversal", Name: "producer", Metadata: metadata}}))
	provider := &recordingContext{FunctionContext: testbed.NewContext(catalog)}
	handle := catalog.Handles()[0]
	producer := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{inner, opaque, flag}))
	outerCandidates := f.candidates(8793001)
	outerItem := outerCandidates.Value().(*testbedvalues.ArrayValue).Elements()[0]
	source := f.call(functions.BuiltInArrayContainsAny, producer, outerCandidates)
	expectedInner := f.call(functions.BuiltInArrayContains, f.input, must(expressions.NewLiteralExpression(innerItem)))
	expectedProducer := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{expectedInner, opaque, flag}))
	expected := f.call(functions.BuiltInArrayContains, expectedProducer, must(expressions.NewLiteralExpression(outerItem)))
	result := verifyNormalizationTraversal(t, traversalCase{
		source: source, expected: expected,
		references: []*expressions.ReferenceExpression{f.input, flag}, opaques: []*expressions.OpaqueExpression{opaque},
		completed: []*expressions.CallExpression{inner, producer, source},
		contexts:  []*recordingContext{f.context, provider}, singletonCount: 2,
	}).(*expressions.CallExpression)
	rebuiltProducer := result.Arguments()[0].(*expressions.CallExpression)
	if rebuiltProducer == producer || rebuiltProducer.Context() != provider || !rebuiltProducer.Handle().EqualsTo(handle) {
		t.Fatal("producer reconstruction lost identity or context")
	}
	args := rebuiltProducer.Arguments()
	normalizedInner := args[0].(*expressions.CallExpression).Arguments()
	if args[1] != opaque || args[2] != flag || normalizedInner[0] != f.input || normalizedInner[1].(*expressions.LiteralExpression).Value() != innerItem || result.Arguments()[1].(*expressions.LiteralExpression).Value() != outerItem {
		t.Fatal("normalization lost retained computations or literal Values")
	}
}
