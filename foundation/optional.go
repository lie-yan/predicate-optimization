package foundation

// Optional holds a value that may be absent. Its zero value is absent.
// Presence is independent of the stored value, including zero or nil values.
// Copying an Optional copies its value; referenced data is not deep-copied.
type Optional[T any] struct {
	value   T
	present bool
}

// Some returns a present value, even when value is zero or nil.
func Some[T any](value T) Optional[T] {
	return Optional[T]{value: value, present: true}
}

// Get returns the value and whether it is present. An absent optional returns
// the zero value of T and false.
func (o Optional[T]) Get() (T, bool) {
	return o.value, o.present
}
