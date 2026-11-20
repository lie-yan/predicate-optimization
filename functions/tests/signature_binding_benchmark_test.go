package functions_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

var (
	bindingSignatureSink *functions.FunctionSignature
	bindingErrorSink     error
)

func BenchmarkSignatureBinding(b *testing.B) {
	integer := bindingMust(types.NewIntType(types.Int64, foundation.NonNull))
	nullable := bindingMust(integer.WithNullability(foundation.MayNull))
	boolean := bindingMust(types.NewBoolType(foundation.NonNull))
	array := bindingMust(types.NewArrayType(integer, foundation.NonNull))
	nullableArray := bindingMust(array.WithNullability(foundation.MayNull))
	differentArray := bindingMust(types.NewArrayType(nullable, foundation.NonNull))
	for _, tc := range []struct {
		name                string
		parameter, argument types.Type
		invalid             bool
	}{
		{"Scalar", integer, integer, false},
		{"NullableScalar", integer, nullable, false},
		{"NullableArray", array, nullableArray, false},
		{"NestedMismatch", array, differentArray, true},
	} {
		arguments := []types.Type{tc.argument, tc.argument}
		signature := bindingMust(functions.NewFunctionSignature([]types.Type{tc.parameter, tc.parameter}, boolean))
		metadata := bindingMust(functions.NewFunctionMetadata(signature, "benchmark", noBuiltIn, noOperator, []functions.NullPolicy{functions.PropagateNull, functions.PropagateNull}))
		call, err := functions.BindFunctionSignature(metadata, arguments)
		if (err != nil) != tc.invalid {
			b.Fatalf("%s: unexpected binding: %v", tc.name, err)
		}
		if tc.invalid {
			call = bindingMust(functions.NewFunctionSignature(arguments, boolean))
		}
		b.Run(tc.name+"/Bind", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				result, err := functions.BindFunctionSignature(metadata, arguments)
				if (err != nil) != tc.invalid {
					b.Fatal("unexpected binding outcome")
				}
				bindingSignatureSink, bindingErrorSink = result, err
			}
		})
		b.Run(tc.name+"/Check", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				err := functions.CheckFunctionBinding(metadata, arguments)
				if (err != nil) != tc.invalid {
					b.Fatal("unexpected check outcome")
				}
				bindingErrorSink = err
			}
		})
		b.Run(tc.name+"/Validate", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				err := functions.ValidateCallSignature(metadata, call)
				if (err != nil) != tc.invalid {
					b.Fatal("unexpected validation outcome")
				}
				bindingErrorSink = err
			}
		})
	}
}
