package randomstream

import (
	"testing"
)

func TestRepeat(t *testing.T) {
	a, e := New(42, "forecast", "one")
	if e != nil {
		t.Fatal(e)
	}
	b, _ := New(42, "forecast", "one")
	for i := 0; i < 100; i++ {
		if a.Int63() != b.Int63() {
			t.Fatal("not repeatable")
		}
	}
}
func TestSeparation(t *testing.T) {
	a, _ := New(42, "environment", "one")
	b, _ := New(42, "forecast", "one")
	if a.Int63() == b.Int63() {
		t.Fatal("streams coincide")
	}
}
func TestUnambiguousKeys(t *testing.T) {
	a, _ := New(1, "x", "ab", "c")
	b, _ := New(1, "x", "a", "bc")
	if a.Int63() == b.Int63() {
		t.Fatal("concatenation ambiguity")
	}
}
func TestNamespaceRequired(t *testing.T) {
	if _, e := New(1, ""); e == nil {
		t.Fatal("accepted unnamed stream")
	}
}
