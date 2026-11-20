package array_membership_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/internal/array_membership"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

// Recognition matches the requested semantic family, not just any valid metadata.
// Shape, null-policy and generic element cases also run through the public rule.
func TestRequestedMembershipFamily(t *testing.T) {
	context := must(testbed.NewStandardContext())
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	array := must(types.NewArrayType(integer, foundation.NonNull))
	for _, actual := range []functions.BuiltInFunctionName{functions.BuiltInArrayContains, functions.BuiltInArrayContainsAny} {
		var candidate types.Type = integer
		if actual == functions.BuiltInArrayContainsAny {
			candidate = array
		}
		handle := must(context.LookupBuiltInFunction(actual, []types.Type{array, candidate}))
		metadata := must(context.Metadata(handle))
		for _, requested := range []functions.BuiltInFunctionName{functions.BuiltInArrayContains, functions.BuiltInArrayContainsAny, functions.BuiltInNot, "custom.membership"} {
			if got := array_membership.RecognizeMetadata(metadata, requested); got != (requested == actual) {
				t.Fatalf("metadata %q, requested %q: recognition=%t", actual, requested, got)
			}
		}
	}
}

func TestUnsupportedTargetLookupClassification(t *testing.T) {
	unsupported := fmt.Errorf("lookup: %w", functions.ErrUnsupportedLookup)
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"uncategorized", errors.New("provider failure"), false},
		{"wrapped unsupported", unsupported, true},
		{"joined unsupported causes", errors.Join(unsupported, functions.ErrUnsupportedLookup), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := array_membership.IsUnsupportedTargetLookup(tc.err); got != tc.want {
				t.Fatalf("classification=%t, want %t", got, tc.want)
			}
		})
	}
	for _, category := range []error{functions.ErrUnknownHandle, functions.ErrIncompatibleHandle, functions.ErrContractViolation, functions.ErrEvaluationFailure, functions.ErrEvaluationUnavailable, functions.ErrEvaluationDisallowed} {
		t.Run(category.Error(), func(t *testing.T) {
			// Nest the failure on either side: cause ordering must not grant fallback.
			failure := fmt.Errorf("provider: %w", errors.Join(errors.New("detail"), category))
			for _, err := range []error{failure, errors.Join(unsupported, failure), errors.Join(failure, unsupported)} {
				if array_membership.IsUnsupportedTargetLookup(err) {
					t.Fatalf("stronger failure became an unsupported miss: %v", err)
				}
			}
		})
	}
}
