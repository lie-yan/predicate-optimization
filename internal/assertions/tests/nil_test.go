package assertions_test

import (
	"testing"
	"unsafe"

	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

type (
	nilFixture     interface{ Probe() }
	pointerFixture struct{}
)

func (*pointerFixture) Probe() { panic("nil detection must not invoke user methods") }

type mapFixture map[string]int

func (mapFixture) Probe() { panic("nil detection must not invoke user methods") }

func TestNilValues(t *testing.T) {
	var pointer *pointerFixture
	var wrapped nilFixture = pointer
	var wrappedMap nilFixture = mapFixture(nil)
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"interface", nil},
		{"pointer", pointer},
		{"pointer interface", wrapped},
		{"map interface", wrappedMap},
		{"map", map[string]int(nil)},
		{"slice", []int(nil)},
		{"function", (func())(nil)},
		{"channel", (chan int)(nil)},
		{"receive channel", (<-chan int)(nil)},
		{"unsafe pointer", unsafe.Pointer(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !assertions.IsNil(tc.value) {
				t.Fatal("typed nil not detected")
			}
		})
	}
}

func TestNonNilValues(t *testing.T) {
	pointer := &pointerFixture{}
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"pointer", pointer},
		{"interface", nilFixture(pointer)},
		{"empty map", mapFixture{}},
		{"empty slice", []int{}},
		{"function", func() { panic("must not call") }},
		{"channel", make(chan int)},
		{"unsafe pointer", unsafe.Pointer(pointer)},
		{"integer zero", 0},
		{"false", false},
		{"empty string", ""},
		{"zero struct", struct{}{}},
		{"zero array", [0]int{}},
		{"nil field", struct{ Value any }{nil}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if assertions.IsNil(tc.value) {
				t.Fatal("non-nil value rejected")
			}
			assertions.NotNil(tc.value)
			if allocations := testing.AllocsPerRun(100, func() { assertions.NotNil(tc.value) }); allocations != 0 {
				t.Fatalf("nil check allocated %g times", allocations)
			}
		})
	}
}

func TestNotNilPointerChecksOnlyPointer(t *testing.T) {
	pointer := &pointerFixture{}
	assertions.NotNilPointer(pointer)
	var nilInterface nilFixture
	var typedNilInterface nilFixture = (*pointerFixture)(nil)
	var nilSlice []int
	var nilMap map[string]int
	var nilPointer *pointerFixture
	assertions.NotNilPointer(&nilInterface)
	assertions.NotNilPointer(&typedNilInterface)
	assertions.NotNilPointer(&nilSlice)
	assertions.NotNilPointer(&nilMap)
	assertions.NotNilPointer(&nilPointer)
	if allocations := testing.AllocsPerRun(100, func() { assertions.NotNilPointer(pointer) }); allocations != 0 {
		t.Fatalf("pointer check allocated %g times", allocations)
	}
}
