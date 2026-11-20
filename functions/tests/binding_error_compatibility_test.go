package functions_test

import (
	"errors"
	"fmt"
	"testing"

	opterrors "github.com/lie-yan/predicate-optimization/errors"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

// Inject an adaptation failure at the result boundary without changing argument
// comparisons. Built-in valid types normally support this adaptation.
type bindingAdaptationFailure struct {
	types.Type
	cause error
}

func (t bindingAdaptationFailure) WithNullability(foundation.Nullability) (types.Type, error) {
	return nil, t.cause
}

type bindingCause struct{}

func (*bindingCause) Error() string { return "adaptation rejected" }

type bindingFailureCase struct {
	name                  string
	run                   func() error
	message               string
	unsupported, contract bool
	cause                 error
	details               bindingErrorDetails
}

type bindingErrorDetails struct {
	code             opterrors.Code
	indexed          bool
	counts           bool
	actual, expected int
	innerCode        opterrors.Code
}

func bindingFailureCases() []bindingFailureCase {
	integer := bindingMust(types.NewIntType(types.Int64, foundation.NonNull))
	nullable := bindingMust(integer.WithNullability(foundation.MayNull))
	boolean := bindingMust(types.NewBoolType(foundation.NonNull))
	metadata := func(result types.Type, policy functions.NullPolicy) *functions.FunctionMetadata {
		signature := bindingMust(functions.NewFunctionSignature([]types.Type{integer}, result))
		return bindingMust(functions.NewFunctionMetadata(signature, "error fixture", noBuiltIn, noOperator, []functions.NullPolicy{policy}))
	}
	regular := metadata(integer, functions.PropagateNull)
	unspecified := metadata(integer, functions.NullPolicyUnspecified)
	cause := &bindingCause{}
	failing := metadata(bindingAdaptationFailure{Type: integer, cause: cause}, functions.PropagateNull)
	bind := func(m *functions.FunctionMetadata, args []types.Type) func() error {
		return func() error {
			_, err := functions.BindFunctionSignature(m, args)
			return err
		}
	}
	invalid := bindingMust(functions.NewFunctionSignature([]types.Type{boolean}, integer))
	wrongResult := bindingMust(functions.NewFunctionSignature([]types.Type{integer}, boolean))
	return []bindingFailureCase{
		{"Arity", bind(regular, nil), "unsupported function lookup: argument count 0 does not match arity 1", true, false, nil, bindingErrorDetails{code: opterrors.CodeBindingArityMismatch, counts: true, expected: 1}},
		{"ArgumentType", bind(regular, []types.Type{boolean}), "unsupported function lookup: argument 0 differs beyond outer nullability", true, false, nil, bindingErrorDetails{code: opterrors.CodeBindingArgumentTypeMismatch, indexed: true}},
		{"NullPolicy", bind(unspecified, []types.Type{nullable}), "unsupported function lookup: argument 0 is nullable with unspecified null policy", true, false, nil, bindingErrorDetails{code: opterrors.CodeBindingNullPolicyUnspecified, indexed: true}},
		{"AdaptationCause", bind(failing, []types.Type{nullable}), "unsupported function lookup: result nullability adaptation: adaptation rejected", true, false, cause, bindingErrorDetails{code: opterrors.CodeBindingResultNullability}},
		{"InvalidCall", func() error { return functions.ValidateCallSignature(regular, invalid) }, "function contract violated: invalid call binding: unsupported function lookup: argument 0 differs beyond outer nullability", true, true, nil, bindingErrorDetails{code: opterrors.CodeInvalidCallBinding, innerCode: opterrors.CodeBindingArgumentTypeMismatch}},
		{"ResultType", func() error { return functions.ValidateCallSignature(regular, wrongResult) }, "function contract violated: effective result type does not match binding", false, true, nil, bindingErrorDetails{code: opterrors.CodeBindingResultTypeMismatch}},
	}
}

func TestBindingErrorContract(t *testing.T) {
	for _, tc := range bindingFailureCases() {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil || err.Error() != tc.message {
				t.Fatalf("got %v, want %q", err, tc.message)
			}
			var detail *functions.BindingError
			if !errors.As(err, &detail) {
				t.Fatalf("missing binding details: %v", err)
			}
			index, indexed := detail.ArgumentIndex()
			actual, expected, counts := detail.ArgumentCounts()
			if index != 0 || indexed != tc.details.indexed || counts != tc.details.counts || actual != tc.details.actual || expected != tc.details.expected {
				t.Fatal("incorrect argument index or counts")
			}
			if tc.details.innerCode != opterrors.CodeUnknown {
				var inner *functions.BindingError
				if !errors.As(errors.Unwrap(detail), &inner) || inner.Code() != tc.details.innerCode {
					t.Fatal("lost inner binding code")
				}
			}
			for _, wrapped := range []error{err, fmt.Errorf("outer: %w", err), errors.Join(errors.New("other failure"), err)} {
				var coded opterrors.CodedError
				if !errors.As(wrapped, &coded) || coded.Code() != tc.details.code {
					t.Fatal("lost structured code")
				}
				if errors.Is(wrapped, functions.ErrUnsupportedLookup) != tc.unsupported || errors.Is(wrapped, functions.ErrContractViolation) != tc.contract {
					t.Fatalf("wrong categories: %v", wrapped)
				}
				if tc.cause != nil {
					var cause *bindingCause
					if !errors.Is(wrapped, tc.cause) || !errors.As(wrapped, &cause) || cause != tc.cause {
						t.Fatalf("lost adaptation cause: %v", wrapped)
					}
				}
			}
		})
	}
	var zero functions.BindingError
	if zero.Code() != opterrors.CodeUnknown || zero.Error() != "unknown" {
		t.Fatal("invalid zero value")
	}
}

var bindingMessageSink string

func BenchmarkBindingErrors(b *testing.B) {
	for _, tc := range bindingFailureCases() {
		for _, message := range []bool{false, true} {
			mode := "Discard"
			if message {
				mode = "Message"
			}
			b.Run(tc.name+"/"+mode, func(b *testing.B) {
				if err := tc.run(); err == nil || err.Error() != tc.message {
					b.Fatalf("unexpected fixture result: %v", err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					err := tc.run()
					bindingErrorSink = err
					if message {
						bindingMessageSink = err.Error()
					}
				}
			})
		}
	}
}
