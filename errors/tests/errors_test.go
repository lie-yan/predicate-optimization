package errors_test

import (
	stderrs "errors"
	"fmt"
	"testing"

	opterrors "github.com/lie-yan/predicate-optimization/errors"
)

func TestCodeAssignments(t *testing.T) {
	for _, tc := range []struct {
		code  opterrors.Code
		value uint32
		name  string
	}{
		{opterrors.CodeUnknown, 0, "unknown"},
		{opterrors.CodeBindingArityMismatch, 0x0101, "binding_arity_mismatch"},
		{opterrors.CodeBindingArgumentTypeMismatch, 0x0102, "binding_argument_type_mismatch"},
		{opterrors.CodeBindingNullPolicyUnspecified, 0x0103, "binding_null_policy_unspecified"},
		{opterrors.CodeBindingResultNullability, 0x0104, "binding_result_nullability"},
		{opterrors.CodeInvalidCallBinding, 0x0105, "invalid_call_binding"},
		{opterrors.CodeBindingResultTypeMismatch, 0x0106, "binding_result_type_mismatch"},
		{opterrors.CodeAnalysisComparisonUnsupported, 0x0201, "analysis_comparison_unsupported"},
		{opterrors.CodeAnalysisLogicalUnsupported, 0x0202, "analysis_logical_unsupported"},
		{opterrors.CodeAnalysisConstantOnly, 0x0203, "analysis_constant_only"},
		{opterrors.Code(0xffffffff), 0xffffffff, "unknown"},
	} {
		if uint32(tc.code) != tc.value || tc.code.String() != tc.name {
			t.Fatalf("unstable code assignment: %d %q", tc.code, tc.code.String())
		}
	}
}

type countedError struct{ calls int }

func (e *countedError) Error() string {
	e.calls++
	return "original cause"
}

// A representative domain error overrides only formatting and embeds the
// shared classification implementation without a separately allocated header.
type detailedError struct{ opterrors.Base }

func (e *detailedError) Error() string {
	return fmt.Sprintf("detail: %s", e.Unwrap().Error())
}

func TestClassificationPreservesCausesWithoutFormatting(t *testing.T) {
	unsupported := stderrs.New("unsupported")
	contract := stderrs.New("contract")
	unrelated := stderrs.New("unrelated")
	cause := &countedError{}
	inner := opterrors.NewBase(opterrors.CodeBindingResultNullability, unsupported, cause)
	outer := &detailedError{opterrors.NewBase(opterrors.CodeInvalidCallBinding, contract, inner)}
	for _, err := range []error{outer, stderrs.Join(unrelated, outer)} {
		for _, target := range []error{unsupported, contract, cause} {
			if !stderrs.Is(err, target) {
				t.Fatal("lost category or cause")
			}
		}
		var concrete *countedError
		var coded opterrors.CodedError
		if !stderrs.As(err, &concrete) || concrete != cause {
			t.Fatal("lost concrete cause")
		}
		if !stderrs.As(err, &coded) || coded.Code() != opterrors.CodeInvalidCallBinding {
			t.Fatal("lost outer code")
		}
	}
	if outer.Is(unsupported) || outer.Is(cause) || outer.Is(nil) || stderrs.Is(outer, unrelated) {
		t.Fatal("direct Is must match only the category")
	}
	if cause.calls != 0 {
		t.Fatal("classification formatted a cause")
	}
	// Test lazy domain formatting with the original cause directly attached.
	detail := &detailedError{opterrors.NewBase(opterrors.CodeBindingResultNullability, unsupported, cause)}
	if detail.Error() != "detail: original cause" || cause.calls != 1 {
		t.Fatal("explicit diagnostic request did not render the cause")
	}
	wrapped := fmt.Errorf("wrapper: %w", detail)
	if !stderrs.Is(wrapped, unsupported) || !stderrs.Is(wrapped, cause) {
		t.Fatal("standard wrapping lost classification")
	}
}

func TestBaseFallbackAndUnknownCode(t *testing.T) {
	var zero opterrors.Base
	if zero.Code() != opterrors.CodeUnknown || zero.Error() != "unknown" || zero.Unwrap() != nil || zero.Is(nil) || stderrs.Is(zero, nil) {
		t.Fatal("invalid zero-value behavior")
	}
	cause := &countedError{}
	unknown := opterrors.NewBase(opterrors.Code(9999), nil, cause)
	if unknown.Code() != 9999 || unknown.Error() != "unknown" || unknown.Unwrap() != cause || cause.calls != 0 {
		t.Fatal("unknown code lost identity or formatted its cause")
	}
	category := stderrs.New("category")
	known := opterrors.NewBase(opterrors.CodeBindingArityMismatch, category, nil)
	if known.Error() != "binding_arity_mismatch" || !stderrs.Is(known, category) {
		t.Fatal("invalid fallback or category match")
	}
	other := opterrors.NewBase(known.Code(), stderrs.New("another category"), nil)
	if stderrs.Is(known, other) {
		t.Fatal("matching codes must not imply matching error identities")
	}
}
