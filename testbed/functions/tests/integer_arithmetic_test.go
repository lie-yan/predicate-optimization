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
)

func TestStandardIntegerArithmetic(t *testing.T) {
	provider, err := testbed.NewStandardContext()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		op      foundation.OperatorType
		args    []int64
		want    int64
		failure error
	}{
		{"negate", foundation.Negate, []int64{7}, -7, nil},
		{"negate negative", foundation.Negate, []int64{-7}, 7, nil},
		{"negate zero", foundation.Negate, []int64{0}, 0, nil},
		{"negate max", foundation.Negate, []int64{math.MaxInt64}, -math.MaxInt64, nil},
		{"negate overflow", foundation.Negate, []int64{math.MinInt64}, 0, shared.ErrEvaluationUnavailable},
		{"add", foundation.Add, []int64{7, -3}, 4, nil},
		{"add max", foundation.Add, []int64{math.MaxInt64, 0}, math.MaxInt64, nil},
		{"add min", foundation.Add, []int64{math.MinInt64, 0}, math.MinInt64, nil},
		{"add overflow", foundation.Add, []int64{math.MaxInt64, 1}, 0, shared.ErrEvaluationUnavailable},
		{"add underflow", foundation.Add, []int64{math.MinInt64, -1}, 0, shared.ErrEvaluationUnavailable},
		{"subtract", foundation.Subtract, []int64{7, 3}, 4, nil},
		{"subtract min", foundation.Subtract, []int64{math.MinInt64, 0}, math.MinInt64, nil},
		{"subtract max", foundation.Subtract, []int64{math.MaxInt64, 0}, math.MaxInt64, nil},
		{"subtract overflow", foundation.Subtract, []int64{math.MaxInt64, -1}, 0, shared.ErrEvaluationUnavailable},
		{"subtract underflow", foundation.Subtract, []int64{math.MinInt64, 1}, 0, shared.ErrEvaluationUnavailable},
		{"multiply", foundation.Multiply, []int64{-7, 3}, -21, nil},
		{"multiply min", foundation.Multiply, []int64{math.MinInt64, 1}, math.MinInt64, nil},
		{"multiply max", foundation.Multiply, []int64{math.MaxInt64, 1}, math.MaxInt64, nil},
		{"multiply zero", foundation.Multiply, []int64{math.MinInt64, 0}, 0, nil},
		{"multiply overflow", foundation.Multiply, []int64{math.MaxInt64, 2}, 0, shared.ErrEvaluationUnavailable},
		{"multiply underflow", foundation.Multiply, []int64{math.MinInt64, 2}, 0, shared.ErrEvaluationUnavailable},
		{"divide", foundation.Divide, []int64{7, 3}, 2, nil},
		{"divide negative", foundation.Divide, []int64{-7, 3}, -2, nil},
		{"divide negative divisor", foundation.Divide, []int64{7, -3}, -2, nil},
		{"divide both negative", foundation.Divide, []int64{-7, -3}, 2, nil},
		{"divide min", foundation.Divide, []int64{math.MinInt64, 1}, math.MinInt64, nil},
		{"divide max", foundation.Divide, []int64{math.MaxInt64, 1}, math.MaxInt64, nil},
		{"divide zero", foundation.Divide, []int64{1, 0}, 0, shared.ErrEvaluationFailure},
		{"divide overflow", foundation.Divide, []int64{math.MinInt64, -1}, 0, shared.ErrEvaluationUnavailable},
		{"modulus", foundation.Modulus, []int64{7, 3}, 1, nil},
		{"modulus negative", foundation.Modulus, []int64{-7, 3}, -1, nil},
		{"modulus negative divisor", foundation.Modulus, []int64{7, -3}, 1, nil},
		{"modulus both negative", foundation.Modulus, []int64{-7, -3}, -1, nil},
		{"modulus min", foundation.Modulus, []int64{math.MinInt64, 1}, 0, nil},
		{"modulus max", foundation.Modulus, []int64{math.MaxInt64, 2}, 1, nil},
		{"modulus zero", foundation.Modulus, []int64{1, 0}, 0, shared.ErrEvaluationFailure},
		{"modulus division overflow", foundation.Modulus, []int64{math.MinInt64, -1}, 0, shared.ErrEvaluationUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
				integer, err := types.NewIntType(types.Int64, n)
				if err != nil {
					t.Fatal(err)
				}
				args := make([]valueinterfaces.Value, len(tc.args))
				argTypes := make([]types.Type, len(args))
				for i, number := range tc.args {
					args[i] = evalInteger(t, integer, number)
					argTypes[i] = integer
				}
				handle, err := provider.ArithmeticFunction(tc.op, argTypes)
				if err != nil {
					t.Fatal(err)
				}
				metadata, err := provider.Metadata(handle)
				if err != nil {
					t.Fatal(err)
				}
				if !metadata.Signature().ResultType().EqualsTo(mustNonNullInt(t, integer)) {
					t.Fatal("wrong implementation result")
				}
				signature := evalSignature(t, metadata, args...)
				result, err := provider.Evaluate(handle, signature, args)
				if tc.failure != nil {
					if result != nil || !errors.Is(err, tc.failure) {
						t.Fatalf("got %v, %v; want %v", result, err, tc.failure)
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					if !result.Type().EqualsTo(signature.ResultType()) || result.(*testbedvalues.IntegerValue).Integer() != tc.want {
						t.Fatalf("incorrect result: %v", result)
					}
				}

				if n == foundation.MayNull {
					null, err := testbedvalues.NewNull(integer)
					if err != nil {
						t.Fatal(err)
					}
					for i := range args {
						nullableArgs := append([]valueinterfaces.Value(nil), args...)
						nullableArgs[i] = null
						if result := evaluateStandard(t, provider, handle, nullableArgs...); !result.IsNull() {
							t.Fatal("null did not propagate")
						}
					}
				}
			}
		})
	}
}

func mustNonNullInt(t *testing.T, integer *types.IntType) types.Type {
	t.Helper()
	result, err := integer.WithNullability(foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
