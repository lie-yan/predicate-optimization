package validation_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/examples/internal/fixtures"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/testbed/touchstone"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// The independent fixture already constructs the documented grouped output.
// Keep its first five conjuncts and explicitly build the seven separate exclusions
// delivered by the current schedule; grouping is outside this validation fixture.
func validationExpectedTouchstone(f *touchstone.Touchstone) expressions.Expression {
	children := f.Expected.Children()[:5]
	integer := f.Columns.TagIDs.Type().(*types.ArrayType).Element().(*types.IntType)
	for _, n := range []int64{8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009} {
		value := fixtures.ScalarLiteral(fixtures.Must(testbedvalues.NewInteger(integer, n)))
		handle := fixtures.Must(f.Context.LookupBuiltInFunction(shared.BuiltInArrayContains, []types.Type{f.Columns.TagIDs.Type(), integer}))
		call := fixtures.Must(expressions.NewCallExpression(f.Context, handle, []expressions.Expression{f.Columns.TagIDs, value}))
		children = append(children, fixtures.ScalarLogical(foundation.Not, call))
	}
	return fixtures.ScalarLogical(foundation.And, children...)
}

// Each slot follows the fixture's complete declared Type. A nil array slice is
// an empty array; NULL is introduced only by the explicit bit mask.
func validationTouchstoneRow(f *touchstone.Touchstone, article string, review, brand int64, tags, categories, malls []int64, nulls int) fixtures.Environment {
	c := f.Columns
	refs := []*expressions.ReferenceExpression{c.ArticleType, c.IsReview, c.BrandID, c.TagIDs, c.CategoryIDs, c.MallIDs}
	var row fixtures.Environment
	for index, ref := range refs {
		var value valueinterfaces.Value
		switch {
		case nulls&(1<<index) != 0:
			value = fixtures.Must(testbedvalues.NewNull(ref.Type()))
		case index == 0:
			value = fixtures.Must(testbedvalues.NewVarChar(ref.Type().(*types.VarCharType), article))
		case index < 3:
			value = fixtures.Must(testbedvalues.NewInteger(ref.Type().(*types.IntType), []int64{review, brand}[index-1]))
		default:
			typ := ref.Type().(*types.ArrayType)
			var elements []valueinterfaces.Value
			for _, n := range [][]int64{tags, categories, malls}[index-3] {
				elements = append(elements, fixtures.Must(testbedvalues.NewInteger(typ.Element().(*types.IntType), n)))
			}
			value = fixtures.Must(testbedvalues.NewArray(typ, elements))
		}
		row = append(row, fixtures.Binding{ID: ref.ID(), Value: value})
	}
	return row
}

func TestValidationTouchstoneComposition(t *testing.T) {
	for _, nullable := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		t.Run(fmt.Sprint(nullable), func(t *testing.T) {
			f := fixtures.Must(touchstone.NewTouchstone(foundation.NonNull, nullable))
			context := &validationRecordingContext{FunctionContext: f.Context}
			input := instrumentValidationCalls(f.Input, context)
			before := expressions.PrettyPrint(input)
			context.queries = validationQueries{}
			if err := utils.ValidateForOptimization(input); err != nil {
				t.Fatal(err)
			}
			reportValidationQueries(t, context, "optimization entry", nil)
			stages := []expressions.Expression{input}
			names := []string{"contextual", "cleanup", "flattening", "normalization"}
			for index, pass := range validationPipeline(context) {
				result := fixtures.Must(pass.Rewrite(stages[len(stages)-1], struct{}{}))
				if !result.Type().EqualsTo(input.Type()) {
					t.Fatal("pass changed full Type")
				}
				t.Logf("%s size: %+v", names[index], reconstruction.MeasureExpression(result))
				reportValidationQueries(t, context, names[index], nil)
				stages = append(stages, result)
			}
			result := stages[len(stages)-1]
			if !result.EqualsTo(validationExpectedTouchstone(f)) {
				t.Fatal("unexpected composed tree")
			}
			if err := utils.ValidateForOptimization(result); err != nil {
				t.Fatal(err)
			}
			if expressions.PrettyPrint(input) != before {
				t.Fatal("pipeline mutated input")
			}
			t.Logf("composed output (%s):\n%s", result.Type(), expressions.PrettyPrint(result))

			// Exercise extraction and reconstruction explicitly on the touchstone
			// article guard, in addition to analysis within contextual rewriting.
			guard := input.Children()[0]
			summary := fixtures.ScalarSummary(context, guard)
			reconstructed, present := fixtures.Must(reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, context).Reconstruct(summary)).Get()
			if !present || !reconstructed.Type().EqualsTo(guard.Type()) {
				t.Fatal("guard reconstruction failed")
			}
			if err := utils.ValidateForOptimization(reconstructed); err != nil {
				t.Fatal(err)
			}
			evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, f.Context)
			rows := 0
			for _, article := range []string{"tsa_01", "tsa_02", "tsa_03", "tsa_04", "unlisted"} {
				masks := 1
				if nullable == foundation.MayNull {
					masks = 32
				}
				for mask := 0; mask < masks; mask++ {
					for scenario := 0; scenario < 14; scenario++ {
						review, brand := int64(8795001), int64(0)
						var tags, categories, malls []int64
						switch {
						case scenario < 7:
							n := []int64{8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009}[scenario]
							tags = []int64{42, n, n}
						case scenario == 7:
							tags = []int64{8791001, 8791002, 8791001}
						case scenario == 8:
							categories = []int64{8792001, 8792001}
						case scenario == 9:
							malls = []int64{8793001, 8793001}
						case scenario == 10:
							review = 3
						case scenario == 11:
							brand = 8794001
						case scenario == 12:
							tags, categories, malls = []int64{42, 42}, []int64{42}, []int64{42}
						}
						row := validationTouchstoneRow(f, article, review, brand, tags, categories, malls, mask<<1)
						want := fixtures.Must(evaluator.Evaluate(f.Expected, row))
						for index, stage := range stages {
							if actual := fixtures.Must(evaluator.Evaluate(stage, row)); !actual.EqualsTo(want) {
								t.Fatalf("stage %d article=%s mask=%d scenario=%d differs", index, article, mask, scenario)
							}
						}
						if !fixtures.Must(evaluator.Evaluate(guard, row)).EqualsTo(fixtures.Must(evaluator.Evaluate(reconstructed, row))) {
							t.Fatal("reconstructed guard changed meaning")
						}
						rows++
					}
				}
			}
			t.Logf("checked %d rows against independent expected expression, across %d stages", rows, len(stages))
		})
	}
}
