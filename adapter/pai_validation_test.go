package adapter

import "testing"

func TestUint64FromAnyRejectsInvalidFloatIDs(t *testing.T) {
	for _, value := range []float64{-1, 1.5} {
		if _, ok := uint64FromAny(value); ok {
			t.Errorf("uint64FromAny(%v) accepted invalid ID", value)
		}
	}
	if got, ok := uint64FromAny(float64(42)); !ok || got != 42 {
		t.Fatalf("uint64FromAny(42) = %d, %v", got, ok)
	}
}
