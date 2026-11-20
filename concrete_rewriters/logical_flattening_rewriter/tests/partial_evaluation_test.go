package logical_flattening_rewriter_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/logical_flattening_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestFlattenPartialEvaluationCompletions(t *testing.T) {
	truths := []foundation.TruthValue{foundation.True, foundation.False, foundation.Null}
	names := []string{"True", "False", "NULL", "missing"}
	// Include wholly nullable/non-nullable declarations and mixed declarations.
	// Missing is valid even for a non-nullable column; a NULL binding is not.
	for _, mask := range []int{0, 5, 7} {
		for _, operator := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
			for _, leftAssociated := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/nullable-mask-%d/left-%t", operator, mask, leftAssociated), func(t *testing.T) {
					var refs [3]*expressions.ReferenceExpression
					var columnTypes [3]*types.BoolType
					for i, name := range []string{"a", "b", "c"} {
						n := foundation.NonNull
						if mask&(1<<i) != 0 {
							n = foundation.MayNull
						}
						columnTypes[i] = must(types.NewBoolType(n))
						refs[i] = must(expressions.NewReferenceExpression(must(ids.NewColumnID("partial", name)), columnTypes[i], name))
					}
					source := logical(operator, refs[0], logical(operator, refs[1], refs[2]))
					if leftAssociated {
						source = logical(operator, logical(operator, refs[0], refs[1]), refs[2])
					}
					tree := rewriter.NewExpressionTreeRewriter[struct{}](logical_flattening_rewriter.NewLogicalFlatteningRewriter())
					flattened := must(tree.Rewrite(source, struct{}{}))
					snapshots := append(snapshotChildren(source), snapshotChildren(flattened)...)
					evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
					// Every three-column combination of True, False, NULL and absence.
					for code := 0; code < 64; code++ {
						states := [3]int{code / 16, (code / 4) % 4, code % 4}
						if !validPartialStates(states, mask) {
							continue
						}
						t.Run(fmt.Sprintf("%s,%s,%s", names[states[0]], names[states[1]], names[states[2]]), func(t *testing.T) {
							env := truthEnvironment{}
							var missing []*expressions.ReferenceExpression
							for i, state := range states {
								if state == 3 {
									missing = append(missing, refs[i])
									continue
								}
								env = append(env, truthBinding{refs[i].ID(), booleanValue(truths[state], columnTypes[i])})
							}
							// Lookup absence and a present typed NULL remain separate cases.
							for i, state := range states {
								value, present := must(env.Lookup(refs[i].ID())).Get()
								if present != (state != 3) {
									t.Fatal("environment confused absence and presence")
								}
								if present && value.IsNull() != (state == 2) {
									t.Fatal("environment confused NULL with Boolean value")
								}
							}
							partials := []interpreter.Result{
								must(evaluator.PartialEvaluate(source, env)),
								must(evaluator.PartialEvaluate(flattened, env)),
							}
							for _, partial := range partials {
								if !partial.Type().EqualsTo(source.Type()) {
									t.Fatal("partial evaluation changed exact Type")
								}
								if len(missing) == 0 {
									value, known := partial.Known()
									want := booleanValue(threeInputTruthTables[operator][states[0]][states[1]][states[2]], source.Type().(*types.BoolType))
									if !known || !value.EqualsTo(want) {
										t.Fatal("fully bound input did not produce expected known value, including NULL")
									}
									continue
								}
								residual, present := partial.Residual()
								if !present {
									t.Fatal("missing input was treated as known, or discarded by a decisive value")
								}
								if !slices.Equal(residualReferences(residual), missing) {
									t.Fatal("residual lost missing references or retained bound references")
								}
								if value, err := evaluator.Evaluate(residual, env); value != nil || !errors.Is(err, interpreter.ErrUnavailableInput) {
									t.Fatalf("residual should still require its missing inputs: %v", err)
								}
							}
							// Complete only absent bindings. Evaluate each residual independently;
							// their structures may legitimately differ after known groups fold.
							completions := 0
							for completeCode := 0; completeCode < 27; completeCode++ {
								complete := [3]int{completeCode / 9, (completeCode / 3) % 3, completeCode % 3}
								if !validPartialStates(complete, mask) {
									continue
								}
								compatible := true
								for i, state := range states {
									if state != 3 && state != complete[i] {
										compatible = false
									}
								}
								if !compatible {
									continue
								}
								completions++
								fullEnv := truthEnvironment{}
								for i, state := range complete {
									fullEnv = append(fullEnv, truthBinding{refs[i].ID(), booleanValue(truths[state], columnTypes[i])})
								}
								want := booleanValue(threeInputTruthTables[operator][complete[0]][complete[1]][complete[2]], source.Type().(*types.BoolType))
								checkEvaluations(t, source, flattened, fullEnv, want)
								for _, partial := range partials {
									value, known := partial.Known()
									if !known {
										residual, _ := partial.Residual()
										value = must(evaluator.Evaluate(residual, fullEnv))
									}
									if !value.EqualsTo(want) || !value.Type().EqualsTo(source.Type()) {
										t.Fatalf("completion %v changed truth or exact Type", complete)
									}
								}
							}
							if completions == 0 {
								t.Fatal("fixture checked no completions")
							}
							checkSnapshots(t, snapshots)
						})
					}
				})
			}
		}
	}
}

func validPartialStates(states [3]int, nullableMask int) bool {
	for i, state := range states {
		if state == 2 && nullableMask&(1<<i) == 0 {
			return false
		}
	}
	return true
}

func residualReferences(node expressions.Expression) []*expressions.ReferenceExpression {
	if reference, ok := node.(*expressions.ReferenceExpression); ok {
		return []*expressions.ReferenceExpression{reference}
	}
	var result []*expressions.ReferenceExpression
	for _, child := range node.Children() {
		result = append(result, residualReferences(child)...)
	}
	return result
}
