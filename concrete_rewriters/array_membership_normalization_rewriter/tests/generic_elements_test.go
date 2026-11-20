package array_membership_normalization_rewriter_test

import (
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
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

// These caller-side overloads deliberately extend beyond the standard catalog.
// They use IEEE equality for floats and typed structural equality otherwise;
// this is a fixture contract, not a claim about Milvus composite membership.
type genericMembershipContext struct {
	functioninterfaces.FunctionContext
}

func (c *genericMembershipContext) Evaluate(handle functioninterfaces.FunctionHandle, signature *functions.FunctionSignature, args []valueinterfaces.Value) (valueinterfaces.Value, error) {
	metadata, err := c.Metadata(handle)
	if err != nil {
		return nil, err
	}
	if err := functions.ValidateCallSignature(metadata, signature); err != nil {
		return nil, err
	}
	if args[0].IsNull() {
		return testbedvalues.NewNull(signature.ResultType())
	}
	input := args[0].(*testbedvalues.ArrayValue).Elements()
	name, _ := metadata.BuiltInName().Get()
	matched := false
	switch name {
	case functions.BuiltInArrayContains:
		for _, element := range input {
			matched = matched || fixtureElementEqual(element, args[1])
		}
	case functions.BuiltInArrayContainsAny:
		for _, candidate := range args[1].(*testbedvalues.ArrayValue).Elements() {
			for _, element := range input {
				matched = matched || fixtureElementEqual(element, candidate)
			}
		}
	default:
		return nil, functions.ErrEvaluationUnavailable
	}
	return testbedvalues.NewBool(signature.ResultType().(*types.BoolType), matched)
}

func fixtureElementEqual(a, b valueinterfaces.Value) bool {
	if floating, ok := a.(*testbedvalues.FloatValue); ok {
		return floating.Float() == b.(*testbedvalues.FloatValue).Float()
	}
	return a.EqualsTo(b)
}

func TestNormalizeGenericElementTypes(t *testing.T) {
	integer := must(types.NewIntType(types.Int32, foundation.NonNull))
	boolean := must(types.NewBoolType(foundation.NonNull))
	maxBytes := uint64(16)
	varchar := must(types.NewVarCharType(&maxBytes, foundation.NonNull))
	float32Type := must(types.NewFloatType(types.Float32, foundation.NonNull))
	float64Type := must(types.NewFloatType(types.Float64, foundation.NonNull))
	nested := must(types.NewArrayType(integer, foundation.NonNull))
	structure := must(types.NewStructType([]types.StructField{{Name: "tag", Type: varchar}}, foundation.NonNull))
	cases := []struct {
		name    string
		element valueinterfaces.Value
	}{
		{"bool", must(testbedvalues.NewBool(boolean, true))},
		{"varchar", must(testbedvalues.NewVarChar(varchar, "tag"))},
		{"float32", must(testbedvalues.NewFloat(float32Type, 1.5))},
		{"float64", must(testbedvalues.NewFloat(float64Type, 1.5))},
		{"NaN", must(testbedvalues.NewFloat(float64Type, math.NaN()))},
		{"negative zero", must(testbedvalues.NewFloat(float64Type, math.Copysign(0, -1)))},
		{"nested array", must(testbedvalues.NewArray(nested, []valueinterfaces.Value{must(testbedvalues.NewInteger(integer, 8))}))},
		{"struct", must(testbedvalues.NewStruct(structure, []valueinterfaces.Value{must(testbedvalues.NewVarChar(varchar, "tag"))}))},
	}
	for _, width := range []types.IntWidth{types.Int8, types.Int16, types.Int32, types.Int64} {
		typ := must(types.NewIntType(width, foundation.NonNull))
		cases = append(cases, struct {
			name    string
			element valueinterfaces.Value
		}{typ.String(), must(testbedvalues.NewInteger(typ, 8))})
	}
	for _, tc := range cases {
		for _, nullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			t.Run(tc.name+nullability.Suffix(), func(t *testing.T) {
				array := must(types.NewArrayType(tc.element.Type(), foundation.NonNull))
				policies := []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified}
				catalog := must(testbed.NewCatalog([]testbed.Definition{
					membershipDefinition(functions.BuiltInArrayContainsAny, []types.Type{array, array}, boolean, policies),
					membershipDefinition(functions.BuiltInArrayContains, []types.Type{array, tc.element.Type()}, boolean, policies),
				}))
				context := &recordingContext{FunctionContext: &genericMembershipContext{testbed.NewContext(catalog)}}
				inputType := must(array.WithNullability(nullability)).(*types.ArrayType)
				input := must(expressions.NewReferenceExpression(must(ids.NewColumnID("generic", "array")), inputType, "array"))
				candidates := must(expressions.NewLiteralExpression(must(testbedvalues.NewArray(array, []valueinterfaces.Value{tc.element}))))
				sourceHandle := must(context.LookupBuiltInFunction(functions.BuiltInArrayContainsAny, []types.Type{inputType, array}))
				source := must(expressions.NewCallExpression(context, sourceHandle, []expressions.Expression{input, candidates}))
				targetHandle := must(context.LookupBuiltInFunction(functions.BuiltInArrayContains, []types.Type{inputType, tc.element.Type()}))
				want := must(expressions.NewCallExpression(context, targetHandle, []expressions.Expression{input, must(expressions.NewLiteralExpression(tc.element))}))
				context.events = nil
				tree := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
				got := must(tree.Rewrite(source, struct{}{}))
				if !got.EqualsTo(want) || !got.Type().EqualsTo(source.Type()) {
					t.Fatalf("generic overload did not normalize: %s", expressions.PrettyPrint(got))
				}
				for _, event := range context.events {
					if event.operation == "evaluate" {
						t.Fatal("rewriting evaluated membership")
					}
				}
				rows := []valueinterfaces.Value{
					must(testbedvalues.NewArray(inputType, nil)),
					must(testbedvalues.NewArray(inputType, []valueinterfaces.Value{tc.element, tc.element})),
				}
				if floating, ok := tc.element.(*testbedvalues.FloatValue); ok && floating.Float() == 0 {
					rows = append(rows, must(testbedvalues.NewArray(inputType, []valueinterfaces.Value{must(testbedvalues.NewFloat(float64Type, 0))})))
				}
				if nullability == foundation.MayNull {
					rows = append(rows, must(testbedvalues.NewNull(inputType)))
				}
				evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
				for _, row := range rows {
					env := &fixtureEnvironment{bindings: []fixtureBinding{{input.ID(), row}}}
					before := must(evaluator.Evaluate(source, env))
					after := must(evaluator.Evaluate(got, env))
					if !before.EqualsTo(after) {
						t.Fatal("generic membership changed evaluation")
					}
				}
			})
		}
	}
}
