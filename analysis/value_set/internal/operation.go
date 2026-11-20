package internal

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Operation carries the original receiver's factory across package dispatch.
// It is internal to value_set; public set interfaces expose neither rebinding
// nor factory access. Run receives a shallow immutable view, never the source.
type Operation struct {
	Factory valueinterfaces.ValueFactory
	Run     func(value_set.ValueSet) (value_set.ValueSet, error)
}

// Apply preserves factory selection when an algorithm delegates to another set.
func Apply(factory valueinterfaces.ValueFactory, receiver value_set.ValueSet, run func(value_set.ValueSet) (value_set.ValueSet, error)) (value_set.ValueSet, error) {
	dispatcher, ok := receiver.(interface {
		ApplyOperation(Operation) (value_set.ValueSet, error)
	})
	if !ok {
		return nil, fmt.Errorf("%w: operation dispatch for %T", value_set.ErrUnsupported, receiver)
	}
	return dispatcher.ApplyOperation(Operation{Factory: factory, Run: run})
}
