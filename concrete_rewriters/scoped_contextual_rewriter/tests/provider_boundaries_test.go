package scoped_contextual_rewriter_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/boolean_cleanup_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

type boundaryProvider struct {
	*contextualProvider
	lookup func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error)
}

func (p *boundaryProvider) ComparisonFunction(op foundation.OperatorType, a, b types.Type) (functioninterfaces.FunctionHandle, error) {
	if p.lookup != nil {
		return p.lookup(op, a, b)
	}
	return p.FunctionContext.ComparisonFunction(op, a, b)
}

// EV.01/EV.09: fail after valid source construction; both passes preserve the
// provider's error categories and return no successful partial replacement.
func TestPublicProviderInspectionFailures(t *testing.T) {
	for _, mode := range []string{"metadata", "foreign lookup handle", "wrong lookup signature", "nil lookup handle", "lookup failure", "unsupported plus contract failure"} {
		t.Run(mode, func(t *testing.T) {
			p := &boundaryProvider{contextualProvider: &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}}
			x := reference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
			guard := membership(x, 1)
			target := comparison(p, foundation.Equal, x, 2)
			source := logical(foundation.And, guard, logical(foundation.Not, target), booleanLiteral(foundation.False, foundation.NonNull))
			before := expressions.PrettyPrint(source)
			var causes []error
			switch mode {
			case "metadata":
				causes = []error{functions.ErrUnknownHandle}
				p.metadata = func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
					return nil, fmt.Errorf("fixture metadata: %w", causes[0])
				}
			case "foreign lookup handle":
				foreign := must(testbed.NewStandardContext())
				h := must(foreign.ComparisonFunction(foundation.Equal, x.Type(), x.Type()))
				p.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
					return h, nil
				}
				causes = []error{functions.ErrIncompatibleHandle}
			case "wrong lookup signature":
				b := must(types.NewBoolType(foundation.NonNull))
				h := must(p.ComparisonFunction(foundation.Equal, b, b))
				p.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
					return h, nil
				}
				causes = []error{functions.ErrContractViolation, functions.ErrUnsupportedLookup}
			case "nil lookup handle":
				p.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
					return nil, nil
				}
				causes = []error{functions.ErrContractViolation}
			default:
				causes = []error{errors.New("equality lookup failed")}
				if mode == "unsupported plus contract failure" {
					causes = []error{functions.ErrUnsupportedLookup, functions.ErrContractViolation}
				}
				p.lookup = func(foundation.OperatorType, types.Type, types.Type) (functioninterfaces.FunctionHandle, error) {
					return nil, fmt.Errorf("fixture lookup: %w", errors.Join(causes...))
				}
			}
			trees := []rewriter.ExpressionTreeRewriter[struct{}]{
				rewriter.NewExpressionTreeRewriter[struct{}](scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, p)),
				rewriter.NewExpressionTreeRewriter[struct{}](boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, p)),
			}
			for _, tree := range trees {
				got, err := tree.Rewrite(source, struct{}{})
				if got != nil || err == nil {
					t.Fatal("provider error returned a candidate", err)
				}
				for _, cause := range causes {
					if !errors.Is(err, cause) {
						t.Fatalf("lost category %v: %v", cause, err)
					}
				}
			}
			if p.evaluations != 0 || expressions.PrettyPrint(source) != before {
				t.Fatal("failure evaluated or mutated source")
			}
			p.lookup, p.metadata = nil, nil
			for _, tree := range trees {
				if _, err := tree.Rewrite(source, struct{}{}); err != nil {
					t.Fatal("retry retained failed state", err)
				}
			}
			if p.evaluations != 0 {
				t.Fatal("retry evaluated expressions")
			}
		})
	}
}

func TestPublicCallSetupRejectsProviderMismatchAndBinding(t *testing.T) {
	provider := &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}
	other := must(testbed.NewStandardContext())
	one := integer(1, foundation.NonNull)
	handle := must(provider.ComparisonFunction(foundation.Equal, one.Type(), one.Type()))
	if call, err := expressions.NewCallExpression(other, handle, []expressions.Expression{one, one}); call != nil || !errors.Is(err, functions.ErrIncompatibleHandle) {
		t.Fatal("foreign handle accepted", err)
	}
	truth := booleanLiteral(foundation.True, foundation.NonNull)
	if call, err := expressions.NewCallExpression(provider, handle, []expressions.Expression{truth, truth}); call != nil || !errors.Is(err, functions.ErrUnsupportedLookup) {
		t.Fatal("bad binding accepted", err)
	}
	if provider.evaluations != 0 {
		t.Fatal("setup evaluated")
	}
}

func boundaryFunction(arguments []types.Type, evaluate testbed.Evaluator) (*contextualProvider, functioninterfaces.FunctionHandle) {
	boolean := must(types.NewBoolType(foundation.NonNull))
	policies := make([]functions.NullPolicy, len(arguments))
	for i := range policies {
		policies[i] = functions.PropagateNull
	}
	metadata := must(functions.NewFunctionMetadata(must(functions.NewFunctionSignature(arguments, boolean)), "foo", foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, policies))
	catalog := must(testbed.NewCatalog([]testbed.Definition{{Namespace: "boundary", Name: "foo", Metadata: metadata}}))
	handle := catalog.Handles()[0]
	context := must(testbed.NewContextWithEvaluators(catalog, []testbed.EvaluationBinding{{Handle: handle, Evaluate: evaluate}}))
	return &contextualProvider{FunctionContext: context}, handle
}

// EV.04: rejecting a changed unknown enclosing call rolls back its child, while
// an independently useful sibling still permits publication of the owner.
func TestPublicUnsupportedParentRollbackPreservesFailure(t *testing.T) {
	provider := &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}
	x := reference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	guard := comparison(provider, foundation.Equal, x, 1)
	shared := comparison(provider, foundation.Equal, x, 1)
	boolean := must(types.NewBoolType(foundation.NonNull))
	failure := errors.New("foo(false) fails")
	foo, handle := boundaryFunction([]types.Type{boolean}, func(args []valueinterfaces.Value) (valueinterfaces.Value, error) {
		if !args[0].(*testbedvalues.BoolValue).Bool() {
			return nil, failure
		}
		return args[0], nil
	})
	call := must(expressions.NewCallExpression(foo, handle, []expressions.Expression{shared}))
	source := logical(foundation.And, guard, call, shared)
	stages := rewriteScalarFixture(t, provider, source, logical(foundation.And, guard, call))
	if stages[1].Children()[1] != call || stages[2].Children()[1] != call || call.Arguments()[0] != shared || foo.evaluations != 0 {
		t.Fatal("rejected call or its child was changed/evaluated")
	}
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
	for _, node := range stages {
		if _, err := evaluator.Evaluate(node, contextualRow{{x, integer(2, foundation.NonNull).Value()}}); !errors.Is(err, failure) || !errors.Is(err, functions.ErrEvaluationFailure) {
			t.Fatal("guard-false failure suppressed", err)
		}
		if got := must(evaluator.Evaluate(node, contextualRow{{x, integer(1, foundation.NonNull).Value()}})); !got.(*testbedvalues.BoolValue).Bool() {
			t.Fatal("guard-true result changed")
		}
	}
}

// EV.03/EV.05/EV.06: changed admitted enclosing uses retain failing siblings
// in order, even when a decisive literal or null propagation determines truth.
func TestPublicChangedEnclosingUsesRetainFailures(t *testing.T) {
	for _, mode := range []string{"absorbing logical", "comparison left", "comparison right", "null propagation"} {
		t.Run(mode, func(t *testing.T) {
			provider := &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}
			x := reference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
			guard := membership(x, 1)
			impossible := comparison(provider, foundation.Equal, x, 2)
			failure := errors.New("retained operand fails")
			failingProvider, h := boundaryFunction(nil, func([]valueinterfaces.Value) (valueinterfaces.Value, error) { return nil, failure })
			failing := must(expressions.NewCallExpression(failingProvider, h, nil))
			var original, replacement expressions.Expression = logical(foundation.And, impossible, failing), logical(foundation.And, booleanLiteral(foundation.False, foundation.NonNull), failing)
			if mode != "absorbing logical" {
				var before, after expressions.Expression = impossible, booleanLiteral(foundation.False, foundation.NonNull)
				if mode == "null propagation" {
					after = booleanLiteral(foundation.Null, foundation.MayNull)
					before = logical(foundation.Not, after)
				}
				args, newArgs := []expressions.Expression{before, failing}, []expressions.Expression{after, failing}
				if mode == "comparison right" {
					args[0], args[1] = args[1], args[0]
					newArgs[0], newArgs[1] = newArgs[1], newArgs[0]
				}
				handle := must(provider.ComparisonFunction(foundation.Equal, args[0].Type(), args[1].Type()))
				original = must(expressions.NewCallExpression(provider, handle, args))
				replacement = must(expressions.NewCallExpression(provider, handle, newArgs))
			}
			source := logical(foundation.And, guard, original)
			stages := rewriteScalarFixture(t, provider, source, logical(foundation.And, guard, replacement))
			if failingProvider.evaluations != 0 {
				t.Fatal("rewriting evaluated retained sibling")
			}
			evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
			for _, n := range []int64{0, 1, 2} {
				for _, node := range stages {
					calls := failingProvider.evaluations
					if _, err := evaluator.Evaluate(node, contextualRow{{x, integer(n, foundation.NonNull).Value()}}); !errors.Is(err, failure) || !errors.Is(err, functions.ErrEvaluationFailure) {
						t.Fatal("operand failure suppressed", err)
					}
					if failingProvider.evaluations != calls+1 {
						t.Fatal("retained occurrence did not execute exactly once")
					}
				}
			}
		})
	}
}

// EV.08: 1/x is safe when x==1, but must still fail at x==0 in the whole region.
func TestPublicGuardDoesNotLicenseArithmeticOmission(t *testing.T) {
	provider := &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}
	x := reference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	guard := comparison(provider, foundation.Equal, x, 1)
	one := integer(1, foundation.NonNull)
	handle := must(provider.ArithmeticFunction(foundation.Divide, []types.Type{x.Type(), x.Type()}))
	division := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{one, x}))
	positive := must(expressions.NewCallExpression(provider, must(provider.ComparisonFunction(foundation.GreaterThan, x.Type(), x.Type())), []expressions.Expression{division, integer(0, foundation.NonNull)}))
	original := logical(foundation.And, positive, booleanLiteral(foundation.False, foundation.NonNull))
	source := logical(foundation.And, guard, original, comparison(provider, foundation.Equal, x, 1))
	stages := rewriteScalarFixture(t, provider, source, logical(foundation.And, guard, original))
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
	for _, node := range stages {
		if _, err := evaluator.Evaluate(node, contextualRow{{x, integer(0, foundation.NonNull).Value()}}); !errors.Is(err, functions.ErrEvaluationFailure) {
			t.Fatal("division by zero suppressed", err)
		}
		for _, n := range []int64{-1, 1, 2} {
			if got := must(evaluator.Evaluate(node, contextualRow{{x, integer(n, foundation.NonNull).Value()}})); got.(*testbedvalues.BoolValue).Bool() {
				t.Fatal("false conjunct lost")
			}
		}
	}
}

// EV.10: admitted omission can reduce execution counts, but the optimizer itself
// performs no execution and the independently evaluated value/Type is unchanged.
func TestPublicAdmittedOmissionEvaluationCounts(t *testing.T) {
	provider := &contextualProvider{FunctionContext: must(testbed.NewStandardContext())}
	x := reference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	guard := comparison(provider, foundation.Equal, x, 1)
	source := logical(foundation.And, guard, guard)
	stages := rewriteScalarFixture(t, provider, source, guard)
	if provider.evaluations != 0 {
		t.Fatal("optimization evaluated")
	}
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
	for _, n := range []int64{0, 1, 2} {
		for j, node := range stages {
			before := provider.evaluations
			got := must(evaluator.Evaluate(node, contextualRow{{x, integer(n, foundation.NonNull).Value()}}))
			wantCalls := 1
			if j == 0 {
				wantCalls = 2
			}
			if provider.evaluations-before != wantCalls || !got.EqualsTo(booleanLiteral(truthOf(n == 1), foundation.NonNull).Value()) {
				t.Fatal("unexpected admitted omission count or result")
			}
		}
	}
}
