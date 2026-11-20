package foundation

// OperatorType identifies a scalar operator for analysis and function lookup.
// Zero is invalid. Classification alone does not establish rewrite safety.
type OperatorType uint8

const (
	Add OperatorType = iota + 1
	Subtract
	Multiply
	Divide
	Modulus
	Negate

	Equal
	NotEqual
	LessThan
	LessThanOrEqual
	GreaterThan
	GreaterThanOrEqual
)

// IsArithmeticOperator reports whether the operator is arithmetic,
// including unary negation.
func (o OperatorType) IsArithmeticOperator() bool {
	switch o {
	case Add, Subtract, Multiply, Divide, Modulus, Negate:
		return true
	default:
		return false
	}
}

// IsComparisonOperator reports whether the operator compares two values.
func (o OperatorType) IsComparisonOperator() bool {
	switch o {
	case Equal, NotEqual, LessThan, LessThanOrEqual, GreaterThan, GreaterThanOrEqual:
		return true
	default:
		return false
	}
}
