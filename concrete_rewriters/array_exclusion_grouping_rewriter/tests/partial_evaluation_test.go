package array_exclusion_grouping_rewriter_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/interpreter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func hasReference(node expressions.Expression, id expressioninterfaces.ColumnID) bool {
	if reference, ok := node.(*expressions.ReferenceExpression); ok && reference.ID().EqualsTo(id) {
		return true
	}
	for _, child := range node.Children() {
		if hasReference(child, id) {
			return true
		}
	}
	return false
}

func partialBoolean(typ *types.BoolType, truth oracleTruth) valueinterfaces.Value {
	if truth == oracleNull {
		return must(testbedvalues.NewNull(typ))
	}
	return must(testbedvalues.NewBool(typ, truth == oracleTrue))
}

func TestMissingArrayResidualCompletion(t *testing.T) {
	for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		for _, flagState := range []oracleTruth{"absent", "missing", oracleTrue, oracleFalse, oracleNull} {
			t.Run(fmt.Sprintf("%v/flag=%s", n, flagState), func(t *testing.T) {
				integer := must(types.NewIntType(types.Int64, foundation.NonNull))
				fixture := newMembershipFixture(must(testbed.NewStandardContext()), integer, n)
				one, two := must(testbedvalues.NewInteger(integer, 1)), must(testbedvalues.NewInteger(integer, 2))
				flagType := must(types.NewBoolType(foundation.MayNull))
				var flag *expressions.ReferenceExpression
				resultNullability := n
				if flagState != "absent" {
					flag = fixtureReference("flag", flagType)
					resultNullability = foundation.MayNull
				}
				source, result := rewriteSemanticGroup(t, fixture, []semanticClause{{true, []valueinterfaces.Value{one}}, {false, []valueinterfaces.Value{two, two}}}, flag)
				var partialBindings []fixtureBinding
				if flag != nil && flagState != "missing" {
					partialBindings = []fixtureBinding{{flag.ID(), partialBoolean(flagType, flagState)}}
				}
				evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
				var residuals []expressions.Expression
				for _, node := range []expressions.Expression{source, result} {
					partial := must(evaluator.PartialEvaluate(node, &fixtureEnvironment{bindings: partialBindings}))
					residual, present := partial.Residual()
					if _, known := partial.Known(); known || !present || !partial.Type().EqualsTo(node.Type()) || !hasReference(residual, fixture.input.ID()) {
						t.Fatal("missing array became known/NULL or lost exact Type")
					}
					if flag != nil && hasReference(residual, flag.ID()) != (flagState == "missing") {
						t.Fatal("known sibling did not fold independently, or missing sibling was lost")
					}
					if err := utils.ValidateForOptimization(residual); err != nil {
						t.Fatal(err)
					}
					residuals = append(residuals, residual)
				}
				// Residual syntax may differ. Complete both using the same independently
				// chosen row and compare to the original expressions and raw-value oracle.
				rows := [][]int64{nil, {1}, {9}, {2, 2}}
				rowCount := len(rows)
				if n == foundation.MayNull {
					rowCount++
				}
				flags := []oracleTruth{flagState}
				if flagState == "missing" {
					flags = []oracleTruth{oracleTrue, oracleFalse, oracleNull}
				}
				typ := must(types.NewBoolType(resultNullability))
				for row := 0; row < rowCount; row++ {
					for _, flagTruth := range flags {
						isNull := row == len(rows)
						var raw []int64
						var array valueinterfaces.Value
						if isNull {
							array = must(testbedvalues.NewNull(fixture.input.Type()))
						} else {
							raw = rows[row]
							var items []valueinterfaces.Value
							for _, number := range raw {
								items = append(items, must(testbedvalues.NewInteger(integer, number)))
							}
							array = fixture.inputValue(items...)
						}
						bindings := []fixtureBinding{{fixture.input.ID(), array}}
						expected := integerExclusionOracle(raw, isNull, [][]int64{{1}, {2, 2}})
						if flag != nil {
							bindings = append(bindings, fixtureBinding{flag.ID(), partialBoolean(flagType, flagTruth)})
							expected = oracleAnd(expected, flagTruth)
						}
						for i, node := range []expressions.Expression{source, result, residuals[0], residuals[1]} {
							verifyOracleResult(t, node, bindings, typ, expected, fmt.Sprintf("form=%d/row=%d/flag=%s", i, row, flagTruth))
						}
					}
				}
			})
		}
	}
}

func TestKnownNullArrayIsNotMissing(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	fixture := newMembershipFixture(must(testbed.NewStandardContext()), integer, foundation.MayNull)
	one, two := must(testbedvalues.NewInteger(integer, 1)), must(testbedvalues.NewInteger(integer, 2))
	source, result := rewriteSemanticGroup(t, fixture, []semanticClause{{true, []valueinterfaces.Value{one}}, {true, []valueinterfaces.Value{two}}}, nil)
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	bindings := []fixtureBinding{{fixture.input.ID(), must(testbedvalues.NewNull(fixture.input.Type()))}}
	for _, node := range []expressions.Expression{source, result} {
		missing := must(evaluator.PartialEvaluate(node, &fixtureEnvironment{}))
		if _, present := missing.Residual(); !present {
			t.Fatal("missing array was not residual")
		}
		known := must(evaluator.PartialEvaluate(node, &fixtureEnvironment{bindings: bindings}))
		value, present := known.Known()
		if !present || !value.IsNull() || !value.Type().EqualsTo(node.Type()) {
			t.Fatal("present NULL was not a known exact-Type NULL")
		}
		// Known NULL can fold while an unrelated Boolean reference stays residual.
		flagType := must(types.NewBoolType(foundation.MayNull))
		flag := fixtureReference("flag", flagType)
		parent := fixtureLogical(foundation.And, node, flag)
		residual, present := must(evaluator.PartialEvaluate(parent, &fixtureEnvironment{bindings: bindings})).Residual()
		if !present || hasReference(residual, fixture.input.ID()) || !hasReference(residual, flag.ID()) {
			t.Fatal("partial NULL/reference distinction lost")
		}
		for _, truth := range []oracleTruth{oracleTrue, oracleFalse, oracleNull} {
			completed := append([]fixtureBinding{{flag.ID(), partialBoolean(flagType, truth)}}, bindings...)
			verifyOracleResult(t, residual, completed, flagType, oracleAnd(oracleNull, truth), "known NULL with missing sibling")
		}
	}
}
