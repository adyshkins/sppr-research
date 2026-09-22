package forecast

import (
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/internal/testfixture"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/plant"
	"math"
	"reflect"
	"testing"
)

func estimate() Estimate {
	return Estimate{ClosedDay: 0, Raw: 450, Finished: [2]float64{20, 12.5}, Backlog: [2]float64{}, Plan: [2]float64{40, 25}, Demand: [2]float64{40, 25}, Capacity: 100, Arrival: 90, Origin: "current_readings_point_estimate_no_initial_state_sampling"}
}
func diagnosis(x Estimate, ids []string, ps []float64) diagnostics.Result {
	c := DefaultConfig()
	bits := [3]bool{x.Demand[0]+x.Demand[1] > 1.1*65, x.Arrival < .85*c.BaseRaw, x.Capacity < .85*c.BaseCapacity}
	d := diagnostics.Result{Status: "completed", Ran: true, ForecastAllowed: true, Relevant: append([]string(nil), ids...), Features: []diagnostics.Feature{}}
	for j, id := range []string{"demand_high", "supply_low", "capacity_low"} {
		v := bits[j]
		d.Features = append(d.Features, diagnostics.Feature{ID: id, Value: &v})
	}
	for i, id := range ids {
		d.Ranked = append(d.Ranked, diagnostics.Ranked{ID: id, Posterior: ps[i], Score: 1 - ps[i]})
		d.ForecastWeights = append(d.ForecastWeights, diagnostics.Weighted{ID: id, Probability: ps[i]})
	}
	return d
}
func baselineInput() (observation.Packet, KnownPlan) {
	p := testfixture.Packet(0)
	return p, KnownPlan{Base: [2]float64{40, 25}, Version: p.Context.PlanVersion, ConstraintVersion: p.Context.ConstraintVersion, SnapshotDay: 0}
}
func deterministic(x Estimate) Ensemble {
	e := Ensemble{Version: Version, ClosedDay: x.ClosedDay, Paths: []ScenarioPath{{ID: "only", Probability: 1, Days: make([]plant.Exogenous, 7)}}}
	for i := range e.Paths[0].Days {
		e.Paths[0].Days[i] = plant.Exogenous{Demand: plant.Pair{40, 25}, Capacity: 100, RawArrival: 90}
	}
	return e
}
func near(t *testing.T, a, b float64) {
	t.Helper()
	if math.Abs(a-b) > 1e-10*math.Max(1, math.Max(math.Abs(a), math.Abs(b))) {
		t.Fatalf("got %.16g want %.16g", a, b)
	}
}
func TestDefaultsValid(t *testing.T) {
	if e := DefaultConfig().Validate(); e != nil {
		t.Fatal(e)
	}
}
func TestInvalidConfigs(t *testing.T) {
	tests := []func(*Config){func(c *Config) { c.Horizon = 0 }, func(c *Config) { c.Horizon = 61 }, func(c *Config) { c.PathsPerHypothesis = 0 }, func(c *Config) { c.RecoveryProbability = 1.1 }, func(c *Config) { c.RelativeSD = math.NaN() }, func(c *Config) { c.NoiseLaw = "implicit" }, func(c *Config) { c.Templates[0].ID = "unknown" }, func(c *Config) { c.Templates[1] = c.Templates[0] }, func(c *Config) { c.Templates[0].Fallback[1] = .5 }, func(c *Config) { c.BaseCapacity = 99 }, func(c *Config) { c.Quantiles = [2]float64{.9, .1} }, func(c *Config) { c.FeatureRatios[0] = 0 }}
	for _, mut := range tests {
		c := DefaultConfig()
		mut(&c)
		if e := c.Validate(); e == nil {
			t.Fatal("accepted invalid config")
		}
	}
}
func TestAdmissionUsesReadings(t *testing.T) {
	p, k := baselineInput()
	p.Readings[observation.RawEnd] = observation.Read(123, 0)
	a, e := Assess(p, k, DefaultConfig())
	if e != nil || !a.Passed {
		t.Fatalf("%v %+v", e, a)
	}
	if a.Estimate.Raw != 123 || a.Estimate.Raw == 450 {
		t.Fatal("not initialized from observed state")
	}
}
func TestForecastRequiresAllSelectedFields(t *testing.T) {
	for _, f := range RequiredFields() {
		p, k := baselineInput()
		p.Readings[f] = observation.Missing()
		a, e := Assess(p, k, DefaultConfig())
		if e != nil || a.Passed || a.Estimate != nil {
			t.Fatalf("accepted missing %s %v", f, e)
		}
	}
}
func TestForecastRejectsStaleFields(t *testing.T) {
	for _, f := range RequiredFields() {
		p, k := baselineInput()
		p.Readings[f] = observation.Read(*p.Readings[f].Value, 1)
		a, e := Assess(p, k, DefaultConfig())
		if e != nil || a.Passed {
			t.Fatalf("accepted stale %s", f)
		}
	}
}
func TestForecastRejectsNegativeState(t *testing.T) {
	p, k := baselineInput()
	p.Readings[observation.FinishedA] = observation.Read(-1, 0)
	a, e := Assess(p, k, DefaultConfig())
	if e != nil || a.Passed {
		t.Fatal("accepted negative state")
	}
}
func TestPlanVersionAndDay(t *testing.T) {
	for _, mut := range []func(*KnownPlan){func(k *KnownPlan) { k.Version = "other" }, func(k *KnownPlan) { k.ConstraintVersion = "other" }, func(k *KnownPlan) { k.SnapshotDay++ }, func(k *KnownPlan) { k.Base[0] = 80 }} {
		p, k := baselineInput()
		mut(&k)
		a, e := Assess(p, k, DefaultConfig())
		if e != nil || a.Passed {
			t.Fatal("accepted wrong register")
		}
	}
}
func TestTemporaryPlanDoesNotBecomeBase(t *testing.T) {
	p, k := baselineInput()
	p.Context.ActionID = "u4"
	p.Context.ExecutedPlan = [2]float64{48, 20}
	a, e := Assess(p, k, DefaultConfig())
	if e != nil || !a.Passed || a.Estimate.Plan != [2]float64{40, 25} {
		t.Fatalf("%v %+v", e, a)
	}
}
func TestPermanentKnownPlan(t *testing.T) {
	p, k := baselineInput()
	p.Context.ActionID = "u6"
	k.Base = [2]float64{44, 27}
	p.Context.ExecutedPlan = k.Base
	a, e := Assess(p, k, DefaultConfig())
	if e != nil || !a.Passed || a.Estimate.Plan != k.Base {
		t.Fatal("new base plan lost")
	}
}
func TestBacklogBalanceAndComplementarity(t *testing.T) {
	p, k := baselineInput()
	p.Readings[observation.BacklogAfterA] = observation.Read(20, 0)
	a, e := Assess(p, k, DefaultConfig())
	if e != nil || a.Passed || len(a.Reasons) < 2 {
		t.Fatal("inconsistent state accepted")
	}
}
func TestNoMutationDuringAdmission(t *testing.T) {
	p, k := baselineInput()
	before := observation.Clone(p)
	a, e := Assess(p, k, DefaultConfig())
	if e != nil {
		t.Fatal(e)
	}
	a.Estimate.Plan[0] = 777
	if !reflect.DeepEqual(before, p) || k.Base[0] != 40 {
		t.Fatal("input changed")
	}
}
func TestDeterministicEnsemble(t *testing.T) {
	x := estimate()
	d := diagnosis(x, []string{"D", "S"}, []float64{.3, .7})
	a, e := Generate(x, d, DefaultConfig(), 9)
	if e != nil {
		t.Fatal(e)
	}
	b, e := Generate(x, d, DefaultConfig(), 9)
	if e != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("nonrepeatable")
	}
	other, _ := Generate(x, d, DefaultConfig(), 10)
	if reflect.DeepEqual(a, other) {
		t.Fatal("seed ignored")
	}
}
func TestPosteriorMassNotScore(t *testing.T) {
	x := estimate()
	d := diagnosis(x, []string{"D", "S"}, []float64{.2, .8})
	e, err := Generate(x, d, DefaultConfig(), 1)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]float64{}
	for _, p := range e.Paths {
		m[p.Hypothesis] += p.Probability
	}
	near(t, m["D"], .2)
	near(t, m["S"], .8)
	if len(e.Paths) != 128 {
		t.Fatal("path count")
	}
}
func TestScoreChangesDoNotChangeEnsemble(t *testing.T) {
	x := estimate()
	d := diagnosis(x, []string{"D", "S"}, []float64{.2, .8})
	a, _ := Generate(x, d, DefaultConfig(), 1)
	d.Ranked[0].Score = 999
	d.Ranked[1].Score = 0
	b, e := Generate(x, d, DefaultConfig(), 1)
	if e != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("score affected scenario probabilities")
	}
}
func TestInvalidDiagnosticInputs(t *testing.T) {
	for _, mut := range []func(*diagnostics.Result){func(d *diagnostics.Result) { d.ForecastAllowed = false }, func(d *diagnostics.Result) { d.Features[0].Value = nil }, func(d *diagnostics.Result) { d.ForecastWeights[0].Probability = .9 }, func(d *diagnostics.Result) { d.Relevant = append(d.Relevant, "unknown") }, func(d *diagnostics.Result) { d.Ranked[0].Posterior = 2 }, func(d *diagnostics.Result) { *d.Features[0].Value = true }} {
		x := estimate()
		d := diagnosis(x, []string{"S"}, []float64{1})
		mut(&d)
		if _, e := Generate(x, d, DefaultConfig(), 1); e == nil {
			t.Fatal("bad diagnosis accepted")
		}
	}
}
func TestObservedSeverityUsed(t *testing.T) {
	x := estimate()
	x.Demand = [2]float64{70, 40}
	x.Arrival = 11
	d := diagnosis(x, []string{"DS"}, []float64{1})
	c := DefaultConfig()
	c.RelativeSD = 0
	e, err := Generate(x, d, c, 1)
	if err != nil {
		t.Fatal(err)
	}
	g := e.Groups[0]
	if g.ActiveMeans.Demand != plant.Pair(x.Demand) || g.ActiveMeans.RawArrival != 11 || g.SeveritySource[0] != "current_observation" {
		t.Fatal("observed event ignored")
	}
}
func TestAbsentSymptomFallbackDeclared(t *testing.T) {
	x := estimate()
	d := diagnosis(x, []string{"S"}, []float64{1})
	e, err := Generate(x, d, DefaultConfig(), 1)
	if err != nil {
		t.Fatal(err)
	}
	near(t, e.Groups[0].ActiveMeans.RawArrival, 27)
	if e.Groups[0].SeveritySource[1] != "declared_fallback" {
		t.Fatal("hidden fallback")
	}
}
func TestRecoveryAfterFirstDay(t *testing.T) {
	x := estimate()
	d := diagnosis(x, []string{"S"}, []float64{1})
	c := DefaultConfig()
	c.RelativeSD = 0
	c.RecoveryProbability = 1
	e, err := Generate(x, d, c, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range e.Paths {
		if p.Days[0].RawArrival != 27 {
			t.Fatal("premature recovery")
		}
		for _, w := range p.Days[1:] {
			if w.RawArrival != 90 {
				t.Fatal("not recovered")
			}
		}
	}
}
func TestNoRecoveryAtZeroProbability(t *testing.T) {
	x := estimate()
	d := diagnosis(x, []string{"S"}, []float64{1})
	c := DefaultConfig()
	c.RelativeSD = 0
	c.RecoveryProbability = 0
	e, err := Generate(x, d, c, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range e.Paths {
		for _, w := range p.Days {
			if w.RawArrival != 27 {
				t.Fatal("spontaneous recovery")
			}
		}
	}
}
func TestDSJointRecovery(t *testing.T) {
	x := estimate()
	d := diagnosis(x, []string{"DS"}, []float64{1})
	c := DefaultConfig()
	c.RelativeSD = 0
	e, err := Generate(x, d, c, 42)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range e.Paths {
		for _, w := range p.Days {
			if (w.RawArrival == 90) != (w.Demand[0] == 40) {
				t.Fatal("joint hypothesis recovered separately")
			}
		}
	}
}
func TestAddingHypothesisDoesNotResampleExisting(t *testing.T) {
	x := estimate()
	one, _ := Generate(x, diagnosis(x, []string{"S"}, []float64{1}), DefaultConfig(), 42)
	two, _ := Generate(x, diagnosis(x, []string{"D", "S"}, []float64{.4, .6}), DefaultConfig(), 42)
	for i, p := range one.Paths {
		if !reflect.DeepEqual(p.Days, two.Paths[i+64].Days) {
			t.Fatal("other hypothesis shifted stream")
		}
	}
}
func TestQuantileAtomsAndZeroWeights(t *testing.T) {
	q, e := Quantile([]float64{-100, 1, 3, 999}, []float64{0, .9, .1, 0}, .95)
	if e != nil || q != 3 {
		t.Fatal(q, e)
	}
	q, e = Quantile([]float64{-100, 1, 3}, []float64{0, .9, .1}, 0)
	if e != nil || q != 1 {
		t.Fatal(q, e)
	}
}
func TestQuantileRejectsScores(t *testing.T) {
	if _, e := Quantile([]float64{1, 2}, []float64{4, 6}, .5); e == nil {
		t.Fatal("scores accepted")
	}
	if _, e := Quantile([]float64{math.NaN()}, []float64{1}, .5); e == nil {
		t.Fatal("nan accepted")
	}
}
func TestBaselineHandCalculation(t *testing.T) {
	x := estimate()
	as, _ := Alternatives(x, DefaultConfig())
	r, e := Evaluate(x, deterministic(x), DefaultConfig(), as[:1])
	if e != nil {
		t.Fatal(e)
	}
	if r[0].Risk.Expected != 0 || r[0].Risk.CVaR != 0 {
		t.Fatal("baseline loss")
	}
	for _, k := range r[0].Trajectories[0].KPI {
		if k != [4]float64{1, 1, .8, 5} {
			t.Fatal(k)
		}
	}
}
func TestOneOffActionAndCost(t *testing.T) {
	x := estimate()
	as, _ := Alternatives(x, DefaultConfig())
	r, e := Evaluate(x, deterministic(x), DefaultConfig(), as)
	if e != nil {
		t.Fatal(e)
	}
	near(t, r[1].Risk.Expected, .05)
	near(t, r[2].Risk.Expected, .12+.15*(.7-80.0/120)/.7)
	for _, k := range r[2].Trajectories[0].KPI[1:] {
		near(t, k[2], .8)
	}
	near(t, r[3].Trajectories[0].FinalPredictedState.Raw, 510)
}
func TestPermanentReplanPersistsWithoutRepeatedCost(t *testing.T) {
	x := estimate()
	as, _ := Alternatives(x, DefaultConfig())
	r, e := Evaluate(x, deterministic(x), DefaultConfig(), as[6:7])
	if e != nil {
		t.Fatal(e)
	}
	s := r[0].Trajectories[0].FinalPredictedState
	if s.Plan != (plant.Pair{38, 23.75}) {
		t.Fatal(s.Plan)
	}
	near(t, s.Raw, 450+7*4.5)
	near(t, r[0].Risk.Expected, .05)
}
func TestAlternativeOrderingAndInputImmutability(t *testing.T) {
	x := estimate()
	e := deterministic(x)
	as, _ := Alternatives(x, DefaultConfig())
	r, err := Evaluate(x, e, DefaultConfig(), as)
	if err != nil {
		t.Fatal(err)
	}
	for i, j := 0, len(as)-1; i < j; i, j = i+1, j-1 {
		as[i], as[j] = as[j], as[i]
	}
	r2, err := Evaluate(x, e, DefaultConfig(), as)
	if err != nil || !reflect.DeepEqual(r, r2) {
		t.Fatal("order matters")
	}
	if x.Raw != 450 || e.Paths[0].Days[0].RawArrival != 90 {
		t.Fatal("modified input")
	}
}
func TestUnregisteredActionAndDuplicateRejected(t *testing.T) {
	x := estimate()
	as, _ := Alternatives(x, DefaultConfig())
	as[1].Cost = 0
	if _, e := Evaluate(x, deterministic(x), DefaultConfig(), as); e == nil {
		t.Fatal("modified cost accepted")
	}
	as, _ = Alternatives(x, DefaultConfig())
	as[1] = as[0]
	if _, e := Evaluate(x, deterministic(x), DefaultConfig(), as); e == nil {
		t.Fatal("duplicate accepted")
	}
}
func TestBadEnsembleRejected(t *testing.T) {
	x := estimate()
	as, _ := Alternatives(x, DefaultConfig())
	for _, mut := range []func(*Ensemble){func(e *Ensemble) { e.ClosedDay = 1 }, func(e *Ensemble) { e.Paths[0].Probability = .5 }, func(e *Ensemble) { e.Paths[0].Days = e.Paths[0].Days[:6] }, func(e *Ensemble) { e.Paths[0].Days[0].Capacity = -1 }} {
		e := deterministic(x)
		mut(&e)
		if _, err := Evaluate(x, e, DefaultConfig(), as); err == nil {
			t.Fatal("bad ensemble")
		}
	}
}
