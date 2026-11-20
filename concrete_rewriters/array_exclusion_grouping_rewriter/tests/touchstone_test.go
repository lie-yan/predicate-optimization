package array_exclusion_grouping_rewriter_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	fixture "github.com/lie-yan/predicate-optimization/testbed/touchstone"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

type groupingTouchstoneRow struct {
	article                 string
	review, brand           int64
	tags, categories, malls []int64
	// Article, review, brand, tags, categories, malls; nil arrays are empty.
	nulls [6]bool
}

func (r groupingTouchstoneRow) environment(f *fixture.Touchstone) *fixtureEnvironment {
	c := f.Columns
	refs := []*expressions.ReferenceExpression{c.ArticleType, c.IsReview, c.BrandID, c.TagIDs, c.CategoryIDs, c.MallIDs}
	env := &fixtureEnvironment{}
	for i, ref := range refs {
		var value valueinterfaces.Value
		if r.nulls[i] {
			value = must(testbedvalues.NewNull(ref.Type()))
		} else {
			switch i {
			case 0:
				value = must(testbedvalues.NewVarChar(ref.Type().(*types.VarCharType), r.article))
			case 1, 2:
				value = must(testbedvalues.NewInteger(ref.Type().(*types.IntType), []int64{r.review, r.brand}[i-1]))
			default:
				typ := ref.Type().(*types.ArrayType)
				var items []valueinterfaces.Value
				for _, n := range [][]int64{r.tags, r.categories, r.malls}[i-3] {
					items = append(items, must(testbedvalues.NewInteger(typ.Element().(*types.IntType), n)))
				}
				value = must(testbedvalues.NewArray(typ, items))
			}
		}
		env.bindings = append(env.bindings, fixtureBinding{ref.ID(), value})
	}
	return env
}

func groupingTouchstoneRows(nullable bool) []groupingTouchstoneRow {
	var rows []groupingTouchstoneRow
	for _, article := range []string{"tsa_01", "tsa_02", "tsa_03", "tsa_05", "tsa_04", "tsa_06", "tsa_07", "tsa_08", "tsa_09", "tsa_10", "tsa_11", "tsa_12", "tsa_13", "tsa_14", "tsa_15", "unlisted", ""} {
		for _, tags := range [][]int64{nil, {42, 42}, {8791001, 8791002, 8791001}, {8791003}, {8791004}, {8791005}, {8791006}, {8791007}, {8791008}, {8791009}, {8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009}} {
			rows = append(rows, groupingTouchstoneRow{review: 8795001, article: article, tags: tags})
		}
		for _, review := range []int64{-1, 8795001, 8795002, 8795003, 3, 8795004, 8795005, 1001} {
			rows = append(rows, groupingTouchstoneRow{article: article, review: review})
		}
		for _, brand := range []int64{-1, 8794001, 8794002, 8794003, 8794004, 8794005, 8794006, 8794007, 8794008, 8794009, 8794010, 8794011} {
			rows = append(rows, groupingTouchstoneRow{review: 8795001, article: article, brand: brand})
		}
		for _, categories := range [][]int64{nil, {42, 42}, {8792001, 8792001}} {
			for _, malls := range [][]int64{nil, {42, 42}, {8793001, 8793001}} {
				rows = append(rows, groupingTouchstoneRow{review: 8795001, article: article, categories: categories, malls: malls})
			}
		}
		if nullable {
			for mask := 1; mask < 32; mask++ {
				row := groupingTouchstoneRow{review: 8795001, article: article}
				for i := 1; i < 6; i++ {
					row.nulls[i] = mask&(1<<(i-1)) != 0
				}
				rows = append(rows, row)
				row.tags, row.categories, row.malls = []int64{8791004}, []int64{8792001}, []int64{8793001}
				rows = append(rows, row)
			}
		}
	}
	return rows
}

// Build the nullable-schema expected result solely from columns and constants.
// Conditional clauses remain; only the three unconditional tag exclusions group.
func expectedNullableGroupedTouchstone(f *fixture.Touchstone) expressions.Expression {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	number := func(n int64) expressions.Expression {
		return fixtureLiteral(must(testbedvalues.NewInteger(integer, n)))
	}
	word := func(s string) expressions.Expression { return fixtureLiteral(must(testbedvalues.NewVarChar(text, s))) }
	in := func(ref *expressions.ReferenceExpression, items []expressions.Expression) expressions.Expression {
		return must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, must(types.NewBoolType(ref.Type().Nullability())), append([]expressions.Expression{ref}, items...)))
	}
	numbers := func(ns ...int64) []expressions.Expression {
		result := make([]expressions.Expression, len(ns))
		for i, n := range ns {
			result[i] = number(n)
		}
		return result
	}
	words := func(ss ...string) []expressions.Expression {
		result := make([]expressions.Expression, len(ss))
		for i, s := range ss {
			result[i] = word(s)
		}
		return result
	}
	call := func(name functions.BuiltInFunctionName, ref *expressions.ReferenceExpression, arg expressions.Expression) expressions.Expression {
		handle := must(f.Context.LookupBuiltInFunction(name, []types.Type{ref.Type(), arg.Type()}))
		return must(expressions.NewCallExpression(f.Context, handle, []expressions.Expression{ref, arg}))
	}
	any := func(ref *expressions.ReferenceExpression, ns ...int64) expressions.Expression {
		var items []valueinterfaces.Value
		for _, n := range ns {
			items = append(items, must(testbedvalues.NewInteger(integer, n)))
		}
		literal := fixtureLiteral(must(testbedvalues.NewArray(must(types.NewArrayType(integer, foundation.NonNull)), items)))
		return call(functions.BuiltInArrayContainsAny, ref, literal)
	}
	not := func(node expressions.Expression) expressions.Expression { return fixtureLogical(foundation.Not, node) }
	c := f.Columns
	common := word("tsa_04")
	equality := must(f.Context.ComparisonFunction(foundation.Equal, c.ArticleType.Type(), common.Type()))
	conditional := func(tag int64, articles []expressions.Expression) expressions.Expression {
		return not(fixtureLogical(foundation.And, call(functions.BuiltInArrayContains, c.TagIDs, number(tag)), in(c.ArticleType, articles)))
	}
	return fixtureLogical(foundation.And,
		in(c.ArticleType, words("tsa_01", "tsa_02", "tsa_03")),
		not(fixtureLogical(foundation.And, any(c.TagIDs, 8791001, 8791002), must(expressions.NewCallExpression(f.Context, equality, []expressions.Expression{c.ArticleType, common})))),
		in(c.IsReview, numbers(8795001, 8795002, 8795003, 8795004, 8795005)),
		not(call(functions.BuiltInArrayContains, c.CategoryIDs, number(8792001))),
		not(in(c.BrandID, numbers(8794001, 8794002, 8794003, 8794004, 8794005, 8794006, 8794007, 8794008, 8794009, 8794010, 8794011))),
		not(call(functions.BuiltInArrayContains, c.MallIDs, number(8793001))),
		not(any(c.TagIDs, 8791003, 8791006, 8791009)),
		conditional(8791004, words("tsa_01", "tsa_05", "tsa_03", "tsa_04", "tsa_06", "tsa_02", "tsa_07", "tsa_08", "tsa_09", "tsa_10", "tsa_11", "tsa_12", "tsa_13", "tsa_14", "tsa_15")),
		conditional(8791005, words("tsa_03", "tsa_02", "tsa_01", "tsa_04")),
		conditional(8791007, words("tsa_03", "tsa_02", "tsa_01", "tsa_04")),
		conditional(8791008, words("tsa_03", "tsa_02", "tsa_01", "tsa_04")),
	)
}

func oracleNot(truth oracleTruth) oracleTruth {
	if truth == oracleNull {
		return oracleNull
	}
	if truth == oracleTrue {
		return oracleFalse
	}
	return oracleTrue
}

func oracleMembership[T comparable](value T, null bool, candidates []T) oracleTruth {
	if null {
		return oracleNull
	}
	if slices.Contains(candidates, value) {
		return oracleTrue
	}
	return oracleFalse
}

// Evaluate the compact input's clauses using plain strings/integers and the
// independent three-valued operations, not expression traversal or a provider.
func (r groupingTouchstoneRow) truth() oracleTruth {
	article := func(ss ...string) oracleTruth { return oracleMembership(r.article, r.nulls[0], ss) }
	contains := func(array []int64, null bool, ns ...int64) oracleTruth {
		return oracleNot(integerExclusionOracle(array, null, [][]int64{ns}))
	}
	tags := func(ns ...int64) oracleTruth { return contains(r.tags, r.nulls[3], ns...) }
	result := article("tsa_01", "tsa_02", "tsa_03")
	clauses := []oracleTruth{
		oracleNot(oracleAnd(tags(8791001, 8791002), article("tsa_04"))),
		oracleMembership(r.review, r.nulls[1], []int64{8795001, 8795002, 8795003, 8795004, 8795005}),
		oracleNot(contains(r.categories, r.nulls[4], 8792001)),
		oracleNot(oracleMembership(r.brand, r.nulls[2], []int64{8794001, 8794002, 8794003, 8794004, 8794005, 8794006, 8794007, 8794008, 8794009, 8794010, 8794011})),
		oracleNot(contains(r.malls, r.nulls[5], 8793001)),
		oracleNot(tags(8791003)),
		oracleNot(oracleAnd(tags(8791004), article("tsa_01", "tsa_05", "tsa_03", "tsa_04", "tsa_06", "tsa_02", "tsa_07", "tsa_08", "tsa_09", "tsa_10", "tsa_11", "tsa_12", "tsa_13", "tsa_14", "tsa_15"))),
		oracleNot(oracleAnd(tags(8791005), article("tsa_03", "tsa_02", "tsa_01", "tsa_04"))),
		oracleNot(tags(8791006)),
		oracleNot(oracleAnd(tags(8791007), article("tsa_03", "tsa_02", "tsa_01", "tsa_04"))),
		oracleNot(oracleAnd(tags(8791008), article("tsa_03", "tsa_02", "tsa_01", "tsa_04"))),
		oracleNot(tags(8791009)),
	}
	for _, clause := range clauses {
		result = oracleAnd(result, clause)
	}
	return result
}

func assertGroupedTouchstoneCalls(t *testing.T, node expressions.Expression, f *fixture.Touchstone, nullableArticle bool) {
	t.Helper()
	// Expected call families/payloads in source occurrence order. This also
	// distinguishes the unconditional group from each remaining conditional test.
	type membershipExpectation struct {
		ref        *expressions.ReferenceExpression
		family     functions.BuiltInFunctionName
		candidates []int64
	}
	want := []membershipExpectation{
		{f.Columns.CategoryIDs, functions.BuiltInArrayContains, []int64{8792001}},
		{f.Columns.MallIDs, functions.BuiltInArrayContains, []int64{8793001}},
		{f.Columns.TagIDs, functions.BuiltInArrayContainsAny, []int64{8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009}},
	}
	if nullableArticle {
		want[2].candidates = []int64{8791003, 8791006, 8791009}
		want = append([]membershipExpectation{{f.Columns.TagIDs, functions.BuiltInArrayContainsAny, []int64{8791001, 8791002}}}, want...)
		for _, tag := range []int64{8791004, 8791005, 8791007, 8791008} {
			want = append(want, membershipExpectation{f.Columns.TagIDs, functions.BuiltInArrayContains, []int64{tag}})
		}
	}
	index := 0
	for _, node := range occurrenceOrder(node) {
		call, ok := node.(*expressions.CallExpression)
		if !ok {
			continue
		}
		name, present := must(call.Context().Metadata(call.Handle())).BuiltInName().Get()
		if !present || name != functions.BuiltInArrayContains && name != functions.BuiltInArrayContainsAny {
			continue
		}
		if index >= len(want) {
			t.Fatal("unexpected extra membership call")
		}
		args := call.Arguments()
		var items []valueinterfaces.Value
		if name == functions.BuiltInArrayContains {
			items = []valueinterfaces.Value{args[1].(*expressions.LiteralExpression).Value()}
		} else {
			items = args[1].(*expressions.LiteralExpression).Value().(*testbedvalues.ArrayValue).Elements()
		}
		var numbers []int64
		for _, item := range items {
			numbers = append(numbers, item.(*testbedvalues.IntegerValue).Integer())
		}
		expected := want[index]
		if args[0] != expected.ref || name != expected.family || !slices.Equal(numbers, expected.candidates) {
			t.Fatalf("membership %d: %s %v", index, name, numbers)
		}
		index++
	}
	if index != len(want) {
		t.Fatalf("membership count=%d, want %d", index, len(want))
	}
}

func TestTouchstoneGroupingSchedules(t *testing.T) {
	for _, articleNullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		for _, otherNullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			f := must(fixture.NewTouchstone(articleNullability, otherNullability))
			expected := f.Expected
			wantSize := reconstruction.ExpressionSize{NodeCount: 39, LiteralOccurrences: 22, MembershipEntries: 19}
			if articleNullability == foundation.MayNull {
				expected = expectedNullableGroupedTouchstone(f)
				wantSize = reconstruction.ExpressionSize{NodeCount: 102, LiteralOccurrences: 55, MembershipEntries: 46}
			}
			rows := groupingTouchstoneRows(otherNullability == foundation.MayNull)
			if articleNullability == foundation.MayNull {
				masks := 1
				if otherNullability == foundation.MayNull {
					masks = 32
				}
				for mask := 0; mask < masks; mask++ {
					for _, tags := range [][]int64{nil, {8791001, 8791002}, {8791004}, {8791005}, {8791007}, {8791008}, {8791003}, {8791006}, {8791009}, {8791004, 8791003}} {
						row := groupingTouchstoneRow{review: 8795001, tags: tags, nulls: [6]bool{true}}
						for i := 1; i < 6; i++ {
							row.nulls[i] = mask&(1<<(i-1)) != 0
						}
						rows = append(rows, row)
					}
				}
			}
			var previous expressions.Expression
			for _, first := range []bool{true, false} {
				t.Run(fmt.Sprintf("article=%v/others=%v/normalize-first=%t", articleNullability, otherNullability, first), func(t *testing.T) {
					stages := groupingCompositionStages(t, f.Input, f.Context, first)
					result := stages[len(stages)-1]
					if !result.EqualsTo(expected) {
						t.Fatalf("unexpected final tree\n%s\nwant\n%s", expressions.PrettyPrint(result), expressions.PrettyPrint(expected))
					}
					if previous != nil && !result.EqualsTo(previous) {
						t.Fatal("pass orders disagree")
					}
					previous = result
					if size := reconstruction.MeasureExpression(result); size != wantSize {
						t.Fatalf("size=%+v, want %+v", size, wantSize)
					}
					assertGroupedTouchstoneCalls(t, result, f, articleNullability == foundation.MayNull)
					if articleNullability == foundation.MayNull && (stages[1] != f.Input || stages[2] != f.Input || result.EqualsTo(f.Expected)) {
						t.Fatal("nullable article authorized unsafe scalar reduction")
					}
					evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, f.Context)
					checks := append(slices.Clone(stages), expected)
					resultNullability := foundation.NonNull
					if articleNullability == foundation.MayNull || otherNullability == foundation.MayNull {
						resultNullability = foundation.MayNull
					}
					boolean := must(types.NewBoolType(resultNullability))
					for rowIndex, row := range rows {
						oracle := partialBoolean(boolean, row.truth())
						for stage, node := range checks {
							actual := must(evaluator.Evaluate(node, row.environment(f)))
							if !actual.EqualsTo(oracle) || !actual.Type().EqualsTo(boolean) || !node.Type().EqualsTo(boolean) {
								t.Fatalf("row=%d/stage=%d changed truth/Type: %+v", rowIndex, stage, row)
							}
						}
					}
					if articleNullability == foundation.MayNull {
						row := groupingTouchstoneRow{review: 8795001, tags: []int64{8791004}, nulls: [6]bool{true}}
						for _, node := range checks {
							if actual := must(evaluator.Evaluate(node, row.environment(f))); !actual.IsNull() {
								t.Fatal("NULL article/tag-8791004 counterexample was erased")
							}
						}
						unsafe := must(evaluator.Evaluate(f.Expected, row.environment(f)))
						if unsafe.IsNull() || unsafe.(*testbedvalues.BoolValue).Bool() {
							t.Fatal("primary expected tree did not expose nullable counterexample")
						}
					}
					t.Logf("verified %d rows at all stages; final size %+v", len(rows), wantSize)
				})
			}
		}
	}
}
