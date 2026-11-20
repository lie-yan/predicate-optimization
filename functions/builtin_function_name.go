// Package functions defines shared function contracts for predicate optimization.
// Operator identities are defined by foundation.OperatorType.
package functions

// BuiltInFunctionName identifies a standard built-in semantic operation,
// independently of a caller's function namespace or display spelling.
// A name does not guarantee lookup or evaluation support for a given signature,
// and recognizing it alone does not establish that a rewrite is safe.
// Providers may define additional names. Metadata requires a non-empty name;
// lookup rejects names or signatures absent from the provider catalog.
// Typed operator names use <operand-type>.<operation>, without nullability.
type BuiltInFunctionName string

const (
	BuiltInNot              BuiltInFunctionName = "not"
	BuiltInArrayContains    BuiltInFunctionName = "array_contains"
	BuiltInArrayContainsAny BuiltInFunctionName = "array_contains_any"
	BuiltInArrayContainsAll BuiltInFunctionName = "array_contains_all"
	BuiltInArrayLength      BuiltInFunctionName = "array_length"
)

// Typed operator names currently supplied by the standard testbed.
const (
	BuiltInBoolEqual                 BuiltInFunctionName = "bool.equal"
	BuiltInInt64Negate               BuiltInFunctionName = "int64.negate"
	BuiltInInt64Add                  BuiltInFunctionName = "int64.add"
	BuiltInInt64Subtract             BuiltInFunctionName = "int64.subtract"
	BuiltInInt64Multiply             BuiltInFunctionName = "int64.multiply"
	BuiltInInt64Divide               BuiltInFunctionName = "int64.divide"
	BuiltInInt64Modulus              BuiltInFunctionName = "int64.modulus"
	BuiltInInt64Equal                BuiltInFunctionName = "int64.equal"
	BuiltInInt64NotEqual             BuiltInFunctionName = "int64.not_equal"
	BuiltInInt64LessThan             BuiltInFunctionName = "int64.less_than"
	BuiltInInt64LessThanOrEqual      BuiltInFunctionName = "int64.less_than_or_equal"
	BuiltInInt64GreaterThan          BuiltInFunctionName = "int64.greater_than"
	BuiltInInt64GreaterThanOrEqual   BuiltInFunctionName = "int64.greater_than_or_equal"
	BuiltInVarCharEqual              BuiltInFunctionName = "varchar.equal"
	BuiltInVarCharNotEqual           BuiltInFunctionName = "varchar.not_equal"
	BuiltInVarCharLessThan           BuiltInFunctionName = "varchar.less_than"
	BuiltInVarCharLessThanOrEqual    BuiltInFunctionName = "varchar.less_than_or_equal"
	BuiltInVarCharGreaterThan        BuiltInFunctionName = "varchar.greater_than"
	BuiltInVarCharGreaterThanOrEqual BuiltInFunctionName = "varchar.greater_than_or_equal"
)

const (
	BuiltInFloat32Equal              BuiltInFunctionName = "float32.equal"
	BuiltInFloat32NotEqual           BuiltInFunctionName = "float32.not_equal"
	BuiltInFloat32LessThan           BuiltInFunctionName = "float32.less_than"
	BuiltInFloat32LessThanOrEqual    BuiltInFunctionName = "float32.less_than_or_equal"
	BuiltInFloat32GreaterThan        BuiltInFunctionName = "float32.greater_than"
	BuiltInFloat32GreaterThanOrEqual BuiltInFunctionName = "float32.greater_than_or_equal"
	BuiltInFloat64Equal              BuiltInFunctionName = "float64.equal"
	BuiltInFloat64NotEqual           BuiltInFunctionName = "float64.not_equal"
	BuiltInFloat64LessThan           BuiltInFunctionName = "float64.less_than"
	BuiltInFloat64LessThanOrEqual    BuiltInFunctionName = "float64.less_than_or_equal"
	BuiltInFloat64GreaterThan        BuiltInFunctionName = "float64.greater_than"
	BuiltInFloat64GreaterThanOrEqual BuiltInFunctionName = "float64.greater_than_or_equal"
)
