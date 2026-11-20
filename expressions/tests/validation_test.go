package expressions_test

import (
	"errors"
	"testing"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

type validationContext struct {
	functioninterfaces.FunctionContext
	queries int
}

func (c *validationContext) Metadata(h functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	c.queries++
	return c.FunctionContext.Metadata(h)
}

func (*validationContext) Evaluate(functioninterfaces.FunctionHandle, *functions.FunctionSignature, []valueinterfaces.Value) (valueinterfaces.Value, error) {
	panic("validation must not evaluate")
}

func TestValidateSharedGraphAndContext(t *testing.T) {
	provider := fixture(testbed.NewStandardContext())
	ctx := &validationContext{FunctionContext: provider}
	integer := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	value := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, 8))))
	handle := fixture(provider.ArithmeticFunction(foundation.Add, []types.Type{integer, integer}))
	var left expressions.Expression = value
	var right expressions.Expression = fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, 8))))
	for i := 0; i < 40; i++ {
		left = fixture(expressions.NewCallExpression(provider, handle, []expressions.Expression{left, left}))
		right = fixture(expressions.NewCallExpression(provider, handle, []expressions.Expression{right, right}))
	}
	if err := utils.Validate(left); err != nil {
		t.Fatal(err)
	}
	if err := utils.ValidateWithContext(left, ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.queries != 40 {
		t.Fatalf("metadata queries = %d, want one per unique call", ctx.queries)
	}
	if !left.EqualsTo(right) || !right.EqualsTo(left) {
		t.Fatal("equal shared DAGs differ")
	}
	foreign := fixture(testbed.NewStandardContext())
	if err := utils.ValidateWithContext(left, foreign); !errors.Is(err, functions.ErrIncompatibleHandle) {
		t.Fatalf("expected incompatible context, got %v", err)
	}
}

func TestValidateIdentityConflicts(t *testing.T) {
	nonnull := fixture(types.NewBoolType(foundation.NonNull))
	nullable := fixture(types.NewBoolType(foundation.MayNull))
	for _, opaque := range []bool{false, true} {
		var a, b expressions.Expression
		if opaque {
			a = fixture(expressions.NewOpaqueExpression(fixture(ids.NewOpaqueID("adapter", "x")), nonnull, ""))
			b = fixture(expressions.NewOpaqueExpression(fixture(ids.NewOpaqueID("adapter", "x")), nullable, "renamed"))
		} else {
			a = fixture(expressions.NewReferenceExpression(externalColumnID{path: []string{"table", "x"}}, nonnull, ""))
			b = fixture(expressions.NewReferenceExpression(externalColumnID{path: []string{"table", "x"}}, nullable, "renamed"))
		}
		root := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{a, b}))
		if utils.Validate(root) == nil {
			t.Fatal("conflicting identity types accepted")
		}
	}
}

func TestValidateMembershipAndOrderedEquality(t *testing.T) {
	typ := fixture(types.NewBoolType(foundation.NonNull))
	a := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewBool(typ, true))))
	b := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewBool(typ, false))))
	membership := fixture(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, typ, []expressions.Expression{a, b}))
	if err := utils.Validate(membership); err != nil {
		t.Fatal(err)
	}
	ab := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{a, b}))
	ba := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{b, a}))
	if ab.EqualsTo(ba) {
		t.Fatal("commutativity is not structural equality")
	}
	if utils.Validate(nil) == nil {
		t.Fatal("nil root accepted")
	}
	var nilRoot *expressions.LiteralExpression
	if utils.Validate(nilRoot) == nil {
		t.Fatal("typed nil root accepted")
	}
}

type changedMetadataContext struct {
	functioninterfaces.FunctionContext
	metadata *functions.FunctionMetadata
}

func (c changedMetadataContext) Metadata(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	return c.metadata, nil
}

func TestValidateContextRejectsChangedResultType(t *testing.T) {
	provider := fixture(testbed.NewStandardContext())
	integer := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	nullable := fixture(integer.WithNullability(foundation.MayNull))
	literal := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, 1))))
	handle := fixture(provider.ArithmeticFunction(foundation.Add, []types.Type{integer, integer}))
	call := fixture(expressions.NewCallExpression(provider, handle, []expressions.Expression{literal, literal}))
	signature := fixture(functions.NewFunctionSignature([]types.Type{integer, integer}, nullable))
	metadata := fixture(functions.NewFunctionMetadata(signature, "", foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, []functions.NullPolicy{functions.PropagateNull, functions.PropagateNull}))
	ctx := changedMetadataContext{FunctionContext: provider, metadata: metadata}
	if err := utils.Validate(call); err != nil {
		t.Fatal(err)
	}
	if err := utils.ValidateWithContext(call, ctx); err == nil {
		t.Fatal("changed effective result type accepted")
	}
}
