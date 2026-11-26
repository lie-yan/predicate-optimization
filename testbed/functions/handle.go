package functions

import (
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/functions"
)

// Handle identifies a canonical entry within its catalog. It is immutable and
// constructed by Catalog. The zero value is invalid. Contexts must recognize
// both the representation and catalog identity, not just the exposed names.
type Handle struct {
	identity *catalogIdentity
	entry    *entry
}

var _ functioninterfaces.FunctionHandle = (*Handle)(nil)

func (h *Handle) Namespace() string {
	h.assertValid()
	return h.entry.namespace
}

func (h *Handle) Name() string {
	h.assertValid()
	return h.entry.name
}

// Shares the catalog's immutable implementation signature.
func (h *Handle) Signature() *functions.FunctionSignature {
	h.assertValid()
	return h.entry.metadata.Signature()
}

// EqualsTo compares catalog and canonical entry identity, not handle addresses.
// Separately allocated handles for one entry are equal. Other representations
// and catalogs are unequal. Nil operands violate the shared precondition.
func (h *Handle) EqualsTo(other functioninterfaces.FunctionHandle) bool {
	h.assertValid()
	assertions.NotNil(other)
	o, ok := other.(*Handle)
	if !ok {
		return false
	}
	o.assertValid()
	return h.identity == o.identity && h.entry == o.entry
}

func (h *Handle) assertValid() {
	if h == nil || h.identity == nil || h.entry == nil {
		panic("testbed/functions: expected constructed Handle")
	}
}
