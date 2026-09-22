package core

import (
	"math"
	"math/rand"
	"testing"
)

func closeTo(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-10*(1+math.Abs(want)) {
		t.Fatalf("got %.17g, want %.17g", got, want)
	}
}
func TestCVaRAtoms(t *testing.T) {
	tests := []struct {
		name                    string
		z, p                    []float64
		a, mean, varValue, cvar float64
	}{
		{"constant", []float64{4, 4}, []float64{.5, .5}, .95, 4, 4, 4},
		{"fractional_boundary", []float64{0, 10}, []float64{.9, .1}, .8, 1, 0, 5},
		{"at_boundary", []float64{0, 10}, []float64{.9, .1}, .9, 1, 0, 10},
		{"inside_atom", []float64{0, 10}, []float64{.9, .1}, .95, 1, 10, 10},
		{"zero_mass", []float64{999, 2}, []float64{0, 1}, .95, 2, 2, 2},
		{"unsorted", []float64{10, 0, 4}, []float64{.1, .5, .4}, .8, 2.6, 4, 7},
	}
	for _, v := range tests {
		t.Run(v.name, func(t *testing.T) {
			r, e := EvaluateRisk(v.z, v.p, v.a)
			if e != nil {
				t.Fatal(e)
			}
			closeTo(t, r.Expected, v.mean)
			closeTo(t, r.VaR, v.varValue)
			closeTo(t, r.CVaR, v.cvar)
		})
	}
}

// Independent slow reference: the minimum over support points of (3.67).
func etaReference(z, p []float64, a float64) float64 {
	best := math.Inf(1)
	for _, eta := range z {
		v := eta
		for i, x := range z {
			v += p[i] * math.Max(x-eta, 0) / (1 - a)
		}
		best = math.Min(best, v)
	}
	return best
}
func TestCVaRAgainstEtaMinimization(t *testing.T) {
	rng := rand.New(rand.NewSource(20260914)) // Test generation only, not research data.
	for k := 0; k < 500; k++ {
		n := 2 + rng.Intn(14)
		z := make([]float64, n)
		p := make([]float64, n)
		sum := 0.0
		for i := range z {
			z[i] = float64(rng.Intn(20))
			p[i] = float64(1 + rng.Intn(9))
			sum += p[i]
		}
		for i := range p {
			p[i] /= sum
		}
		alpha := []float64{.5, .8, .9, .95, .99}[k%5]
		got, err := EvaluateRisk(z, p, alpha)
		if err != nil {
			t.Fatal(err)
		}
		closeTo(t, got.CVaR, etaReference(z, p, alpha))
		if got.CVaR+1e-10 < got.VaR || got.CVaR+1e-10 < got.Expected {
			t.Fatal("risk ordering violated")
		}
	}
}
func TestRiskRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		z, p []float64
		a    float64
	}{
		{nil, nil, .95}, {[]float64{1}, []float64{.5}, .95}, {[]float64{1}, []float64{1}, 0},
		{[]float64{1}, []float64{1}, 1}, {[]float64{1}, []float64{-1}, .95},
		{[]float64{-1}, []float64{1}, .95}, {[]float64{math.NaN()}, []float64{1}, .95},
		{[]float64{1}, []float64{math.Inf(1)}, .95}, {[]float64{1, 2}, []float64{1}, .95},
	}
	for i, v := range tests {
		if _, err := EvaluateRisk(v.z, v.p, v.a); err == nil {
			t.Fatalf("accepted invalid input %d", i)
		}
	}
}
func TestCostIsCountedOnce(t *testing.T) {
	z, err := HorizonLoss([]float64{1, 1, 1}, .5, 2)
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, z, 3.75)
	r, err := EvaluateRisk([]float64{z, z}, []float64{.5, .5}, .95)
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, .65*r.Expected+.35*r.CVaR, 3.75)
}
func TestRiskTranslation(t *testing.T) {
	a, _ := EvaluateRisk([]float64{1, 4, 10}, []float64{.5, .4, .1}, .9)
	b, _ := EvaluateRisk([]float64{3, 6, 12}, []float64{.5, .4, .1}, .9)
	closeTo(t, b.Expected-a.Expected, 2)
	closeTo(t, b.VaR-a.VaR, 2)
	closeTo(t, b.CVaR-a.CVaR, 2)
}
func TestHorizonRejectsInvalidInputs(t *testing.T) {
	for _, v := range []struct {
		z    []float64
		g, c float64
	}{{nil, .97, 0}, {[]float64{1}, 0, 0}, {[]float64{1}, 2, 0}, {[]float64{1}, .97, -1}, {[]float64{-1}, .97, 0}, {[]float64{math.Inf(1)}, .97, 0}} {
		if _, err := HorizonLoss(v.z, v.g, v.c); err == nil {
			t.Fatal("accepted invalid horizon input")
		}
	}
}
