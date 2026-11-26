package utils_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/functions/utils"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestMetadataInspectionReadsEveryTime(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	boolean := must(types.NewBoolType(foundation.NonNull))
	metadata := inspectionMetadata([]types.Type{integer, integer})
	context := &metadataReadContext{metadata: metadata}
	handle := &metadataReadHandle{signature: metadata.Signature()}
	for i := 1; i <= 2; i++ {
		got, err := utils.InspectFunction(context, handle)
		if err != nil || got != metadata || context.queries != i || handle.reads != i {
			t.Fatal("inspection changed its read contract", err)
		}
	}
	// Separate invalid inputs retain exact mismatch diagnostics and read counts.
	for _, tc := range []struct {
		args    []types.Type
		message string
	}{
		{nil, "function contract violated: handle/metadata arity mismatch"},
		{[]types.Type{integer, boolean}, "function contract violated: handle/metadata argument 1 mismatch"},
	} {
		c := &metadataReadContext{metadata: metadata}
		h := &metadataReadHandle{signature: inspectionMetadata(tc.args).Signature()}
		got, err := utils.InspectFunction(c, h)
		if got != nil || !errors.Is(err, functions.ErrContractViolation) || err.Error() != tc.message || c.queries != 1 || h.reads != 1 {
			t.Fatalf("unexpected mismatch behavior: %v", err)
		}
	}
}
