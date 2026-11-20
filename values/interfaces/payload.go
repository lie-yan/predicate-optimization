package interfaces

// Payload interfaces expose the existing logical domains without prescribing
// storage. Consumers must check IsNull before reading a payload, dispatch by
// Type, and then require the corresponding interface. Matching an accessor alone
// does not establish the semantic domain. Known nulls need no payload interface.

// BoolValue exposes the payload of a valid non-null Bool value.
type BoolValue interface {
	Value
	Bool() bool
}

// IntegerValue exposes a valid non-null integer fitting its declared width.
type IntegerValue interface {
	Value
	Integer() int64
}

// FloatValue exposes a valid non-null Float32 or Float64 in a float64 carrier.
// Finite Float32 payloads are exactly representable at their declared width.
// NaNs are canonicalized; infinities and signed zeros are preserved.
type FloatValue interface {
	Value
	Float() float64
}

// VarCharValue exposes the exact bytes of a valid non-null VarChar value.
type VarCharValue interface {
	Value
	VarChar() string
}

// ArrayValue exposes the ordered children of a valid non-null Array value.
type ArrayValue interface {
	Value
	// Elements returns an independent slice preserving order, duplicates and
	// null children. Immutable child values may be shared.
	Elements() []Value
	// ElementCount returns the number of children without copying them, including
	// duplicates and nulls.
	ElementCount() int
	// ElementAt returns the immutable child at index in the same order as Elements.
	// Index must be in [0, ElementCount()); an out-of-range index panics.
	ElementAt(index int) Value
}

// StructValue exposes the children of a valid non-null Struct value.
type StructValue interface {
	Value
	// Fields returns an independent slice in declared field order, preserving
	// null children. Immutable child values may be shared.
	Fields() []Value
}
