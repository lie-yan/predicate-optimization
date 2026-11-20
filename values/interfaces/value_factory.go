package interfaces

import "github.com/lie-yan/predicate-optimization/types"

// ValueFactory constructs immutable values in a caller-selected representation.
// Types and existing children must be valid and non-nil, including no typed nils.
// Successful results are non-nil and have exactly the requested Type and content.
// Invalid new payloads return an error and a nil Value, with no implicit coercion.
// Factories must support all declared domains they are used to construct.
// Concurrent use requires an explicit guarantee from the implementation.
type ValueFactory interface {
	// NewNull requires a nullable Type, retaining its complete semantic identity.
	NewNull(types.Type) (Value, error)
	NewBool(*types.BoolType, bool) (Value, error)
	// NewInteger rejects payloads outside the declared signed width.
	NewInteger(*types.IntType, int64) (Value, error)
	// NewFloat requires exact Float32 representability, canonicalizes NaNs and
	// preserves infinities and signed zeros. No implicit rounding is performed.
	NewFloat(*types.FloatType, float64) (Value, error)
	// NewVarChar preserves bytes and enforces the declared byte bound.
	NewVarChar(*types.VarCharType, string) (Value, error)
	// NewArray preserves order, duplicates and null children. Child Types must
	// match exactly. The factory must not retain mutable input slice storage;
	// immutable children may be shared regardless of their concrete representation.
	NewArray(*types.ArrayType, []Value) (Value, error)
	// NewStruct preserves declared field order and requires exact field count
	// and child Types. Input slice ownership follows NewArray's contract.
	NewStruct(*types.StructType, []Value) (Value, error)
}
