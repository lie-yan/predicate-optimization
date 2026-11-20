package functions

import (
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/internal/assertions"

	shared "github.com/lie-yan/predicate-optimization/functions"
)

// Context inspects, resolves and evaluates functions in an immutable catalog.
// Multiple contexts may share one catalog and accept its handles. Contexts for
// independently created catalogs reject each other's handles.
// Use NewContext; the zero value is invalid. Inspection is safe for concurrent
// readers under the catalog's immutability contract. Concurrent evaluation also
// requires thread-safe evaluator callbacks.
type Context struct {
	catalog    *Catalog
	entries    map[*entry]struct{}
	evaluation map[*entry]EvaluationBinding
}

var _ functioninterfaces.FunctionContext = (*Context)(nil)

// NewContext creates an inspection context for an existing immutable catalog.
func NewContext(catalog *Catalog) *Context {
	if catalog == nil || catalog.identity == nil {
		panic("testbed/functions: expected constructed Catalog")
	}
	entries := make(map[*entry]struct{}, len(catalog.entries))
	for _, definition := range catalog.entries {
		entries[definition] = struct{}{}
	}
	return &Context{catalog: catalog, entries: entries}
}

func (c *Context) Metadata(handle functioninterfaces.FunctionHandle) (*shared.FunctionMetadata, error) {
	definition, err := c.resolveEntry(handle)
	if err != nil {
		return nil, err
	}
	return definition.metadata, nil
}

func (c *Context) IsArithmeticFunction(handle functioninterfaces.FunctionHandle) (bool, error) {
	metadata, err := c.Metadata(handle)
	if err != nil {
		return false, err
	}
	operator, present := metadata.Operator().Get()
	return present && operator.IsArithmeticOperator(), nil
}

func (c *Context) IsComparisonFunction(handle functioninterfaces.FunctionHandle) (bool, error) {
	metadata, err := c.Metadata(handle)
	if err != nil {
		return false, err
	}
	operator, present := metadata.Operator().Get()
	return present && operator.IsComparisonOperator(), nil
}

func (c *Context) resolveEntry(handle functioninterfaces.FunctionHandle) (*entry, error) {
	c.assertValid()
	assertions.NotNil(handle)
	h, ok := handle.(*Handle)
	if !ok || h.identity != c.catalog.identity {
		return nil, shared.ErrIncompatibleHandle
	}
	if _, exists := c.entries[h.entry]; !exists {
		return nil, shared.ErrUnknownHandle
	}
	return h.entry, nil
}

func (c *Context) assertValid() {
	if c == nil || c.catalog == nil || c.catalog.identity == nil {
		panic("testbed/functions: expected constructed Context")
	}
}
