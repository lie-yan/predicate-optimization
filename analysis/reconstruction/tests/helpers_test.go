package reconstruction_test

import (
	"github.com/lie-yan/predicate-optimization/analysis/fact_extraction"
	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	testexpressions "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func reference(typ types.Type) *expressions.ReferenceExpression {
	return must(expressions.NewReferenceExpression(must(testexpressions.NewColumnID("t", "x")), typ, "x"))
}

func literal(v valueinterfaces.Value) *expressions.LiteralExpression {
	return must(expressions.NewLiteralExpression(v))
}

func logical(op foundation.LogicalOperator, args ...expressions.Expression) *expressions.LogicalExpression {
	return must(expressions.NewLogicalExpression(op, args))
}

func membership(ref *expressions.ReferenceExpression, entries ...valueinterfaces.Value) *expressions.SpecialFormExpression {
	args := []expressions.Expression{ref}
	for _, entry := range entries {
		args = append(args, literal(entry))
	}
	return must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, must(types.NewBoolType(ref.Type().Nullability())), args))
}

func summaryOf(context functioninterfaces.FunctionContext, source expressions.Expression) *facts.PredicateSummary {
	result := must(fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, context).Extract(source))
	summary, present := result.Summary().Get()
	if !present {
		panic("expected exact summary")
	}
	return summary
}

type scalarEnvironment struct{ value valueinterfaces.Value }

func (e scalarEnvironment) Lookup(expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	return foundation.Some(e.value), nil
}
