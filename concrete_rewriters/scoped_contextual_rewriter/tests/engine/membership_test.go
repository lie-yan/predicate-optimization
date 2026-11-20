package engine_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/internal/boolean_cleanup"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"

	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"

	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

type membershipEligibilityContext struct {
	*eligibilityContext
	lookup  func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error)
	lookups int
}

func (c *membershipEligibilityContext) ComparisonFunction(op foundation.OperatorType, left, right types.Type) (functioninterfaces.FunctionHandle, error) {
	c.lookups++
	if c.lookup != nil {
		return c.lookup(op, left, right)
	}
	return c.FunctionContext.ComparisonFunction(op, left, right)
}

func membershipEligibilityProvider() *membershipEligibilityContext {
	return &membershipEligibilityContext{eligibilityContext: &eligibilityContext{FunctionContext: must(testbed.NewStandardContext())}}
}

func membershipExpression(target expressions.Expression, candidates ...expressions.Expression) *expressions.SpecialFormExpression {
	nullability := target.Type().Nullability()
	for _, candidate := range candidates {
		if candidate.Type().Nullability() == foundation.MayNull {
			nullability = foundation.MayNull
		}
	}
	return must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, must(types.NewBoolType(nullability)), append([]expressions.Expression{target}, candidates...)))
}

func membershipCandidate(typ types.Type) *expressions.LiteralExpression {
	var value valueinterfaces.Value
	switch typ := typ.(type) {
	case *types.BoolType:
		value = must(testbedvalues.NewBool(typ, true))
	case *types.IntType:
		value = must(testbedvalues.NewInteger(typ, 1))
	case *types.FloatType:
		value = must(testbedvalues.NewFloat(typ, math.NaN()))
	case *types.VarCharType:
		value = must(testbedvalues.NewVarChar(typ, "x"))
	default:
		panic("unsupported fixture type")
	}
	return must(expressions.NewLiteralExpression(value))
}

func TestCanOmitMembershipScalarDomains(t *testing.T) {
	for _, typ := range []types.Type{
		must(types.NewBoolType(foundation.NonNull)),
		must(types.NewIntType(types.Int64, foundation.NonNull)),
		must(types.NewFloatType(types.Float32, foundation.NonNull)),
		must(types.NewFloatType(types.Float64, foundation.NonNull)),
		must(types.NewVarCharType(nil, foundation.NonNull)),
	} {
		for _, nullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			context := membershipEligibilityProvider()
			targetType := must(typ.WithNullability(nullability))
			candidate := membershipCandidate(typ)
			context.lookup = func(op foundation.OperatorType, left, right types.Type) (functioninterfaces.FunctionHandle, error) {
				if op != foundation.Equal || left != targetType || right != typ {
					t.Fatal("wrong membership lookup types/operator")
				}
				return context.FunctionContext.ComparisonFunction(op, left, right)
			}
			// Identical candidates still require inspection at every position.
			source := membershipExpression(eligibilityReference(targetType), candidate, candidate)
			before := expressions.PrettyPrint(source)
			if ok, err := boolean_cleanup.New(testbedvalues.ValueFactory{}, context).CanOmit(source); !ok || err != nil {
				t.Fatalf("%s: %v, %v", targetType, ok, err)
			}
			if context.lookups != 2 || context.queries != 2 {
				t.Fatalf("lookup/metadata counts: %d/%d", context.lookups, context.queries)
			}
			if source.Type().Nullability() != nullability || expressions.PrettyPrint(source) != before {
				t.Fatal("membership changed during admission")
			}
		}
	}
}

func TestCanOmitMembershipProviderSpecificDomains(t *testing.T) {
	boolean := must(types.NewBoolType(foundation.NonNull))
	bound := uint64(8)
	for _, typ := range []types.Type{must(types.NewIntType(types.Int8, foundation.NonNull)), must(types.NewIntType(types.Int16, foundation.NonNull)), must(types.NewIntType(types.Int32, foundation.NonNull)), must(types.NewVarCharType(&bound, foundation.NonNull))} {
		metadata := eligibilityMetadata([]types.Type{typ, typ}, boolean, foundation.Some(foundation.Equal), foundation.Optional[functions.BuiltInFunctionName]{}, []functions.NullPolicy{functions.PropagateNull, functions.PropagateNull})
		catalog := must(testbed.NewCatalog([]testbed.Definition{{Namespace: "fixture", Name: "equal", Metadata: metadata}}))
		context := &membershipEligibilityContext{eligibilityContext: &eligibilityContext{FunctionContext: testbed.NewContext(catalog)}}
		// This fixture checks structural admission only; execution guarantees remain caller-owned.
		source := membershipExpression(eligibilityReference(must(typ.WithNullability(foundation.MayNull))), membershipCandidate(typ))
		if ok, err := boolean_cleanup.New(testbedvalues.ValueFactory{}, context).CanOmit(source); !ok || err != nil {
			t.Fatalf("%s: %v, %v", typ, ok, err)
		}
	}
}

func TestCanOmitMembershipRejectsOriginalCandidateShape(t *testing.T) {
	context := membershipEligibilityProvider()
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	nullable := must(types.NewIntType(types.Int64, foundation.MayNull))
	candidate := membershipCandidate(integer)
	provider := context.FunctionContext
	sum := must(expressions.NewCallExpression(context, must(provider.ArithmeticFunction(foundation.Add, []types.Type{integer, integer})), []expressions.Expression{candidate, candidate}))
	// An erroring target/provider must not be inspected for an unsupported original list.
	target := sum
	context.metadata = func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
		t.Fatal("shape rejection inspected metadata")
		return nil, nil
	}
	context.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
		t.Fatal("shape rejection resolved equality")
		return nil, nil
	}
	context.queries = 0
	for _, candidates := range [][]expressions.Expression{
		nil,
		{eligibilityReference(integer)},
		{sum}, // Foldable arithmetic is still a dynamic candidate.
		{membershipCandidate(nullable)},
		{must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(nullable))))},
		{candidate, sum},
	} {
		source := membershipExpression(target, candidates...)
		if ok, err := boolean_cleanup.New(testbedvalues.ValueFactory{}, context).CanOmit(source); ok || err != nil {
			t.Fatalf("unsupported candidates: %v, %v", ok, err)
		}
	}
	if context.queries != 0 || context.lookups != 0 {
		t.Fatal("unsupported shape queried provider")
	}
}

func TestCanOmitMembershipTargetAndEveryInvocation(t *testing.T) {
	context := membershipEligibilityProvider()
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	candidate := membershipCandidate(integer)
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("fixture", "x")), integer, "x"))
	nullable := must(integer.WithNullability(foundation.MayNull))
	null := must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(nullable))))
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, context)
	if ok, err := rule.CanOmit(membershipExpression(null, candidate)); !ok || err != nil {
		t.Fatalf("NULL target: %v, %v", ok, err)
	}
	if ok, err := rule.CanOmit(membershipExpression(opaque, candidate)); ok || err != nil {
		t.Fatalf("opaque target: %v, %v", ok, err)
	}
	failure := errors.New("later equality lookup failed")
	for _, target := range []expressions.Expression{candidate, opaque} {
		context.lookups = 0
		context.lookup = func(op foundation.OperatorType, left, right types.Type) (functioninterfaces.FunctionHandle, error) {
			if context.lookups == 2 {
				return nil, failure
			}
			return context.FunctionContext.ComparisonFunction(op, left, right)
		}
		if ok, err := rule.CanOmit(membershipExpression(target, candidate, candidate)); ok || !errors.Is(err, failure) {
			t.Fatalf("later invocation hidden: %v, %v", ok, err)
		}
	}
	// Nested targets inspect their own calls through the call context, not the membership context.
	callContext := &eligibilityContext{FunctionContext: must(testbed.NewStandardContext())}
	comparison := must(expressions.NewCallExpression(callContext, must(callContext.FunctionContext.ComparisonFunction(foundation.Equal, integer, integer)), []expressions.Expression{candidate, candidate}))
	boolean := must(types.NewBoolType(foundation.NonNull))
	context.lookup = nil
	callContext.queries = 0
	if ok, err := rule.CanOmit(membershipExpression(comparison, membershipCandidate(boolean))); !ok || err != nil || callContext.queries != 1 {
		t.Fatalf("computed target: %v, %v", ok, err)
	}
	callContext.metadata = func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) { return nil, failure }
	if ok, err := rule.CanOmit(membershipExpression(comparison, membershipCandidate(boolean))); ok || !errors.Is(err, failure) {
		t.Fatalf("target error lost: %v, %v", ok, err)
	}
}

func TestCanOmitMembershipProviderFailures(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	source := membershipExpression(eligibilityReference(integer), membershipCandidate(integer))
	for _, cause := range []error{functions.ErrUnsupportedLookup, functions.ErrUnknownHandle, functions.ErrIncompatibleHandle, functions.ErrContractViolation, errors.Join(functions.ErrUnsupportedLookup, functions.ErrContractViolation), errors.New("lookup failure")} {
		context := membershipEligibilityProvider()
		context.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
			return nil, cause
		}
		if ok, err := boolean_cleanup.New(testbedvalues.ValueFactory{}, context).CanOmit(source); ok || !errors.Is(err, cause) {
			t.Fatalf("lookup error lost: %v, %v", ok, err)
		}
		context.lookup = nil
		context.metadata = func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) { return nil, cause }
		if ok, err := boolean_cleanup.New(testbedvalues.ValueFactory{}, context).CanOmit(source); ok || !errors.Is(err, cause) {
			t.Fatalf("metadata error lost: %v, %v", ok, err)
		}
	}
	foreign := must(testbed.NewStandardContext())
	context := membershipEligibilityProvider()
	provider := context.FunctionContext
	float := must(types.NewFloatType(types.Float64, foundation.NonNull))
	for _, tc := range []struct {
		handle functioninterfaces.FunctionHandle
		cause  error
	}{
		{nil, functions.ErrContractViolation},
		{(*testbed.Handle)(nil), functions.ErrContractViolation},
		{must(foreign.ComparisonFunction(foundation.Equal, integer, integer)), functions.ErrIncompatibleHandle},
		{must(provider.ComparisonFunction(foundation.NotEqual, integer, integer)), functions.ErrContractViolation},
		{must(provider.ComparisonFunction(foundation.Equal, float, float)), functions.ErrUnsupportedLookup},
	} {
		context.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
			return tc.handle, nil
		}
		ok, err := boolean_cleanup.New(testbedvalues.ValueFactory{}, context).CanOmit(source)
		if ok || !errors.Is(err, tc.cause) {
			t.Fatalf("bad lookup result: %v, %v", ok, err)
		}
		if tc.cause == functions.ErrUnsupportedLookup && !errors.Is(err, functions.ErrContractViolation) {
			t.Fatal("bad binding lost contract error")
		}
	}
	context.lookup = nil
	context.metadata = func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) { return nil, nil }
	if ok, err := boolean_cleanup.New(testbedvalues.ValueFactory{}, context).CanOmit(source); ok || !errors.Is(err, functions.ErrContractViolation) {
		t.Fatalf("nil metadata: %v, %v", ok, err)
	}
}

func TestCanOmitMembershipUnsupportedContracts(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	nullable := must(integer.WithNullability(foundation.MayNull))
	boolean := must(types.NewBoolType(foundation.NonNull))
	for _, tc := range []struct {
		parameter types.Type
		result    types.Type
		policy    functions.NullPolicy
	}{
		{integer, boolean, functions.NullPolicyUnspecified},
		{nullable, boolean, functions.PassNull},
		{integer, must(boolean.WithNullability(foundation.MayNull)), functions.PropagateNull},
		{integer, integer, functions.PropagateNull},
	} {
		metadata := eligibilityMetadata([]types.Type{tc.parameter, integer}, tc.result, foundation.Some(foundation.Equal), foundation.Optional[functions.BuiltInFunctionName]{}, []functions.NullPolicy{tc.policy, functions.PropagateNull})
		catalog := must(testbed.NewCatalog([]testbed.Definition{{Namespace: "fixture", Name: "equal", Metadata: metadata}}))
		context := &membershipEligibilityContext{eligibilityContext: &eligibilityContext{FunctionContext: testbed.NewContext(catalog)}}
		source := membershipExpression(eligibilityReference(integer), membershipCandidate(integer))
		if ok, err := boolean_cleanup.New(testbedvalues.ValueFactory{}, context).CanOmit(source); ok || err != nil {
			t.Fatalf("unsupported null/result contract: %v, %v", ok, err)
		}
	}
}
