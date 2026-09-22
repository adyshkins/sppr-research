package execution

import (
	"dissertation.local/sppr-reconstruction/control"
	"dissertation.local/sppr-reconstruction/forecast"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/plant"
	"reflect"
	"testing"
)

func reg(day int) Register {
	l := control.DefaultLimits()
	l.SnapshotDay = day - 1
	return Register{Plan: forecast.KnownPlan{Base: [2]float64{40, 25}, Version: "p", ConstraintVersion: l.Version, SnapshotDay: day - 1}, Limits: l}
}
func assignment(id string) *control.Assignment {
	r := reg(1)
	rp := plant.Pair{55, 32}
	a, _ := plant.FixedAction(id, &rp)
	h, _ := control.Hash("fixture")
	return &control.Assignment{ID: "case/0/next-day", ExecuteDay: 1, Action: a, Basis: control.Basis{ClosedDay: 0, InformationHash: h, PlanVersion: r.Plan.Version, ConstraintVersion: r.Limits.Version}, Route: "auto", ExecutionMode: control.HonorRoutes}
}
func TestEachDeclaredActionAuthorized(t *testing.T) {
	for _, id := range control.DefaultLimits().AllowedIDs {
		r, e := Resolve(1, assignment(id), reg(1))
		if e != nil || r.AppliedAssignmentID == nil || r.Action.ID != id {
			t.Fatal(id, e, r)
		}
	}
}
func TestNoPendingIsNotSelectedNoop(t *testing.T) {
	r, e := Resolve(0, nil, reg(0))
	if e != nil || r.Action.ID != "u0" || r.AppliedAssignmentID != nil || r.Requested != nil {
		t.Fatal(e, r)
	}
}
func TestOvertimeAndRawOneDayOnly(t *testing.T) {
	r, e := Resolve(1, assignment("u7"), reg(1))
	if e != nil {
		t.Fatal(e)
	}
	n, e := Resolve(2, nil, r.After)
	if e != nil || n.Action.Overtime != 0 || n.Action.ExtraRaw != 0 || n.Action.Cost != 0 {
		t.Fatal(e, n)
	}
}
func TestReplanPersistsInRegisterAndPlant(t *testing.T) {
	r, e := Resolve(1, assignment("u6"), reg(1))
	if e != nil || r.After.Plan.Base != [2]float64{55, 32} || r.After.Plan.Version == "p" {
		t.Fatal(e, r)
	}
	n, e := Resolve(2, nil, r.After)
	if e != nil || n.After.Plan.Base != r.After.Plan.Base || n.Action.PermanentPlan != nil {
		t.Fatal(e, n)
	}
	s := plant.DissertationInitialState()
	w := plant.Exogenous{Demand: plant.Pair{40, 25}, Capacity: 100, RawArrival: 90}
	a, _ := plant.Step(s, w, r.Action, plant.DissertationParameters())
	b, _ := plant.Step(a.Next, w, n.Action, plant.DissertationParameters())
	if b.Next.Plan != (plant.Pair{55, 32}) || b.ActionCost != 0 {
		t.Fatal(b)
	}
}
func TestTemporaryPriorityDoesNotChangeBase(t *testing.T) {
	r, e := Resolve(1, assignment("u4"), reg(1))
	if e != nil || r.After.Plan.Base != [2]float64{40, 25} || r.After.Plan.Version != "p" {
		t.Fatal(e)
	}
}
func TestBudgetSpentExactlyOnce(t *testing.T) {
	r, e := Resolve(1, assignment("u7"), reg(1))
	if e != nil || r.After.Limits.RemainingBudget != 100-.27 {
		t.Fatal(e)
	}
	n, e := Resolve(2, nil, r.After)
	if e != nil || n.After.Limits.RemainingBudget != r.After.Limits.RemainingBudget {
		t.Fatal(e)
	}
}
func TestBudgetZeroAllowsOnlyAffordableCommands(t *testing.T) {
	b := reg(1)
	b.Limits.RemainingBudget = 0
	r, e := Resolve(1, assignment("u1"), b)
	if e != nil || r.AppliedAssignmentID != nil || r.Action.ID != "u0" {
		t.Fatal(e, r)
	}
}
func TestLateAssignmentCancelled(t *testing.T) {
	r, e := Resolve(2, assignment("u1"), reg(2))
	if e != nil || r.AppliedAssignmentID != nil || r.CancellationReasons[0] != "EXECUTION_DAY_MISMATCH" {
		t.Fatal(e, r)
	}
}
func TestEarlyAssignmentCancelled(t *testing.T) {
	r, e := Resolve(0, assignment("u1"), reg(0))
	if e != nil || r.AppliedAssignmentID != nil {
		t.Fatal(e, r)
	}
}
func TestChangedPlanCancelsWithoutSilentReplan(t *testing.T) {
	b := reg(1)
	b.Plan.Version = "p2"
	b.Plan.Base = [2]float64{30, 20}
	r, e := Resolve(1, assignment("u6"), b)
	if e != nil || r.AppliedAssignmentID != nil || r.After.Plan.Base != b.Plan.Base {
		t.Fatal(e, r)
	}
}
func TestChangedPermissionVersionCancels(t *testing.T) {
	b := reg(1)
	b.Limits.Version = "other"
	b.Plan.ConstraintVersion = "other"
	r, e := Resolve(1, assignment("u1"), b)
	if e != nil || r.AppliedAssignmentID != nil {
		t.Fatal(e, r)
	}
}
func TestSameVersionDeniedPermissionRechecked(t *testing.T) {
	b := reg(1)
	b.Limits.MaxOvertime = 0
	r, e := Resolve(1, assignment("u1"), b)
	if e != nil || r.AppliedAssignmentID != nil {
		t.Fatal(e, r)
	}
}
func TestExpiredAuthorityCancels(t *testing.T) {
	b := reg(1)
	b.Limits.ValidThrough = 0
	r, e := Resolve(1, assignment("u1"), b)
	if e != nil || r.AppliedAssignmentID != nil {
		t.Fatal(e, r)
	}
}
func TestBudgetIsNotChargedForCancelledCommand(t *testing.T) {
	b := reg(1)
	b.Plan.Version = "p2"
	r, e := Resolve(1, assignment("u7"), b)
	if e != nil || r.After.Limits.RemainingBudget != 100 {
		t.Fatal(e, r)
	}
}
func TestAliasIsolation(t *testing.T) {
	a := assignment("u6")
	b := reg(1)
	r, e := Resolve(1, a, b)
	if e != nil {
		t.Fatal(e)
	}
	a.Action.PermanentPlan[0] = 999
	b.Limits.AllowedIDs[0] = "x"
	if r.Action.PermanentPlan[0] == 999 || r.After.Limits.AllowedIDs[0] == "x" {
		t.Fatal("aliased")
	}
	r.Requested.Action.PermanentPlan[0] = 777
	if r.Action.PermanentPlan[0] == 777 {
		t.Fatal("receipt aliased")
	}
}
func TestMalformedAssignmentIsErrorNotScientificRefusal(t *testing.T) {
	a := assignment("u7")
	a.Action.Cost = 0
	if _, e := Resolve(1, a, reg(1)); e == nil {
		t.Fatal("accepted")
	}
}
func TestRegistryDayAndIdentityRequired(t *testing.T) {
	b := reg(1)
	b.Plan.SnapshotDay = 1
	if _, e := Resolve(1, nil, b); e == nil {
		t.Fatal("same-day registry")
	}
	b = reg(1)
	b.Plan.ConstraintVersion = "other"
	if _, e := Resolve(1, nil, b); e == nil {
		t.Fatal("mismatch")
	}
}
func TestContextChecksActionAndPlan(t *testing.T) {
	r, _ := Resolve(1, assignment("u6"), reg(1))
	p := observation.Packet{Day: 1, Context: observation.Context{ActionID: "u6", Overtime: 0, PlanVersion: r.After.Plan.Version, ConstraintVersion: r.After.Limits.Version, ExecutedPlan: r.After.Plan.Base}}
	if e := VerifyObservedContext(p, r); e != nil {
		t.Fatal(e)
	}
	p.Context.ExecutedPlan[0]++
	if VerifyObservedContext(p, r) == nil {
		t.Fatal("bad plan")
	}
}
func TestResolvePureAndReplayable(t *testing.T) {
	b := reg(1)
	a := assignment("u7")
	r, e := Resolve(1, a, b)
	n, f := Resolve(1, a, b)
	if e != nil || f != nil || !reflect.DeepEqual(r, n) || b.Limits.RemainingBudget != 100 {
		t.Fatal(e, f)
	}
}
