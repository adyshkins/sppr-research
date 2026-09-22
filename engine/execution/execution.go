// Package execution resolves one pending command against current registered
// permissions. It does not observe true plant state or future disturbances.
// Resolve authorizes an action; a physical execution record is made only after
// plant.Step succeeds in the experiment-side runner.
package execution

import (
	"dissertation.local/sppr-reconstruction/control"
	"dissertation.local/sppr-reconstruction/forecast"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/plant"
	"fmt"
	"reflect"
)

type Register struct {
	Plan   forecast.KnownPlan `json:"plan"`
	Limits control.Limits     `json:"permissions"`
}

func CloneRegister(r Register) Register { r.Limits = control.CloneLimits(r.Limits); return r }
func (r Register) Validate(day int) error {
	if day < 0 || r.Plan.Version == "" || r.Plan.ConstraintVersion != r.Limits.Version || r.Plan.SnapshotDay != day-1 || r.Limits.SnapshotDay != day-1 {
		return fmt.Errorf("invalid execution register identity/day")
	}
	for _, v := range r.Plan.Base {
		if !observation.FiniteNonnegative(v) {
			return fmt.Errorf("bad registered plan")
		}
	}
	return r.Limits.Validate()
}

type Resolution struct {
	Day                 int                 `json:"day"`
	Requested           *control.Assignment `json:"requested_assignment"`
	Action              plant.Action        `json:"authorized_action"`
	AppliedAssignmentID *string             `json:"authorized_assignment_id"`
	Reason              string              `json:"reason"`
	CancellationReasons []string            `json:"cancellation_reasons"`
	After               Register            `json:"register_after_successful_execution"`
}

func Resolve(day int, pending *control.Assignment, before Register) (Resolution, error) {
	out := Resolution{Day: day, Requested: control.CloneAssignment(pending), After: CloneRegister(before), CancellationReasons: []string{}}
	if e := before.Validate(day); e != nil {
		return out, e
	}
	out.Action, _ = plant.FixedAction("u0", nil)
	out.After.Plan.SnapshotDay = day
	out.After.Limits.SnapshotDay = day
	out.Reason = "NO_ASSIGNMENT_BASE_PLAN_CONTINUES_NOT_CERTIFIED_SAFE"
	if pending == nil {
		return out, nil
	}
	if e := pending.Validate(); e != nil {
		return out, e
	}
	if pending.ExecuteDay != day {
		out.CancellationReasons = append(out.CancellationReasons, "EXECUTION_DAY_MISMATCH")
	}
	if pending.Basis.PlanVersion != before.Plan.Version {
		out.CancellationReasons = append(out.CancellationReasons, "PLAN_VERSION_CHANGED")
	}
	if pending.Basis.ConstraintVersion != before.Limits.Version {
		out.CancellationReasons = append(out.CancellationReasons, "PERMISSION_VERSION_CHANGED")
	}
	if day < before.Limits.ValidFrom || day > before.Limits.ValidThrough {
		out.CancellationReasons = append(out.CancellationReasons, "PERMISSION_EXPIRED")
	}
	checks, e := control.CheckAction(pending.Action, before.Limits)
	if e != nil {
		return out, e
	}
	for _, c := range checks {
		if c.Residual > 0 {
			out.CancellationReasons = append(out.CancellationReasons, "HARD_CONSTRAINT:"+c.ID)
		}
	}
	if len(out.CancellationReasons) > 0 {
		out.Reason = "ASSIGNMENT_CANCELLED_BASE_PLAN_CONTINUES_NOT_CERTIFIED_SAFE"
		return out, nil
	}
	out.Action = control.CloneAction(pending.Action)
	id := pending.ID
	out.AppliedAssignmentID = &id
	out.Reason = "ASSIGNED_ACTION_AUTHORIZED_FOR_THIS_DAY"
	if pending.ExecutionMode == control.ResearchNoReview && pending.Route != "auto" {
		out.Reason = "RESEARCH_BYPASS_AUTHORIZED_NOT_HUMAN_APPROVAL"
	}
	if out.Action.PermanentPlan != nil && out.After.Plan.Base != [2]float64(*out.Action.PermanentPlan) {
		out.After.Plan.Base = [2]float64(*out.Action.PermanentPlan)
		h, _ := control.Hash(struct {
			Previous, Assignment string
			Plan                 [2]float64
		}{before.Plan.Version, id, out.After.Plan.Base})
		out.After.Plan.Version = "plan-v05-" + h
	}
	if out.Action.Cost > 0 {
		out.After.Limits.RemainingBudget -= out.Action.Cost
		h, _ := control.Hash(struct {
			Previous, Assignment string
			Remaining            float64
		}{before.Limits.Version, id, out.After.Limits.RemainingBudget})
		out.After.Limits.Version = "permissions-v05-" + h
	}
	out.After.Plan.ConstraintVersion = out.After.Limits.Version
	return out, out.After.Validate(day + 1)
}

// VerifyObservedContext compares only registered/executed facts, not sensors.
func VerifyObservedContext(p observation.Packet, r Resolution) error {
	if p.Day != r.Day {
		return fmt.Errorf("observation/actuation day mismatch")
	}
	if p.Context.ActionID != r.Action.ID || p.Context.Overtime != r.Action.Overtime || p.Context.PlanVersion != r.After.Plan.Version || p.Context.ConstraintVersion != r.After.Limits.Version {
		return fmt.Errorf("observation/execution context mismatch")
	}
	want := [2]float64{r.After.Plan.Base[0] * r.Action.TemporaryFactors[0], r.After.Plan.Base[1] * r.Action.TemporaryFactors[1]}
	if !reflect.DeepEqual(p.Context.ExecutedPlan, want) {
		return fmt.Errorf("executed plan context mismatch")
	}
	return nil
}
