package interpreter_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// These fixtures intentionally violate the rewriters' execution preconditions.
// They exercise the provider/interpreter contract without invoking optimization.
func equalityContractCatalog() *testbed.Catalog {
	standard := must(testbed.NewStandardContext())
	typ := intLiteral(1, foundation.NonNull).Type()
	handle := must(standard.ComparisonFunction(foundation.Equal, typ, typ))
	metadata := must(standard.Metadata(handle))
	return must(testbed.NewCatalog([]testbed.Definition{{Namespace: "contract", Name: "int64.equal", Metadata: metadata}}))
}

// EV.02: supported operator metadata does not itself supply executable behavior.
func TestAdmittedEqualityRequiresAvailableExecution(t *testing.T) {
	for _, disallowed := range []bool{false, true} {
		catalog := equalityContractCatalog()
		handle := catalog.Handles()[0]
		provider := testbed.NewContext(catalog)
		want := functions.ErrEvaluationUnavailable
		if disallowed {
			provider = must(testbed.NewContextWithEvaluators(catalog, []testbed.EvaluationBinding{{Handle: handle, Disallowed: true}}))
			want = functions.ErrEvaluationDisallowed
		}
		one := intLiteral(1, foundation.NonNull)
		call := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{one, one}))
		// Both inspection and signature binding succeed in this invalid-for-optimizer
		// configuration. Only independent execution exposes the missing permission/code.
		if value, err := provider.Evaluate(handle, call.Signature(), []valueinterfaces.Value{one.Value(), one.Value()}); value != nil || !errors.Is(err, want) {
			t.Fatal("provider lost execution category", err)
		}
		evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
		for _, node := range []expressions.Expression{call, membership(one, one)} {
			if value, err := evaluator.Evaluate(node, &environment{}); value != nil || !errors.Is(err, want) {
				t.Fatal("interpreter lost execution category", err)
			}
			partial := must(evaluator.PartialEvaluate(node, &environment{}))
			if residual, present := partial.Residual(); !present || !residual.Type().EqualsTo(node.Type()) {
				t.Fatal("unavailable execution became a value")
			}
		}
	}
}

// EV.07: a successful early equality must not hide a later equality failure.
func TestMembershipLaterEqualityFailureAfterMatch(t *testing.T) {
	catalog := equalityContractCatalog()
	handle := catalog.Handles()[0]
	metadata := must(testbed.NewContext(catalog).Metadata(handle))
	failure := errors.New("equality for candidate 2 fails")
	var visited []int64
	provider := must(testbed.NewContextWithEvaluators(catalog, []testbed.EvaluationBinding{{Handle: handle, Evaluate: func(args []valueinterfaces.Value) (valueinterfaces.Value, error) {
		left, right := args[0].(*testbedvalues.IntegerValue).Integer(), args[1].(*testbedvalues.IntegerValue).Integer()
		visited = append(visited, right)
		if right == 2 {
			return nil, failure
		}
		return testbedvalues.NewBool(metadata.Signature().ResultType().(*types.BoolType), left == right)
	}}}))
	one, two := intLiteral(1, foundation.NonNull), intLiteral(2, foundation.NonNull)
	match := must(provider.Evaluate(handle, metadata.Signature(), []valueinterfaces.Value{one.Value(), one.Value()}))
	if !match.(*testbedvalues.BoolValue).Bool() {
		t.Fatal("first equality did not match")
	}
	root := membership(one, one, two)
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
	for _, partial := range []bool{false, true} {
		visited = nil
		var err error
		if partial {
			var result interpreter.Result
			result, err = evaluator.PartialEvaluate(root, &environment{})
			if result.IsValid() {
				t.Fatal("failed equality returned a residual or known value")
			}
		} else {
			var result valueinterfaces.Value
			result, err = evaluator.Evaluate(root, &environment{})
			if result != nil {
				t.Fatal("failed equality returned a value")
			}
		}
		if !errors.Is(err, failure) || !errors.Is(err, functions.ErrEvaluationFailure) {
			t.Fatal("later equality failure hidden", err)
		}
		if len(visited) != 2 || visited[0] != 1 || visited[1] != 2 {
			t.Fatal("equality order or eager behavior changed", visited)
		}
	}
}
