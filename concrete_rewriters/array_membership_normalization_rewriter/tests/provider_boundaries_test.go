package array_membership_normalization_rewriter_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

func boundaryProvider(wrapped bool) functioninterfaces.FunctionContext {
	var provider functioninterfaces.FunctionContext = must(testbed.NewStandardContext())
	if wrapped {
		provider = &wrappedContext{delegate: provider}
	}
	return provider
}

func assertNoBoundaryEvaluation(t *testing.T, contexts ...*recordingContext) {
	t.Helper()
	for _, context := range contexts {
		for _, event := range context.events {
			if event.operation == "evaluate" {
				t.Fatal("normalization evaluated a function")
			}
		}
	}
}

func TestNormalizationPhaseFailureCategoriesAndRetry(t *testing.T) {
	categories := []struct {
		name  string
		cause error
	}{
		{"unsupported", nil},
		{"unknown handle", functions.ErrUnknownHandle},
		{"incompatible handle", functions.ErrIncompatibleHandle},
		{"contract", functions.ErrContractViolation},
		{"evaluation failure", functions.ErrEvaluationFailure},
		{"unavailable", functions.ErrEvaluationUnavailable},
		{"disallowed", functions.ErrEvaluationDisallowed},
	}
	phases := []string{"source metadata", "target lookup", "target metadata", "target construction", "enclosing reconstruction"}
	for _, wrapped := range []bool{false, true} {
		for _, phase := range phases {
			for _, category := range categories {
				t.Run(fmt.Sprintf("wrapped=%t/%s/%s", wrapped, phase, category.name), func(t *testing.T) {
					f := newNormalizationFixture(boundaryProvider(wrapped), foundation.MayNull)
					source := f.call(functions.BuiltInArrayContainsAny, f.input, f.candidates(8792001))
					target := f.call(functions.BuiltInArrayContains, f.input, f.element(8792001))
					// An independent parent provider proves that reconstruction
					// uses the enclosing call's own context after child work.
					parent := &recordingContext{FunctionContext: boundaryProvider(wrapped)}
					not := must(parent.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{source.Type()}))
					root := must(expressions.NewCallExpression(parent, not, []expressions.Expression{source}))
					expected := must(expressions.NewCallExpression(parent, not, []expressions.Expression{target}))
					if err := utils.ValidateForOptimization(root); err != nil {
						t.Fatal(err)
					}
					before := expressions.PrettyPrint(root)
					sentinel := errors.New("phase fault")
					failure := fmt.Errorf("provider operation: %w", errors.Join(functions.ErrUnsupportedLookup, category.cause, sentinel))
					hit, targetQueries := false, 0
					f.context.fault = func(event functionEvent) error {
						match := false
						if event.operation == "lookup" {
							match = phase == "target lookup"
						}
						if event.operation == "metadata" {
							if event.handle.EqualsTo(source.Handle()) {
								match = phase == "source metadata"
							}
							if event.handle.EqualsTo(target.Handle()) {
								targetQueries++
								match = phase == "target metadata" && targetQueries == 1 || phase == "target construction" && targetQueries == 2
							}
						}
						if match {
							hit = true
							return failure
						}
						return nil
					}
					parent.fault = func(event functionEvent) error {
						if phase == "enclosing reconstruction" && event.operation == "metadata" && event.handle.EqualsTo(not) {
							hit = true
							return failure
						}
						return nil
					}
					f.context.events, parent.events = nil, nil
					tree := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
					result, err := tree.Rewrite(root, struct{}{})
					if !hit {
						t.Fatal("fixture failed to reach selected phase")
					}
					benign := phase == "target lookup" && category.cause == nil
					if benign {
						if err != nil || result != root {
							t.Fatal("unsupported explicit lookup did not retain original", err)
						}
					} else {
						if result != nil || !errors.Is(err, sentinel) || !errors.Is(err, functions.ErrUnsupportedLookup) {
							t.Fatal("failure lost its causes or published a partial tree", err)
						}
						if category.cause != nil && !errors.Is(err, category.cause) {
							t.Fatal("stronger joined category was hidden", err)
						}
					}
					if (phase == "target construction" || phase == "enclosing reconstruction") && targetQueries != 2 {
						t.Fatal("failure preceded checked child construction", targetQueries)
					}
					if expressions.PrettyPrint(root) != before || root.Arguments()[0] != source {
						t.Fatal("failure mutated original tree")
					}
					assertNoBoundaryEvaluation(t, f.context, parent)
					// Reuse the same tree service and immutable original after
					// removing only the operation faults; provider meanings stay stable.
					f.context.fault, parent.fault = nil, nil
					f.context.events, parent.events = nil, nil
					retry := must(tree.Rewrite(root, struct{}{}))
					if !retry.EqualsTo(expected) || !retry.Type().EqualsTo(root.Type()) {
						t.Fatal("clean retry changed expected tree or full Type")
					}
					enclosing := retry.(*expressions.CallExpression)
					child := enclosing.Arguments()[0].(*expressions.CallExpression)
					if enclosing.Context() != parent || child.Context() != f.context || !child.Handle().EqualsTo(target.Handle()) {
						t.Fatal("reconstruction crossed provider domains")
					}
					assertNoBoundaryEvaluation(t, f.context, parent)
					if err := utils.ValidateForOptimization(retry); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}

func TestNormalizationDistinctCatalogPairing(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		for _, nullable := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			t.Run(fmt.Sprintf("wrapped=%t/nullable=%v", wrapped, nullable), func(t *testing.T) {
				left := newNormalizationFixture(boundaryProvider(wrapped), nullable)
				right := newNormalizationFixture(boundaryProvider(wrapped), nullable)
				sources := []*expressions.CallExpression{
					left.call(functions.BuiltInArrayContainsAny, left.input, left.candidates(8792001)),
					right.call(functions.BuiltInArrayContainsAny, right.input, right.candidates(8793001)),
				}
				targets := []*expressions.CallExpression{
					left.call(functions.BuiltInArrayContains, left.input, left.element(8792001)),
					right.call(functions.BuiltInArrayContains, right.input, right.element(8793001)),
				}
				if sources[0].Handle().Name() != sources[1].Handle().Name() || sources[0].Handle().EqualsTo(sources[1].Handle()) {
					t.Fatal("fixture must have identical names and different identity domains")
				}
				root := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{sources[0], sources[1]}))
				expected := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{targets[0], targets[1]}))
				if err := utils.ValidateForOptimization(root); err != nil {
					t.Fatal(err)
				}
				before := expressions.PrettyPrint(root)
				left.context.events, right.context.events = nil, nil
				tree := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
				result := must(tree.Rewrite(root, struct{}{}))
				if !result.EqualsTo(expected) || !result.Type().EqualsTo(root.Type()) {
					t.Fatal("paired normalization changed expected tree or full Type")
				}
				for index, context := range []*recordingContext{left.context, right.context} {
					call := result.Children()[index].(*expressions.CallExpression)
					if call.Context() != context || !call.Handle().EqualsTo(targets[index].Handle()) {
						t.Fatal("replacement used another provider")
					}
					for _, event := range context.events {
						if event.operation == "metadata" && !event.handle.EqualsTo(sources[index].Handle()) && !event.handle.EqualsTo(targets[index].Handle()) {
							t.Fatal("foreign metadata query")
						}
					}
				}
				assertNoBoundaryEvaluation(t, left.context, right.context)
				if expressions.PrettyPrint(root) != before {
					t.Fatal("source changed")
				}
				if err := utils.ValidateForOptimization(result); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestNormalizationForeignLookupHandleAndRetry(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		t.Run(fmt.Sprintf("wrapped=%t", wrapped), func(t *testing.T) {
			provider := &substitutedTargetContext{FunctionContext: boundaryProvider(wrapped)}
			f := newNormalizationFixture(provider, foundation.MayNull)
			source := f.call(functions.BuiltInArrayContainsAny, f.input, f.candidates(8792001))
			target := f.call(functions.BuiltInArrayContains, f.input, f.element(8792001))
			if err := utils.ValidateForOptimization(source); err != nil {
				t.Fatal(err)
			}
			foreign := boundaryProvider(wrapped)
			handle := must(foreign.LookupBuiltInFunction(functions.BuiltInArrayContains, []types.Type{f.input.Type(), f.integer}))
			if handle.Name() != target.Handle().Name() || handle.EqualsTo(target.Handle()) {
				t.Fatal("fixture identity domains are not distinct")
			}
			provider.target = foundation.Some(handle)
			before := expressions.PrettyPrint(source)
			f.context.events = nil
			tree := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
			result, err := tree.Rewrite(source, struct{}{})
			if result != nil || !errors.Is(err, functions.ErrIncompatibleHandle) {
				t.Fatal("foreign handle was accepted or hidden", err)
			}
			if expressions.PrettyPrint(source) != before {
				t.Fatal("source changed")
			}
			assertNoBoundaryEvaluation(t, f.context)
			provider.target = foundation.Optional[functioninterfaces.FunctionHandle]{}
			retry := must(tree.Rewrite(source, struct{}{}))
			if !retry.EqualsTo(target) || !retry.Type().EqualsTo(source.Type()) {
				t.Fatal("retry did not restore the paired replacement")
			}
			assertNoBoundaryEvaluation(t, f.context)
		})
	}
}
