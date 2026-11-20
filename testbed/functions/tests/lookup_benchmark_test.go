package functions_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

var lookupHandleSink functioninterfaces.FunctionHandle

func BenchmarkComparisonLookup(b *testing.B) {
	context, err := testbed.NewStandardContext()
	if err != nil {
		b.Fatal(err)
	}
	for _, nullable := range []bool{false, true} {
		name, nullability := "NonNull", foundation.NonNull
		if nullable {
			name, nullability = "Nullable", foundation.MayNull
		}
		integer, err := types.NewIntType(types.Int64, nullability)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				handle, err := context.ComparisonFunction(foundation.Equal, integer, integer)
				if err != nil {
					b.Fatal(err)
				}
				lookupHandleSink = handle
			}
		})
	}
}
