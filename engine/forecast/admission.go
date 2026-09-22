package forecast

import (
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/plant"
	"fmt"
	"math"
)

// KnownPlan is the registered BASE plan after the closed day, not the temporary
// executed plan. SnapshotDay is the day for which this register snapshot applies.
type KnownPlan struct {
	Base              [2]float64 `json:"base_plan"`
	Version           string     `json:"plan_version"`
	ConstraintVersion string     `json:"constraint_version"`
	SnapshotDay       int        `json:"snapshot_day"`
}
type Estimate struct {
	ClosedDay int        `json:"closed_day"`
	Raw       float64    `json:"raw_estimate"`
	Finished  [2]float64 `json:"finished_estimate"`
	Backlog   [2]float64 `json:"backlog_estimate"`
	Plan      [2]float64 `json:"registered_base_plan"`
	Demand    [2]float64 `json:"observed_demand"`
	Capacity  float64    `json:"observed_capacity"`
	Arrival   float64    `json:"observed_arrival"`
	Origin    string     `json:"origin"`
}
type Admission struct {
	Passed   bool                `json:"passed"`
	Reasons  []string            `json:"reasons"`
	Required []observation.Field `json:"required_fields"`
	Estimate *Estimate           `json:"estimate"`
}

func RequiredFields() []observation.Field {
	return []observation.Field{observation.RawEnd, observation.FinishedA, observation.FinishedB, observation.BacklogAfterA, observation.BacklogAfterB, observation.DemandA, observation.DemandB, observation.Capacity, observation.RawArrival, observation.ShippedA, observation.ShippedB, observation.BacklogBeforeA, observation.BacklogBeforeB}
}

// Assess never restores missing forecast-state channels from initial constants,
// never treats monitor.KPI as a plant state, and never receives true plant.State.
func Assess(p observation.Packet, k KnownPlan, c Config) (Admission, error) {
	r := Admission{Reasons: []string{}, Required: RequiredFields()}
	if e := p.Validate(); e != nil {
		return r, e
	}
	if e := c.Validate(); e != nil {
		return r, e
	}
	if k.Version == "" || k.ConstraintVersion == "" || k.SnapshotDay < 0 {
		return r, fmt.Errorf("known plan identity required")
	}
	for _, x := range k.Base {
		if !observation.FiniteNonnegative(x) {
			return r, fmt.Errorf("invalid registered plan")
		}
	}
	if k.Version != p.Context.PlanVersion {
		r.Reasons = append(r.Reasons, "PLAN_VERSION_MISMATCH")
	}
	if k.ConstraintVersion != p.Context.ConstraintVersion {
		r.Reasons = append(r.Reasons, "CONSTRAINT_VERSION_MISMATCH")
	}
	if k.SnapshotDay != p.Day {
		r.Reasons = append(r.Reasons, "REGISTER_SNAPSHOT_DAY_MISMATCH")
	}
	rp := plant.Pair(k.Base)
	a, e := plant.FixedAction(p.Context.ActionID, &rp)
	if e != nil {
		return r, e
	}
	if !close(a.Overtime, p.Context.Overtime) {
		r.Reasons = append(r.Reasons, "EXECUTED_OVERTIME_MISMATCH")
	}
	for i := 0; i < 2; i++ {
		if !close(k.Base[i]*a.TemporaryFactors[i], p.Context.ExecutedPlan[i]) {
			r.Reasons = append(r.Reasons, fmt.Sprintf("EXECUTED_PLAN_MISMATCH_%d", i))
		}
	}
	for _, f := range r.Required {
		m := p.Readings[f]
		if !m.Valid() || m.AgeDays != 0 {
			r.Reasons = append(r.Reasons, "FRESH_FIELD_REQUIRED:"+string(f))
		}
	}
	q, e := observation.EvaluateQuality(p, c.Quality)
	if e != nil {
		return r, e
	}
	if !q.GatePassed {
		r.Reasons = append(r.Reasons, "QUALITY_BELOW_MINIMUM")
	}
	if len(r.Reasons) > 0 {
		return r, nil
	}
	v := func(f observation.Field) float64 { return *p.Readings[f].Value }
	for i, fs := range [][5]observation.Field{{observation.DemandA, observation.BacklogBeforeA, observation.ShippedA, observation.BacklogAfterA, observation.FinishedA}, {observation.DemandB, observation.BacklogBeforeB, observation.ShippedB, observation.BacklogAfterB, observation.FinishedB}} {
		available := v(fs[0]) + v(fs[1])
		expected := available - v(fs[2])
		actual := v(fs[3])
		if !finite(available) || !finite(expected) || expected < -c.BalanceTolerance*math.Max(1, available) || math.Abs(actual-math.Max(0, expected)) > c.BalanceTolerance*math.Max(1, available) {
			r.Reasons = append(r.Reasons, fmt.Sprintf("BACKLOG_BALANCE_MISMATCH_%d", i))
		}
		// Proportional observation noise preserves true zero, so simultaneous
		// positive backlog and finished stock is rejected in this declared model.
		if actual > 1e-9 && v(fs[4]) > 1e-9 {
			r.Reasons = append(r.Reasons, fmt.Sprintf("SIMULTANEOUS_BACKLOG_AND_FINISHED_%d", i))
		}
	}
	if len(r.Reasons) > 0 {
		return r, nil
	}
	r.Estimate = &Estimate{ClosedDay: p.Day, Raw: v(observation.RawEnd), Finished: [2]float64{v(observation.FinishedA), v(observation.FinishedB)}, Backlog: [2]float64{v(observation.BacklogAfterA), v(observation.BacklogAfterB)}, Plan: k.Base, Demand: [2]float64{v(observation.DemandA), v(observation.DemandB)}, Capacity: v(observation.Capacity), Arrival: v(observation.RawArrival), Origin: "current_readings_point_estimate_no_initial_state_sampling"}
	r.Passed = true
	return r, nil
}
func (x Estimate) validate() error {
	if x.ClosedDay < 0 || x.Origin != "current_readings_point_estimate_no_initial_state_sampling" {
		return fmt.Errorf("invalid estimate origin/day")
	}
	for _, v := range []float64{x.Raw, x.Finished[0], x.Finished[1], x.Backlog[0], x.Backlog[1], x.Plan[0], x.Plan[1], x.Demand[0], x.Demand[1], x.Capacity, x.Arrival} {
		if !observation.FiniteNonnegative(v) {
			return fmt.Errorf("invalid estimate")
		}
	}
	return nil
}
