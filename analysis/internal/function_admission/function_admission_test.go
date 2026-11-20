package function_admission

import (
	"errors"
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	expressiontestbed "github.com/lie-yan/predicate-optimization/testbed/expressions"
	functiontestbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Shared admission checks are exercised with independent provider fixtures.
func admissionMust[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func admissionReference(typ types.Type) *expressions.ReferenceExpression {
	return admissionMust(expressions.NewReferenceExpression(admissionMust(expressiontestbed.NewColumnID("t", "x")), typ, "x"))
}

// Unexpected evaluation fails immediately; admission must inspect metadata only.
type admissionContext struct {
	functioninterfaces.FunctionContext
	metadata func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error)
	lookup   func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error)
}

func (c *admissionContext) Metadata(h functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	if c.metadata != nil {
		return c.metadata(h)
	}
	return c.FunctionContext.Metadata(h)
}

func (c *admissionContext) ComparisonFunction(op foundation.OperatorType, l, r types.Type) (functioninterfaces.FunctionHandle, error) {
	if c.lookup != nil {
		return c.lookup(op, l, r)
	}
	return c.FunctionContext.ComparisonFunction(op, l, r)
}

func (*admissionContext) Evaluate(functioninterfaces.FunctionHandle, *functions.FunctionSignature, []valueinterfaces.Value) (valueinterfaces.Value, error) {
	panic("admission must not evaluate")
}

func TestStandardComparisonAdmission(t *testing.T) {
	context := &admissionContext{FunctionContext: admissionMust(functiontestbed.NewStandardContext())}
	for _, typ := range []types.Type{
		admissionMust(types.NewBoolType(foundation.NonNull)),
		admissionMust(types.NewIntType(types.Int64, foundation.NonNull)),
		admissionMust(types.NewFloatType(types.Float32, foundation.NonNull)),
		admissionMust(types.NewFloatType(types.Float64, foundation.NonNull)),
		admissionMust(types.NewVarCharType(nil, foundation.NonNull)),
	} {
		t.Run(typ.String(), func(t *testing.T) {
			for _, leftNull := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
				for _, rightNull := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
					left, right := admissionMust(typ.WithNullability(leftNull)), admissionMust(typ.WithNullability(rightNull))
					for _, operator := range []foundation.OperatorType{foundation.Equal, foundation.NotEqual, foundation.LessThan, foundation.LessThanOrEqual, foundation.GreaterThan, foundation.GreaterThanOrEqual} {
						if _, boolean := typ.(*types.BoolType); boolean && operator != foundation.Equal {
							continue
						}
						handle := admissionMust(context.ComparisonFunction(operator, left, right))
						call := admissionMust(expressions.NewCallExpression(context, handle, []expressions.Expression{admissionReference(left), admissionReference(right)}))
						got, err := CheckComparison(call)
						if err != nil || got != operator {
							t.Fatalf("operator %v: %v, %v", operator, got, err)
						}
						wantNull := foundation.NonNull
						if leftNull == foundation.MayNull || rightNull == foundation.MayNull {
							wantNull = foundation.MayNull
						}
						if call.Type().Nullability() != wantNull {
							t.Fatal("effective nullability lost")
						}
					}
					if err := CheckMembershipEquality(context, left, right); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func admissionProvider(args []types.Type, result types.Type, operator foundation.Optional[foundation.OperatorType], policies []functions.NullPolicy) (*admissionContext, functioninterfaces.FunctionHandle) {
	metadata := admissionMust(functions.NewFunctionMetadata(admissionMust(functions.NewFunctionSignature(args, result)), "int64.equal", foundation.Optional[functions.BuiltInFunctionName]{}, operator, policies))
	catalog := admissionMust(functiontestbed.NewCatalog([]functiontestbed.Definition{{Namespace: "fixture", Name: "int64.equal", Metadata: metadata}}))
	return &admissionContext{FunctionContext: functiontestbed.NewContext(catalog)}, catalog.Handles()[0]
}

func TestComparisonAdmissionRejectsUnsupportedSemantics(t *testing.T) {
	integer := admissionMust(types.NewIntType(types.Int64, foundation.NonNull))
	narrow := admissionMust(types.NewIntType(types.Int8, foundation.NonNull))
	boolean := admissionMust(types.NewBoolType(foundation.NonNull))
	text := admissionMust(types.NewVarCharType(nil, foundation.NonNull))
	bound := uint64(4)
	bounded := admissionMust(types.NewVarCharType(&bound, foundation.NonNull))
	array := admissionMust(types.NewArrayType(integer, foundation.NonNull))
	structure := admissionMust(types.NewStructType(nil, foundation.NonNull))
	nullable := admissionMust(integer.WithNullability(foundation.MayNull))
	propagate := []functions.NullPolicy{functions.PropagateNull, functions.PropagateNull}
	for _, tc := range []struct {
		name     string
		args     []types.Type
		result   types.Type
		operator foundation.Optional[foundation.OperatorType]
		policies []functions.NullPolicy
	}{
		{"diagnostic name only", []types.Type{integer, integer}, boolean, foundation.Optional[foundation.OperatorType]{}, propagate},
		{"arithmetic", []types.Type{integer, integer}, integer, foundation.Some(foundation.Add), propagate},
		{"wrong arity", []types.Type{integer}, boolean, foundation.Some(foundation.Equal), propagate[:1]},
		{"mixed widths", []types.Type{integer, narrow}, boolean, foundation.Some(foundation.Equal), propagate},
		{"mixed bounds", []types.Type{text, bounded}, boolean, foundation.Some(foundation.Equal), propagate},
		{"array", []types.Type{array, array}, boolean, foundation.Some(foundation.Equal), propagate},
		{"struct", []types.Type{structure, structure}, boolean, foundation.Some(foundation.Equal), propagate},
		{"Boolean ordering", []types.Type{boolean, boolean}, boolean, foundation.Some(foundation.LessThan), propagate},
		{"non Boolean result", []types.Type{integer, integer}, integer, foundation.Some(foundation.Equal), propagate},
		{"nullable implementation result", []types.Type{integer, integer}, admissionMust(boolean.WithNullability(foundation.MayNull)), foundation.Some(foundation.Equal), propagate},
		{"unspecified left", []types.Type{integer, integer}, boolean, foundation.Some(foundation.Equal), []functions.NullPolicy{functions.NullPolicyUnspecified, functions.PropagateNull}},
		{"unspecified right", []types.Type{integer, integer}, boolean, foundation.Some(foundation.Equal), []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified}},
		{"pass null", []types.Type{nullable, integer}, boolean, foundation.Some(foundation.Equal), []functions.NullPolicy{functions.PassNull, functions.PropagateNull}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			context, handle := admissionProvider(tc.args, tc.result, tc.operator, tc.policies)
			arguments := make([]expressions.Expression, len(tc.args))
			for i, typ := range tc.args {
				arguments[i] = admissionReference(typ)
			}
			call := admissionMust(expressions.NewCallExpression(context, handle, arguments))
			if _, err := CheckComparison(call); !errors.Is(err, value_set.ErrUnsupported) {
				t.Fatalf("expected unsupported, got %v", err)
			}
		})
	}
}

func TestAdmissionUsesPairedContexts(t *testing.T) {
	typ := admissionMust(types.NewIntType(types.Int64, foundation.MayNull))
	nonNull := admissionMust(typ.WithNullability(foundation.NonNull))
	own := &admissionContext{FunctionContext: admissionMust(functiontestbed.NewStandardContext())}
	other := admissionMust(functiontestbed.NewStandardContext())
	handle := admissionMust(own.ComparisonFunction(foundation.Equal, typ, nonNull))
	call := admissionMust(expressions.NewCallExpression(own, handle, []expressions.Expression{admissionReference(typ), admissionReference(nonNull)}))
	own.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
		panic("inspection must not resolve call again")
	}
	if _, err := CheckComparison(call); err != nil {
		t.Fatal(err)
	}
	lookups := 0
	membership := &admissionContext{FunctionContext: other}
	membership.lookup = func(op foundation.OperatorType, left, right types.Type) (functioninterfaces.FunctionHandle, error) {
		lookups++
		if op != foundation.Equal || left != typ || right != nonNull {
			t.Fatal("incorrect equality lookup")
		}
		return other.ComparisonFunction(op, left, right)
	}
	if err := CheckMembershipEquality(membership, typ, nonNull); err != nil || lookups != 1 {
		t.Fatalf("membership context: %d, %v", lookups, err)
	}
	membership.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
		return handle, nil
	}
	if err := CheckMembershipEquality(membership, typ, nonNull); !errors.Is(err, functions.ErrIncompatibleHandle) || errors.Is(err, value_set.ErrUnsupported) {
		t.Fatalf("foreign handle: %v", err)
	}
}

func TestMembershipAdmissionErrors(t *testing.T) {
	typ := admissionMust(types.NewIntType(types.Int64, foundation.NonNull))
	context := &admissionContext{FunctionContext: admissionMust(functiontestbed.NewStandardContext())}
	bound := uint64(3)
	for _, unsupported := range []types.Type{admissionMust(types.NewIntType(types.Int8, foundation.NonNull)), admissionMust(types.NewVarCharType(&bound, foundation.NonNull))} {
		err := CheckMembershipEquality(context, unsupported, unsupported)
		if !errors.Is(err, value_set.ErrUnsupported) || !errors.Is(err, functions.ErrUnsupportedLookup) {
			t.Fatalf("unavailable overload: %v", err)
		}
	}
	for _, cause := range []error{functions.ErrUnknownHandle, functions.ErrIncompatibleHandle, functions.ErrContractViolation, errors.Join(functions.ErrContractViolation, functions.ErrUnsupportedLookup)} {
		context.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
			return nil, fmt.Errorf("fixture: %w", cause)
		}
		if err := CheckMembershipEquality(context, typ, typ); !errors.Is(err, cause) || errors.Is(err, value_set.ErrUnsupported) {
			t.Fatalf("provider error reclassified: %v", err)
		}
	}
	for _, handle := range []functioninterfaces.FunctionHandle{nil, (*functiontestbed.Handle)(nil)} {
		context.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
			return handle, nil
		}
		if err := CheckMembershipEquality(context, typ, typ); !errors.Is(err, functions.ErrContractViolation) {
			t.Fatalf("nil success: %v", err)
		}
	}
	standard := admissionMust(functiontestbed.NewStandardContext())
	for _, handle := range []functioninterfaces.FunctionHandle{
		admissionMust(standard.ComparisonFunction(foundation.NotEqual, typ, typ)),
		admissionMust(standard.ComparisonFunction(foundation.Equal, admissionMust(types.NewFloatType(types.Float64, foundation.NonNull)), admissionMust(types.NewFloatType(types.Float64, foundation.NonNull)))),
	} {
		context.FunctionContext = standard
		context.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
			return handle, nil
		}
		if err := CheckMembershipEquality(context, typ, typ); !errors.Is(err, functions.ErrContractViolation) || errors.Is(err, value_set.ErrUnsupported) {
			t.Fatalf("wrong resolved operator/signature: %v", err)
		}
	}
}

func TestComparisonProviderContractFailures(t *testing.T) {
	typ := admissionMust(types.NewIntType(types.Int64, foundation.NonNull))
	context := &admissionContext{FunctionContext: admissionMust(functiontestbed.NewStandardContext())}
	handle := admissionMust(context.ComparisonFunction(foundation.Equal, typ, typ))
	call := admissionMust(expressions.NewCallExpression(context, handle, []expressions.Expression{admissionReference(typ), admissionReference(typ)}))
	// Deliberately violate provider stability after constructing the call.
	for _, cause := range []error{functions.ErrUnknownHandle, functions.ErrIncompatibleHandle, functions.ErrContractViolation} {
		context.metadata = func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) { return nil, cause }
		if _, err := CheckComparison(call); !errors.Is(err, cause) || errors.Is(err, value_set.ErrUnsupported) {
			t.Fatalf("metadata failure: %v", err)
		}
	}
	// Nil/mismatched metadata and changed stored signatures are boundary tests in
	// expressions/tests/optimization_validation_test.go and call_boundary_test.go.
	// Such invalid input is outside CheckComparison's trusted-input precondition.
}

func TestAdmissionUsesProviderCapabilitiesBeyondStandardTestbed(t *testing.T) {
	boolean := admissionMust(types.NewBoolType(foundation.NonNull))
	bound := uint64(8)
	for _, typ := range []types.Type{
		boolean,
		admissionMust(types.NewIntType(types.Int8, foundation.NonNull)),
		admissionMust(types.NewIntType(types.Int16, foundation.NonNull)),
		admissionMust(types.NewIntType(types.Int32, foundation.NonNull)),
		admissionMust(types.NewVarCharType(&bound, foundation.NonNull)),
	} {
		// This provider has metadata but no evaluator; names deliberately do not
		// match the actual operand domain. Only standard operator identity matters.
		context, handle := admissionProvider([]types.Type{typ, typ}, boolean, foundation.Some(foundation.Equal), []functions.NullPolicy{functions.PropagateNull, functions.PropagateNull})
		nullable := admissionMust(typ.WithNullability(foundation.MayNull))
		call := admissionMust(expressions.NewCallExpression(context, handle, []expressions.Expression{admissionReference(nullable), admissionReference(typ)}))
		if _, err := CheckComparison(call); err != nil {
			t.Fatal(err)
		}
		if err := CheckMembershipEquality(context, nullable, typ); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMembershipRejectsUnsupportedNullContracts(t *testing.T) {
	integer := admissionMust(types.NewIntType(types.Int64, foundation.NonNull))
	nullable := admissionMust(integer.WithNullability(foundation.MayNull))
	boolean := admissionMust(types.NewBoolType(foundation.NonNull))
	for _, tc := range []struct {
		parameter types.Type
		result    types.Type
		policy    functions.NullPolicy
	}{
		{integer, boolean, functions.NullPolicyUnspecified},
		{nullable, boolean, functions.PassNull},
		{integer, admissionMust(boolean.WithNullability(foundation.MayNull)), functions.PropagateNull},
	} {
		context, _ := admissionProvider([]types.Type{tc.parameter, integer}, tc.result, foundation.Some(foundation.Equal), []functions.NullPolicy{tc.policy, functions.PropagateNull})
		if err := CheckMembershipEquality(context, integer, integer); !errors.Is(err, value_set.ErrUnsupported) || errors.Is(err, functions.ErrContractViolation) {
			t.Fatalf("unsupported null contract: %v", err)
		}
	}
}

// A provider-defined result can fail adaptation even when the argument domain
// is valid. Admission must bind before rejecting the result's semantics.
type admissionAdaptationFailure struct {
	types.Type
	cause error
}

func (t admissionAdaptationFailure) WithNullability(foundation.Nullability) (types.Type, error) {
	return nil, t.cause
}

func TestMembershipAdmissionPreservesResultAdaptationFailure(t *testing.T) {
	integer := admissionMust(types.NewIntType(types.Int64, foundation.NonNull))
	nullable := admissionMust(integer.WithNullability(foundation.MayNull))
	boolean := admissionMust(types.NewBoolType(foundation.NonNull))
	cause := errors.New("result adaptation failed")
	context, handle := admissionProvider([]types.Type{integer, integer}, admissionAdaptationFailure{Type: boolean, cause: cause}, foundation.Some(foundation.Equal), []functions.NullPolicy{functions.PropagateNull, functions.PropagateNull})
	// Resolve the handle without letting the testbed's own binding hide which
	// boundary detected the failure.
	context.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
		return handle, nil
	}
	err := CheckMembershipEquality(context, nullable, integer)
	if !errors.Is(err, cause) || !errors.Is(err, functions.ErrContractViolation) || !errors.Is(err, functions.ErrUnsupportedLookup) || errors.Is(err, value_set.ErrUnsupported) {
		t.Fatalf("adaptation failure lost or treated as unsupported semantics: %v", err)
	}
	want := "function contract violated: resolved equality cannot bind requested types: unsupported function lookup: result nullability adaptation: result adaptation failed"
	if err.Error() != want {
		t.Fatalf("got %v, want %s", err, want)
	}
}
