package functions_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
	fixtures "github.com/lie-yan/predicate-optimization/values/interfaces/tests/testutil"
)

func externalMust[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// Preserve the wrapper during context nullability adaptation, so execution
// cannot accidentally fall back to the bundled concrete array representation.
type externalArray struct {
	valueinterfaces.Value
	children []valueinterfaces.Value
}

func (v externalArray) ElementCount() int                         { return len(v.children) }
func (v externalArray) ElementAt(index int) valueinterfaces.Value { return v.children[index] }
func (v externalArray) Elements() []valueinterfaces.Value {
	return append([]valueinterfaces.Value(nil), v.children...)
}

func (v externalArray) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	adapted, err := v.Value.WithNullability(n)
	if err != nil {
		return nil, err
	}
	return externalArray{adapted, v.children}, nil
}

func TestStandardExternalScalarInputs(t *testing.T) {
	provider := externalMust(testbed.NewStandardContext())
	integer := externalMust(types.NewIntType(types.Int64, foundation.MayNull))
	text := externalMust(types.NewVarCharType(nil, foundation.MayNull))
	boolean := externalMust(types.NewBoolType(foundation.MayNull))
	pairs := [][2]valueinterfaces.Value{
		{externalMust(testbedvalues.NewInteger(integer, math.MinInt64)), externalMust(testbedvalues.NewInteger(integer, math.MaxInt64))},
		{externalMust(testbedvalues.NewVarChar(text, "a\x00")), externalMust(testbedvalues.NewVarChar(text, "a\x01"))},
	}
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := externalMust(types.NewFloatType(width, foundation.MayNull))
		for _, pair := range [][2]float64{{-1, 2}, {math.NaN(), 1}, {math.Inf(-1), math.Inf(1)}, {math.Copysign(0, -1), 0}} {
			pairs = append(pairs, [2]valueinterfaces.Value{externalMust(testbedvalues.NewFloat(typ, pair[0])), externalMust(testbedvalues.NewFloat(typ, pair[1]))})
		}
	}
	for _, pair := range pairs {
		for _, operator := range []foundation.OperatorType{foundation.Equal, foundation.NotEqual, foundation.LessThan, foundation.LessThanOrEqual, foundation.GreaterThan, foundation.GreaterThanOrEqual} {
			handle := externalMust(provider.ComparisonFunction(operator, pair[0].Type(), pair[1].Type()))
			expected := evaluateStandard(t, provider, handle, pair[0], pair[1])
			for _, args := range [][]valueinterfaces.Value{{fixtures.WrapScalar(pair[0]), pair[1]}, {pair[0], fixtures.WrapScalar(pair[1])}, {fixtures.WrapScalar(pair[0]), fixtures.WrapScalar(pair[1])}} {
				result := evaluateStandard(t, provider, handle, args...)
				if !result.EqualsTo(expected) {
					t.Fatalf("external comparison differs for %s, operator %v", pair[0].Type(), operator)
				}
				// Feed the output into a second interface-based evaluator.
				not := externalMust(provider.LookupBuiltInFunction(shared.BuiltInNot, []types.Type{result.Type()}))
				inverted := evaluateStandard(t, provider, not, fixtures.WrapScalar(result))
				if inverted.(valueinterfaces.BoolValue).Bool() == result.(valueinterfaces.BoolValue).Bool() {
					t.Fatal("result could not be consumed")
				}
			}
			null := fixtures.WrapScalar(externalMust(testbedvalues.NewNull(pair[0].Type())))
			if !evaluateStandard(t, provider, handle, null, fixtures.WrapScalar(pair[1])).IsNull() {
				t.Fatal("external null did not propagate")
			}
		}
	}
	equal := externalMust(provider.LookupBuiltInFunction(shared.BuiltInBoolEqual, []types.Type{boolean, boolean}))
	yes := fixtures.WrapScalar(externalMust(testbedvalues.NewBool(boolean, true)))
	if !evaluateStandard(t, provider, equal, yes, yes).(valueinterfaces.BoolValue).Bool() {
		t.Fatal("boolean equality failed")
	}
}

func TestStandardExternalArithmetic(t *testing.T) {
	provider := externalMust(testbed.NewStandardContext())
	typ := externalMust(types.NewIntType(types.Int64, foundation.MayNull))
	for _, tc := range []struct {
		name              shared.BuiltInFunctionName
		left, right, want int64
		unary             bool
		category          error
	}{
		{shared.BuiltInInt64Add, 7, 3, 10, false, nil},
		{shared.BuiltInInt64Subtract, 7, 3, 4, false, nil},
		{shared.BuiltInInt64Multiply, 7, 3, 21, false, nil},
		{shared.BuiltInInt64Divide, 7, 3, 2, false, nil},
		{shared.BuiltInInt64Modulus, 7, 3, 1, false, nil},
		{shared.BuiltInInt64Negate, 7, 0, -7, true, nil},
		{shared.BuiltInInt64Add, math.MaxInt64, 1, 0, false, shared.ErrEvaluationUnavailable},
		{shared.BuiltInInt64Divide, 7, 0, 0, false, shared.ErrEvaluationFailure},
	} {
		t.Run(string(tc.name), func(t *testing.T) {
			args := []valueinterfaces.Value{fixtures.WrapScalar(externalMust(testbedvalues.NewInteger(typ, tc.left)))}
			params := []types.Type{typ}
			if !tc.unary {
				args = append(args, externalMust(testbedvalues.NewInteger(typ, tc.right)))
				params = append(params, typ)
			}
			handle := externalMust(provider.LookupBuiltInFunction(tc.name, params))
			metadata := externalMust(provider.Metadata(handle))
			result, err := provider.Evaluate(handle, evalSignature(t, metadata, args...), args)
			if tc.category != nil {
				if result != nil || !errors.Is(err, tc.category) {
					t.Fatalf("lost error category: %v", err)
				}
				return
			}
			if err != nil || result.(valueinterfaces.IntegerValue).Integer() != tc.want || !result.Type().EqualsTo(typ) {
				t.Fatalf("wrong arithmetic result: %v, %v", result, err)
			}
		})
	}
}

func TestStandardExternalArrays(t *testing.T) {
	f := newArrayFixture(t)
	children := []valueinterfaces.Value{fixtures.WrapScalar(evalInteger(t, f.integer, 1)), evalInteger(t, f.integer, 2), fixtures.WrapScalar(evalInteger(t, f.integer, 2))}
	source := externalMust(testbedvalues.NewArray(f.array, children))
	array := externalArray{source, children}
	for _, nullable := range []bool{false, true} {
		var input valueinterfaces.Value = array
		if nullable {
			input = externalMust(input.WithNullability(foundation.MayNull))
		}
		for _, n := range []int64{2, 3} {
			candidate := fixtures.WrapScalar(evalInteger(t, f.integer, n))
			want := foundation.False
			if n == 2 {
				want = foundation.True
			}
			if f.evaluate(t, shared.BuiltInArrayContains, input, candidate) != want {
				t.Fatal("external array contains failed")
			}
			candidates := externalArray{externalMust(testbedvalues.NewArray(f.array, []valueinterfaces.Value{candidate})), []valueinterfaces.Value{candidate}}
			if f.evaluate(t, shared.BuiltInArrayContainsAny, input, candidates) != want {
				t.Fatal("external array any failed")
			}
		}
	}
	empty := externalArray{externalMust(testbedvalues.NewArray(f.array, nil)), nil}
	if f.evaluate(t, shared.BuiltInArrayContainsAny, array, empty) != foundation.False {
		t.Fatal("external empty array failed")
	}
	nullable := externalMust(f.array.WithNullability(foundation.MayNull))
	null := fixtures.WrapScalar(externalMust(testbedvalues.NewNull(nullable)))
	if f.evaluate(t, shared.BuiltInArrayContainsAny, null, array) != foundation.Null {
		t.Fatal("external null array did not propagate")
	}
}

func TestExternalEvaluatorResultsAndContracts(t *testing.T) {
	catalog, handle, metadata := evalFixture(t, []shared.NullPolicy{shared.PropagateNull})
	provider := externalMust(testbed.NewContextWithEvaluators(catalog, []testbed.EvaluationBinding{{Handle: handle, Evaluate: func(args []valueinterfaces.Value) (valueinterfaces.Value, error) {
		if _, ok := args[0].(fixtures.ScalarView); !ok {
			t.Fatal("adaptation lost external representation")
		}
		if args[0].Type().Nullability() != foundation.NonNull {
			t.Fatal("argument not adapted")
		}
		return fixtures.WrapScalar(evalInteger(t, metadata.Signature().ResultType(), 9)), nil
	}}}))
	nullable := externalMust(metadata.Signature().ArgumentTypes()[0].WithNullability(foundation.MayNull))
	arg := fixtures.WrapScalar(evalInteger(t, nullable, 1))
	signature := evalSignature(t, metadata, arg)
	result := externalMust(provider.Evaluate(handle, signature, []valueinterfaces.Value{arg}))
	if _, ok := result.(fixtures.ScalarView); !ok || !result.Type().EqualsTo(signature.ResultType()) {
		t.Fatal("external result adaptation failed")
	}
	standard := externalMust(testbed.NewStandardContext())
	equal := externalMust(standard.ComparisonFunction(foundation.Equal, result.Type(), result.Type()))
	if !evaluateStandard(t, standard, equal, result, evalInteger(t, result.Type(), 9)).(valueinterfaces.BoolValue).Bool() {
		t.Fatal("external output could not be consumed")
	}
	foreign := externalMust(testbed.NewStandardContext())
	if _, err := foreign.Evaluate(handle, signature, []valueinterfaces.Value{arg}); !errors.Is(err, shared.ErrIncompatibleHandle) {
		t.Fatalf("lost handle pairing: %v", err)
	}
	wrong := fixtures.WrapScalar(externalMust(testbedvalues.NewBool(externalMust(types.NewBoolType(foundation.NonNull)), true)))
	if _, err := provider.Evaluate(handle, signature, []valueinterfaces.Value{wrong}); !errors.Is(err, shared.ErrContractViolation) {
		t.Fatalf("lost argument validation: %v", err)
	}
}
