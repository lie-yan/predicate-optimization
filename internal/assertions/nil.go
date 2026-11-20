// Package assertions provides internal checks for programmer preconditions.
package assertions

import "reflect"

// IsNil detects nil interfaces and typed nils without invoking user methods.
// Validation paths can use it to return an error instead of asserting.
func IsNil(input any) bool {
	if input == nil {
		return true
	}
	value := reflect.ValueOf(input)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return value.IsNil()
	default:
		return false
	}
}
