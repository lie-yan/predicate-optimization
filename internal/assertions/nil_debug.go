//go:build debug

package assertions

// Enabled reports whether the debug build tag enables precondition assertions.
const Enabled = true

// NotNil panics for a nil interface or a typed nil value. It does not validate
// construction state, inspect children, or treat empty non-nil containers as nil.
func NotNil(input any) {
	if IsNil(input) {
		panic("assertions: expected non-nil input")
	}
}

// NotNilPointer checks a statically known pointer without reflection. It checks
// only the pointer itself, even if the pointed-to value is a nil interface.
func NotNilPointer[T any](input *T) {
	if input == nil {
		panic("assertions: expected non-nil input")
	}
}
