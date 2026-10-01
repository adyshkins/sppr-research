package cert

import (
	"math"
	"math/rand"
	"testing"
)

// An independent variational CVaR reference. It enumerates all candidate eta
// values, and does not use sorting or the weighted-tail implementation.
func variational(z []float64, den int) float64 {
	best := math.Inf(1)
	for _, eta := range z {
		v := eta
		for _, x := range z {
			v += float64(den) / float64(len(z)) * math.Max(x-eta, 0)
		}
		if v < best {
			best = v
		}
	}
	return best
}
func TestRiskAgainstVariational(t *testing.T) {
	r := rand.New(rand.NewSource(88101))
	for trial := 0; trial < 4000; trial++ {
		n := 1 + r.Intn(129)
		z := make([]float64, n)
		for i := range z {
			z[i] = float64(r.Intn(100)) / 7
		}
		den := 2 + r.Intn(30)
		cfg := Config{.35, true, 1, den, 8}
		v := Evaluate(z, cfg).Risk
		want := variational(z, den)
		if math.Abs(v-want) > 1e-10*math.Max(1, want) {
			t.Fatalf("n=%d den=%d got %g want %g", n, den, v, want)
		}
	}
}
func TestSelectorRandomMatrices(t *testing.T) {
	r := rand.New(rand.NewSource(88102))
	for trial := 0; trial < 12000; trial++ {
		n := []int{3, 8, 31, 64}[trial%4]
		m := 2 + r.Intn(11)
		c := make([]Candidate, m)
		z := make([][]float64, m)
		for j := range c {
			c[j] = Candidate{j, float64(r.Intn(5)) / 10, float64(r.Intn(3)) / 10, r.Float64() > .15}
			z[j] = make([]float64, n)
			for s := range z[j] {
				z[j][s] = c[j].Cost + float64(r.Intn(30))/10
			}
		}
		cfg := Config{[]float64{0, .35, 1}[trial%3], trial%2 == 0, 2.0, []int{2, 10, 20}[trial%3], []int{1, 4, 8, 16}[trial%4]}
		rr := Evaluate(z[r.Intn(m)], cfg).Risk
		switch trial % 5 {
		case 0:
			cfg.Limit = rr
		case 1:
			cfg.Limit = math.Nextafter(rr, math.Inf(1))
		case 2:
			cfg.Limit = math.Nextafter(rr, math.Inf(-1))
		case 3:
			cfg.Limit = 0
		case 4:
			cfg.Limit = 4
		}
		if cfg.Limit < 0 {
			cfg.Limit = 0
		}
		order := r.Perm(n)
		warm := r.Intn(m+1) - 1
		oracle := func(j, s int) (float64, error) { return z[j][s], nil }
		a, err := Select(c, n, cfg, order, warm, "full", oracle, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, mode := range []string{"cost", "natural", "tail"} {
			v, err := Select(c, n, cfg, order, warm, mode, oracle, trial%100 == 0)
			if err != nil {
				t.Fatal(err)
			}
			if v.Choice != a.Choice || v.Reason != a.Reason || v.Score != a.Score {
				t.Fatalf("trial %d %s got %+v want %+v", trial, mode, v, a)
			}
			if v.Queries > a.Queries {
				t.Fatal("extra oracle calls")
			}
			if trial%100 == 0 {
				if err := VerifyTrace(c, n, cfg, order, warm, mode, v); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
func TestBoundaryAndRejectMissing(t *testing.T) {
	cfg := Config{.35, true, 1.25, 20, 8}
	c := []Candidate{{0, 0, 0, true}, {1, .05, 0, true}}
	order := make([]int, 64)
	for i := range order {
		order[i] = i
	}
	for _, loss := range []float64{1.25, math.Nextafter(1.25, math.Inf(1)), math.Nextafter(1.25, math.Inf(-1))} {
		a, _ := Select(c, 64, cfg, order, -1, "full", func(j, s int) (float64, error) { return loss, nil }, false)
		b, err := Select(c, 64, cfg, order, -1, "tail", func(j, s int) (float64, error) { return loss, nil }, true)
		if err != nil || a.Choice != b.Choice || a.Reason != b.Reason {
			t.Fatal("boundary mismatch")
		}
		if err := VerifyTrace(c, 64, cfg, order, -1, "tail", b); err != nil {
			t.Fatal(err)
		}
		b.Choice = 99
		if VerifyTrace(c, 64, cfg, order, -1, "tail", b) == nil {
			t.Fatal("forged choice accepted")
		}
	}
	if _, err := Select(c, 64, cfg, order, -1, "tail", func(j, s int) (float64, error) { return -1, nil }, false); err == nil {
		t.Fatal("negative loss accepted")
	}
}
func TestWorstCaseNeedsEveryQuery(t *testing.T) {
	n, m := 64, 8
	cfg := Config{.35, true, 1, 20, 1}
	c := make([]Candidate, m)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	for j := range c {
		c[j] = Candidate{ID: j, Hard: true}
	}
	v, e := Select(c, n, cfg, order, -1, "natural", func(j, s int) (float64, error) {
		if s == n-1 {
			return 4, nil
		}
		return 0, nil
	}, true)
	if e != nil || v.Reason != "RISK_EMPTY" || v.Queries != m*n {
		t.Fatalf("worst case %+v %v", v, e)
	}
}

// Check necessity and sufficiency of the rectangular-box certificate by
// exhaustive endpoint enumeration (200 boxes, 3 actions x 2 scenarios).
func TestSharpCertificateVertices(t *testing.T) {
	r := rand.New(rand.NewSource(88103))
	cfg := Config{.35, true, 2, 2, 1}
	c := []Candidate{{0, 0, 0, true}, {1, 0, 0, true}, {2, 0, 0, true}}
	order := []int{0, 1}
	for trial := 0; trial < 200; trial++ {
		lo := make([][]float64, 3)
		hi := make([][]float64, 3)
		for j := range lo {
			lo[j] = []float64{float64(r.Intn(4)), float64(r.Intn(4))}
			hi[j] = []float64{lo[j][0] + float64(r.Intn(3)), lo[j][1] + float64(r.Intn(3))}
		}
		for chosen := -1; chosen < 3; chosen++ {
			certified := true
			if chosen < 0 {
				for j := range lo {
					if Evaluate(lo[j], cfg).Risk <= cfg.Limit {
						certified = false
					}
				}
			} else {
				high := Evaluate(hi[chosen], cfg)
				if high.Risk > cfg.Limit {
					certified = false
				}
				for j := range lo {
					if j == chosen {
						continue
					}
					low := Evaluate(lo[j], cfg)
					if low.Risk <= cfg.Limit && (low.Objective < high.Objective || (low.Objective == high.Objective && tie(c[j], c[chosen]))) {
						certified = false
					}
				}
			}
			invariant := true
			for mask := 0; mask < 64; mask++ {
				z := make([][]float64, 3)
				for j := range z {
					z[j] = make([]float64, 2)
					for s := range z[j] {
						z[j][s] = lo[j][s]
						if (mask>>(j*2+s))&1 == 1 {
							z[j][s] = hi[j][s]
						}
					}
				}
				v, err := Select(c, 2, cfg, order, -1, "full", func(j, s int) (float64, error) { return z[j][s], nil }, false)
				if err != nil {
					t.Fatal(err)
				}
				if v.Choice != chosen {
					invariant = false
					break
				}
			}
			if certified != invariant {
				t.Fatalf("certificate mismatch trial=%d chosen=%d", trial, chosen)
			}
		}
	}
}
