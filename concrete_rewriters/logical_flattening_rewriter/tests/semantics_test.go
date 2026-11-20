package logical_flattening_rewriter_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/logical_flattening_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Each axis is ordered True, False, NULL. These explicit three-input tables
// are independent of the interpreter and the production TruthValue operators.
var threeInputTruthTables = map[foundation.LogicalOperator][3][3][3]foundation.TruthValue{
	foundation.And: {
		{{foundation.True, foundation.False, foundation.Null}, {foundation.False, foundation.False, foundation.False}, {foundation.Null, foundation.False, foundation.Null}},
		{{foundation.False, foundation.False, foundation.False}, {foundation.False, foundation.False, foundation.False}, {foundation.False, foundation.False, foundation.False}},
		{{foundation.Null, foundation.False, foundation.Null}, {foundation.False, foundation.False, foundation.False}, {foundation.Null, foundation.False, foundation.Null}},
	},
	foundation.Or: {
		{{foundation.True, foundation.True, foundation.True}, {foundation.True, foundation.True, foundation.True}, {foundation.True, foundation.True, foundation.True}},
		{{foundation.True, foundation.True, foundation.True}, {foundation.True, foundation.False, foundation.Null}, {foundation.True, foundation.Null, foundation.Null}},
		{{foundation.True, foundation.True, foundation.True}, {foundation.True, foundation.Null, foundation.Null}, {foundation.True, foundation.Null, foundation.Null}},
	},
}

type truthBinding struct {
	id    expressioninterfaces.ColumnID
	value valueinterfaces.Value
}

type truthEnvironment []truthBinding

func (e truthEnvironment) Lookup(id expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	for _, binding := range e {
		if binding.id.EqualsTo(id) {
			return foundation.Some(binding.value), nil
		}
	}
	return foundation.Optional[valueinterfaces.Value]{}, nil
}

func booleanValue(truth foundation.TruthValue, typ *types.BoolType) valueinterfaces.Value {
	if truth == foundation.Null {
		return must(testbedvalues.NewNull(typ))
	}
	return must(testbedvalues.NewBool(typ, truth == foundation.True))
}

func TestFlattenThreeValuedAssociativity(t *testing.T) {
	truths := []foundation.TruthValue{foundation.True, foundation.False, foundation.Null}
	truthNames := []string{"True", "False", "NULL"}
	// Cover all eight declaration combinations. Mask 7 supplies all 27 nullable
	// assignments; other masks permit only non-NULL bindings for non-null columns.
	for mask := 0; mask < 8; mask++ {
		for _, operator := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
			t.Run(fmt.Sprintf("%s/nullable-mask-%d", operator, mask), func(t *testing.T) {
				var refs [3]*expressions.ReferenceExpression
				var columnTypes [3]*types.BoolType
				var columnIDs [3]expressioninterfaces.ColumnID
				for i, name := range []string{"a", "b", "c"} {
					nullability := foundation.NonNull
					if mask&(1<<i) != 0 {
						nullability = foundation.MayNull
					}
					columnTypes[i] = must(types.NewBoolType(nullability))
					columnIDs[i] = must(ids.NewColumnID("truth", name))
					refs[i] = must(expressions.NewReferenceExpression(columnIDs[i], columnTypes[i], name))
				}
				resultNullability := foundation.NonNull
				if mask != 0 {
					resultNullability = foundation.MayNull
				}
				resultType := must(types.NewBoolType(resultNullability))
				for _, tc := range []struct {
					name   string
					source expressions.Expression
				}{
					{"left", logical(operator, logical(operator, refs[0], refs[1]), refs[2])},
					{"right", logical(operator, refs[0], logical(operator, refs[1], refs[2]))},
					{"flat", logical(operator, refs[0], refs[1], refs[2])},
				} {
					t.Run(tc.name, func(t *testing.T) {
						tree := rewriter.NewExpressionTreeRewriter[struct{}](logical_flattening_rewriter.NewLogicalFlatteningRewriter())
						result := must(tree.Rewrite(tc.source, struct{}{}))
						checkFlatTypes(t, tc.source, result, resultType, []expressions.Expression{refs[0], refs[1], refs[2]})
						for a, av := range truths {
							for b, bv := range truths {
								for c, cv := range truths {
									if (a == 2 && mask&1 == 0) || (b == 2 && mask&2 == 0) || (c == 2 && mask&4 == 0) {
										continue
									}
									t.Run(fmt.Sprintf("%s,%s,%s", truthNames[a], truthNames[b], truthNames[c]), func(t *testing.T) {
										env := truthEnvironment{
											{columnIDs[0], booleanValue(av, columnTypes[0])},
											{columnIDs[1], booleanValue(bv, columnTypes[1])},
											{columnIDs[2], booleanValue(cv, columnTypes[2])},
										}
										want := booleanValue(threeInputTruthTables[operator][a][b][c], resultType)
										checkEvaluations(t, tc.source, result, env, want)
									})
								}
							}
						}
					})
				}
			})
		}
	}
}

func checkFlatTypes(t *testing.T, source, result expressions.Expression, want types.Type, operands []expressions.Expression) {
	t.Helper()
	if !source.Type().EqualsTo(want) || !result.Type().EqualsTo(want) {
		t.Fatal("full result Type changed or differs from declared expectation")
	}
	children := result.Children()
	if len(children) != len(operands) {
		t.Fatal("flattening folded or removed operands")
	}
	for i, operand := range operands {
		if children[i] != operand || !children[i].Type().EqualsTo(operand.Type()) {
			t.Fatalf("operand %d lost its instance or full Type", i)
		}
	}
}

func checkEvaluations(t *testing.T, source, result expressions.Expression, env truthEnvironment, want valueinterfaces.Value) {
	t.Helper()
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	// Evaluate both trees independently and check each against the oracle, so
	// agreement between two equally incorrect evaluations cannot pass this test.
	for _, node := range []expressions.Expression{source, result} {
		got, err := evaluator.Evaluate(node, env)
		if err != nil {
			t.Fatal(err)
		}
		if !got.EqualsTo(want) || !got.Type().EqualsTo(want.Type()) {
			t.Fatalf("%s: got %v (%s), want %v (%s)", node, got, got.Type(), want, want.Type())
		}
	}
}

func TestFlattenRetainsLiteralTruthAndDeclaredTypes(t *testing.T) {
	nullable := must(types.NewBoolType(foundation.MayNull))
	nonnull := must(types.NewBoolType(foundation.NonNull))
	literal := func(truth foundation.TruthValue, typ *types.BoolType) *expressions.LiteralExpression {
		return must(expressions.NewLiteralExpression(booleanValue(truth, typ)))
	}
	yes, no := literal(foundation.True, nonnull), literal(foundation.False, nonnull)
	nullableYes, nullableNo, null := literal(foundation.True, nullable), literal(foundation.False, nullable), literal(foundation.Null, nullable)
	for _, tc := range []struct {
		name     string
		operator foundation.LogicalOperator
		operands []expressions.Expression
		want     foundation.TruthValue
		typ      *types.BoolType
	}{
		{"AND decisive false still nullable", foundation.And, []expressions.Expression{no, null, nullableYes}, foundation.False, nullable},
		{"OR decisive true still nullable", foundation.Or, []expressions.Expression{yes, null, nullableNo}, foundation.True, nullable},
		{"AND known NULL", foundation.And, []expressions.Expression{yes, null, nullableYes}, foundation.Null, nullable},
		{"OR known NULL", foundation.Or, []expressions.Expression{no, null, nullableNo}, foundation.Null, nullable},
		{"AND all non-null", foundation.And, []expressions.Expression{yes, yes, yes}, foundation.True, nonnull},
		{"OR all non-null", foundation.Or, []expressions.Expression{no, no, no}, foundation.False, nonnull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := logical(tc.operator, tc.operands[0], logical(tc.operator, tc.operands[1], tc.operands[2]))
			tree := rewriter.NewExpressionTreeRewriter[struct{}](logical_flattening_rewriter.NewLogicalFlatteningRewriter())
			result := must(tree.Rewrite(source, struct{}{}))
			checkFlatTypes(t, source, result, tc.typ, tc.operands)
			checkEvaluations(t, source, result, truthEnvironment{}, booleanValue(tc.want, tc.typ))
		})
	}
}
