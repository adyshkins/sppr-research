// Package cert provides deterministic partial-scenario screening. The same
// scenario weights are retained after screening; this is not subsampling.
package cert

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

type Config struct {
	Lambda          float64 `json:"lambda"`
	Filter          bool    `json:"filter"`
	Limit           float64 `json:"limit"`
	TailDenominator int     `json:"tail_denominator"`
	Block           int     `json:"block"`
}
type Candidate struct {
	ID       int     `json:"id"`
	Cost     float64 `json:"cost"`
	Distance float64 `json:"distance"`
	Hard     bool    `json:"hard"`
}
type Score struct{ Mean, Risk, Objective float64 }
type Observation struct {
	Scenario int     `json:"scenario"`
	Loss     float64 `json:"loss"`
}
type Record struct {
	Candidate int           `json:"candidate"`
	Reason    string        `json:"reason"`
	Bound     Score         `json:"bound"`
	Incumbent int           `json:"incumbent"`
	Samples   []Observation `json:"samples,omitempty"`
}
type Result struct {
	Choice          int      `json:"choice"`
	Reason          string   `json:"reason"`
	Score           Score    `json:"score"`
	Queries         int      `json:"queries"`
	BoundChecks     int      `json:"bound_checks"`
	RiskPruned      int      `json:"risk_pruned"`
	ObjectivePruned int      `json:"objective_pruned"`
	Records         []Record `json:"records,omitempty"`
}
type Oracle func(candidate, scenario int) (float64, error)

func valid(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
func validate(c []Candidate, n int, cfg Config, order []int) error {
	if n < 1 || len(c) < 1 || cfg.TailDenominator < 2 || cfg.Block < 1 || !valid(cfg.Lambda) || cfg.Lambda < 0 || cfg.Lambda > 1 || !valid(cfg.Limit) || cfg.Limit < 0 {
		return errors.New("invalid dimensions or risk configuration")
	}
	ids := map[int]bool{}
	for _, v := range c {
		if ids[v.ID] || v.ID < 0 || !valid(v.Cost) || v.Cost < 0 || !valid(v.Distance) || v.Distance < 0 {
			return errors.New("invalid candidate")
		}
		ids[v.ID] = true
	}
	if len(order) != n {
		return errors.New("scenario order length mismatch")
	}
	seen := make([]bool, n)
	for _, i := range order {
		if i < 0 || i >= n || seen[i] {
			return errors.New("scenario order is not a permutation")
		}
		seen[i] = true
	}
	return nil
}

// Evaluate uses alpha=1-1/den and equal original weights. The fractional
// boundary atom is represented by integer coefficients; e.g. n=64, den=20:
// CVaR=(20*(z1+z2+z3)+4*z4)/64, sorted descending.
func Evaluate(z []float64, cfg Config) Score {
	n := len(z)
	var sum float64
	for _, v := range z {
		sum += v
	}
	v := append([]float64(nil), z...)
	sort.Sort(sort.Reverse(sort.Float64Slice(v)))
	k := n / cfg.TailDenominator
	rem := n % cfg.TailDenominator
	tail := 0.0
	for i := 0; i < k; i++ {
		tail += float64(cfg.TailDenominator) * v[i]
	}
	if rem > 0 {
		tail += float64(rem) * v[k]
	}
	r := tail / float64(n)
	m := sum / float64(n)
	return Score{m, r, (1-cfg.Lambda)*m + cfg.Lambda*r}
}
func tie(a, b Candidate) bool {
	if a.Cost != b.Cost {
		return a.Cost < b.Cost
	}
	if a.Distance != b.Distance {
		return a.Distance < b.Distance
	}
	return a.ID < b.ID
}
func better(sa Score, a Candidate, sb Score, b Candidate) bool {
	if sa.Objective != sb.Objective {
		return sa.Objective < sb.Objective
	}
	return tie(a, b)
}

// A conservative numerical separation guard. Ambiguous comparisons force more
// queries. Exact real-arithmetic statements are proved separately in the paper.
func guard(a, b float64) float64 { return 1e-11 * math.Max(1, math.Max(math.Abs(a), math.Abs(b))) }
func candidateOrder(c []Candidate, warm int) []int {
	ids := make([]int, len(c))
	for i := range ids {
		ids[i] = i
	}
	sort.SliceStable(ids, func(i, j int) bool {
		a, b := c[ids[i]], c[ids[j]]
		if (a.ID == warm) != (b.ID == warm) {
			return a.ID == warm
		}
		return tie(a, b)
	})
	return ids
}

// mode is full, cost (constant-cost bound only), natural, or tail. The latter
// two use exactly the same screening rule, but different caller-supplied orders.
func Select(c []Candidate, n int, cfg Config, order []int, warm int, mode string, oracle Oracle, trace bool) (Result, error) {
	out := Result{Choice: -1, Reason: "ADM_EMPTY"}
	if err := validate(c, n, cfg, order); err != nil {
		return out, err
	}
	if mode != "full" && mode != "cost" && mode != "natural" && mode != "tail" {
		return out, errors.New("unknown mode")
	}
	incumbent := -1
	anyHard := false
	for _, j := range candidateOrder(c, warm) {
		a := c[j]
		if !a.Hard {
			if trace {
				out.Records = append(out.Records, Record{Candidate: a.ID, Reason: "HARD", Incumbent: out.Choice})
			}
			continue
		}
		anyHard = true
		rec := Record{Candidate: a.ID, Incumbent: out.Choice}
		// All unevaluated horizon losses are at least the action cost.
		z := make([]float64, n)
		for i := range z {
			z[i] = a.Cost
		}
		lb := Score{a.Cost, a.Cost, a.Cost}
		pruned := false
		if mode != "full" {
			out.BoundChecks++
			if cfg.Filter && lb.Risk > cfg.Limit+guard(lb.Risk, cfg.Limit) {
				rec.Reason = "RISK_BOUND"
				out.RiskPruned++
				pruned = true
			} else if incumbent >= 0 && lb.Objective > out.Score.Objective+guard(lb.Objective, out.Score.Objective) {
				rec.Reason = "OBJECTIVE_BOUND"
				out.ObjectivePruned++
				pruned = true
			}
		}
		if !pruned {
			block := n
			if mode == "natural" || mode == "tail" {
				block = cfg.Block
			}
			for done := 0; done < n; {
				end := done + block
				if end > n {
					end = n
				}
				for _, s := range order[done:end] {
					v, err := oracle(j, s)
					if err != nil {
						return out, err
					}
					if !valid(v) || v < a.Cost {
						return out, fmt.Errorf("oracle violates finite lower bound for action %d", a.ID)
					}
					z[s] = v
					out.Queries++
					if trace {
						rec.Samples = append(rec.Samples, Observation{s, v})
					}
				}
				done = end
				lb = Evaluate(z, cfg)
				if done < n && (mode == "natural" || mode == "tail") {
					out.BoundChecks++
					if cfg.Filter && lb.Risk > cfg.Limit+guard(lb.Risk, cfg.Limit) {
						rec.Reason = "RISK_BOUND"
						out.RiskPruned++
						pruned = true
						break
					}
					if incumbent >= 0 && lb.Objective > out.Score.Objective+guard(lb.Objective, out.Score.Objective) {
						rec.Reason = "OBJECTIVE_BOUND"
						out.ObjectivePruned++
						pruned = true
						break
					}
				}
			}
		}
		if !pruned {
			if cfg.Filter && lb.Risk > cfg.Limit {
				rec.Reason = "RISK_EXACT"
			} else {
				rec.Reason = "COMPLETE"
				if incumbent < 0 || better(lb, a, out.Score, c[incumbent]) {
					incumbent = j
					out.Choice = a.ID
					out.Score = lb
				}
			}
		}
		rec.Bound = lb
		if trace {
			out.Records = append(out.Records, rec)
		}
	}
	if anyHard {
		out.Reason = "RISK_EMPTY"
	}
	if out.Choice >= 0 {
		out.Reason = "RECOMMENDATION"
	}
	return out, nil
}

// VerifyTrace replays queried values through the selection procedure and checks
// that no additional oracle value is needed. It does NOT establish that the
// supplied queried losses came from a correct physical simulator.
func VerifyTrace(c []Candidate, n int, cfg Config, order []int, warm int, mode string, r Result) error {
	vals := map[[2]int]float64{}
	idToIndex := map[int]int{}
	for i, a := range c {
		idToIndex[a.ID] = i
	}
	for _, rec := range r.Records {
		j, ok := idToIndex[rec.Candidate]
		if !ok {
			return errors.New("unknown traced action")
		}
		for _, v := range rec.Samples {
			key := [2]int{j, v.Scenario}
			if _, ok := vals[key]; ok {
				return errors.New("duplicate traced query")
			}
			vals[key] = v.Loss
		}
	}
	q, err := Select(c, n, cfg, order, warm, mode, func(j, s int) (float64, error) {
		v, ok := vals[[2]int{j, s}]
		if !ok {
			return 0, errors.New("missing queried loss")
		}
		return v, nil
	}, true)
	if err != nil {
		return err
	}
	if q.Choice != r.Choice || q.Reason != r.Reason || q.Score != r.Score || q.Queries != r.Queries || q.BoundChecks != r.BoundChecks || q.RiskPruned != r.RiskPruned || q.ObjectivePruned != r.ObjectivePruned || len(q.Records) != len(r.Records) {
		return errors.New("certificate summary mismatch")
	}
	for i, v := range q.Records {
		w := r.Records[i]
		if v.Candidate != w.Candidate || v.Reason != w.Reason || v.Bound != w.Bound || v.Incumbent != w.Incumbent || len(v.Samples) != len(w.Samples) {
			return errors.New("certificate record mismatch")
		}
		for k := range v.Samples {
			if v.Samples[k] != w.Samples[k] {
				return errors.New("certificate sample order mismatch")
			}
		}
	}
	return nil
}
