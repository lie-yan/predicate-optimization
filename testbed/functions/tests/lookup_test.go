package functions_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func lookupDefinition(t *testing.T, name string, args []types.Type, result types.Type, builtIn foundation.Optional[shared.BuiltInFunctionName], operator foundation.Optional[foundation.OperatorType], policies []shared.NullPolicy) testbed.Definition {
	t.Helper()
	signature, err := shared.NewFunctionSignature(args, result)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := shared.NewFunctionMetadata(signature, "diagnostic only", builtIn, operator, policies)
	if err != nil {
		t.Fatal(err)
	}
	return testbed.Definition{Namespace: "caller_namespace", Name: name, Metadata: metadata}
}

func TestBuiltInLookupForTouchstone(t *testing.T) {
	integer, err := types.NewIntType(types.Int64, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	array, err := types.NewArrayType(integer, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	nullableArray, err := array.WithNullability(foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	boolean, err := types.NewBoolType(foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	noOperator := foundation.Optional[foundation.OperatorType]{}
	policies := []shared.NullPolicy{shared.PropagateNull, shared.NullPolicyUnspecified}
	contains := lookupDefinition(t, "member", []types.Type{array, integer}, boolean, foundation.Some(shared.BuiltInArrayContains), noOperator, policies)
	any := lookupDefinition(t, "any_member", []types.Type{array, array}, boolean, foundation.Some(shared.BuiltInArrayContainsAny), noOperator, policies)
	catalog, err := testbed.NewCatalog([]testbed.Definition{contains, any})
	if err != nil {
		t.Fatal(err)
	}
	provider := testbed.NewContext(catalog)
	for _, tc := range []struct {
		name     shared.BuiltInFunctionName
		args     []types.Type
		metadata *shared.FunctionMetadata
	}{
		{shared.BuiltInArrayContains, []types.Type{nullableArray, integer}, contains.Metadata},
		{shared.BuiltInArrayContainsAny, []types.Type{nullableArray, array}, any.Metadata},
	} {
		handle, err := provider.LookupBuiltInFunction(tc.name, tc.args)
		if err != nil {
			t.Fatal(err)
		}
		repeated, err := provider.LookupBuiltInFunction(tc.name, tc.args)
		if err != nil || !handle.EqualsTo(repeated) {
			t.Fatalf("unstable lookup: %v", err)
		}
		metadata, err := provider.Metadata(handle)
		if err != nil || metadata != tc.metadata {
			t.Fatalf("handle not paired with context: %v", err)
		}
		if handle.Signature().ArgumentTypes()[0].Nullability() != foundation.NonNull {
			t.Fatal("handle must expose implementation types")
		}
		bound, err := shared.BindFunctionSignature(metadata, tc.args)
		if err != nil || bound.ResultType().Nullability() != foundation.MayNull {
			t.Fatalf("nullable call failed: %v", err)
		}
		if arithmetic, err := provider.IsArithmeticFunction(handle); err != nil || arithmetic {
			t.Fatal("array function classified as arithmetic")
		}
	}
	// Concrete candidate arrays can have any length without changing the handle.
	one, err := testbedvalues.NewInteger(integer, 8792001)
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := testbedvalues.NewArray(array, []valueinterfaces.Value{one})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := provider.LookupBuiltInFunction(shared.BuiltInArrayContainsAny, []types.Type{array, candidates.Type()})
	if err != nil {
		t.Fatal(err)
	}
	input, err := testbedvalues.NewArray(array, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := provider.Evaluate(handle, any.Metadata.Signature(), []valueinterfaces.Value{input, candidates}); result != nil || !errors.Is(err, shared.ErrEvaluationUnavailable) {
		t.Fatalf("metadata-only lookup must not imply execution: %v", err)
	}
	for _, args := range [][]types.Type{{array}, {integer, integer}, {array, nullableArray}} {
		if handle, err := provider.LookupBuiltInFunction(shared.BuiltInArrayContainsAny, args); handle != nil || !errors.Is(err, shared.ErrUnsupportedLookup) {
			t.Fatalf("unsupported arguments accepted: %v", err)
		}
	}
	if _, err := provider.LookupBuiltInFunction("member", []types.Type{array, integer}); !errors.Is(err, shared.ErrUnsupportedLookup) {
		t.Fatal("caller name mistaken for built-in identity")
	}
}

func TestOperatorLookup(t *testing.T) {
	integer, err := types.NewIntType(types.Int64, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	boolean, err := types.NewBoolType(foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	noName := foundation.Optional[shared.BuiltInFunctionName]{}
	defs := []testbed.Definition{
		lookupDefinition(t, "neg", []types.Type{integer}, integer, noName, foundation.Some(foundation.Negate), []shared.NullPolicy{shared.PropagateNull}),
		lookupDefinition(t, "add", []types.Type{integer, integer}, integer, noName, foundation.Some(foundation.Add), []shared.NullPolicy{shared.PropagateNull, shared.PropagateNull}),
		lookupDefinition(t, "equal", []types.Type{integer, integer}, boolean, noName, foundation.Some(foundation.Equal), []shared.NullPolicy{shared.PropagateNull, shared.PropagateNull}),
	}
	catalog, err := testbed.NewCatalog(defs)
	if err != nil {
		t.Fatal(err)
	}
	provider := testbed.NewContext(catalog)
	// A separately allocated but structurally equal Type must resolve.
	otherInteger, err := types.NewIntType(types.Int64, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		op   foundation.OperatorType
		args []types.Type
	}{
		{foundation.Negate, []types.Type{otherInteger}},
		{foundation.Add, []types.Type{otherInteger, integer}},
	} {
		handle, err := provider.ArithmeticFunction(tc.op, tc.args)
		if err != nil {
			t.Fatal(err)
		}
		if yes, err := provider.IsArithmeticFunction(handle); err != nil || !yes {
			t.Fatal("lookup and recognition disagree")
		}
	}
	comparison, err := provider.ComparisonFunction(foundation.Equal, integer, otherInteger)
	if err != nil {
		t.Fatal(err)
	}
	if yes, err := provider.IsComparisonFunction(comparison); err != nil || !yes {
		t.Fatal("comparison lookup and recognition disagree")
	}
	for _, tc := range []struct {
		op   foundation.OperatorType
		args []types.Type
	}{
		{foundation.Equal, []types.Type{integer, integer}},
		{foundation.Negate, []types.Type{integer, integer}},
		{foundation.Add, []types.Type{integer}},
		{foundation.OperatorType(255), nil},
	} {
		if _, err := provider.ArithmeticFunction(tc.op, tc.args); !errors.Is(err, shared.ErrUnsupportedLookup) {
			t.Fatal("invalid arithmetic request accepted")
		}
	}
	if _, err := provider.ComparisonFunction(foundation.Add, integer, integer); !errors.Is(err, shared.ErrUnsupportedLookup) {
		t.Fatal("arithmetic operator accepted as comparison")
	}
	narrow, err := types.NewIntType(types.Int8, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.ComparisonFunction(foundation.Equal, integer, narrow); !errors.Is(err, shared.ErrUnsupportedLookup) {
		t.Fatal("lookup silently coerced integer width")
	}
}

func TestLookupRejectsAmbiguity(t *testing.T) {
	boolean, err := types.NewBoolType(foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	first := lookupDefinition(t, "first", []types.Type{boolean}, boolean, foundation.Some(shared.BuiltInNot), foundation.Optional[foundation.OperatorType]{}, []shared.NullPolicy{shared.PropagateNull})
	second := first
	second.Name = "second"
	for _, defs := range [][]testbed.Definition{{first, second}, {second, first}} {
		catalog, err := testbed.NewCatalog(defs)
		if err != nil {
			t.Fatal(err)
		}
		if handle, err := testbed.NewContext(catalog).LookupBuiltInFunction(shared.BuiltInNot, []types.Type{boolean}); handle != nil || !errors.Is(err, shared.ErrUnsupportedLookup) {
			t.Fatal("ambiguous lookup selected insertion-order winner")
		}
	}
}

func TestIndexedLookupCanonicalizationAndAmbiguity(t *testing.T) {
	integer, err := types.NewIntType(types.Int64, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	arguments := []types.Type{integer, integer}
	first := lookupDefinition(t, "add", arguments, integer, foundation.Some(shared.BuiltInInt64Add), foundation.Some(foundation.Add), []shared.NullPolicy{shared.PropagateNull, shared.PropagateNull})
	second := first
	second.Namespace = "another.provider"
	for _, tc := range []struct {
		name        string
		definitions []testbed.Definition
		ambiguous   bool
	}{
		{"canonical duplicates", []testbed.Definition{first, first}, false},
		{"distinct entries", []testbed.Definition{first, second}, true},
		{"reverse order", []testbed.Definition{second, first}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalog, err := testbed.NewCatalog(tc.definitions)
			if err != nil {
				t.Fatal(err)
			}
			// Both contexts share the catalog's immutable indexes and identity.
			for _, provider := range []*testbed.Context{testbed.NewContext(catalog), testbed.NewContext(catalog)} {
				named, nameErr := provider.LookupBuiltInFunction(shared.BuiltInInt64Add, arguments)
				operator, operatorErr := provider.ArithmeticFunction(foundation.Add, arguments)
				if tc.ambiguous {
					if named != nil || operator != nil || !errors.Is(nameErr, shared.ErrUnsupportedLookup) || !errors.Is(operatorErr, shared.ErrUnsupportedLookup) {
						t.Fatal("indexed lookup lost ambiguity")
					}
				} else {
					if nameErr != nil || operatorErr != nil {
						t.Fatalf("duplicate canonical entry indexed twice: %v, %v", nameErr, operatorErr)
					}
					if !named.EqualsTo(operator) || !named.EqualsTo(catalog.Handles()[0]) {
						t.Fatal("indexes disagree on canonical identity")
					}
				}
			}
		})
	}
}
