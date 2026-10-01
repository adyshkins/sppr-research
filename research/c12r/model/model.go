// Package model is a new, self-contained experimental implementation of the
// two-product balance equations and action catalog in sppr-research/engine/plant.
// It is NOT the missing F10/E11 driver. See docs/provenance.md.
package model

import "math"

type Pair [2]float64
type State struct {
	Raw                     float64
	Finished, Backlog, Plan Pair
}
type Forcing struct {
	Demand               Pair
	Capacity, RawArrival float64
}
type Action struct {
	ID                 int
	Overtime, ExtraRaw float64
	Factors            Pair
	Replan             Pair
	Permanent          bool
	Cost, Distance     float64
}
type Result struct {
	Next          State
	KPI           [4]float64
	Loss, Backlog float64
}

func Initial() State   { return State{450, Pair{20, 12.5}, Pair{}, Pair{40, 25}} }
func Baseline() Action { return Action{ID: 0, Factors: Pair{1, 1}} }
func Catalog(s State, demand Pair) []Action {
	a := make([]Action, 8)
	for i := range a {
		a[i] = Baseline()
		a[i].ID = i
	}
	a[1].Overtime = .1
	a[1].Cost = .05
	a[2].Overtime = .2
	a[2].Cost = .12
	a[3].ExtraRaw = 60
	a[3].Cost = .15
	a[4].Factors = Pair{1.2, .8}
	a[4].Cost = .03
	a[5].Factors = Pair{.8, 1.2}
	a[5].Cost = .03
	a[6].Permanent = true
	a[6].Cost = .05
	for i, b := range []float64{40, 25} {
		a[6].Replan[i] = math.Min(1.75*b, math.Max(.25*b, demand[i]+.25*s.Backlog[i]-.1*s.Finished[i]))
	}
	a[7].Overtime = .2
	a[7].ExtraRaw = 60
	a[7].Cost = .27
	for j := range a {
		var dist float64
		for i := 0; i < 2; i++ {
			p := s.Plan[i]
			if a[j].Permanent {
				p = a[j].Replan[i]
			}
			dist += math.Abs(p*a[j].Factors[i] - s.Plan[i])
		}
		a[j].Distance = dist / math.Max(1, s.Plan[0]+s.Plan[1])
	}
	return a
}
func Step(s State, w Forcing, a Action) Result {
	next := s
	if a.Permanent {
		next.Plan = a.Replan
	}
	p := Pair{next.Plan[0] * a.Factors[0], next.Plan[1] * a.Factors[1]}
	capacity := w.Capacity * (1 + a.Overtime)
	raw := s.Raw + w.RawArrival + a.ExtraRaw
	reqC := p[0] + 1.6*p[1]
	reqR := p[0] + 2*p[1]
	scale := 1.0
	if reqC > 0 {
		scale = math.Min(scale, capacity/reqC)
	}
	if reqR > 0 {
		scale = math.Min(scale, raw/reqR)
	}
	var demand, ship, plan, dev, usedC, usedR float64
	for i := 0; i < 2; i++ {
		output := p[i] * scale
		need := w.Demand[i] + s.Backlog[i]
		stock := s.Finished[i] + output
		sent := math.Min(need, stock)
		next.Backlog[i] = need - sent
		next.Finished[i] = stock - sent
		usedC += []float64{1, 1.6}[i] * output
		usedR += []float64{1, 2}[i] * output
		demand += need
		ship += sent
		plan += p[i]
		dev += math.Abs(output - p[i])
	}
	next.Raw = math.Max(0, raw-usedR)
	k := [4]float64{1, 1, 0, next.Raw / 90}
	if demand > 0 {
		k[0] = ship / demand
	}
	if plan > 0 {
		k[1] = math.Max(0, 1-dev/plan)
	}
	if capacity > 0 {
		k[2] = usedC / capacity
	}
	lo := [4]float64{.95, .95, .7, 3}
	hi := [4]float64{1, 1, .9, 7}
	wt := [4]float64{.35, .25, .15, .25}
	above := [4]float64{0, 0, .5, .25}
	var loss float64
	for i := range k {
		loss += wt[i] * (math.Max(lo[i]-k[i], 0) + above[i]*math.Max(k[i]-hi[i], 0)) / lo[i]
	}
	return Result{next, k, loss, next.Backlog[0] + next.Backlog[1]}
}

// Rollout: candidate at h=0, then continue the resulting registered plan.
// One-off cost is charged exactly once, and all period losses are nonnegative.
func Rollout(s State, path []Forcing, a Action, discount float64) float64 {
	z := a.Cost
	g := 1.0
	for h, w := range path {
		u := Baseline()
		if h == 0 {
			u = a
		}
		r := Step(s, w, u)
		z += g * r.Loss
		g *= discount
		s = r.Next
	}
	return z
}
