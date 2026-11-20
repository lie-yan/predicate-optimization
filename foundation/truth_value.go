// Package foundation defines basic types for predicate optimization.
package foundation

// TruthValue is the result of a predicate in three-valued logic.
// Its zero value is False. An indeterminate analysis result is represented
// separately, not by a TruthValue.
type TruthValue uint8

const (
	False TruthValue = iota
	True
	// Null is a known null Boolean result, corresponding to SQL UNKNOWN.
	Null
)
