// Package plant implements ONLY the deterministic material-balance core of
// equations (4.1)-(4.3). Exogenous quantities must be supplied explicitly.
// There is no inferred random generator or hidden forecast distribution here.
package plant

import (
	"fmt"
	"math"
)

type Pair [2]float64
type State struct {
	Raw      float64 `json:"raw"`
	Finished Pair    `json:"finished"`
	Backlog  Pair    `json:"backlog"`
	Plan     Pair    `json:"plan"`
}
type Parameters struct {
	BaseDemand         Pair    `json:"base_demand"`
	CapacityPerUnit    Pair    `json:"capacity_per_unit"`
	RawPerUnit         Pair    `json:"raw_per_unit"`
	RawDaysDenominator float64 `json:"raw_days_denominator"`
}

func DissertationParameters() Parameters {
	return Parameters{Pair{40, 25}, Pair{1, 1.6}, Pair{1, 2}, 90}
}
func DissertationInitialState() State { return State{450, Pair{20, 12.5}, Pair{0, 0}, Pair{40, 25}} }

type Exogenous struct {
	Demand     Pair    `json:"demand"`
	Capacity   float64 `json:"capacity"`
	RawArrival float64 `json:"raw_arrival"`
}

// Action parameters are explicit. A permanent plan is a snapshot calculated
// by the controller earlier, not a new calculation using future demand.
type Action struct {
	ID               string  `json:"id"`
	Overtime         float64 `json:"overtime"`
	ExtraRaw         float64 `json:"extra_raw"`
	TemporaryFactors Pair    `json:"temporary_factors"`
	PermanentPlan    *Pair   `json:"permanent_plan"`
	Cost             float64 `json:"cost"`
}

type Result struct {
	Next              State      `json:"next"`
	ExecutedPlan      Pair       `json:"executed_plan"`
	Output            Pair       `json:"output"`
	Shipped           Pair       `json:"shipped"`
	EffectiveCapacity float64    `json:"effective_capacity"`
	CapacityUsed      float64    `json:"capacity_used"`
	RawUsed           float64    `json:"raw_used"`
	Scale             float64    `json:"scale"`
	KPI               [4]float64 `json:"kpi"`
	ActionCost        float64    `json:"action_cost"`
}

func valid(x float64) bool  { return !math.IsNaN(x) && !math.IsInf(x, 0) && x >= 0 }
func validPair(x Pair) bool { return valid(x[0]) && valid(x[1]) }
func (p Parameters) Validate() error {
	if !validPair(p.BaseDemand) || p.BaseDemand[0] <= 0 || p.BaseDemand[1] <= 0 || !validPair(p.CapacityPerUnit) || !validPair(p.RawPerUnit) || !valid(p.RawDaysDenominator) || p.RawDaysDenominator <= 0 {
		return fmt.Errorf("invalid plant parameters")
	}
	return nil
}

// FixedAction maps the explicit entries of table 4.2. u6 requires a supplied
// plan snapshot; nil does not trigger an invented replan or access to true state.
func FixedAction(id string, replan *Pair) (Action, error) {
	a := Action{ID: id, TemporaryFactors: Pair{1, 1}}
	switch id {
	case "u0":
	case "u1":
		a.Overtime = .1
		a.Cost = .05
	case "u2":
		a.Overtime = .2
		a.Cost = .12
	case "u3":
		a.ExtraRaw = 60
		a.Cost = .15
	case "u4":
		a.TemporaryFactors = Pair{1.2, .8}
		a.Cost = .03
	case "u5":
		a.TemporaryFactors = Pair{.8, 1.2}
		a.Cost = .03
	case "u6":
		if replan == nil || !validPair(*replan) {
			return a, fmt.Errorf("u6 requires a finite nonnegative plan snapshot")
		}
		copyP := *replan
		a.PermanentPlan = &copyP
		a.Cost = .05
	case "u7":
		a.Overtime = .2
		a.ExtraRaw = 60
		a.Cost = .27
	default:
		return a, fmt.Errorf("unknown action %q", id)
	}
	return a, nil
}

// Replan accepts estimates, not a plant State. Source: paragraph P1781.
func Replan(demandEstimate, backlogEstimate, finishedEstimate, baseDemand Pair) (Pair, error) {
	if !validPair(demandEstimate) || !validPair(backlogEstimate) || !validPair(finishedEstimate) || !validPair(baseDemand) || baseDemand[0] <= 0 || baseDemand[1] <= 0 {
		return Pair{}, fmt.Errorf("invalid replan input")
	}
	var out Pair
	for i := 0; i < 2; i++ {
		value := demandEstimate[i] + .25*backlogEstimate[i] - .1*finishedEstimate[i]
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return Pair{}, fmt.Errorf("replan overflow")
		}
		out[i] = math.Min(1.75*baseDemand[i], math.Max(.25*baseDemand[i], value))
	}
	return out, nil
}

// Step executes one already assigned action. It does not select the action;
// selecting from this step's outcomes for the SAME step would be look-ahead.
func Step(s State, w Exogenous, a Action, p Parameters) (Result, error) {
	var r Result
	if err := p.Validate(); err != nil {
		return r, err
	}
	if !valid(s.Raw) || !validPair(s.Finished) || !validPair(s.Backlog) || !validPair(s.Plan) || !validPair(w.Demand) || !valid(w.Capacity) || !valid(w.RawArrival) || a.ID == "" || !valid(a.Overtime) || !valid(a.ExtraRaw) || !validPair(a.TemporaryFactors) || !valid(a.Cost) {
		return r, fmt.Errorf("invalid state, forcing, or action")
	}
	next := s
	if a.PermanentPlan != nil {
		if !validPair(*a.PermanentPlan) {
			return r, fmt.Errorf("invalid plan")
		}
		next.Plan = *a.PermanentPlan
	}
	for i := 0; i < 2; i++ {
		r.ExecutedPlan[i] = next.Plan[i] * a.TemporaryFactors[i]
	}
	capacity := w.Capacity * (1 + a.Overtime)
	raw := s.Raw + w.RawArrival + a.ExtraRaw
	reqC := p.CapacityPerUnit[0]*r.ExecutedPlan[0] + p.CapacityPerUnit[1]*r.ExecutedPlan[1]
	reqR := p.RawPerUnit[0]*r.ExecutedPlan[0] + p.RawPerUnit[1]*r.ExecutedPlan[1]
	if !valid(capacity) || !valid(raw) || !valid(reqC) || !valid(reqR) {
		return r, fmt.Errorf("resource calculation overflow")
	}
	scale := 1.0
	if reqC > 0 {
		scale = math.Min(scale, capacity/reqC)
	}
	if reqR > 0 {
		scale = math.Min(scale, raw/reqR)
	}
	totalDemand, totalShip, totalPlan, deviation := 0.0, 0.0, 0.0, 0.0
	for i := 0; i < 2; i++ {
		r.Output[i] = scale * r.ExecutedPlan[i]
		demand := w.Demand[i] + s.Backlog[i]
		stock := s.Finished[i] + r.Output[i]
		if !valid(demand) || !valid(stock) {
			return r, fmt.Errorf("flow calculation overflow")
		}
		r.Shipped[i] = math.Min(demand, stock)
		next.Backlog[i] = demand - r.Shipped[i]
		next.Finished[i] = stock - r.Shipped[i]
		r.CapacityUsed += p.CapacityPerUnit[i] * r.Output[i]
		r.RawUsed += p.RawPerUnit[i] * r.Output[i]
		totalDemand += demand
		totalShip += r.Shipped[i]
		totalPlan += r.ExecutedPlan[i]
		deviation += math.Abs(r.Output[i] - r.ExecutedPlan[i])
	}
	next.Raw = raw - r.RawUsed
	// Numerical roundoff only: larger negative balances are an error.
	if next.Raw < 0 && next.Raw >= -1e-12*math.Max(1, raw) {
		next.Raw = 0
	}
	if !valid(next.Raw) || !valid(totalDemand) || !valid(totalShip) || !valid(totalPlan) || !valid(deviation) || !valid(r.RawUsed) || !valid(r.CapacityUsed) {
		return r, fmt.Errorf("invalid material balance")
	}
	r.KPI[0] = 1
	if totalDemand > 0 {
		r.KPI[0] = totalShip / totalDemand
	}
	r.KPI[1] = 1
	if totalPlan > 0 {
		r.KPI[1] = math.Max(0, 1-deviation/totalPlan)
	}
	if capacity > 0 {
		r.KPI[2] = r.CapacityUsed / capacity
	}
	r.KPI[3] = next.Raw / p.RawDaysDenominator
	r.Next = next
	r.Scale = scale
	r.EffectiveCapacity = capacity
	r.ActionCost = a.Cost
	return r, nil
}

// PeriodLoss uses the ranges, scales and coefficients explicitly listed in
// paragraph P1782. This is a computational interpretation of that paragraph,
// not evidence of identity to the lost implementation.
func PeriodLoss(k [4]float64) (float64, error) {
	lo := [4]float64{.95, .95, .7, 3}
	hi := [4]float64{1, 1, .9, 7}
	weights := [4]float64{.35, .25, .15, .25}
	scale := [4]float64{.95, .95, .7, 3}
	above := [4]float64{0, 0, .5, .25}
	loss := 0.0
	for i, v := range k {
		if !valid(v) {
			return 0, fmt.Errorf("KPI must be finite and nonnegative")
		}
		loss += weights[i] * (math.Max(lo[i]-v, 0) + above[i]*math.Max(v-hi[i], 0)) / scale[i]
	}
	if !valid(loss) {
		return 0, fmt.Errorf("loss overflow")
	}
	return loss, nil
}
