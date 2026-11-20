//go:build !debug

package assertions

// Enabled reports whether the debug build tag enables precondition assertions.
const Enabled = false

// NotNil is a no-op without the debug build tag. Arguments are still evaluated.
func NotNil(input any) {}

// NotNilPointer is a no-op without the debug build tag.
func NotNilPointer[T any](input *T) {}
