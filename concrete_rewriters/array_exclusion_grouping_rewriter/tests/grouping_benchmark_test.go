package array_exclusion_grouping_rewriter_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Counters avoid the allocation/retention costs of recording event slices.
type benchmarkContext struct {
	functioninterfaces.FunctionContext
	metadata, lookups, evaluations int
}

func (c *benchmarkContext) Metadata(h functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	c.metadata++
	return c.FunctionContext.Metadata(h)
}

func (c *benchmarkContext) LookupBuiltInFunction(n functions.BuiltInFunctionName, args []types.Type) (functioninterfaces.FunctionHandle, error) {
	c.lookups++
	return c.FunctionContext.LookupBuiltInFunction(n, args)
}

func (c *benchmarkContext) Evaluate(h functioninterfaces.FunctionHandle, s *functions.FunctionSignature, args []valueinterfaces.Value) (valueinterfaces.Value, error) {
	c.evaluations++
	return c.FunctionContext.Evaluate(h, s, args)
}

var (
	groupingBenchmarkExpression expressions.Expression
	groupingBenchmarkValue      valueinterfaces.Value
)

func groupingWorkload(shape string, count, width int) (expressions.Expression, []*benchmarkContext, *membershipFixture) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	base := must(testbed.NewStandardContext())
	contexts := []*benchmarkContext{{FunctionContext: base}}
	fixture := newMembershipFixture(contexts[0], integer, foundation.NonNull)
	var children []expressions.Expression
	for i := 0; i < count; i++ {
		context := contexts[0]
		input := fixture.input
		switch shape {
		case "distinct":
			input = fixtureReference(fmt.Sprintf("column%d", i), fixture.input.Type())
		case "interleaved":
			input = fixtureReference(fmt.Sprintf("column%d", i%(count/2)), fixture.input.Type())
		case "providers":
			index := i % (count / 2)
			if index >= len(contexts) {
				contexts = append(contexts, &benchmarkContext{FunctionContext: base})
			}
			context = contexts[index]
		}
		f := newMembershipFixture(context, integer, foundation.NonNull)
		var items []valueinterfaces.Value
		for j := 0; j < width; j++ {
			items = append(items, must(testbedvalues.NewInteger(integer, int64(i*width+j))))
		}
		var call *expressions.CallExpression
		if width == 1 {
			call = f.contains(input, items[0])
		} else {
			call = f.containsAny(input, items...)
		}
		children = append(children, fixtureExclude(call))
	}
	return fixtureLogical(foundation.And, children...), contexts, fixture
}

func BenchmarkArrayGroupingRewrite(b *testing.B) {
	for _, shape := range []string{"compatible", "distinct", "interleaved", "providers", "lists"} {
		counts, widths := []int{128, 512, 2048}, []int{1}
		if shape == "lists" {
			counts, widths = []int{32}, []int{64, 1024, 8192}
		}
		for _, count := range counts {
			for _, width := range widths {
				b.Run(fmt.Sprintf("%s/n=%d/width=%d", shape, count, width), func(b *testing.B) {
					source, contexts, _ := groupingWorkload(shape, count, width)
					if err := utils.ValidateForOptimization(source); err != nil {
						b.Fatal(err)
					}
					tree := boundaryTree()
					warm := must(tree.Rewrite(source, struct{}{}))
					if (warm == source) != (shape == "distinct") {
						b.Fatal("unexpected workload grouping")
					}
					for _, context := range contexts {
						context.metadata, context.lookups, context.evaluations = 0, 0, 0
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						groupingBenchmarkExpression = must(tree.Rewrite(source, struct{}{}))
					}
					b.StopTimer()
					reads, lookups, evaluations := 0, 0, 0
					for _, context := range contexts {
						reads += context.metadata
						lookups += context.lookups
						evaluations += context.evaluations
					}
					groups := 1
					switch shape {
					case "distinct":
						groups = 0
					case "interleaved", "providers":
						groups = count / 2
					}
					if reads != b.N*(count+2*groups) || lookups != b.N*groups {
						b.Fatal("provider-query work changed")
					}
					b.ReportMetric(float64(reads)/float64(b.N), "metadata/op")
					b.ReportMetric(float64(lookups)/float64(b.N), "lookup/op")
					b.ReportMetric(float64(count*width), "candidates/op")
					if evaluations != 0 {
						b.Fatal("rewrite evaluated input")
					}
				})
			}
		}
	}
}

// Evaluation workloads bind exactly one column. Lookup returns its immutable
// Value without event logging or a linear search through fixture bindings.
type benchmarkEnvironment struct{ value valueinterfaces.Value }

func (e benchmarkEnvironment) Lookup(expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	return foundation.Some(e.value), nil
}

func BenchmarkArrayGroupingEvaluation(b *testing.B) {
	for _, spec := range []struct{ count, width int }{{16, 1}, {128, 1}, {512, 1}, {32, 64}, {32, 1024}} {
		for _, arrayLength := range []int{0, 32, 1024} {
			source, contexts, fixture := groupingWorkload("compatible", spec.count, spec.width)
			result := must(boundaryTree().Rewrite(source, struct{}{}))
			var items []valueinterfaces.Value
			integer := fixture.element.(*types.IntType)
			for i := 0; i < arrayLength; i++ {
				items = append(items, must(testbedvalues.NewInteger(integer, -int64(i)-1)))
			}
			environment := benchmarkEnvironment{fixture.inputValue(items...)}
			evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
			for _, node := range []expressions.Expression{source, result} {
				if !must(evaluator.Evaluate(node, environment)).(*testbedvalues.BoolValue).Bool() {
					b.Fatal("miss workload did not evaluate TRUE")
				}
			}
			for _, grouped := range []bool{false, true} {
				b.Run(fmt.Sprintf("n=%d/width=%d/array=%d/grouped=%t", spec.count, spec.width, arrayLength, grouped), func(b *testing.B) {
					node := source
					if grouped {
						node = result
					}
					contexts[0].evaluations = 0
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						groupingBenchmarkValue = must(evaluator.Evaluate(node, environment))
					}
					b.StopTimer()
					b.ReportMetric(float64(contexts[0].evaluations)/float64(b.N), "evaluate/op")
					b.ReportMetric(float64(spec.count*spec.width), "candidates/op")
				})
			}
		}
	}
}

// Capacity hints are based on candidate positions, not distinct testbedvalues. Keep a
// duplicate-heavy case alongside unique candidates to expose over-allocation.
func BenchmarkArrayContainsAnyDistribution(b *testing.B) {
	for _, count := range []int{1024, 32768} {
		for _, duplicate := range []bool{false, true} {
			b.Run(fmt.Sprintf("candidates=%d/duplicate=%t", count, duplicate), func(b *testing.B) {
				integer := must(types.NewIntType(types.Int64, foundation.NonNull))
				fixture := newMembershipFixture(must(testbed.NewStandardContext()), integer, foundation.NonNull)
				items := make([]valueinterfaces.Value, count)
				for i := range items {
					number := int64(i)
					if duplicate {
						number = 0
					}
					items[i] = must(testbedvalues.NewInteger(integer, number))
				}
				expression := fixture.containsAny(fixture.input, items...)
				environment := benchmarkEnvironment{fixture.inputValue(must(testbedvalues.NewInteger(integer, -1)))}
				evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
				if must(evaluator.Evaluate(expression, environment)).(*testbedvalues.BoolValue).Bool() {
					b.Fatal("expected membership miss")
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					groupingBenchmarkValue = must(evaluator.Evaluate(expression, environment))
				}
			})
		}
	}
}
