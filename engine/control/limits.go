// Package control joins the observation-only A3-A5 pipeline to A6 and scheduling.
// Hard constraints below are NEW declared command permissions, not recovered
// industrial safety limits. Physical shortages still belong to plant balances.
package control

import (
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/forecast"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/plant"
	"fmt"
	"math"
	"reflect"
)

const Version = "closed-loop-control-0.5.0"
const LimitsSchema = "declared-command-permissions-v05"

type Limits struct {
	Schema          string     `json:"schema"`
	Version         string     `json:"version"`
	SnapshotDay     int        `json:"snapshot_day"`
	ValidFrom       int        `json:"valid_from_day"`
	ValidThrough    int        `json:"valid_through_day"`
	AllowedIDs      []string   `json:"allowed_action_ids"`
	MaxOvertime     float64    `json:"max_overtime"`
	MaxExtraRaw     float64    `json:"max_extra_raw"`
	MaxActionCost   float64    `json:"max_action_cost"`
	RemainingBudget float64    `json:"remaining_normalized_action_budget"`
	ReplanMin       plant.Pair `json:"replan_min"`
	ReplanMax       plant.Pair `json:"replan_max"`
}

// Defaults preserve the full eight-action library. Budget 100 is a NEW,
// deliberately explicit engineering authorization account, not money/data.
func DefaultLimits() Limits {
	return Limits{LimitsSchema, "permissions-v05-initial", -1, 0, 365,
		[]string{"u0", "u1", "u2", "u3", "u4", "u5", "u6", "u7"}, .2, 60, .27, 100,
		plant.Pair{10, 6.25}, plant.Pair{70, 43.75}}
}
func CloneLimits(l Limits) Limits { l.AllowedIDs = append([]string{}, l.AllowedIDs...); return l }
func (l Limits) Validate() error {
	if l.Schema != LimitsSchema || l.Version == "" || l.SnapshotDay < -1 || l.ValidFrom < 0 || l.ValidThrough < l.ValidFrom {
		return fmt.Errorf("invalid permission identity/validity")
	}
	for _, v := range []float64{l.MaxOvertime, l.MaxExtraRaw, l.MaxActionCost, l.RemainingBudget, l.ReplanMin[0], l.ReplanMin[1], l.ReplanMax[0], l.ReplanMax[1]} {
		if !observation.FiniteNonnegative(v) {
			return fmt.Errorf("invalid permission magnitude")
		}
	}
	for j := 0; j < 2; j++ {
		if l.ReplanMin[j] > l.ReplanMax[j] {
			return fmt.Errorf("invalid plan bounds")
		}
	}
	seen := map[string]bool{}
	rp := plant.Pair{40, 25}
	for _, id := range l.AllowedIDs {
		if seen[id] {
			return fmt.Errorf("duplicate permission")
		}
		if _, e := plant.FixedAction(id, &rp); e != nil {
			return e
		}
		seen[id] = true
	}
	return nil
}
func ConstraintIDs() []string {
	return []string{"action_permission", "overtime_permission", "extra_raw_permission", "single_action_cost", "remaining_budget", "replan_a_lower", "replan_a_upper", "replan_b_lower", "replan_b_upper"}
}
func CheckAction(a plant.Action, l Limits) ([]core.Constraint, error) {
	if e := l.Validate(); e != nil {
		return nil, e
	}
	declared, e := plant.FixedAction(a.ID, a.PermanentPlan)
	if e != nil {
		return nil, e
	}
	if !reflect.DeepEqual(declared, a) {
		return nil, fmt.Errorf("action differs from declared library")
	}
	allowed := false
	for _, id := range l.AllowedIDs {
		if id == a.ID {
			allowed = true
		}
	}
	denied := 1.0
	if allowed {
		denied = 0
	}
	out := []core.Constraint{{ID: "action_permission", Residual: denied}, {ID: "overtime_permission", Residual: a.Overtime - l.MaxOvertime}, {ID: "extra_raw_permission", Residual: a.ExtraRaw - l.MaxExtraRaw}, {ID: "single_action_cost", Residual: a.Cost - l.MaxActionCost}, {ID: "remaining_budget", Residual: a.Cost - l.RemainingBudget}}
	for j, name := range []string{"a", "b"} {
		lo, hi := 0., 0.
		if a.PermanentPlan != nil {
			lo = l.ReplanMin[j] - a.PermanentPlan[j]
			hi = a.PermanentPlan[j] - l.ReplanMax[j]
		}
		out = append(out, core.Constraint{ID: "replan_" + name + "_lower", Residual: lo}, core.Constraint{ID: "replan_" + name + "_upper", Residual: hi})
	}
	return out, nil
}

// PlanDistance is NEW explicit relative L1 distance of the requested next-day
// plan from the registered base plan; it is not a distance to hidden state.
func PlanDistance(a plant.Action, k forecast.KnownPlan) (float64, error) {
	sum, d := 0., 0.
	for j, v := range k.Base {
		if !observation.FiniteNonnegative(v) {
			return 0, fmt.Errorf("bad base plan")
		}
		sum += v
		next := v
		if a.PermanentPlan != nil {
			next = a.PermanentPlan[j]
		}
		d += math.Abs(next*a.TemporaryFactors[j] - v)
	}
	if !observation.Finite(sum) || !observation.Finite(d) {
		return 0, fmt.Errorf("plan distance overflow")
	}
	return d / math.Max(1, sum), nil
}
func LimitsReasons(l Limits, k forecast.KnownPlan, day int) ([]string, error) {
	if e := l.Validate(); e != nil {
		return nil, e
	}
	r := []string{}
	if l.SnapshotDay != day {
		r = append(r, "PERMISSION_SNAPSHOT_DAY_MISMATCH")
	}
	if l.Version != k.ConstraintVersion {
		r = append(r, "PERMISSION_VERSION_MISMATCH")
	}
	if day+1 < l.ValidFrom || day+1 > l.ValidThrough {
		r = append(r, "PERMISSION_NOT_VALID_FOR_EXECUTION_DAY")
	}
	return r, nil
}
