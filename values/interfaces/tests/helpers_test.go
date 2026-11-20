package interfaces_test

func fixtureValue[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
