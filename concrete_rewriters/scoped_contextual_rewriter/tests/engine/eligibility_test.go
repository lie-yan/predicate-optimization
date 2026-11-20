package engine_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/internal/boolean_cleanup"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/internal/assertions"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"

	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"

	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Wrapping independent public providers lets inspection failures be injected
// after valid construction. Evaluation and re-resolution are forbidden here.
type eligibilityContext struct {
	functioninterfaces.FunctionContext
	metadata func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error)
	queries  int
}

func (c *eligibilityContext) Metadata(h functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	c.queries++
	if c.metadata != nil {
		return c.metadata(h)
	}
	return c.FunctionContext.Metadata(h)
}

func (*eligibilityContext) Evaluate(functioninterfaces.FunctionHandle, *functions.FunctionSignature, []valueinterfaces.Value) (valueinterfaces.Value, error) {
	panic("eligibility must not evaluate")
}

func (*eligibilityContext) ComparisonFunction(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
	panic("eligibility must not resolve a call again")
}

func (*eligibilityContext) LookupBuiltInFunction(functions.BuiltInFunctionName, []types.Type) (functioninterfaces.FunctionHandle, error) {
	panic("Inspection must not look up built-ins")
}

func eligibilityReference(typ types.Type) *expressions.ReferenceExpression {
	return must(expressions.NewReferenceExpression(must(ids.NewColumnID("t", "x")), typ, "x"))
}

func eligibilityMetadata(args []types.Type, result types.Type, op foundation.Optional[foundation.OperatorType], name foundation.Optional[functions.BuiltInFunctionName], policies []functions.NullPolicy) *functions.FunctionMetadata {
	return must(functions.NewFunctionMetadata(must(functions.NewFunctionSignature(args, result)), "int64.equal", name, op, policies))
}

func eligibilityCall(metadata *functions.FunctionMetadata, args []expressions.Expression) (*eligibilityContext, *expressions.CallExpression) {
	catalog := must(testbed.NewCatalog([]testbed.Definition{{Namespace: "fixture", Name: "not", Metadata: metadata}}))
	context := &eligibilityContext{FunctionContext: testbed.NewContext(catalog)}
	return context, must(expressions.NewCallExpression(context, catalog.Handles()[0], args))
}

func TestCanOmitStandardScalarCalls(t *testing.T) {
	membership := &eligibilityContext{FunctionContext: must(testbed.NewStandardContext())}
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, membership)
	// Both providers have their own catalog identity and differ from the rule's membership context.
	for providerIndex := 0; providerIndex < 2; providerIndex++ {
		provider := must(testbed.NewStandardContext())
		context := &eligibilityContext{FunctionContext: provider}
		for _, typ := range []types.Type{
			must(types.NewBoolType(foundation.NonNull)),
			must(types.NewIntType(types.Int64, foundation.NonNull)),
			must(types.NewFloatType(types.Float32, foundation.NonNull)),
			must(types.NewFloatType(types.Float64, foundation.NonNull)),
			must(types.NewVarCharType(nil, foundation.NonNull)),
		} {
			for _, leftNull := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
				for _, rightNull := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
					left, right := must(typ.WithNullability(leftNull)), must(typ.WithNullability(rightNull))
					for _, op := range []foundation.OperatorType{foundation.Equal, foundation.NotEqual, foundation.LessThan, foundation.LessThanOrEqual, foundation.GreaterThan, foundation.GreaterThanOrEqual} {
						if _, boolean := typ.(*types.BoolType); boolean && op != foundation.Equal {
							continue
						}
						handle := must(provider.ComparisonFunction(op, left, right))
						call := must(expressions.NewCallExpression(context, handle, []expressions.Expression{eligibilityReference(left), eligibilityReference(right)}))
						context.queries = 0
						admitted, err := rule.CanOmit(call)
						if err != nil || !admitted || context.queries != 1 {
							t.Fatalf("provider %d %s/%s operator %d: %v, %v, queries=%d", providerIndex, left, right, op, admitted, err, context.queries)
						}
						expected := foundation.NonNull
						if leftNull == foundation.MayNull || rightNull == foundation.MayNull {
							expected = foundation.MayNull
						}
						if call.Type().Nullability() != expected {
							t.Fatal("effective nullability changed")
						}
					}
				}
			}
		}
		for _, nullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			typ := must(types.NewBoolType(nullability))
			handle := must(provider.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{typ}))
			call := must(expressions.NewCallExpression(context, handle, []expressions.Expression{eligibilityReference(typ)}))
			if ok, err := rule.CanOmit(call); !ok || err != nil {
				t.Fatalf("NOT %s: %v, %v", typ, ok, err)
			}
		}
	}
	if membership.queries != 0 {
		t.Fatal("call admission used membership context")
	}
}

func TestCanOmitSupportedProviderDomains(t *testing.T) {
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, must(testbed.NewStandardContext()))
	boolean := must(types.NewBoolType(foundation.NonNull))
	bound := uint64(8)
	for _, typ := range []types.Type{boolean, must(types.NewIntType(types.Int8, foundation.NonNull)), must(types.NewIntType(types.Int16, foundation.NonNull)), must(types.NewIntType(types.Int32, foundation.NonNull)), must(types.NewVarCharType(&bound, foundation.NonNull))} {
		// Inspection-only fixtures exercise structural capabilities beyond the standard catalog;
		// admitting them in production also requires the caller's evaluation guarantee.
		op := foundation.Equal
		if typ == boolean {
			op = foundation.NotEqual
		}
		metadata := eligibilityMetadata([]types.Type{typ, typ}, boolean, foundation.Some(op), foundation.Optional[functions.BuiltInFunctionName]{}, []functions.NullPolicy{functions.PropagateNull, functions.PropagateNull})
		nullable := must(typ.WithNullability(foundation.MayNull))
		_, call := eligibilityCall(metadata, []expressions.Expression{eligibilityReference(nullable), eligibilityReference(typ)})
		if ok, err := rule.CanOmit(call); !ok || err != nil {
			t.Fatalf("%s: %v, %v", typ, ok, err)
		}
	}
}

func TestCanOmitBoundariesAndRecursion(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	context := &eligibilityContext{FunctionContext: provider}
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, context)
	boolean := must(types.NewBoolType(foundation.MayNull))
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	array := must(types.NewArrayType(integer, foundation.NonNull))
	ref := eligibilityReference(boolean)
	yes := must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, boolean, foundation.True))
	null := must(boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, boolean, foundation.Null))
	list := must(expressions.NewLiteralExpression(must(testbedvalues.NewArray(array, nil))))
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("fixture", "opaque")), boolean, "opaque"))
	arithmetic := must(expressions.NewCallExpression(context, must(provider.ArithmeticFunction(foundation.Add, []types.Type{integer, integer})), []expressions.Expression{eligibilityReference(integer), eligibilityReference(integer)}))
	comparison := must(expressions.NewCallExpression(context, must(provider.ComparisonFunction(foundation.Equal, integer, integer)), []expressions.Expression{arithmetic, eligibilityReference(integer)}))
	notCall := must(expressions.NewCallExpression(context, must(provider.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{boolean})), []expressions.Expression{opaque}))
	in := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{ref, yes}))
	contains := must(expressions.NewCallExpression(context, must(provider.LookupBuiltInFunction(functions.BuiltInArrayContains, []types.Type{array, integer})), []expressions.Expression{list, eligibilityReference(integer)}))
	for _, node := range []expressions.Expression{ref, yes, null, list, eligibilityReference(array), contains} {
		if ok, err := rule.CanOmit(node); !ok || err != nil {
			t.Fatalf("leaf %T: %v, %v", node, ok, err)
		}
	}
	for _, node := range []expressions.Expression{opaque, arithmetic, comparison, notCall, in, struct{ expressions.Expression }{}} {
		if ok, err := rule.CanOmit(node); ok || err != nil {
			t.Fatalf("boundary %T: %v, %v", node, ok, err)
		}
	}
	for _, op := range []foundation.LogicalOperator{foundation.And, foundation.Or, foundation.Not} {
		for _, unsupported := range []bool{false, true} {
			var child expressions.Expression = ref
			if unsupported {
				child = opaque
			}
			args := []expressions.Expression{child}
			if op != foundation.Not {
				args = append(args, yes, null, child)
			}
			node := must(expressions.NewLogicalExpression(op, args))
			if ok, err := rule.CanOmit(node); ok == unsupported || err != nil {
				t.Fatalf("logical %v: %v, %v", op, ok, err)
			}
		}
	}
}

func TestCanOmitUnsupportedSignatures(t *testing.T) {
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, must(testbed.NewStandardContext()))
	boolean := must(types.NewBoolType(foundation.NonNull))
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	narrow := must(types.NewIntType(types.Int8, foundation.NonNull))
	nullable := must(integer.WithNullability(foundation.MayNull))
	array := must(types.NewArrayType(integer, foundation.NonNull))
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	bound := uint64(8)
	bounded := must(types.NewVarCharType(&bound, foundation.NonNull))
	for _, tc := range []struct {
		name     string
		args     []types.Type
		result   types.Type
		op       foundation.OperatorType
		builtin  functions.BuiltInFunctionName
		policies []functions.NullPolicy
	}{
		{"diagnostic name only", []types.Type{integer, integer}, boolean, 0, "", nil},
		{"arithmetic", []types.Type{integer, integer}, integer, foundation.Add, "", nil},
		{"comparison arity", []types.Type{integer}, boolean, foundation.Equal, "", nil},
		{"mixed widths", []types.Type{integer, narrow}, boolean, foundation.Equal, "", nil},
		{"mixed bounds", []types.Type{text, bounded}, boolean, foundation.Equal, "", nil},
		{"array equality", []types.Type{array, array}, boolean, foundation.Equal, "", nil},
		{"Boolean ordering", []types.Type{boolean, boolean}, boolean, foundation.LessThan, "", nil},
		{"non Boolean result", []types.Type{integer, integer}, integer, foundation.Equal, "", nil},
		{"nullable result", []types.Type{integer, integer}, must(boolean.WithNullability(foundation.MayNull)), foundation.Equal, "", nil},
		{"unspecified policy", []types.Type{integer, integer}, boolean, foundation.Equal, "", []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified}},
		{"pass null", []types.Type{nullable, integer}, boolean, foundation.Equal, "", []functions.NullPolicy{functions.PassNull, functions.PropagateNull}},
		{"NOT arity", []types.Type{boolean, boolean}, boolean, 0, functions.BuiltInNot, nil},
		{"NOT domain", []types.Type{integer}, boolean, 0, functions.BuiltInNot, nil},
		{"NOT policy", []types.Type{boolean}, boolean, 0, functions.BuiltInNot, []functions.NullPolicy{functions.NullPolicyUnspecified}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var op foundation.Optional[foundation.OperatorType]
			if tc.op != 0 {
				op = foundation.Some(tc.op)
			}
			var name foundation.Optional[functions.BuiltInFunctionName]
			if tc.builtin != "" {
				name = foundation.Some(tc.builtin)
			}
			policies := tc.policies
			if policies == nil {
				policies = make([]functions.NullPolicy, len(tc.args))
				for i := range policies {
					policies[i] = functions.PropagateNull
				}
			}
			args := make([]expressions.Expression, len(tc.args))
			for i, typ := range tc.args {
				args[i] = eligibilityReference(typ)
			}
			_, call := eligibilityCall(eligibilityMetadata(tc.args, tc.result, op, name, policies), args)
			if ok, err := rule.CanOmit(call); ok || err != nil {
				t.Fatalf("unsupported invocation: %v, %v", ok, err)
			}
		})
	}
}

func TestCanOmitProviderFailures(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	context := &eligibilityContext{FunctionContext: provider}
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, provider)
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	boolean := must(types.NewBoolType(foundation.NonNull))
	handle := must(provider.ComparisonFunction(foundation.Equal, integer, integer))
	call := must(expressions.NewCallExpression(context, handle, []expressions.Expression{eligibilityReference(integer), eligibilityReference(integer)}))
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("fixture", "opaque")), boolean, "opaque"))
	region := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{opaque, call}))
	for _, cause := range []error{functions.ErrUnknownHandle, functions.ErrIncompatibleHandle, functions.ErrContractViolation, functions.ErrUnsupportedLookup, errors.New("metadata failure")} {
		context.metadata = func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
			return nil, fmt.Errorf("fixture: %w", cause)
		}
		for _, source := range []expressions.Expression{call, region} {
			if ok, err := rule.CanOmit(source); ok || !errors.Is(err, cause) {
				t.Fatalf("provider error lost: %v, %v", ok, err)
			}
		}
	}
	// Deliberately break the provider's stability/paired-handle contract after construction.
	foreign := must(testbed.NewStandardContext())
	context.metadata = foreign.Metadata
	if ok, err := rule.CanOmit(call); ok || !errors.Is(err, functions.ErrIncompatibleHandle) {
		t.Fatalf("foreign pair: %v, %v", ok, err)
	}
	// Successful nil/inconsistent metadata and invalid stored signatures violate
	// the trusted-input contract. Their diagnosis is covered by expression entry
	// validation, call construction and functions/utils inspection tests.
}

func TestCanOmitNilInputs(t *testing.T) {
	rule := boolean_cleanup.New(testbedvalues.ValueFactory{}, must(testbed.NewStandardContext()))
	for _, call := range []func(){
		func() { _, _ = rule.CanOmit(nil) },
		func() { _, _ = rule.CanOmit((*expressions.CallExpression)(nil)) },
		func() {
			var nilRule *boolean_cleanup.Cleaner
			_, _ = nilRule.CanOmit(eligibilityReference(must(types.NewBoolType(foundation.NonNull))))
		},
	} {
		func() {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Error("nil input did not assert")
				}
			}()
			call()
		}()
	}
}
