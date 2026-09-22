package analysispipe_test

import (
	"dissertation.local/sppr-reconstruction/analysispipe"
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/internal/testfixture"
	"dissertation.local/sppr-reconstruction/monitor"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/sufficiency"
	"reflect"
	"testing"
)

func pipe(t *testing.T, mode string) *analysispipe.Pipeline {
	t.Helper()
	p, e := analysispipe.New(testfixture.Config(mode), 0)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func step(t *testing.T, p *analysispipe.Pipeline, o observation.Packet) analysispipe.Result {
	t.Helper()
	r, e := p.Process(o)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestBaselineMonitorUnchanged(t *testing.T) {
	a := pipe(t, analysispipe.AggregateOnly)
	m, e := monitor.New(monitor.DefaultConfig(), 0)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range testfixture.Packets() {
		r := step(t, a, p)
		old, e := m.Observe(p)
		if e != nil || !reflect.DeepEqual(old, r.Monitor) || !reflect.DeepEqual(a.Snapshot(), m.Snapshot()) {
			t.Fatalf("changed baseline day %d: %v", p.Day, e)
		}
	}
}
func TestAllInvalidNoLongerNormal(t *testing.T) {
	p := testfixture.Packet(0)
	for f := range p.Readings {
		p.Readings[f] = observation.Read(-1, 0)
	}
	base := step(t, pipe(t, analysispipe.AggregateOnly), p)
	guard := step(t, pipe(t, analysispipe.EvidenceRequired), p)
	if base.Monitor.Status != core.Normal || guard.Monitor.Status != core.DataIncident || !guard.Monitor.Quality.GatePassed || guard.Monitor.Reason != "INSUFFICIENT_INFORMATION" || guard.Monitor.KPI != nil || guard.Monitor.Event != nil || guard.Diagnosis.Ran || guard.Diagnosis.Status != "data_check" {
		t.Fatal(base, guard)
	}
}
func TestFreshKPIDistinguishedFromDiagnosticData(t *testing.T) {
	p := testfixture.Packet(0)
	p.Readings[observation.RawEnd] = observation.Read(90, 0)
	p.Readings[observation.RawArrival] = observation.Missing()
	r := step(t, pipe(t, analysispipe.EvidenceRequired), p)
	if !r.Admission.Passed || r.Monitor.Status != core.Deviation || r.Diagnosis.Status != "data_check" || r.Diagnosis.Reason != "DIAGNOSTIC_FEATURES_UNAVAILABLE" || r.Diagnosis.Ran {
		t.Fatal(r)
	}
}
func TestBlockedDayDoesNotUpdateHistoryOrStatistics(t *testing.T) {
	a := pipe(t, analysispipe.EvidenceRequired)
	p := testfixture.Packet(0)
	p.Readings[observation.RawEnd] = observation.Read(90, 0)
	step(t, a, p)
	before := a.Snapshot()
	p = testfixture.Packet(1)
	p.Readings[observation.RawEnd] = observation.Missing()
	r := step(t, a, p)
	after := a.Snapshot()
	if r.Monitor.Computed || after.NextDay != 2 || after.Statistics != before.Statistics || !reflect.DeepEqual(after.Cache, before.Cache) {
		t.Fatal("blocked step corrupted state")
	}
	r = step(t, a, testfixture.Packet(2))
	if !r.Monitor.Resumed || !r.Monitor.Computed {
		t.Fatal(r)
	}
}
func TestMalformedDayAtomic(t *testing.T) {
	a := pipe(t, analysispipe.EvidenceRequired)
	before := a.Snapshot()
	if _, e := a.Process(testfixture.Packet(2)); e == nil {
		t.Fatal("skipped day")
	}
	if !reflect.DeepEqual(before, a.Snapshot()) {
		t.Fatal("state changed")
	}
	p := testfixture.Packet(0)
	delete(p.Readings, observation.RawEnd)
	if _, e := a.Process(p); e == nil {
		t.Fatal("bad schema")
	}
	if !reflect.DeepEqual(before, a.Snapshot()) {
		t.Fatal("state changed")
	}
}
func TestRepeatedImputationDoesNotRenewHistory(t *testing.T) {
	c := testfixture.Config(analysispipe.EvidenceRequired)
	c.Admission = sufficiency.Config{Policy: sufficiency.BoundedRawHistory, RawStockMaxAge: 1}
	a, e := analysispipe.New(c, 0)
	if e != nil {
		t.Fatal(e)
	}
	step(t, a, testfixture.Packet(0))
	for day := 1; day <= 2; day++ {
		p := testfixture.Packet(day)
		p.Readings[observation.RawEnd] = observation.Missing()
		r := step(t, a, p)
		if r.Monitor.Computed != (day == 1) {
			t.Fatal(day, r)
		}
		if a.Snapshot().Cache[observation.RawEnd].SampleDay != 0 {
			t.Fatal("imputed sample became new evidence")
		}
	}
}
func TestMissingRawAtStartNotPriorEvidence(t *testing.T) {
	c := testfixture.Config(analysispipe.EvidenceRequired)
	c.Admission = sufficiency.Config{Policy: sufficiency.BoundedRawHistory, RawStockMaxAge: 1}
	a, e := analysispipe.New(c, 0)
	if e != nil {
		t.Fatal(e)
	}
	p := testfixture.Packet(0)
	p.Readings[observation.RawEnd] = observation.Missing()
	if step(t, a, p).Monitor.Status != core.DataIncident {
		t.Fatal("initial constant admitted")
	}
}
func TestQualityGateStillRequired(t *testing.T) {
	a := pipe(t, analysispipe.EvidenceRequired)
	p := testfixture.Packet(0)
	for f := range p.Readings {
		p.Readings[f] = observation.Missing()
	}
	r := step(t, a, p)
	if r.Monitor.Reason != "QUALITY_BELOW_MINIMUM" {
		t.Fatal(r)
	}
}
func TestConfigAndResultsCannotMutatePipeline(t *testing.T) {
	c := testfixture.Config(analysispipe.EvidenceRequired)
	a, e := analysispipe.New(c, 0)
	if e != nil {
		t.Fatal(e)
	}
	c.Diagnosis.Hypotheses[0].Likelihood[0] = .01
	c.Diagnosis.Hypotheses[0].Causes[0] = "changed"
	c.Monitor.InitialValues[observation.RawEnd] = 0
	b := pipe(t, analysispipe.EvidenceRequired)
	p := testfixture.Packet(0)
	p.Readings[observation.RawEnd] = observation.Read(90, 0)
	x := step(t, a, p)
	y := step(t, b, p)
	if !reflect.DeepEqual(x, y) {
		t.Fatal("config alias")
	}
	x.Monitor.Resolution.Values[observation.RawEnd] = 5000
	if a.Snapshot().Cache[observation.RawEnd].Value != 90 {
		t.Fatal("result alias")
	}
}
func TestInvalidPipelineConfig(t *testing.T) {
	c := testfixture.Config("bad")
	if _, e := analysispipe.New(c, 0); e == nil {
		t.Fatal("mode")
	}
	c = testfixture.Config(analysispipe.EvidenceRequired)
	c.Diagnosis.KPIThreshold[0] = .5
	if _, e := analysispipe.New(c, 0); e == nil {
		t.Fatal("inconsistent scale")
	}
}
func TestBlockedCaseHasNoAutomaticCorrectionInExistingCore(t *testing.T) {
	p := testfixture.Packet(0)
	p.Readings[observation.RawEnd] = observation.Missing()
	r := step(t, pipe(t, analysispipe.EvidenceRequired), p)
	d, e := core.CompareFactors(core.Case{ID: "missing", InformationVersion: "v3", Status: r.Monitor.Status, Config: core.Config{Horizon: 7, Discount: .97, Alpha: .95, RiskWeight: .35, RiskLimit: 1.25}})
	if e != nil {
		t.Fatal(e)
	}
	for _, x := range d {
		if x.RecommendationID != nil || x.Reason != "DATA_CHECK" {
			t.Fatal(x)
		}
	}
}
func TestSeparateInstancesSamePrefix(t *testing.T) {
	a := pipe(t, analysispipe.EvidenceRequired)
	b := pipe(t, analysispipe.EvidenceRequired)
	for _, p := range testfixture.Packets() {
		if !reflect.DeepEqual(step(t, a, p), step(t, b, observation.Clone(p))) {
			t.Fatal("prefix nondeterministic")
		}
	}
}
