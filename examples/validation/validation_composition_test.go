package validation_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/examples/internal/fixtures"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/logical_flattening_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	"github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestValidatedPipelinePreservesSharedCallsAndChecksReconstruction(t *testing.T) {
	provider := fixtures.Must(functions.NewStandardContext())
	retainedContext := &validationRecordingContext{FunctionContext: provider}
	rebuiltContext := &validationRecordingContext{FunctionContext: provider}
	boolean := fixtures.Must(types.NewBoolType(foundation.NonNull))
	flag := fixtures.ScalarReference("flag", boolean)
	notHandle := fixtures.Must(provider.LookupBuiltInFunction(shared.BuiltInNot, []types.Type{boolean}))
	retained := fixtures.Must(expressions.NewCallExpression(retainedContext, notHandle, []expressions.Expression{flag}))
	integer := fixtures.Must(types.NewIntType(types.Int64, foundation.NonNull))
	array := fixtures.Must(types.NewArrayType(integer, foundation.NonNull))
	nullableArray := fixtures.Must(array.WithNullability(foundation.MayNull)).(*types.ArrayType)
	input := fixtures.ScalarReference("array", nullableArray)
	one := fixtures.Must(testbedvalues.NewInteger(integer, 1))
	candidates := fixtures.ScalarLiteral(fixtures.Must(testbedvalues.NewArray(array, []valueinterfaces.Value{one})))
	anyHandle := fixtures.Must(provider.LookupBuiltInFunction(shared.BuiltInArrayContainsAny, []types.Type{input.Type(), array}))
	membership := fixtures.Must(expressions.NewCallExpression(rebuiltContext, anyHandle, []expressions.Expression{input, candidates}))
	enclosing := fixtures.Must(expressions.NewCallExpression(rebuiltContext, notHandle, []expressions.Expression{membership}))
	source := fixtures.ScalarLogical(foundation.And, fixtures.ScalarLogical(foundation.And, retained, retained), enclosing)
	before := expressions.PrettyPrint(source)

	// Independently construct the expected tree, keeping the same immutable
	// reference identities and Types rather than introducing new associations.
	containsHandle := fixtures.Must(provider.LookupBuiltInFunction(shared.BuiltInArrayContains, []types.Type{input.Type(), integer}))
	contains := fixtures.Must(expressions.NewCallExpression(provider, containsHandle, []expressions.Expression{input, fixtures.ScalarLiteral(one)}))
	wantNot := fixtures.Must(expressions.NewCallExpression(provider, notHandle, []expressions.Expression{contains}))
	want := fixtures.ScalarLogical(foundation.And, retained, retained, wantNot)
	retainedContext.queries, rebuiltContext.queries = validationQueries{}, validationQueries{}
	if err := utils.ValidateForOptimization(source); err != nil {
		t.Fatal(err)
	}
	if retainedContext.queries.metadata != 1 {
		t.Fatal("entry should inspect the shared call once")
	}
	retainedContext.queries, rebuiltContext.queries = validationQueries{}, validationQueries{}

	// Validate only at entry. Normalization reconstructs the enclosing NOT call
	// through its stored context; flattening retains both shared call occurrences.
	normalizer := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
	normalized := fixtures.Must(normalizer.Rewrite(source, struct{}{}))
	flattener := rewriter.NewExpressionTreeRewriter[struct{}](logical_flattening_rewriter.NewLogicalFlatteningRewriter())
	result := fixtures.Must(flattener.Rewrite(normalized, struct{}{}))
	children := result.Children()
	if !result.EqualsTo(want) || !result.Type().EqualsTo(source.Type()) || children[0] != retained || children[1] != retained {
		t.Fatal("pipeline lost expected meaning, Type or shared identity")
	}
	rebuiltNot := children[2].(*expressions.CallExpression)
	if rebuiltNot == enclosing || rebuiltNot.Context() != rebuiltContext || !rebuiltNot.Handle().EqualsTo(notHandle) {
		t.Fatal("changed enclosing call did not retain its paired context and handle")
	}
	if retainedContext.queries != (validationQueries{}) {
		t.Fatal("passes unnecessarily queried the retained shared call")
	}
	if rebuiltContext.queries.metadata == 0 || rebuiltContext.queries.evaluation != 0 || expressions.PrettyPrint(source) != before {
		t.Fatal("reconstruction bypassed metadata, evaluated a function or mutated input")
	}
	// Output validation is an integration assertion, not another pipeline pass.
	if err := utils.ValidateForOptimization(result); err != nil {
		t.Fatal(err)
	}
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
	for _, flagValue := range []bool{false, true} {
		for _, arrayValue := range []valueinterfaces.Value{
			fixtures.Must(testbedvalues.NewArray(nullableArray, nil)),
			fixtures.Must(testbedvalues.NewArray(nullableArray, []valueinterfaces.Value{one})),
			fixtures.Must(testbedvalues.NewNull(nullableArray)),
		} {
			bindings := fixtures.Environment{{ID: flag.ID(), Value: fixtures.Must(testbedvalues.NewBool(boolean, flagValue))}, {ID: input.ID(), Value: arrayValue}}
			original := fixtures.Must(evaluator.Evaluate(source, bindings))
			actual := fixtures.Must(evaluator.Evaluate(result, bindings))
			if !original.EqualsTo(actual) {
				t.Fatal("pipeline changed evaluation, including NULL behavior")
			}
		}
	}
}
