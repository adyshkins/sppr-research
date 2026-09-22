package recoverycontrol

import (
	"dissertation.local/sppr-reconstruction/analysispipe"
	"dissertation.local/sppr-reconstruction/control"
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/execution"
	"dissertation.local/sppr-reconstruction/forecast"
	"dissertation.local/sppr-reconstruction/forecastpipe"
	"dissertation.local/sppr-reconstruction/internal/testfixture"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/recovery"
	"testing"
)

func fixture() (Config, control.Input) {
	f := forecast.DefaultConfig()
	f.PathsPerHypothesis = 2
	f.Horizon = 3
	c := Config{Base: control.Config{Pipeline: forecastpipe.Config{Analysis: testfixture.Config(analysispipe.EvidenceRequired), Forecast: f}, Choice: core.Config{Horizon: 3, Discount: .97, Alpha: .95, RiskWeight: .35, RiskLimit: 0}, Policy: core.FactorialPolicies()[3], Routing: control.DefaultRouting(), ExecutionMode: control.ResearchNoReview}, Recovery: recovery.Config{Policy: recovery.Tail, PermitResearchAssignment: true}}
	p := testfixture.Packet(0)
	p.Readings[observation.RawEnd] = observation.Read(90, 0)
	l := control.DefaultLimits()
	l.SnapshotDay = 0
	l.Version = p.Context.ConstraintVersion
	in := control.Input{CaseID: "fixture", Forecast: forecastpipe.Input{Packet: p, Plan: forecast.KnownPlan{Base: [2]float64{40, 25}, Version: p.Context.PlanVersion, ConstraintVersion: l.Version, SnapshotDay: 0}, ForecastSeed: 17}, Limits: l}
	return c, in
}
func TestExplicitExceptionKeepsPrimaryRefusal(t *testing.T) {
	c, i := fixture()
	x, _ := New(c, 0)
	o, e := x.Process(i)
	if e != nil {
		t.Fatal(e)
	}
	if o.Primary.Selection.Reason != "RISK_EMPTY" || o.Primary.Selection.Recommendation != nil || o.Primary.Selection.Assignment != nil || o.Assignment() == nil || o.Recovery.RiskCertified || o.Recovery.Selected.Excess <= 0 {
		t.Fatalf("%+v", o.Recovery)
	}
	if o.Assignment().ExecuteDay != 1 || o.Assignment().Route != "committee" {
		t.Fatal("route/time")
	}
}
func TestNoSeparateAuthorizationNoExceptionExecution(t *testing.T) {
	c, i := fixture()
	c.Recovery.PermitResearchAssignment = false
	x, _ := New(c, 0)
	o, e := x.Process(i)
	if e != nil || o.Recovery.Proposal == nil || o.Assignment() != nil {
		t.Fatal(e)
	}
}
func TestHonorRoutesNeverFabricatesHumanApproval(t *testing.T) {
	c, i := fixture()
	c.Base.ExecutionMode = control.HonorRoutes
	x, _ := New(c, 0)
	o, e := x.Process(i)
	if e != nil || o.Recovery.Proposal == nil || o.Assignment() != nil {
		t.Fatal(e)
	}
}
func TestDataIncidentNeverUsesRecovery(t *testing.T) {
	c, i := fixture()
	i.Forecast.Packet.Readings[observation.RawEnd] = observation.Missing()
	x, _ := New(c, 0)
	o, e := x.Process(i)
	if e != nil || o.Recovery.Triggered || o.Assignment() != nil {
		t.Fatal(e)
	}
}
func TestHardEmptyCannotBeSoftened(t *testing.T) {
	c, i := fixture()
	i.Limits.AllowedIDs = []string{}
	x, _ := New(c, 0)
	o, e := x.Process(i)
	if e != nil || o.Primary.Selection.Reason != "ADM_EMPTY" || o.Recovery.Triggered || o.Assignment() != nil {
		t.Fatal(e)
	}
}
func TestNormalDoesNotLaunchRecovery(t *testing.T) {
	c, i := fixture()
	i.Forecast.Packet.Readings[observation.RawEnd] = observation.Read(450, 0)
	x, _ := New(c, 0)
	o, e := x.Process(i)
	if e != nil || o.Primary.Selection.Reason != "NO_DEVIATION" || o.Recovery.Triggered {
		t.Fatal(e)
	}
}
func TestOrdinaryAdmissibleDecisionUnchanged(t *testing.T) {
	c, i := fixture()
	c.Base.Choice.RiskLimit = 100
	x, _ := New(c, 0)
	o, e := x.Process(i)
	if e != nil || o.Primary.Selection.Recommendation == nil || o.Recovery.Triggered || o.Recovery.Assignment != nil {
		t.Fatal(e)
	}
}
func TestZeroBudgetRestrictsExceptionToZeroCost(t *testing.T) {
	c, i := fixture()
	i.Limits.RemainingBudget = 0
	x, _ := New(c, 0)
	o, e := x.Process(i)
	if e != nil || o.Assignment() == nil || o.Assignment().Action.Cost != 0 {
		t.Fatal(e)
	}
}
func TestExecutionRechecksExceptionBudgetAndVersions(t *testing.T) {
	c, i := fixture()
	x, _ := New(c, 0)
	o, e := x.Process(i)
	if e != nil {
		t.Fatal(e)
	}
	a := o.Assignment()
	if a == nil {
		t.Fatal("no assignment")
	}
	reg := execution.Register{Plan: i.Forecast.Plan, Limits: i.Limits}
	reg.Limits.Version = "changed"
	reg.Plan.ConstraintVersion = "changed"
	r, e := execution.Resolve(1, a, reg)
	if e != nil || r.AppliedAssignmentID != nil || r.Action.ID != "u0" {
		t.Fatal(e)
	}
}
func TestFailureRollsBackMonitor(t *testing.T) {
	c, i := fixture()
	x, _ := New(c, 0)
	h, _ := control.Hash(x.Snapshot())
	i.Limits.RemainingBudget = -1
	if _, e := x.Process(i); e == nil {
		t.Fatal("invalid accepted")
	}
	g, _ := control.Hash(x.Snapshot())
	if g != h {
		t.Fatal("state committed on failure")
	}
}
