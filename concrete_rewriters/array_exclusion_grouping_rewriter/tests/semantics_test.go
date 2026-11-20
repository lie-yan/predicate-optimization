package array_exclusion_grouping_rewriter_test

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// This test oracle uses ordinary Go values and its own truth table, independent
// of the interpreter, function evaluators and production TruthValue operations.
type oracleTruth string

const (
	oracleTrue  oracleTruth = "T"
	oracleFalse oracleTruth = "F"
	oracleNull  oracleTruth = "NULL"
)

func oracleAnd(left, right oracleTruth) oracleTruth {
	if left == oracleFalse || right == oracleFalse {
		return oracleFalse
	}
	if left == oracleNull || right == oracleNull {
		return oracleNull
	}
	return oracleTrue
}

func integerExclusionOracle(array []int64, null bool, clauses [][]int64) oracleTruth {
	if null {
		return oracleNull
	}
	result := oracleTrue
	for _, candidates := range clauses {
		exclusion := oracleTrue
		for _, item := range array {
			for _, candidate := range candidates {
				if item == candidate {
					exclusion = oracleFalse
				}
			}
		}
		result = oracleAnd(result, exclusion)
	}
	return result
}

func verifyOracleResult(t *testing.T, expression expressions.Expression, bindings []fixtureBinding, typ types.Type, truth oracleTruth, label string) {
	t.Helper()
	if !expression.Type().EqualsTo(typ) {
		t.Fatalf("%s: expression Type %s, want %s", label, expression.Type(), typ)
	}
	actual := must(interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{}).Evaluate(expression, &fixtureEnvironment{bindings: bindings}))
	if !actual.Type().EqualsTo(typ) {
		t.Fatalf("%s: Value Type %s, want %s", label, actual.Type(), typ)
	}
	if truth == oracleNull {
		if !actual.IsNull() {
			t.Fatalf("%s: got %v, want NULL", label, actual)
		}
	} else if actual.IsNull() || actual.(*testbedvalues.BoolValue).Bool() != (truth == oracleTrue) {
		t.Fatalf("%s: got %v, want %s", label, actual, truth)
	}
}

type semanticClause struct {
	// Scalar contains uses one candidate; otherwise use contains-any.
	scalar     bool
	candidates []valueinterfaces.Value
}

// Build from explicit fixture clauses, then inspect the actual generated tree.
// Compare ordered Value identities, so NaN equality or signed-zero equivalence
// cannot conceal deduplication, reordering or flattening composite elements.
func rewriteSemanticGroup(t *testing.T, fixture *membershipFixture, clauses []semanticClause, sibling *expressions.ReferenceExpression) (expressions.Expression, expressions.Expression) {
	t.Helper()
	var children []expressions.Expression
	var positions []valueinterfaces.Value
	for i, clause := range clauses {
		var call *expressions.CallExpression
		if clause.scalar {
			call = fixture.contains(fixture.input, clause.candidates[0])
		} else {
			call = fixture.containsAny(fixture.input, clause.candidates...)
		}
		children = append(children, fixtureExclude(call))
		positions = append(positions, clause.candidates...)
		if i == 0 && sibling != nil {
			children = append(children, sibling)
		}
	}
	source := fixtureLogical(foundation.And, children...)
	if err := utils.ValidateForOptimization(source); err != nil {
		t.Fatal(err)
	}
	result := must(boundaryTree().Rewrite(source, struct{}{}))
	if err := utils.ValidateForOptimization(result); err != nil {
		t.Fatal(err)
	}
	grouped := result
	if sibling != nil {
		logical, ok := result.(*expressions.LogicalExpression)
		if !ok || logical.Operator() != foundation.And || logical.ChildCount() != 2 || logical.Children()[1] != sibling {
			t.Fatal("unrelated sibling was lost/reordered")
		}
		grouped = logical.Children()[0]
	}
	exclusion, ok := grouped.(*expressions.LogicalExpression)
	if !ok || exclusion.Operator() != foundation.Not {
		t.Fatalf("missing grouped exclusion: %s", grouped)
	}
	call, ok := exclusion.Children()[0].(*expressions.CallExpression)
	if !ok {
		t.Fatal("grouped exclusion is not a call")
	}
	metadata := must(call.Context().Metadata(call.Handle()))
	if name, present := metadata.BuiltInName().Get(); !present || name != functions.BuiltInArrayContainsAny {
		t.Fatal("wrong grouped family")
	}
	args := call.Arguments()
	if args[0] != fixture.input {
		t.Fatal("array operand changed")
	}
	literal := args[1].(*expressions.LiteralExpression).Value().(*testbedvalues.ArrayValue)
	if !literal.Type().EqualsTo(fixture.array) || !slices.Equal(literal.Elements(), positions) {
		t.Fatal("candidate positions, identity, full Type or order changed")
	}
	if reconstruction.MeasureExpression(result).NodeCount >= reconstruction.MeasureExpression(source).NodeCount {
		t.Fatal("group did not reduce expression nodes")
	}
	if !result.Type().EqualsTo(source.Type()) {
		t.Fatal("group changed result Type")
	}
	return source, result
}

func TestIntegerGroupingAgainstIndependentOracle(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	type clause struct {
		scalar bool
		items  []int64
	}
	patterns := []struct {
		name    string
		clauses []clause
	}{
		{"scalar pair", []clause{{true, []int64{-1}}, {true, []int64{1}}}},
		{"duplicate scalars", []clause{{true, []int64{1}}, {true, []int64{1}}}},
		{"mixed lists", []clause{{true, []int64{0}}, {false, []int64{-1, 1, 1}}, {true, []int64{0}}}},
		{"lists", []clause{{false, []int64{1, 1}}, {false, []int64{-1, 0, -1}}}},
		{"extremes", []clause{{true, []int64{math.MinInt64}}, {false, []int64{math.MaxInt64, 0, math.MinInt64}}}},
	}
	// Exhaust all arrays of length <= 2 over {-1,0,1}, plus extrema and misses.
	rows := [][]int64{nil}
	for _, x := range []int64{-1, 0, 1} {
		rows = append(rows, []int64{x})
		for _, y := range []int64{-1, 0, 1} {
			rows = append(rows, []int64{x, y})
		}
	}
	rows = append(rows, []int64{math.MinInt64}, []int64{math.MaxInt64}, []int64{math.MinInt64, math.MaxInt64}, []int64{42, 42})
	for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		for _, pattern := range patterns {
			for _, siblingTruth := range []oracleTruth{"absent", oracleTrue, oracleFalse, oracleNull} {
				t.Run(fmt.Sprintf("%v/%s/sibling=%s", n, pattern.name, siblingTruth), func(t *testing.T) {
					fixture := newMembershipFixture(must(testbed.NewStandardContext()), integer, n)
					var clauses []semanticClause
					var oracleClauses [][]int64
					for _, clause := range pattern.clauses {
						items := make([]valueinterfaces.Value, len(clause.items))
						for i, item := range clause.items {
							items[i] = must(testbedvalues.NewInteger(integer, item))
						}
						clauses = append(clauses, semanticClause{clause.scalar, items})
						oracleClauses = append(oracleClauses, clause.items)
					}
					var sibling *expressions.ReferenceExpression
					var siblingValue valueinterfaces.Value
					resultNullability := n
					if siblingTruth != "absent" {
						boolean := must(types.NewBoolType(foundation.MayNull))
						sibling = fixtureReference("flag", boolean)
						if siblingTruth == oracleNull {
							siblingValue = must(testbedvalues.NewNull(boolean))
						} else {
							siblingValue = must(testbedvalues.NewBool(boolean, siblingTruth == oracleTrue))
						}
						resultNullability = foundation.MayNull
					}
					source, result := rewriteSemanticGroup(t, fixture, clauses, sibling)
					expectedType := must(types.NewBoolType(resultNullability))
					count := len(rows)
					if n == foundation.MayNull {
						count++
					}
					for row := 0; row < count; row++ {
						isNull := row == len(rows)
						var raw []int64
						var array valueinterfaces.Value
						if isNull {
							array = must(testbedvalues.NewNull(fixture.input.Type()))
						} else {
							raw = rows[row]
							var items []valueinterfaces.Value
							for _, item := range raw {
								items = append(items, must(testbedvalues.NewInteger(integer, item)))
							}
							array = fixture.inputValue(items...)
						}
						expected := integerExclusionOracle(raw, isNull, oracleClauses)
						bindings := []fixtureBinding{{fixture.input.ID(), array}}
						if sibling != nil {
							expected = oracleAnd(expected, siblingTruth)
							bindings = append(bindings, fixtureBinding{sibling.ID(), siblingValue})
						}
						label := fmt.Sprintf("row=%v/null=%t", raw, isNull)
						verifyOracleResult(t, source, bindings, expectedType, expected, "source/"+label)
						verifyOracleResult(t, result, bindings, expectedType, expected, "result/"+label)
					}
				})
			}
		}
	}
}

// Construct concrete nonmatching payloads rather than using the provider's
// equality callback to predict whether a row should match.
func genericMissValue(typ types.Type) valueinterfaces.Value {
	switch typed := typ.(type) {
	case *types.BoolType:
		return must(testbedvalues.NewBool(typed, false))
	case *types.IntType:
		return must(testbedvalues.NewInteger(typed, -9))
	case *types.FloatType:
		return must(testbedvalues.NewFloat(typed, 7))
	case *types.VarCharType:
		return must(testbedvalues.NewVarChar(typed, "other"))
	case *types.ArrayType:
		return must(testbedvalues.NewArray(typed, []valueinterfaces.Value{genericMissValue(typed.Element())}))
	case *types.StructType:
		var fields []valueinterfaces.Value
		for _, field := range typed.Fields() {
			fields = append(fields, genericMissValue(field.Type))
		}
		return must(testbedvalues.NewStruct(typed, fields))
	default:
		panic("unsupported semantic fixture Type")
	}
}

func TestGenericGroupingAgainstDeclaredTruth(t *testing.T) {
	elements := fixtureElements()
	integer := must(types.NewIntType(types.Int32, foundation.NonNull))
	array := must(types.NewArrayType(integer, foundation.NonNull))
	empty := must(testbedvalues.NewArray(array, nil))
	elements = append(elements, elementFixture{"empty array element", empty, empty, true})
	structure := must(types.NewStructType([]types.StructField{{Name: "tags", Type: array}}, foundation.NonNull))
	nested := must(types.NewArrayType(structure, foundation.NonNull))
	deep := must(testbedvalues.NewArray(nested, []valueinterfaces.Value{must(testbedvalues.NewStruct(structure, []valueinterfaces.Value{must(testbedvalues.NewArray(array, []valueinterfaces.Value{must(testbedvalues.NewInteger(integer, 8))}))}))}))
	elements = append(elements, elementFixture{"array of struct of array", deep, deep, true})
	for _, element := range elements {
		for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			t.Run(fmt.Sprintf("%s/%v", element.name, n), func(t *testing.T) {
				context := executableMembershipContext(membershipCatalog(element.candidate.Type()), fixtureElementEqual)
				fixture := newMembershipFixture(context, element.candidate.Type(), n)
				// For signed zero this is [-0,+0,-0]; for NaN it retains all three NaNs.
				clauses := []semanticClause{{true, []valueinterfaces.Value{element.candidate}}, {false, []valueinterfaces.Value{element.matching, element.candidate}}}
				source, result := rewriteSemanticGroup(t, fixture, clauses, nil)
				miss := genericMissValue(element.candidate.Type())
				matchingTruth := oracleTrue
				if element.matches {
					matchingTruth = oracleFalse
				}
				rows := []struct {
					items []valueinterfaces.Value
					truth oracleTruth
				}{
					{nil, oracleTrue},
					{[]valueinterfaces.Value{element.matching}, matchingTruth},
					{[]valueinterfaces.Value{element.matching, element.matching}, matchingTruth},
					{[]valueinterfaces.Value{miss}, oracleTrue},
					{[]valueinterfaces.Value{miss, element.matching}, matchingTruth},
				}
				expectedType := must(types.NewBoolType(n))
				for i, row := range rows {
					bindings := []fixtureBinding{{fixture.input.ID(), fixture.inputValue(row.items...)}}
					verifyOracleResult(t, source, bindings, expectedType, row.truth, fmt.Sprintf("source row %d", i))
					verifyOracleResult(t, result, bindings, expectedType, row.truth, fmt.Sprintf("result row %d", i))
				}
				if n == foundation.MayNull {
					bindings := []fixtureBinding{{fixture.input.ID(), must(testbedvalues.NewNull(fixture.input.Type()))}}
					verifyOracleResult(t, source, bindings, expectedType, oracleNull, "source NULL")
					verifyOracleResult(t, result, bindings, expectedType, oracleNull, "result NULL")
				}
			})
		}
	}
}
