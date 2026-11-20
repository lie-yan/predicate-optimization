package foundation

// LogicalOperator identifies Boolean composition independently of scalar
// arithmetic/comparison OperatorType. The zero value is invalid.
type LogicalOperator uint8

const (
	Not LogicalOperator = iota + 1
	And
	Or
)

func (op LogicalOperator) IsValid() bool { return op == Not || op == And || op == Or }

func (op LogicalOperator) String() string {
	switch op {
	case Not:
		return "NOT"
	case And:
		return "AND"
	case Or:
		return "OR"
	default:
		return "INVALID"
	}
}
