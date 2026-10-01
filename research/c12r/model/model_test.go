package model

import (
	"math"
	"math/rand"
	"testing"
)

func TestNominalBalance(t *testing.T) {
	s := Initial()
	r := Step(s, Forcing{Pair{40, 25}, 100, 90}, Baseline())
	if r.Next != s || r.Loss != 0 {
		t.Fatalf("nominal %+v", r)
	}
}
func TestNonnegativeAndMaterialConservation(t *testing.T) {
	r := rand.New(rand.NewSource(88104))
	s := Initial()
	for k := 0; k < 10000; k++ {
		w := Forcing{Pair{80 * r.Float64(), 50 * r.Float64()}, 150 * r.Float64(), 150 * r.Float64()}
		a := Catalog(s, w.Demand)[r.Intn(8)]
		v := Step(s, w, a)
		if v.Loss < 0 || math.IsNaN(v.Loss) || v.Next.Raw < 0 {
			t.Fatal("invalid balance")
		}
		for i := 0; i < 2; i++ {
			if v.Next.Finished[i] < 0 || v.Next.Backlog[i] < 0 || v.Next.Finished[i]*v.Next.Backlog[i] > 1e-10 {
				t.Fatal("inventory complementarity")
			}
		}
		s = v.Next
	}
}
func TestCostExactlyOnce(t *testing.T) {
	s := Initial()
	a := Catalog(s, Pair{40, 25})[3]
	path := []Forcing{{Pair{40, 25}, 100, 90}, {Pair{40, 25}, 100, 90}, {Pair{40, 25}, 100, 90}}
	z := Rollout(s, path, a, 1)
	if z != a.Cost {
		t.Fatalf("got %g cost %g", z, a.Cost)
	}
}
