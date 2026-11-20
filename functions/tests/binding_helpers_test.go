package functions_test

func bindingMust[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
