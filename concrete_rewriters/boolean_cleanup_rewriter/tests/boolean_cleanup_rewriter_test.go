package boolean_cleanup_rewriter_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/boolean_cleanup_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func literal(truth foundation.TruthValue, nullable foundation.Nullability) *expressions.LiteralExpression {
	typ := must(types.NewBoolType(nullable))
	if truth == foundation.Null {
		return must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(typ))))
	}
	return must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(typ, truth == foundation.True))))
}

func logical(op foundation.LogicalOperator, args ...expressions.Expression) *expressions.LogicalExpression {
	return must(expressions.NewLogicalExpression(op, args))
}

func reference(name string, typ types.Type) *expressions.ReferenceExpression {
	return must(expressions.NewReferenceExpression(must(ids.NewColumnID("cleanup", name)), typ, name))
}

type provider struct {
	functioninterfaces.FunctionContext
	evaluations   int
	metadataError error
}

func (p *provider) Evaluate(h functioninterfaces.FunctionHandle, s *functions.FunctionSignature, args []valueinterfaces.Value) (valueinterfaces.Value, error) {
	p.evaluations++
	return p.FunctionContext.Evaluate(h, s, args)
}

func (p *provider) Metadata(h functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	if p.metadataError != nil {
		return nil, p.metadataError
	}
	return p.FunctionContext.Metadata(h)
}

type row struct {
	a, b  *expressions.ReferenceExpression
	truth foundation.TruthValue
}

func (r row) Lookup(id expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	for _, ref := range []*expressions.ReferenceExpression{r.a, r.b} {
		if ref != nil && id.EqualsTo(ref.ID()) {
			return foundation.Some(literal(r.truth, ref.Type().Nullability()).Value()), nil
		}
	}
	return foundation.Optional[valueinterfaces.Value]{}, errors.New("unexpected reference")
}

func TestBooleanCleanupStandalone(t *testing.T) {
	context := &provider{FunctionContext: must(testbed.NewStandardContext())}
	tree := rewriter.NewExpressionTreeRewriter[struct{}](boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, context))
	truth, falsity := literal(foundation.True, foundation.NonNull), literal(foundation.False, foundation.NonNull)
	null := literal(foundation.Null, foundation.MayNull)
	maybeTrue, maybeFalse := literal(foundation.True, foundation.MayNull), literal(foundation.False, foundation.MayNull)
	a := reference("a", truth.Type())
	b := reference("b", maybeTrue.Type())
	inapplicable := logical(foundation.And, a, maybeTrue)
	untouched := logical(foundation.Or, a, logical(foundation.Not, a))
	for _, tc := range []struct {
		name         string
		source, want expressions.Expression
		nullable     bool
	}{
		{"bottom-up literals", logical(foundation.Not, logical(foundation.And, truth, falsity)), truth, false},
		{"nested neutral groups", logical(foundation.Or, logical(foundation.And, a, truth), falsity), a, false},
		{"all-neutral AND", logical(foundation.And, truth, truth), truth, false},
		{"all-neutral OR", logical(foundation.Or, falsity, falsity), falsity, false},
		{"NULL inversion", logical(foundation.Not, null), null, true},
		{"NULL with neutral", logical(foundation.And, truth, null), null, true},
		{"NULL with decisive", logical(foundation.And, falsity, null), maybeFalse, true},
		{"nullable reference absorption", logical(foundation.And, b, falsity), maybeFalse, true},
		{"nullable neutral", logical(foundation.Or, b, falsity), b, true},
		{"preserve declared Type", inapplicable, inapplicable, false},
		{"retain rebuilt children when Type prevents collapse", logical(foundation.And, a, logical(foundation.Not, maybeFalse)), inapplicable, false},
		{"no identity", untouched, untouched, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, calls := expressions.PrettyPrint(tc.source), context.evaluations
			got := must(tree.Rewrite(tc.source, struct{}{}))
			if context.evaluations != calls || expressions.PrettyPrint(tc.source) != before || !got.EqualsTo(tc.want) || !got.Type().EqualsTo(tc.source.Type()) || reconstruction.MeasureExpression(got).NodeCount > reconstruction.MeasureExpression(tc.source).NodeCount {
				t.Fatalf("incorrect cleanup\n%s\nwant\n%s", expressions.PrettyPrint(got), expressions.PrettyPrint(tc.want))
			}
			if tc.source == tc.want && got != tc.source {
				t.Fatal("unchanged expression lost identity")
			}
			oracle := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, context)
			for _, truth := range []foundation.TruthValue{foundation.False, foundation.True, foundation.Null} {
				if truth == foundation.Null && !tc.nullable {
					continue
				}
				env := row{a: a, b: b, truth: truth}
				if !must(oracle.Evaluate(tc.source, env)).EqualsTo(must(oracle.Evaluate(got, env))) {
					t.Fatal("cleanup changed complete-input evaluation")
				}
			}
		})
	}
}

func TestBooleanCleanupPreservesOpaqueAndFailingComputations(t *testing.T) {
	context := must(testbed.NewStandardContext())
	tree := rewriter.NewExpressionTreeRewriter[struct{}](boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, context))
	truth, falsity := literal(foundation.True, foundation.NonNull), literal(foundation.False, foundation.NonNull)
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("cleanup", "opaque")), truth.Type(), "unsupported"))
	if got := must(tree.Rewrite(logical(foundation.And, opaque, truth), struct{}{})); got != opaque {
		t.Fatal("neutral cleanup lost opaque node")
	}
	masked := logical(foundation.And, opaque, falsity)
	if got := must(tree.Rewrite(masked, struct{}{})); got != masked {
		t.Fatal("absorbing cleanup removed opaque computation")
	}

	signature := must(functions.NewFunctionSignature(nil, truth.Type()))
	metadata := must(functions.NewFunctionMetadata(signature, "failure", foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, nil))
	catalog := must(testbed.NewCatalog([]testbed.Definition{{Namespace: "cleanup", Name: "failure", Metadata: metadata}}))
	handle := catalog.Handles()[0]
	failure := errors.New("retained evaluation failure")
	calls := 0
	failingContext := must(testbed.NewContextWithEvaluators(catalog, []testbed.EvaluationBinding{{Handle: handle, Evaluate: func([]valueinterfaces.Value) (valueinterfaces.Value, error) { calls++; return nil, failure }}}))
	call := must(expressions.NewCallExpression(failingContext, handle, nil))
	source := logical(foundation.And, truth, call, falsity)
	got := must(tree.Rewrite(source, struct{}{}))
	if calls != 0 || !got.EqualsTo(logical(foundation.And, call, falsity)) || got.Children()[0] != call {
		t.Fatal("failed to retain the erroring computation")
	}
	oracle := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	for _, expr := range []expressions.Expression{source, got} {
		if _, err := oracle.Evaluate(expr, row{}); !errors.Is(err, failure) {
			t.Fatal("evaluation error was suppressed", err)
		}
	}
}

func TestBooleanCleanupSupportedOmissionAndErrors(t *testing.T) {
	context := &provider{FunctionContext: must(testbed.NewStandardContext())}
	tree := rewriter.NewExpressionTreeRewriter[struct{}](boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, context))
	truth, falsity := literal(foundation.True, foundation.NonNull), literal(foundation.False, foundation.NonNull)
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	x := reference("x", integer)
	one := must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(integer, 1))))
	handle := must(context.ComparisonFunction(foundation.Equal, integer, integer))
	comparison := must(expressions.NewCallExpression(context, handle, []expressions.Expression{x, one}))
	in := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, truth.Type(), []expressions.Expression{x, one}))
	for _, predicate := range []expressions.Expression{comparison, in} {
		if got := must(tree.Rewrite(logical(foundation.And, predicate, falsity), struct{}{})); !got.EqualsTo(falsity) {
			t.Fatal("supported omission was not used")
		}
	}
	if context.evaluations != 0 {
		t.Fatal("cleanup evaluated a supported predicate")
	}
	// Ineligibility of the first operand must not hide a later inspection error.
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("cleanup", "opaque")), truth.Type(), "unsupported"))
	source := logical(foundation.Or, opaque, comparison, truth)
	failure := errors.New("metadata unavailable")
	context.metadataError = failure
	if got, err := tree.Rewrite(source, struct{}{}); got != nil || !errors.Is(err, failure) {
		t.Fatal("inspection failure returned a partial result", err)
	}
	context.metadataError = nil

	// Ordinary descent also cleans arguments inside non-logical parents, keeping
	// the stored function binding. Reconstruction errors still propagate.
	a := reference("a", truth.Type())
	not := must(context.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{truth.Type()}))
	call := must(expressions.NewCallExpression(context, not, []expressions.Expression{logical(foundation.And, a, truth)}))
	got := must(tree.Rewrite(call, struct{}{})).(*expressions.CallExpression)
	if got.Arguments()[0] != a || got.Context() != context || !got.Handle().EqualsTo(not) {
		t.Fatal("changed call binding during argument cleanup")
	}
	context.metadataError = failure
	if got, err := tree.Rewrite(call, struct{}{}); got != nil || !errors.Is(err, failure) {
		t.Fatal("reconstruction failure returned a partial call", err)
	}
	if context.evaluations != 0 {
		t.Fatal("cleanup evaluated expressions")
	}
}

type countingRule struct {
	*boolean_cleanup_rewriter.BooleanCleanupRewriter
	visits int
}

func (r *countingRule) RewriteReference(_ *expressions.ReferenceExpression, _ struct{}, _ rewriter.ExpressionTreeRewriter[struct{}]) (foundation.Optional[expressions.Expression], error) {
	r.visits++
	return foundation.Optional[expressions.Expression]{}, nil
}

func TestBooleanCleanupVisitsEachOccurrenceOnce(t *testing.T) {
	// No provider access is needed for this unchanged tree.
	context := &provider{}
	rule := &countingRule{BooleanCleanupRewriter: boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, context)}
	a := reference("a", must(types.NewBoolType(foundation.NonNull)))
	source := logical(foundation.And, a, logical(foundation.Not, a))
	got := must(rewriter.NewExpressionTreeRewriter[struct{}](rule).Rewrite(source, struct{}{}))
	if rule.visits != 2 || got != source {
		t.Fatal("post-order callback revisited children or changed source", rule.visits)
	}
}

func TestBooleanCleanupConstruction(t *testing.T) {
	// Nil embedded provider methods expose accidental constructor queries.
	if boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, &provider{}) == nil {
		t.Fatal("constructor returned nil")
	}
	for _, context := range []functioninterfaces.FunctionContext{nil, (*testbed.Context)(nil)} {
		if !assertions.Enabled {
			continue
		}
		func() {
			defer func() {
				if recover() == nil {
					t.Error("nil context did not assert")
				}
			}()
			boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, context)
		}()
	}
	for _, rule := range []*boolean_cleanup_rewriter.BooleanCleanupRewriter{nil, {}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("unconstructed rule did not assert")
				}
			}()
			rule.RewriteLogical(nil, struct{}{}, nil)
		}()
	}
}
