package functions

// NullPolicy describes how a function handles a null argument at one position.
// Its zero value establishes no null-handling guarantee.
type NullPolicy uint8

const (
	NullPolicyUnspecified NullPolicy = iota
	// PropagateNull returns a typed null without executing the underlying function.
	PropagateNull
	// PassNull passes null to a parameter whose implementation type admits null.
	PassNull
)

// IsValid reports whether the policy is a defined state, including unspecified.
func (p NullPolicy) IsValid() bool {
	switch p {
	case NullPolicyUnspecified, PropagateNull, PassNull:
		return true
	default:
		return false
	}
}
