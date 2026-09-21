package utils

import "testing"

func TestLogLevelStringHandlesUnknownValues(t *testing.T) {
	for _, level := range []LogLevel{-1, 99} {
		if got := level.String(); got == "" {
			t.Fatalf("String() returned empty value for %d", level)
		}
	}
}
