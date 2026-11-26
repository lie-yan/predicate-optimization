package array_membership_normalization_rewriter_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestPairedFixtureMembershipAndEnvironment(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		for _, nullable := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			var provider functioninterfaces.FunctionContext = must(testbed.NewStandardContext())
			if wrapped {
				provider = &wrappedContext{delegate: provider}
			}
			f := newNormalizationFixture(provider, nullable)
			source := f.call(functions.BuiltInArrayContainsAny, f.input, f.candidates(8792001))
			expected := f.call(functions.BuiltInArrayContains, f.input, f.element(8792001))
			evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
			for _, tc := range []struct {
				numbers []int64
				want    bool
			}{{nil, false}, {[]int64{42}, false}, {[]int64{8792001}, true}, {[]int64{8792001, 8792001}, true}} {
				input := f.arrayValue(f.input.Type().(*types.ArrayType), tc.numbers...)
				// A distinct ID object with equal scoped identity resolves the binding.
				env := &fixtureEnvironment{bindings: []fixtureBinding{{must(ids.NewColumnID("normalization", "array")), input}}}
				want := must(testbedvalues.NewBool(source.Type().(*types.BoolType), tc.want))
				for _, node := range []expressions.Expression{source, expected} {
					got := must(evaluator.Evaluate(node, env))
					if !got.EqualsTo(want) || !got.Type().EqualsTo(source.Type()) {
						t.Fatal("paired singleton fixture truth/Type mismatch")
					}
				}
				if len(env.lookups) != 2 {
					t.Fatal("expected one lookup per evaluation")
				}
			}
			env := &fixtureEnvironment{}
			if _, present := must(env.Lookup(f.input.ID())).Get(); present {
				t.Fatal("missing binding became present")
			}
			if residual, present := must(evaluator.PartialEvaluate(source, env)).Residual(); !present || !residual.Type().EqualsTo(source.Type()) {
				t.Fatal("missing input did not remain residual")
			}
			if nullable == foundation.MayNull {
				null := must(testbedvalues.NewNull(f.input.Type()))
				env.bindings = []fixtureBinding{{f.input.ID(), null}}
				if value, present := must(env.Lookup(f.input.ID())).Get(); !present || !value.IsNull() {
					t.Fatal("known NULL became absent")
				}
				for _, node := range []expressions.Expression{source, expected} {
					if value := must(evaluator.Evaluate(node, env)); !value.IsNull() || !value.Type().EqualsTo(source.Type()) {
						t.Fatal("NULL lost its effective Type")
					}
				}
			}
			sentinel := errors.New("lookup failed")
			env.failure = sentinel
			if _, err := evaluator.Evaluate(source, env); !errors.Is(err, sentinel) {
				t.Fatal("environment failure lost", err)
			}
			f.context.events = nil
			tree := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
			if got := must(tree.Rewrite(source, struct{}{})); !got.EqualsTo(expected) {
				t.Fatal("paired fixture failed to normalize")
			}
			for _, event := range f.context.events {
				if event.operation == "evaluate" {
					t.Fatal("normalization evaluated the paired fixture")
				}
			}
		}
	}
}

func TestFixtureOperationFaultsAndRetry(t *testing.T) {
	f := newNormalizationFixture(must(testbed.NewStandardContext()), foundation.NonNull)
	source := f.call(functions.BuiltInArrayContainsAny, f.input, f.candidates(8792001))
	target := f.call(functions.BuiltInArrayContains, f.input, f.element(8792001))
	originalMetadata := must(f.context.Metadata(target.Handle()))
	sentinel := errors.New("injected operation failure")
	queries := 0
	f.context.events = nil
	f.context.fault = func(event functionEvent) error {
		if event.operation == "metadata" && event.handle.EqualsTo(target.Handle()) {
			queries++
			if queries == 2 {
				return sentinel
			}
		}
		return nil
	}
	if must(f.context.Metadata(target.Handle())) != originalMetadata {
		t.Fatal("metadata changed")
	}
	if _, err := f.context.Metadata(target.Handle()); !errors.Is(err, sentinel) {
		t.Fatal("later query fault missing", err)
	}
	if must(f.context.Metadata(source.Handle())) == nil {
		t.Fatal("unrelated handle was blocked")
	}
	for _, operation := range []string{"lookup", "evaluate"} {
		f.context.fault = func(event functionEvent) error {
			if event.operation == operation {
				return sentinel
			}
			return nil
		}
		var err error
		if operation == "lookup" {
			_, err = f.context.LookupBuiltInFunction(functions.BuiltInArrayContains, []types.Type{f.input.Type(), f.integer})
		} else {
			_, err = f.context.Evaluate(target.Handle(), target.Signature(), []valueinterfaces.Value{f.arrayValue(f.array, 8792001), f.element(8792001).Value()})
		}
		if !errors.Is(err, sentinel) || f.context.events[len(f.context.events)-1].operation != operation {
			t.Fatal("fault/event lost", err)
		}
	}
	f.context.fault = nil
	if must(f.context.Metadata(target.Handle())) != originalMetadata {
		t.Fatal("retry changed successful metadata")
	}
	if got := must(f.context.Evaluate(target.Handle(), target.Signature(), []valueinterfaces.Value{f.arrayValue(f.array, 8792001), f.element(8792001).Value()})); !got.(*testbedvalues.BoolValue).Bool() {
		t.Fatal("retry failed")
	}
}

func TestAlternativeCatalogFixtures(t *testing.T) {
	f := newNormalizationFixture(must(testbed.NewStandardContext()), foundation.NonNull)
	boolean := must(types.NewBoolType(foundation.NonNull))
	definition := membershipDefinition(functions.BuiltInArrayContainsAny, []types.Type{f.array, f.array}, boolean, []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified})
	catalog := must(testbed.NewCatalog([]testbed.Definition{definition}))
	first, second := testbed.NewContext(catalog), testbed.NewContext(must(testbed.NewCatalog([]testbed.Definition{definition})))
	handle := must(first.LookupBuiltInFunction(functions.BuiltInArrayContainsAny, []types.Type{f.array, f.array}))
	if _, err := first.LookupBuiltInFunction(functions.BuiltInArrayContains, []types.Type{f.array, f.integer}); !errors.Is(err, functions.ErrUnsupportedLookup) {
		t.Fatal("missing target overload fixture failed", err)
	}
	other := must(second.LookupBuiltInFunction(functions.BuiltInArrayContainsAny, []types.Type{f.array, f.array}))
	if handle.Name() != other.Name() || handle.EqualsTo(other) {
		t.Fatal("catalog identity fixture failed")
	}
	if _, err := first.Metadata(other); !errors.Is(err, functions.ErrIncompatibleHandle) {
		t.Fatal("foreign catalog handle accepted", err)
	}
	nullableArray := must(f.array.WithNullability(foundation.MayNull))
	nullableBool := must(boolean.WithNullability(foundation.MayNull))
	alternative := membershipDefinition(functions.BuiltInArrayContainsAny, []types.Type{f.array, nullableArray}, nullableBool, []functions.NullPolicy{functions.PropagateNull, functions.PassNull})
	provider := testbed.NewContext(must(testbed.NewCatalog([]testbed.Definition{alternative})))
	nullableCandidate := must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(nullableArray))))
	h := must(provider.LookupBuiltInFunction(functions.BuiltInArrayContainsAny, []types.Type{f.input.Type(), nullableArray}))
	call := must(expressions.NewCallExpression(provider, h, []expressions.Expression{f.input, nullableCandidate}))
	if !call.Type().EqualsTo(nullableBool) {
		t.Fatal("valid alternative policy fixture failed")
	}
	// A metadata-only catalog supplies construction/admission fixtures, not a
	// provider satisfying the intended executable normalization contract.
}

func TestWrappedHandleContextPairing(t *testing.T) {
	base := must(testbed.NewStandardContext())
	first, second := &wrappedContext{delegate: base}, &wrappedContext{delegate: base}
	f := newNormalizationFixture(first, foundation.NonNull)
	call := f.call(functions.BuiltInArrayContains, f.input, f.element(8792001))
	handle := call.Handle()
	again := must(first.LookupBuiltInFunction(functions.BuiltInArrayContains, []types.Type{f.array, f.integer}))
	if reflect.TypeOf(handle).Comparable() || !handle.EqualsTo(again) || !again.EqualsTo(handle) {
		t.Fatal("non-comparable identity fixture failed")
	}
	if _, err := second.Metadata(handle); !errors.Is(err, functions.ErrIncompatibleHandle) {
		t.Fatal("foreign wrapper accepted", err)
	}
	if _, err := base.Metadata(handle); !errors.Is(err, functions.ErrIncompatibleHandle) {
		t.Fatal("unwrapped provider accepted wrapper", err)
	}
	if must(first.IsArithmeticFunction(handle)) || must(first.IsComparisonFunction(handle)) {
		t.Fatal("membership classification changed")
	}
	arithmetic := must(first.ArithmeticFunction(foundation.Add, []types.Type{f.integer, f.integer}))
	comparison := must(first.ComparisonFunction(foundation.Equal, f.integer, f.integer))
	if !must(first.IsArithmeticFunction(arithmetic)) || !must(first.IsComparisonFunction(comparison)) {
		t.Fatal("wrapped operator lookup/classification failed")
	}
	args := handle.Signature().ArgumentTypes()
	args[0] = f.integer
	if !handle.Signature().ArgumentTypes()[0].EqualsTo(f.array) || handle.Namespace() == "" {
		t.Fatal("wrapper getter ownership changed")
	}
	other := must(second.LookupBuiltInFunction(functions.BuiltInArrayContains, []types.Type{f.array, f.integer}))
	if handle.EqualsTo(other) || other.EqualsTo(handle) {
		t.Fatal("different wrapper domains compare equal")
	}
}
