package expressions

import (
	"fmt"

	shared "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// OpaqueID uses a nonempty owner/token pair as immutable testbed identity.
// Owner names are assigned by the caller; equal pairs intentionally identify
// the same region, even across separately constructed instances.
type OpaqueID struct {
	owner string
	token string
	key   string
}

var _ shared.OpaqueID = (*OpaqueID)(nil)

func NewOpaqueID(owner, token string) (*OpaqueID, error) {
	if owner == "" || token == "" {
		return nil, fmt.Errorf("region owner and token must be nonempty")
	}
	return &OpaqueID{owner: owner, token: token, key: fmt.Sprintf("github.com/lie-yan/predicate-optimization/testbed/opaque:%q:%q", owner, token)}, nil
}

func (id *OpaqueID) EqualsTo(other shared.OpaqueID) bool {
	id.assertValid()
	assertions.NotNil(other)
	return id.key == other.Key()
}

func (id *OpaqueID) String() string {
	id.assertValid()
	return fmt.Sprintf("OpaqueID(owner=%q, token=%q)", id.owner, id.token)
}

func (id *OpaqueID) assertValid() {
	if id == nil || id.owner == "" || id.token == "" {
		panic("testbed/expressions: expected constructed OpaqueID")
	}
}

// Key returns the precomputed, domain-qualified canonical identity.
func (id *OpaqueID) Key() string {
	id.assertValid()
	return id.key
}
