package functions_test

import (
	"errors"
	"testing"

	opterrors "github.com/lie-yan/predicate-optimization/errors"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestBindingProbeCompatibility(t *testing.T) {
	integer := bindingInt(t, types.Int64, foundation.NonNull)
	nullable := bindingInt(t, types.Int64, foundation.MayNull)
	boolean := bindingMust(types.NewBoolType(foundation.NonNull))
	cause := errors.Join(errors.New("adaptation"), functions.ErrContractViolation)
	for _, tc := range []struct {
		name        string
		args        []types.Type
		result      types.Type
		policy      functions.NullPolicy
		code        opterrors.Code
		message     string
		joinedCause bool
	}{
		{"Scalar", []types.Type{integer}, integer, functions.PropagateNull, opterrors.CodeUnknown, "", false},
		{"Nullable", []types.Type{nullable}, integer, functions.PropagateNull, opterrors.CodeUnknown, "", false},
		{"MissingArgument", nil, integer, functions.PropagateNull, opterrors.CodeBindingArityMismatch, "unsupported function lookup: argument count 0 does not match arity 1", false},
		{"ExtraArgument", []types.Type{integer, integer}, integer, functions.PropagateNull, opterrors.CodeBindingArityMismatch, "unsupported function lookup: argument count 2 does not match arity 1", false},
		{"TypeMismatch", []types.Type{boolean}, integer, functions.PropagateNull, opterrors.CodeBindingArgumentTypeMismatch, "unsupported function lookup: argument 0 differs beyond outer nullability", false},
		{"UnspecifiedPolicy", []types.Type{nullable}, integer, functions.NullPolicyUnspecified, opterrors.CodeBindingNullPolicyUnspecified, "unsupported function lookup: argument 0 is nullable with unspecified null policy", false},
		{"JoinedCause", []types.Type{nullable}, bindingAdaptationFailure{Type: integer, cause: cause}, functions.PropagateNull, opterrors.CodeBindingResultNullability, "unsupported function lookup: result nullability adaptation: adaptation\nfunction contract violated", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata := bindingMetadata(t, []types.Type{integer}, tc.result, []functions.NullPolicy{tc.policy})
			check := functions.ProbeFunctionBinding(metadata, tc.args)
			got := check.Err()
			failed := tc.code != opterrors.CodeUnknown
			if check.Supported() == failed || (got != nil) != failed {
				t.Fatal("unexpected probe outcome")
			}
			// Check both adapters against independent expectations, not only each other.
			for _, err := range []error{got, functions.CheckFunctionBinding(metadata, tc.args)} {
				if (err != nil) != failed {
					t.Fatal("unexpected binding outcome")
				}
				if failed {
					var coded opterrors.CodedError
					if err.Error() != tc.message || !errors.As(err, &coded) || coded.Code() != tc.code {
						t.Fatalf("unexpected diagnostic: %v", err)
					}
				}
				for _, target := range []struct {
					err  error
					want bool
				}{
					{functions.ErrUnsupportedLookup, failed},
					{functions.ErrContractViolation, tc.joinedCause},
					{cause, tc.joinedCause},
				} {
					if errors.Is(err, target.err) != target.want || check.Is(target.err) != target.want {
						t.Fatal("unexpected category or cause")
					}
				}
				// errors.Is(nil, nil) is true; BindingCheck.Is(nil) is always false.
				if errors.Is(err, nil) != !failed || check.Is(nil) {
					t.Fatal("unexpected nil-target behavior")
				}
			}
		})
	}

	metadata := bindingMetadata(t, []types.Type{integer}, integer, []functions.NullPolicy{functions.PropagateNull})
	args := []types.Type{boolean}
	if n := testing.AllocsPerRun(100, func() {
		check := functions.ProbeFunctionBinding(metadata, args)
		if check.Supported() || !check.Is(functions.ErrUnsupportedLookup) {
			panic("wrong rejection")
		}
	}); n != 0 {
		t.Fatalf("probe allocated %g times", n)
	}
}
