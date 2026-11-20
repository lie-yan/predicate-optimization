package logical_flattening_rewriter_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/logical_flattening_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
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

// Observe provider entry, even when the underlying evaluator would fail or
// propagate NULL. A reconstruction fault leaves the actual metadata unchanged.
type observedContext struct {
	functioninterfaces.FunctionContext
	events        *[]string
	metadataCalls int
	metadataFault error
}

func (p *observedContext) Metadata(handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	p.metadataCalls++
	if p.metadataFault != nil {
		return nil, p.metadataFault
	}
	return p.FunctionContext.Metadata(handle)
}

func (p *observedContext) Evaluate(handle functioninterfaces.FunctionHandle, signature *functions.FunctionSignature, args []valueinterfaces.Value) (valueinterfaces.Value, error) {
	*p.events = append(*p.events, "call:"+handle.Name())
	return p.FunctionContext.Evaluate(handle, signature, args)
}

type tracedEnvironment struct {
	bindings truthEnvironment
	names    []string
	events   *[]string
}

func (e *tracedEnvironment) Lookup(id expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	for i, binding := range e.bindings {
		if binding.id.EqualsTo(id) {
			*e.events = append(*e.events, "lookup:"+e.names[i])
			return foundation.Some(binding.value), nil
		}
	}
	return foundation.Optional[valueinterfaces.Value]{}, nil
}

func TestFlattenEvaluationOrderAndFailures(t *testing.T) {
	boolean := must(types.NewBoolType(foundation.NonNull))
	aID, bID := must(ids.NewColumnID("events", "a")), must(ids.NewColumnID("events", "b"))
	a := must(expressions.NewReferenceExpression(aID, boolean, "a"))
	b := must(expressions.NewReferenceExpression(bID, boolean, "b"))
	yes, no := must(testbedvalues.NewBool(boolean, true)), must(testbedvalues.NewBool(boolean, false))
	firstFailure, secondFailure := errors.New("first evaluator failed"), errors.New("second evaluator failed")
	var events []string
	signature := must(functions.NewFunctionSignature([]types.Type{boolean}, boolean))
	var definitions []testbed.Definition
	for _, name := range []string{"pass", "first", "second"} {
		metadata := must(functions.NewFunctionMetadata(signature, name, foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, []functions.NullPolicy{functions.PropagateNull}))
		definitions = append(definitions, testbed.Definition{Namespace: "flattening", Name: name, Metadata: metadata})
	}
	catalog := must(testbed.NewCatalog(definitions))
	handles := catalog.Handles()
	provider := &observedContext{events: &events, FunctionContext: must(testbed.NewContextWithEvaluators(catalog, []testbed.EvaluationBinding{
		{Handle: handles[0], Evaluate: func(args []valueinterfaces.Value) (valueinterfaces.Value, error) { return args[0], nil }},
		{Handle: handles[1], Evaluate: func([]valueinterfaces.Value) (valueinterfaces.Value, error) { return nil, firstFailure }},
		{Handle: handles[2], Evaluate: func([]valueinterfaces.Value) (valueinterfaces.Value, error) { return nil, secondFailure }},
	}))}
	env := &tracedEnvironment{bindings: truthEnvironment{{aID, yes}, {bID, no}}, names: []string{"a", "b"}, events: &events}
	pass := must(expressions.NewCallExpression(provider, handles[0], []expressions.Expression{a}))
	first := must(expressions.NewCallExpression(provider, handles[1], []expressions.Expression{a}))
	second := must(expressions.NewCallExpression(provider, handles[2], []expressions.Expression{b}))
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("flattening", "unavailable")), boolean, "unavailable"))
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	for _, operator := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
		t.Run(operator.String(), func(t *testing.T) {
			group := func(children ...expressions.Expression) *expressions.LogicalExpression {
				return logical(operator, children...)
			}
			decisiveValue := valueinterfaces.Value(no)
			if operator == foundation.Or {
				decisiveValue = yes
			}
			decisive := must(expressions.NewLiteralExpression(decisiveValue))
			for _, tc := range []struct {
				name            string
				source          expressions.Expression
				trace           []string
				cause, category error
			}{
				{"shared call and references", group(group(pass, b), pass, a), []string{"lookup:a", "call:pass", "lookup:b", "lookup:a", "call:pass", "lookup:a"}, nil, nil},
				{"decisive literal before first failure", group(decisive, group(pass, first), second, b), []string{"lookup:a", "call:pass", "lookup:a", "call:first"}, firstFailure, functions.ErrEvaluationFailure},
				{"second failure encountered first", group(decisive, group(pass, second), first, a), []string{"lookup:a", "call:pass", "lookup:b", "call:second"}, secondFailure, functions.ErrEvaluationFailure},
				{"opaque remains unavailable", group(decisive, group(pass, opaque), second), []string{"lookup:a", "call:pass"}, interpreter.ErrEvaluationUnavailable, interpreter.ErrEvaluationUnavailable},
			} {
				t.Run(tc.name, func(t *testing.T) {
					snapshots := snapshotChildren(tc.source)
					events = nil
					provider.metadataCalls = 0
					tree := rewriter.NewExpressionTreeRewriter[struct{}](logical_flattening_rewriter.NewLogicalFlatteningRewriter())
					result := must(tree.Rewrite(tc.source, struct{}{}))
					if len(events) != 0 || provider.metadataCalls != 0 {
						t.Fatal("flattening executed inputs or rebound unchanged calls")
					}
					for _, node := range []expressions.Expression{tc.source, result} {
						events = nil
						value, err := evaluator.Evaluate(node, env)
						if !slices.Equal(events, tc.trace) {
							t.Fatalf("evaluation trace: got %v, want %v", events, tc.trace)
						}
						if tc.cause != nil {
							if value != nil || !errors.Is(err, tc.cause) || !errors.Is(err, tc.category) {
								t.Fatalf("lost failure cause/category or returned a value: %v", err)
							}
							if tc.cause == firstFailure && errors.Is(err, secondFailure) || tc.cause == secondFailure && errors.Is(err, firstFailure) {
								t.Fatal("later failure leaked into result")
							}
						} else if err != nil || !value.EqualsTo(decisiveValue) || !value.Type().EqualsTo(node.Type()) {
							t.Fatalf("unexpected successful evaluation: %v, %v", value, err)
						}
					}
					checkSnapshots(t, snapshots)
				})
			}
		})
	}
}

func TestFlattenCallReconstructionFailureAndRetry(t *testing.T) {
	a, b, c := reference("a"), reference("b"), reference("c")
	var events []string
	provider := &observedContext{FunctionContext: must(testbed.NewStandardContext()), events: &events}
	argument := logical(foundation.Or, a, logical(foundation.Or, b, c))
	handle := must(provider.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{argument.Type()}))
	call := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{argument}))
	// The preceding AND has already been expanded when call reconstruction fails.
	source := logical(foundation.And, logical(foundation.And, a, b), call)
	expectedCall := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{logical(foundation.Or, a, b, c)}))
	want := logical(foundation.And, a, b, expectedCall)
	snapshots := snapshotChildren(source)
	cause := errors.New("injected metadata lookup failure")
	provider.metadataFault = fmt.Errorf("%w: %w", functions.ErrUnknownHandle, cause)
	provider.metadataCalls = 0
	tree := rewriter.NewExpressionTreeRewriter[struct{}](logical_flattening_rewriter.NewLogicalFlatteningRewriter())
	result, err := tree.Rewrite(source, struct{}{})
	if result != nil || !errors.Is(err, cause) || !errors.Is(err, functions.ErrUnknownHandle) {
		t.Fatalf("reconstruction failure published a partial result or lost its cause: %v", err)
	}
	if provider.metadataCalls != 1 || len(events) != 0 {
		t.Fatal("expected one metadata query and no evaluation")
	}
	checkSnapshots(t, snapshots)

	provider.metadataFault = nil
	provider.metadataCalls = 0
	result = must(tree.Rewrite(source, struct{}{}))
	if !result.EqualsTo(want) || !result.Type().EqualsTo(source.Type()) {
		t.Fatal("retry did not produce the complete flattened tree")
	}
	rebuiltCall := result.Children()[2].(*expressions.CallExpression)
	if rebuiltCall.Context() != provider || !rebuiltCall.Handle().EqualsTo(handle) {
		t.Fatal("reconstruction changed the paired context/handle")
	}
	if provider.metadataCalls != 1 || len(events) != 0 {
		t.Fatal("retry evaluated inputs or did not rebind changed call")
	}
	checkSnapshots(t, snapshots)
	// Once normalized, a second rewrite need not consult metadata at all.
	provider.metadataFault = cause
	provider.metadataCalls = 0
	if again := must(tree.Rewrite(result, struct{}{})); again != result {
		t.Fatal("unchanged retry result lost identity")
	}
	if provider.metadataCalls != 0 || len(events) != 0 {
		t.Fatal("unchanged calls were rebound or evaluated")
	}
}
