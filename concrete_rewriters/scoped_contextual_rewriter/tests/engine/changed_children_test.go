package engine_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/internal/boolean_cleanup"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/internal/assertions"

	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"

	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"

	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

type changedChildrenEnvironment struct{ value valueinterfaces.Value }

func (e changedChildrenEnvironment) Lookup(expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	if e.value == nil {
		panic("unexpected reference lookup")
	}
	return foundation.Some(e.value), nil
}

func changedChildrenFunction(args []types.Type, result types.Type, evaluate testbed.Evaluator) (functioninterfaces.FunctionContext, functioninterfaces.FunctionHandle) {
	policies := make([]functions.NullPolicy, len(args))
	for i := range policies {
		policies[i] = functions.PropagateNull
	}
	metadata := eligibilityMetadata(args, result, foundation.Optional[foundation.OperatorType]{}, foundation.Optional[functions.BuiltInFunctionName]{}, policies)
	catalog := must(testbed.NewCatalog([]testbed.Definition{{Namespace: "fixture", Name: "foo", Metadata: metadata}}))
	handle := catalog.Handles()[0]
	return must(testbed.NewContextWithEvaluators(catalog, []testbed.EvaluationBinding{{Handle: handle, Evaluate: evaluate}})), handle
}

// EV.04: a valid scoped truth replacement is unsafe inside an unsupported foo.
func TestCanChangeChildrenRejectsValueSensitiveEnclosingCall(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, provider)
	boolean := must(types.NewBoolType(foundation.NonNull))
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	ref := eligibilityReference(integer)
	one := membershipCandidate(integer)
	predicate := must(expressions.NewCallExpression(provider, must(provider.ComparisonFunction(foundation.Equal, integer, integer)), []expressions.Expression{ref, one}))
	failure := errors.New("foo(false) fails")
	evaluations := 0
	context, handle := changedChildrenFunction([]types.Type{boolean}, boolean, func(args []valueinterfaces.Value) (valueinterfaces.Value, error) {
		evaluations++
		if !args[0].(*testbedvalues.BoolValue).Bool() {
			return nil, failure
		}
		return testbedvalues.NewBool(boolean, true)
	})
	call := must(expressions.NewCallExpression(context, handle, []expressions.Expression{predicate}))
	replacement := must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, boolean, foundation.True))
	if ok, err := rule.CanOmit(predicate); !ok || err != nil {
		t.Fatal("predicate should be omittable", err)
	}
	if ok, err := rule.CanChangeChildren(call, []expressions.Expression{replacement}); ok || err != nil {
		t.Fatal("unsupported foo admitted", err)
	}
	if evaluations != 0 {
		t.Fatal("admission evaluated foo")
	}
	original := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{predicate, call}))
	hypothetical := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{predicate, must(call.WithChildren([]expressions.Expression{replacement}))}))
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	for _, x := range []int64{1, 2} {
		environment := changedChildrenEnvironment{must(testbedvalues.NewInteger(integer, x))}
		before, err := evaluator.Evaluate(original, environment)
		after := must(evaluator.Evaluate(hypothetical, environment))
		if x == 1 {
			if err != nil || !before.EqualsTo(after) {
				t.Fatal("guard-true outcomes differ", err)
			}
		} else if !errors.Is(err, failure) || after.(*testbedvalues.BoolValue).Bool() {
			t.Fatal("fixture did not expose guard-false failure suppression", err)
		}
	}
}

// EV.05/EV.06: changing a supported enclosing use must keep erroring siblings.
func TestCanChangeChildrenRetainsFailingSiblings(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, provider)
	boolean := must(types.NewBoolType(foundation.NonNull))
	nullable := must(types.NewBoolType(foundation.MayNull))
	failure := errors.New("retained sibling fails")
	evaluations := 0
	context, handle := changedChildrenFunction(nil, boolean, func([]valueinterfaces.Value) (valueinterfaces.Value, error) { evaluations++; return nil, failure })
	failing := must(expressions.NewCallExpression(context, handle, nil))
	yes := must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, boolean, foundation.True))
	no := must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, boolean, foundation.False))
	folded := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{yes, no}))
	null := must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, nullable, foundation.Null))
	nullSource := must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{null}))
	for _, tc := range []struct {
		name     string
		source   expressions.Expression
		children []expressions.Expression
	}{
		{"logical", must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{folded, failing})), []expressions.Expression{no, failing}},
		{"comparison", must(expressions.NewCallExpression(provider, must(provider.ComparisonFunction(foundation.Equal, boolean, boolean)), []expressions.Expression{folded, failing})), []expressions.Expression{no, failing}},
		{"null propagation", must(expressions.NewCallExpression(provider, must(provider.ComparisonFunction(foundation.Equal, nullable, boolean)), []expressions.Expression{nullSource, failing})), []expressions.Expression{null, failing}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evaluations = 0
			if ok, err := rule.CanOmit(tc.source); ok || err != nil {
				t.Fatal("erroring source must not be omittable", err)
			}
			if ok, err := rule.CanChangeChildren(tc.source, tc.children); !ok || err != nil {
				t.Fatal("supported enclosing use rejected", err)
			}
			if evaluations != 0 {
				t.Fatal("admission performed evaluation")
			}
			rebuilt := must(tc.source.WithChildren(tc.children))
			if rebuilt.Children()[1] != failing || tc.source.Children()[1] != failing {
				t.Fatal("sibling identity/order changed")
			}
			for _, expression := range []expressions.Expression{tc.source, rebuilt} {
				if _, err := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{}).Evaluate(expression, changedChildrenEnvironment{}); !errors.Is(err, failure) {
					t.Fatal("sibling failure suppressed", err)
				}
			}
			if evaluations != 2 {
				t.Fatal("retained sibling not evaluated once per tree")
			}
		})
	}
	// A total enclosing NOT also accepts a justified change and preserves its result.
	call := must(expressions.NewCallExpression(provider, must(provider.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{boolean})), []expressions.Expression{folded}))
	if ok, err := rule.CanChangeChildren(call, []expressions.Expression{no}); !ok || err != nil {
		t.Fatal(err)
	}
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	before := must(evaluator.Evaluate(call, changedChildrenEnvironment{}))
	after := must(evaluator.Evaluate(must(call.WithChildren([]expressions.Expression{no})), changedChildrenEnvironment{}))
	if !before.EqualsTo(after) {
		t.Fatal("equivalent argument change altered result")
	}
}

func TestCanChangeChildrenShapesAndTypes(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, provider)
	boolean := must(types.NewBoolType(foundation.NonNull))
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	array := must(types.NewArrayType(integer, foundation.NonNull))
	ref := eligibilityReference(integer)
	one := membershipCandidate(integer)
	two := must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(integer, 2))))
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("fixture", "integer")), integer, "integer"))
	list := eligibilityArray(array, 1)
	empty := eligibilityArray(array)
	arrayRef := eligibilityReference(array)
	contains := must(expressions.NewCallExpression(provider, must(provider.LookupBuiltInFunction(functions.BuiltInArrayContains, []types.Type{array, integer})), []expressions.Expression{arrayRef, ref}))
	any := must(expressions.NewCallExpression(provider, must(provider.LookupBuiltInFunction(functions.BuiltInArrayContainsAny, []types.Type{array, array})), []expressions.Expression{arrayRef, list}))
	dynamicAny := must(any.WithChildren([]expressions.Expression{arrayRef, arrayRef}))
	for _, tc := range []struct {
		name     string
		source   expressions.Expression
		children []expressions.Expression
		want     bool
	}{
		{"IN target", membershipExpression(ref, one), []expressions.Expression{two, one}, true},
		{"IN retained opaque target", membershipExpression(opaque, one), []expressions.Expression{opaque, two}, true},
		{"IN new dynamic candidate", membershipExpression(ref, one), []expressions.Expression{ref, ref}, false},
		{"IN original dynamic candidate", membershipExpression(ref, ref), []expressions.Expression{ref, one}, false},
		{"IN empty", membershipExpression(ref), []expressions.Expression{one}, false},
		{"contains", contains, []expressions.Expression{arrayRef, one}, true},
		{"contains-any empty list", any, []expressions.Expression{arrayRef, empty}, true},
		{"contains-any new dynamic list", any, []expressions.Expression{arrayRef, arrayRef}, false},
		{"contains-any original dynamic list", dynamicAny, []expressions.Expression{arrayRef, list}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := expressions.PrettyPrint(tc.source)
			if ok, err := rule.CanChangeChildren(tc.source, tc.children); ok != tc.want || err != nil {
				t.Fatalf("got %v, %v", ok, err)
			}
			if expressions.PrettyPrint(tc.source) != original {
				t.Fatal("admission mutated source")
			}
		})
	}
	nullableOne := membershipCandidate(must(integer.WithNullability(foundation.MayNull)))
	for _, pair := range []struct {
		source   expressions.Expression
		children []expressions.Expression
	}{
		{membershipExpression(ref, nullableOne), []expressions.Expression{one, nullableOne}},
		{membershipExpression(ref, one), []expressions.Expression{ref}},
		{membershipExpression(ref, one), []expressions.Expression{ref, nullableOne}},
		{membershipExpression(ref, one), []expressions.Expression{ref, membershipCandidate(boolean)}},
	} {
		ok, err := rule.CanChangeChildren(pair.source, pair.children)
		if pair.source.Children()[1].Type().Nullability() == foundation.MayNull {
			if ok || err != nil {
				t.Fatalf("unsupported nullable shape: %v, %v", ok, err)
			}
		} else if ok || !errors.Is(err, rewriter.ErrInvalidReplacement) {
			t.Fatalf("bad replacement not rejected: %v, %v", ok, err)
		}
	}
}

func TestCanChangeChildrenUnchangedAndProviderErrors(t *testing.T) {
	context := &eligibilityContext{FunctionContext: must(testbed.NewStandardContext())}
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, context)
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	one := membershipCandidate(integer)
	two := must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(integer, 2))))
	source := must(expressions.NewCallExpression(context, must(context.FunctionContext.ComparisonFunction(foundation.Equal, integer, integer)), []expressions.Expression{one, two}))
	failure := errors.New("metadata failure")
	context.metadata = func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) { return nil, failure }
	context.queries = 0
	if ok, err := rule.CanChangeChildren(source, source.Children()); !ok || err != nil || context.queries != 0 {
		t.Fatal("unchanged call inspected provider", err)
	}
	if ok, err := rule.CanChangeChildren(source, []expressions.Expression{two, two}); ok || !errors.Is(err, failure) {
		t.Fatal("provider error hidden", err)
	}
	unknown, call := eligibilityCall(eligibilityMetadata([]types.Type{integer}, integer, foundation.Optional[foundation.OperatorType]{}, foundation.Optional[functions.BuiltInFunctionName]{}, []functions.NullPolicy{functions.PropagateNull}), []expressions.Expression{one})
	unknown.queries = 0
	if ok, err := rule.CanChangeChildren(call, call.Children()); !ok || err != nil || unknown.queries != 0 {
		t.Fatal("unchanged unknown call inspected", err)
	}
	if ok, err := rule.CanChangeChildren(call, []expressions.Expression{two}); ok || err != nil {
		t.Fatal("unknown changed call admitted", err)
	}
	membership := membershipEligibilityProvider()
	membership.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
		return nil, failure
	}
	if ok, err := boolean_cleanup.New(testbedvalues.ValueFactory{}, membership).CanChangeChildren(membershipExpression(one, one), []expressions.Expression{two, one}); ok || !errors.Is(err, failure) {
		t.Fatal("IN lookup error hidden", err)
	}
}

func TestCanChangeChildrenNilInputs(t *testing.T) {
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, must(testbed.NewStandardContext()))
	boolean := must(types.NewBoolType(foundation.NonNull))
	yes := must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, boolean, foundation.True))
	source := must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{yes}))
	for _, call := range []func(){
		func() { _, _ = rule.CanChangeChildren(nil, nil) },
		func() { _, _ = rule.CanChangeChildren((*expressions.LogicalExpression)(nil), nil) },
		func() { _, _ = rule.CanChangeChildren(source, []expressions.Expression{nil}) },
		func() {
			_, _ = rule.CanChangeChildren(source, []expressions.Expression{(*expressions.LiteralExpression)(nil)})
		},
		func() {
			var nilRule *boolean_cleanup.Cleaner
			_, _ = nilRule.CanChangeChildren(source, source.Children())
		},
	} {
		func() {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Error("nil input did not assert")
				}
			}()
			call()
		}()
	}
}
