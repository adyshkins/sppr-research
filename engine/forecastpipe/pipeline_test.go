package forecastpipe

import (
	"dissertation.local/sppr-reconstruction/analysispipe"
	"dissertation.local/sppr-reconstruction/forecast"
	"dissertation.local/sppr-reconstruction/internal/testfixture"
	"dissertation.local/sppr-reconstruction/observation"
	"reflect"
	"testing"
)

func testConfig() Config {
	c := Config{testfixture.Config(analysispipe.EvidenceRequired), forecast.DefaultConfig()}
	c.Forecast.PathsPerHypothesis = 2
	c.Forecast.Horizon = 3
	return c
}
func input(day int) Input {
	p := testfixture.Packet(day)
	p.Readings[observation.RawEnd] = observation.Read(90, 0)
	return Input{p, forecast.KnownPlan{Base: [2]float64{40, 25}, Version: p.Context.PlanVersion, ConstraintVersion: p.Context.ConstraintVersion, SnapshotDay: day}, 9}
}
func TestKPIEnoughButForecastNotEnough(t *testing.T) {
	p, e := New(testConfig(), 0)
	if e != nil {
		t.Fatal(e)
	}
	in := input(0)
	in.Packet.Readings[observation.FinishedA] = observation.Missing()
	r, e := p.Process(in)
	if e != nil {
		t.Fatal(e)
	}
	if !r.Analysis.Monitor.Computed || !r.Analysis.Diagnosis.ForecastAllowed || r.Forecast.Status != "data_check" || r.Forecast.Ensemble != nil {
		t.Fatal("stage-specific sufficiency failed")
	}
	if p.Snapshot().NextDay != 1 {
		t.Fatal("valid refusal did not consume day")
	}
}
func TestFreshRecoveryAfterForecastRefusal(t *testing.T) {
	p, _ := New(testConfig(), 0)
	in := input(0)
	in.Packet.Readings[observation.BacklogAfterA] = observation.Missing()
	_, e := p.Process(in)
	if e != nil {
		t.Fatal(e)
	}
	r, e := p.Process(input(1))
	if e != nil || r.Forecast.Status != "ready" {
		t.Fatal("forecast did not recover", e)
	}
}
func TestMalformedDownstreamDoesNotCommitMonitor(t *testing.T) {
	p, _ := New(testConfig(), 0)
	before := p.Snapshot()
	in := input(0)
	in.Plan.Version = ""
	if _, e := p.Process(in); e == nil {
		t.Fatal("bad register accepted")
	}
	if !reflect.DeepEqual(before, p.Snapshot()) {
		t.Fatal("partial state commit")
	}
	if _, e := p.Process(input(0)); e != nil {
		t.Fatal(e)
	}
}
func TestLowConfidenceNotExecutionPermission(t *testing.T) {
	c := testConfig()
	c.Analysis.Diagnosis.MinimumConfidence = 1
	p, _ := New(c, 0)
	r, e := p.Process(input(0))
	if e != nil || r.Forecast.Status != "ready" || !r.Forecast.RequiresExpert {
		t.Fatal("expert requirement dropped", e)
	}
}
func TestNoForecastFromNormalOrMissing(t *testing.T) {
	p, _ := New(testConfig(), 0)
	in := input(0)
	in.Packet = testfixture.Packet(0)
	r, e := p.Process(in)
	if e != nil || r.Forecast.Status != "not_required" || r.Forecast.Ensemble != nil {
		t.Fatal("forecast for normal", e)
	}
	in = input(1)
	for f := range in.Packet.Readings {
		in.Packet.Readings[f] = observation.Missing()
	}
	r, e = p.Process(in)
	if e != nil || r.Forecast.Status != "data_check" || r.Forecast.Ensemble != nil {
		t.Fatal("forecast with absent data")
	}
}
func TestBaseParameterMismatch(t *testing.T) {
	c := testConfig()
	c.Analysis.Diagnosis.BaseRaw = 91
	if _, e := New(c, 0); e == nil {
		t.Fatal("inconsistent baselines")
	}
}
func TestConfigIsCopied(t *testing.T) {
	c := testConfig()
	p, _ := New(c, 0)
	c.Analysis.Diagnosis.Hypotheses[0].Causes[0] = "changed"
	c.Forecast.Templates[0].Fallback[0] = 10
	a, e := p.Process(input(0))
	q, _ := New(testConfig(), 0)
	b, f := q.Process(input(0))
	if e != nil || f != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("external config mutated pipeline")
	}
}
func TestAnalysisUnaffectedByAddingForecast(t *testing.T) {
	c := testConfig()
	p, _ := New(c, 0)
	q, _ := analysispipe.New(c.Analysis, 0)
	for day := 0; day < 5; day++ {
		in := input(day)
		a, e := p.Process(in)
		b, f := q.Process(in.Packet)
		if e != nil || f != nil || !reflect.DeepEqual(a.Analysis, b) || !reflect.DeepEqual(p.Snapshot(), q.Snapshot()) {
			t.Fatal("changed old analysis")
		}
	}
}
func TestChangingObservationChangesEstimateNotTruth(t *testing.T) {
	p, _ := New(testConfig(), 0)
	q, _ := New(testConfig(), 0)
	a, e := p.Process(input(0))
	in := input(0)
	in.Packet.Readings[observation.RawEnd] = observation.Read(180, 0)
	b, f := q.Process(in)
	if e != nil || f != nil {
		t.Fatal(e, f)
	}
	if a.Forecast.Admission.Estimate.Raw != 90 || b.Forecast.Admission.Estimate.Raw != 180 {
		t.Fatal("forecast ignored observation")
	}
}
