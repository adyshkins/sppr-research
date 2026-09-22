package control

import (
	"dissertation.local/sppr-reconstruction/analysispipe"
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/forecast"
	"dissertation.local/sppr-reconstruction/forecastpipe"
	"dissertation.local/sppr-reconstruction/internal/testfixture"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/plant"
	"math"
	"reflect"
	"testing"
)

func cfg() Config {
	f := forecast.DefaultConfig()
	f.PathsPerHypothesis = 2
	f.Horizon = 3
	return Config{Pipeline: forecastpipe.Config{Analysis: testfixture.Config(analysispipe.EvidenceRequired), Forecast: f}, Choice: core.Config{Horizon: 3, Discount: .97, Alpha: .95, RiskWeight: .35, RiskLimit: 1.25}, Policy: core.FactorialPolicies()[0], Routing: DefaultRouting(), ExecutionMode: ResearchNoReview}
}
func input(day int) Input {
	p := testfixture.Packet(day)
	p.Readings[observation.RawEnd] = observation.Read(90, 0)
	l := DefaultLimits()
	l.SnapshotDay = day
	l.Version = p.Context.ConstraintVersion
	return Input{CaseID: "case", Forecast: forecastpipe.Input{Packet: p, Plan: forecast.KnownPlan{Base: [2]float64{40, 25}, Version: p.Context.PlanVersion, ConstraintVersion: l.Version, SnapshotDay: day}, ForecastSeed: 9}, Limits: l}
}
func forecasted(t *testing.T) (Config, Input, forecastpipe.Result) {
	t.Helper()
	c := cfg()
	p, e := forecastpipe.New(c.Pipeline, 0)
	if e != nil {
		t.Fatal(e)
	}
	i := input(0)
	r, e := p.Process(i.Forecast)
	if e != nil || r.Forecast.Status != "ready" {
		t.Fatal(e, r.Forecast.Status)
	}
	return c, i, r
}
func TestFullSelectionSchedulesOnlyNextDay(t *testing.T) {
	c := cfg()
	p, _ := New(c, 0)
	r, e := p.Process(input(0))
	if e != nil || r.Selection.Assignment == nil || r.Selection.Assignment.ExecuteDay != 1 {
		t.Fatal(e, r.Selection)
	}
	if p.Snapshot().NextDay != 1 {
		t.Fatal("not committed")
	}
}
func TestFactorialUsesSameForecast(t *testing.T) {
	c, i, r := forecasted(t)
	before, _ := Hash(r)
	for _, policy := range core.FactorialPolicies() {
		c.Policy = policy
		s, e := Select(i, r, c)
		if e != nil {
			t.Fatal(e)
		}
		for _, v := range s.Decision.Evaluations {
			if v.Risk != nil {
				if policy.RiskConstraint && v.Objective != nil && !*v.RiskAdmissible {
					t.Fatal("risk bypass")
				}
			}
		}
	}
	after, _ := Hash(r)
	if before != after {
		t.Fatal("forecast mutated")
	}
}
func TestExplicitRiskEmptyNotNormal(t *testing.T) {
	c, i, r := forecasted(t)
	c.Policy = core.FactorialPolicies()[2]
	c.Choice.RiskLimit = 0
	s, e := Select(i, r, c)
	if e != nil || s.Reason != "RISK_EMPTY" || s.Assignment != nil || s.Route != "no_solution" {
		t.Fatal(e, s)
	}
}
func TestExplicitHardEmptyIncludesNoop(t *testing.T) {
	c, i, r := forecasted(t)
	i.Limits.AllowedIDs = []string{}
	s, e := Select(i, r, c)
	if e != nil || s.Reason != "ADM_EMPTY" || s.Assignment != nil {
		t.Fatal(e, s)
	}
}
func TestHonorReviewDoesNotApprove(t *testing.T) {
	c, i, r := forecasted(t)
	c.ExecutionMode = HonorRoutes
	r.Forecast.RequiresExpert = true
	s, e := Select(i, r, c)
	if e != nil || s.Recommendation == nil || s.Assignment != nil || s.Route == "auto" {
		t.Fatal(e, s)
	}
}
func TestResearchBypassExplicit(t *testing.T) {
	c, i, r := forecasted(t)
	r.Forecast.RequiresExpert = true
	s, e := Select(i, r, c)
	if e != nil || s.Assignment == nil || s.AssignmentReason != "RESEARCH_NO_REVIEW_BYPASS_NOT_EXPERT_APPROVAL" {
		t.Fatal(e, s)
	}
}
func TestPermissionVersionMismatchBlocks(t *testing.T) {
	c, i, r := forecasted(t)
	i.Limits.Version = "new"
	s, e := Select(i, r, c)
	if e != nil || s.Route != "data_check" || s.Decision != nil {
		t.Fatal(e, s)
	}
}
func TestPermissionTimestampMismatchBlocks(t *testing.T) {
	c, i, r := forecasted(t)
	i.Limits.SnapshotDay = -1
	s, e := Select(i, r, c)
	if e != nil || s.Route != "data_check" {
		t.Fatal(e, s)
	}
}
func TestExpiredPermissionBlocks(t *testing.T) {
	c, i, r := forecasted(t)
	i.Limits.ValidThrough = 0
	s, e := Select(i, r, c)
	if e != nil || s.Route != "data_check" {
		t.Fatal(e, s)
	}
}
func TestLowQualityNoDecision(t *testing.T) {
	c := cfg()
	p, _ := New(c, 0)
	i := input(0)
	for f := range i.Forecast.Packet.Readings {
		i.Forecast.Packet.Readings[f] = observation.Missing()
	}
	r, e := p.Process(i)
	if e != nil || r.Selection.Route != "data_check" || r.Selection.Assignment != nil {
		t.Fatal(e, r.Selection)
	}
}
func TestNormalNoCorrection(t *testing.T) {
	c := cfg()
	p, _ := New(c, 0)
	i := input(0)
	i.Forecast.Packet = testfixture.Packet(0)
	r, e := p.Process(i)
	if e != nil || r.Selection.Route != "normal" || r.Selection.Recommendation != nil {
		t.Fatal(e, r.Selection)
	}
}
func TestMissingForecastStateBlocksOnlyDownstream(t *testing.T) {
	c := cfg()
	p, _ := New(c, 0)
	i := input(0)
	i.Forecast.Packet.Readings[observation.FinishedA] = observation.Missing()
	r, e := p.Process(i)
	if e != nil || !r.Pipeline.Analysis.Monitor.Computed || r.Selection.Assignment != nil || r.Selection.Route != "data_check" {
		t.Fatal(e)
	}
}
func TestExpertReviewReasonNotDataDefect(t *testing.T) {
	c, i, r := forecasted(t)
	r.Forecast.Status = "expert_review"
	r.Forecast.Reason = "NO_CANDIDATE"
	s, e := Select(i, r, c)
	if e != nil || s.Route != "expert" || s.Decision != nil {
		t.Fatal(e, s)
	}
}
func TestStructuralFailureAtomic(t *testing.T) {
	c := cfg()
	p, _ := New(c, 0)
	before := p.Snapshot()
	i := input(0)
	i.Limits.RemainingBudget = math.NaN()
	if _, e := p.Process(i); e == nil {
		t.Fatal("accepted")
	}
	if !reflect.DeepEqual(before, p.Snapshot()) {
		t.Fatal("partial commit")
	}
	if _, e := p.Process(input(0)); e != nil {
		t.Fatal(e)
	}
}
func TestLateStructuralFailureAtomic(t *testing.T) {
	p, _ := New(cfg(), 0)
	i := input(0)
	i.Forecast.Plan.Version = ""
	before := p.Snapshot()
	if _, e := p.Process(i); e == nil {
		t.Fatal("accepted")
	}
	if !reflect.DeepEqual(before, p.Snapshot()) {
		t.Fatal("partial commit")
	}
}
func TestConfigMismatchRejected(t *testing.T) {
	c := cfg()
	c.Choice.Horizon++
	if _, e := New(c, 0); e == nil {
		t.Fatal("mismatch")
	}
	c = cfg()
	c.ExecutionMode = "implicit_approval"
	if _, e := New(c, 0); e == nil {
		t.Fatal("mode")
	}
}
func TestControllerCopiesConfiguration(t *testing.T) {
	c := cfg()
	p, _ := New(c, 0)
	c.Pipeline.Analysis.Diagnosis.Hypotheses[0].Likelihood[0] = .01
	r, e := p.Process(input(0))
	q, _ := New(cfg(), 0)
	s, f := q.Process(input(0))
	if e != nil || f != nil || !reflect.DeepEqual(r, s) {
		t.Fatal(e, f)
	}
}
func TestSelectionDoesNotAliasActions(t *testing.T) {
	c, i, r := forecasted(t)
	s, e := Select(i, r, c)
	if e != nil {
		t.Fatal(e)
	}
	if s.Recommendation != nil && s.Recommendation.PermanentPlan != nil {
		s.Recommendation.PermanentPlan[0] = 999
		if s.Assignment.Action.PermanentPlan[0] == 999 {
			t.Fatal("aliased")
		}
	}
}
func TestDeclinedReviewCannotBeForgedIntoAssignment(t *testing.T) {
	c, i, r := forecasted(t)
	r.Forecast.RequiresExpert = true
	s, _ := Select(i, r, c)
	a := *s.Assignment
	a.ExecutionMode = HonorRoutes
	if a.Validate() == nil {
		t.Fatal("review accepted")
	}
}
func TestAssignmentMalformedTimeAndHash(t *testing.T) {
	c, i, r := forecasted(t)
	s, _ := Select(i, r, c)
	a := *s.Assignment
	a.ExecuteDay = 0
	if a.Validate() == nil {
		t.Fatal("same-day accepted")
	}
	a = *s.Assignment
	a.Basis.InformationHash = "no"
	if a.Validate() == nil {
		t.Fatal("hash")
	}
}
func TestNineNamedConstraintsAllActions(t *testing.T) {
	l := DefaultLimits()
	rp := plant.Pair{40, 25}
	for _, id := range l.AllowedIDs {
		a, _ := plant.FixedAction(id, &rp)
		cs, e := CheckAction(a, l)
		if e != nil || len(cs) != 9 {
			t.Fatal(e)
		}
		for _, c := range cs {
			if c.Residual > 0 {
				t.Fatalf("default excludes %s %s", id, c.ID)
			}
		}
	}
}
func TestAuthorizationNotPhysicalClipping(t *testing.T) {
	l := DefaultLimits()
	a, _ := plant.FixedAction("u7", nil)
	l.MaxExtraRaw = 0
	cs, _ := CheckAction(a, l)
	found := false
	for _, c := range cs {
		if c.ID == "extra_raw_permission" && c.Residual > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("no permission filter")
	}
}
func TestSingleCostAndBudgetConstraints(t *testing.T) {
	l := DefaultLimits()
	l.RemainingBudget = .04
	l.MaxActionCost = .04
	a, _ := plant.FixedAction("u1", nil)
	cs, _ := CheckAction(a, l)
	n := 0
	for _, v := range cs {
		if v.Residual > 0 {
			n++
		}
	}
	if n != 2 {
		t.Fatal(n)
	}
}
func TestNoopIsNotResourceSafetyCertificate(t *testing.T) {
	l := DefaultLimits()
	a, _ := plant.FixedAction("u0", nil)
	cs, e := CheckAction(a, l)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range cs {
		if v.Residual > 0 {
			t.Fatal("no-op invalid")
		}
	}
	s := plant.DissertationInitialState()
	s.Raw = 0
	r, e := plant.Step(s, plant.Exogenous{Demand: plant.Pair{40, 25}}, a, plant.DissertationParameters())
	if e != nil || r.KPI[1] != 0 {
		t.Fatal(e, "shortage concealed")
	}
}
func TestReplanBoundsEnforced(t *testing.T) {
	l := DefaultLimits()
	rp := plant.Pair{71, 25}
	a, _ := plant.FixedAction("u6", &rp)
	cs, _ := CheckAction(a, l)
	found := false
	for _, v := range cs {
		if v.ID == "replan_a_upper" && v.Residual > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("out of bound plan accepted")
	}
}
func TestActionLibraryTamperRejected(t *testing.T) {
	l := DefaultLimits()
	a, _ := plant.FixedAction("u1", nil)
	a.Cost = 0
	if _, e := CheckAction(a, l); e == nil {
		t.Fatal("cost tamper")
	}
}
func TestBadLimitsRejected(t *testing.T) {
	for _, mut := range []func(*Limits){func(l *Limits) { l.MaxOvertime = -1 }, func(l *Limits) { l.ReplanMin[0] = 100 }, func(l *Limits) { l.AllowedIDs = []string{"u0", "u0"} }, func(l *Limits) { l.AllowedIDs = []string{"x"} }, func(l *Limits) { l.ValidThrough = -1 }, func(l *Limits) { l.Version = "" }} {
		l := DefaultLimits()
		mut(&l)
		if l.Validate() == nil {
			t.Fatal("invalid allowed")
		}
	}
}
func TestDistanceUsesRequestedPlan(t *testing.T) {
	k := forecast.KnownPlan{Base: [2]float64{40, 25}}
	a, _ := plant.FixedAction("u4", nil)
	d, e := PlanDistance(a, k)
	if e != nil || math.Abs(d-.2) > 1e-12 {
		t.Fatal(d, e)
	}
}
func TestRouteBoundaries(t *testing.T) {
	r := DefaultRouting()
	for _, x := range []struct {
		risk, dist float64
		expert     bool
		want       string
	}{{.34, .09, false, "auto"}, {.35, 0, false, "expert"}, {.75, 0, false, "committee"}, {0, .10, false, "expert"}, {0, .25, false, "committee"}, {0, 0, true, "expert"}} {
		s, e := r.Route(x.risk, x.dist, x.expert)
		if e != nil || s != x.want {
			t.Fatal(s, e)
		}
	}
}
func TestBadRouteInput(t *testing.T) {
	r := DefaultRouting()
	if _, e := r.Route(math.NaN(), 0, false); e == nil {
		t.Fatal("NaN")
	}
	r.ExpertRisk = r.CommitteeRisk
	if r.Validate() == nil {
		t.Fatal("thresholds")
	}
}
