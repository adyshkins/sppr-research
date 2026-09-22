package cvar

import (
	"math"
	"testing"
)

func assertFloatEqual(t *testing.T, got float64, want float64) {
	t.Helper()

	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
