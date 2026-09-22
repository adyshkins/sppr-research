package forecast

import (
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/plant"
	"fmt"
	"math"
	"sort"
)

type Trajectory struct {
	ScenarioID          string       `json:"scenario_id"`
	KPI                 [][4]float64 `json:"kpi"`
	PeriodLosses        []float64    `json:"period_losses"`
	HorizonLoss         float64      `json:"horizon_loss"`
	FinalPredictedState plant.State  `json:"final_predicted_state_not_environment_truth"`
}
type Band struct {
	HorizonDay    int        `json:"horizon_day"`
	Mean          [4]float64 `json:"mean"`
	Lower         [4]float64 `json:"lower"`
	Upper         [4]float64 `json:"upper"`
	ViolationMass [4]float64 `json:"scenario_violation_mass"`
}
type ActionForecast struct {
	Action           plant.Action `json:"action"`
	Trajectories     []Trajectory `json:"trajectories"`
	Bands            []Band       `json:"bands"`
	Risk             core.Risk    `json:"predicted_risk"`
	AnyViolationMass float64      `json:"any_violation_on_horizon_mass"`
}

// Quantile is the left-inverse CDF for a finite weighted distribution. Zero
// weight points cannot set endpoints, and scores are not normalized into weights.
func Quantile(values, weights []float64, tau float64) (float64, error) {
	if len(values) == 0 || len(values) != len(weights) || !unit(tau) {
		return 0, fmt.Errorf("invalid quantile inputs")
	}
	type atom struct{ v, p float64 }
	a := []atom{}
	sum := 0.0
	for i, v := range values {
		if !finite(v) || !unit(weights[i]) {
			return 0, fmt.Errorf("invalid quantile value/weight")
		}
		sum += weights[i]
		if weights[i] > 0 {
			a = append(a, atom{v, weights[i]})
		}
	}
	if len(a) == 0 || math.Abs(sum-1) > 1e-12 {
		return 0, fmt.Errorf("quantile weights must sum to one")
	}
	sort.Slice(a, func(i, j int) bool { return a[i].v < a[j].v })
	cum := 0.0
	for _, x := range a {
		cum += x.p / sum
		if cum >= tau {
			return x.v, nil
		}
	}
	return a[len(a)-1].v, nil
}
func Alternatives(x Estimate, c Config) ([]plant.Action, error) {
	if e := c.Validate(); e != nil {
		return nil, e
	}
	if e := x.validate(); e != nil {
		return nil, e
	}
	rp, e := plant.Replan(plant.Pair(x.Demand), plant.Pair(x.Backlog), plant.Pair(x.Finished), c.Parameters.BaseDemand)
	if e != nil {
		return nil, e
	}
	out := []plant.Action{}
	for _, id := range []string{"u0", "u1", "u2", "u3", "u4", "u5", "u6", "u7"} {
		a, e := plant.FixedAction(id, &rp)
		if e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, nil
}
func validatePaths(e Ensemble, c Config) error {
	if e.Version != Version || e.ClosedDay < 0 || len(e.Paths) == 0 || len(e.Paths) > 4*c.PathsPerHypothesis {
		return fmt.Errorf("invalid forecast ensemble")
	}
	seen := map[string]bool{}
	sum := 0.0
	for _, p := range e.Paths {
		if p.ID == "" || seen[p.ID] || len(p.Days) != c.Horizon || !unit(p.Probability) {
			return fmt.Errorf("invalid forecast path")
		}
		seen[p.ID] = true
		sum += p.Probability
		for _, w := range p.Days {
			for _, v := range []float64{w.Demand[0], w.Demand[1], w.Capacity, w.RawArrival} {
				if !finite(v) || v < 0 {
					return fmt.Errorf("invalid forecast forcing")
				}
			}
		}
	}
	if math.Abs(sum-1) > 1e-12 {
		return fmt.Errorf("ensemble weights do not sum to one")
	}
	return nil
}

// Evaluate applies the candidate only on h=1, then u0. A permanent plan persists.
// Candidate costs are counted ONCE. These are predictions, not feasible-action
// certificates: numerical hard constraints and execution policy remain separate.
func Evaluate(x Estimate, e Ensemble, c Config, actions []plant.Action) ([]ActionForecast, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if err := x.validate(); err != nil {
		return nil, err
	}
	if err := validatePaths(e, c); err != nil {
		return nil, err
	}
	if e.ClosedDay != x.ClosedDay || len(actions) == 0 {
		return nil, fmt.Errorf("estimate/ensemble day mismatch or empty alternatives")
	}
	paths := append([]ScenarioPath(nil), e.Paths...)
	sort.Slice(paths, func(i, j int) bool { return paths[i].ID < paths[j].ID })
	as := append([]plant.Action(nil), actions...)
	sort.Slice(as, func(i, j int) bool { return as[i].ID < as[j].ID })
	ids := map[string]bool{}
	weights := make([]float64, len(paths))
	for i, p := range paths {
		weights[i] = p.Probability
	}
	lo, hi := ranges()
	out := []ActionForecast{}
	noop, _ := plant.FixedAction("u0", nil)
	for _, a := range as {
		if ids[a.ID] {
			return nil, fmt.Errorf("duplicate candidate")
		}
		ids[a.ID] = true
		// Enforce the declared eight-action library, including the previously
		// calculated permanent plan snapshot; arbitrary controls need a new model.
		declared, err := plant.FixedAction(a.ID, a.PermanentPlan)
		if err != nil {
			return nil, err
		}
		if a.Overtime != declared.Overtime || a.ExtraRaw != declared.ExtraRaw || a.TemporaryFactors != declared.TemporaryFactors || a.Cost != declared.Cost || (a.ID != "u6" && a.PermanentPlan != nil) {
			return nil, fmt.Errorf("candidate differs from declared action library")
		}
		fa := ActionForecast{Action: a, Trajectories: []Trajectory{}, Bands: make([]Band, c.Horizon)}
		if a.PermanentPlan != nil {
			v := *a.PermanentPlan
			fa.Action.PermanentPlan = &v
		}
		losses := make([]float64, len(paths))
		for i, p := range paths {
			// A fresh value initialized from the OBSERVATION estimate for every action.
			s := plant.State{Raw: x.Raw, Finished: plant.Pair(x.Finished), Backlog: plant.Pair(x.Backlog), Plan: plant.Pair(x.Plan)}
			tr := Trajectory{ScenarioID: p.ID, KPI: make([][4]float64, c.Horizon), PeriodLosses: make([]float64, c.Horizon)}
			violated := false
			for h, w := range p.Days {
				u := noop
				if h == 0 {
					u = a
				}
				r, err := plant.Step(s, w, u, c.Parameters)
				if err != nil {
					return nil, err
				}
				tr.KPI[h] = r.KPI
				tr.PeriodLosses[h], err = plant.PeriodLoss(r.KPI)
				if err != nil {
					return nil, err
				}
				s = r.Next
				for j, v := range r.KPI {
					if v < lo[j] || v > hi[j] {
						violated = true
					}
				}
			}
			tr.FinalPredictedState = s
			tr.HorizonLoss, err = core.HorizonLoss(tr.PeriodLosses, c.Discount, a.Cost)
			if err != nil {
				return nil, err
			}
			losses[i] = tr.HorizonLoss
			fa.Trajectories = append(fa.Trajectories, tr)
			if violated {
				fa.AnyViolationMass += p.Probability
			}
		}
		fa.Risk, err = risk(losses, weights, c.Alpha)
		if err != nil {
			return nil, err
		}
		for h := 0; h < c.Horizon; h++ {
			b := Band{HorizonDay: h + 1}
			for j := 0; j < 4; j++ {
				values := make([]float64, len(paths))
				for i, t := range fa.Trajectories {
					v := t.KPI[h][j]
					values[i] = v
					b.Mean[j] += weights[i] * v
					if v < lo[j] || v > hi[j] {
						b.ViolationMass[j] += weights[i]
					}
				}
				b.Lower[j], err = Quantile(values, weights, c.Quantiles[0])
				if err != nil {
					return nil, err
				}
				b.Upper[j], err = Quantile(values, weights, c.Quantiles[1])
				if err != nil {
					return nil, err
				}
				b.ViolationMass[j] = math.Min(1, b.ViolationMass[j])
				if !finite(b.Mean[j]) {
					return nil, fmt.Errorf("forecast mean overflow")
				}
			}
			fa.Bands[h] = b
		}
		fa.AnyViolationMass = math.Min(1, fa.AnyViolationMass)
		out = append(out, fa)
	}
	return out, nil
}
