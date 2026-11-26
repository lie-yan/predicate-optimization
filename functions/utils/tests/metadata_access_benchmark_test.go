package utils_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/functions/utils"
	"github.com/lie-yan/predicate-optimization/types"
)

type metadataReadContext struct {
	functioninterfaces.FunctionContext
	metadata *functions.FunctionMetadata
	queries  int
}

func (c *metadataReadContext) Metadata(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	c.queries++
	return c.metadata, nil
}

type metadataReadHandle struct {
	functioninterfaces.FunctionHandle
	signature *functions.FunctionSignature
	reads     int
}

func (h *metadataReadHandle) Signature() *functions.FunctionSignature {
	h.reads++
	return h.signature
}

var (
	metadataSink    *functions.FunctionMetadata
	recognitionSink foundation.Optional[foundation.OperatorType]
)

func BenchmarkMetadataInspection(b *testing.B) {
	for _, nullable := range []bool{false, true} {
		n, name := foundation.NonNull, "NonNull"
		if nullable {
			n, name = foundation.MayNull, "Nullable"
		}
		integer := must(types.NewIntType(types.Int64, n))
		for _, arity := range []int{0, 2, 64} {
			args := make([]types.Type, arity)
			for i := range args {
				args[i] = integer
			}
			metadata := inspectionMetadata(args)
			b.Run(fmt.Sprintf("%s/Arity%d", name, arity), func(b *testing.B) {
				context := &metadataReadContext{metadata: metadata}
				handle := &metadataReadHandle{signature: metadata.Signature()}
				if got, err := utils.InspectFunction(context, handle); err != nil || got != metadata {
					b.Fatal("invalid fixture", err)
				}
				context.queries, handle.reads = 0, 0
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					got, err := utils.InspectFunction(context, handle)
					if err != nil || got != metadata {
						b.Fatal("inspection changed", err)
					}
					metadataSink = got
				}
				b.StopTimer()
				if context.queries != b.N || handle.reads != b.N {
					b.Fatal("inspection skipped or repeated provider access")
				}
			})
		}
	}
}

func BenchmarkScalarRecognition(b *testing.B) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	narrow := must(types.NewIntType(types.Int8, foundation.NonNull))
	nullable := must(integer.WithNullability(foundation.MayNull))
	boolean := must(types.NewBoolType(foundation.NonNull))
	for _, tc := range []struct {
		name       string
		args       []types.Type
		op         foundation.OperatorType
		policy     functions.NullPolicy
		recognized bool
	}{
		{"NonNull", []types.Type{integer, integer}, foundation.Equal, functions.PropagateNull, true},
		{"Nullable", []types.Type{nullable, nullable}, foundation.Equal, functions.PropagateNull, true},
		{"ZeroArity", nil, foundation.Equal, functions.PropagateNull, false},
		{"Unary", []types.Type{integer}, foundation.Equal, functions.PropagateNull, false},
		{"Arithmetic", []types.Type{integer, integer}, foundation.Add, functions.PropagateNull, false},
		{"TypeMismatch", []types.Type{integer, narrow}, foundation.Equal, functions.PropagateNull, false},
		{"UnspecifiedPolicy", []types.Type{integer, integer}, foundation.Equal, functions.NullPolicyUnspecified, false},
	} {
		policies := make([]functions.NullPolicy, len(tc.args))
		for i := range policies {
			policies[i] = tc.policy
		}
		metadata := comparisonMetadata(tc.args, boolean, foundation.Some(tc.op), policies)
		signature := must(functions.BindFunctionSignature(metadata, tc.args))
		b.Run(tc.name, func(b *testing.B) {
			if _, present := utils.RecognizeScalarComparison(metadata, signature).Get(); present != tc.recognized {
				b.Fatal("invalid fixture")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result := utils.RecognizeScalarComparison(metadata, signature)
				op, present := result.Get()
				if present != tc.recognized || present && op != tc.op {
					b.Fatal("recognition changed")
				}
				recognitionSink = result
			}
		})
	}
}
