package diagnostics_test

import (
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/internal/testfixture"
	"dissertation.local/sppr-reconstruction/monitor"
	"dissertation.local/sppr-reconstruction/observation"
	"math"
	"reflect"
	"testing"
)

func mon(t *testing.T, p observation.Packet) monitor.Result {
	t.Helper()
	m, e := monitor.New(monitor.DefaultConfig(), p.Day)
	if e != nil {
		t.Fatal(e)
	}
	r, e := m.Observe(p)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func problem() observation.Packet {
	p := testfixture.Packet(0)
	p.Readings[observation.RawEnd] = observation.Read(90, 0)
	return p
}
func infer(t *testing.T, p observation.Packet, m diagnostics.Model) diagnostics.Result {
	t.Helper()
	r, e := diagnostics.Diagnose(p, mon(t, p), m)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func near(t *testing.T, x, y float64) {
	t.Helper()
	if math.Abs(x-y) > 1e-12 {
		t.Fatalf("got %.17g want %.17g", x, y)
	}
}
func TestNormalDoesNotRunDiagnosis(t *testing.T) {
	r := infer(t, testfixture.Packet(0), testfixture.Model())
	if r.Status != "not_required" || r.Ran || r.Leading != nil || r.Confidence != nil || len(r.Ranked) != 0 {
		t.Fatal(r)
	}
}
func TestDataIncidentNotPhysicalHypothesis(t *testing.T) {
	p := testfixture.Packet(0)
	for f := range p.Readings {
		p.Readings[f] = observation.Missing()
	}
	r := infer(t, p, testfixture.Model())
	if r.Status != "data_check" || r.Ran || len(r.Candidates) != 0 {
		t.Fatal(r)
	}
}
func TestAbsentSymptomDiffersFromMissingReading(t *testing.T) {
	p := problem()
	m := testfixture.Model()
	f, e := diagnostics.ExtractFeatures(p, m)
	if e != nil || f[1].Value == nil || *f[1].Value {
		t.Fatal(f, e)
	}
	p.Readings[observation.RawArrival] = observation.Missing()
	f, e = diagnostics.ExtractFeatures(p, m)
	if e != nil || f[1].Value != nil {
		t.Fatal(f, e)
	}
	r := infer(t, p, m)
	if r.Ran || r.Status != "data_check" {
		t.Fatal(r)
	}
}
func TestStaleDiagnosticFeatureIsUnknown(t *testing.T) {
	p := problem()
	p.Readings[observation.RawArrival] = observation.Read(40, 1)
	r := infer(t, p, testfixture.Model())
	if r.Ran || r.Features[1].Value != nil || r.Reason != "DIAGNOSTIC_FEATURES_UNAVAILABLE" {
		t.Fatal(r)
	}
}
func TestStrictBinaryThresholdBoundaries(t *testing.T) {
	p := testfixture.Packet(0)
	p.Readings[observation.DemandA] = observation.Read(46.5, 0)
	p.Readings[observation.RawArrival] = observation.Read(76.5, 0)
	p.Readings[observation.Capacity] = observation.Read(85, 0)
	f, e := diagnostics.ExtractFeatures(p, testfixture.Model())
	if e != nil {
		t.Fatal(e)
	}
	for _, x := range f {
		if x.Value == nil || *x.Value {
			t.Fatal(f)
		}
	}
}
func TestAllEightBernoulliPatternsAgainstIndependentProduct(t *testing.T) {
	m := testfixture.Model()
	for mask := 0; mask < 8; mask++ {
		p := problem()
		bits := [3]bool{mask&1 != 0, mask&2 != 0, mask&4 != 0}
		if bits[0] {
			p.Readings[observation.DemandA] = observation.Read(60, 0)
		}
		if bits[1] {
			p.Readings[observation.RawArrival] = observation.Read(45, 0)
		}
		if bits[2] {
			p.Readings[observation.Capacity] = observation.Read(70, 0)
		}
		r := infer(t, p, m)
		if len(r.Ranked) != 4 {
			t.Fatal(mask, r)
		}
		probs := map[string]float64{}
		total := 0.0
		for _, h := range m.Hypotheses {
			x := h.Prior
			for i, b := range bits {
				if b {
					x *= h.Likelihood[i]
				} else {
					x *= 1 - h.Likelihood[i]
				}
			}
			probs[h.ID] = x
			total += x
		}
		sum := 0.0
		for _, h := range r.Ranked {
			near(t, h.Posterior, probs[h.ID]/total)
			sum += h.Posterior
		}
		near(t, sum, 1)
	}
}
func TestCombinedHypothesisRemainsConfiguration(t *testing.T) {
	p := problem()
	p.Readings[observation.DemandA] = observation.Read(60, 0)
	p.Readings[observation.RawArrival] = observation.Read(40, 0)
	r := infer(t, p, testfixture.Model())
	if r.Leading == nil || *r.Leading != "DS" || !reflect.DeepEqual(r.Ranked[0].Causes, []string{"D", "S"}) {
		t.Fatal(r)
	}
}
func TestRankingScoresAreNotProbabilities(t *testing.T) {
	r := infer(t, problem(), testfixture.Model())
	sum := 0.0
	for _, x := range r.Ranked {
		sum += x.Score
	}
	if math.Abs(sum-1) < 1e-6 {
		t.Fatal("fixture failed to distinguish score and posterior")
	}
	mass := 0.0
	for _, id := range r.Relevant {
		for _, x := range r.Ranked {
			if x.ID == id {
				mass += x.Posterior
			}
		}
	}
	for _, w := range r.ForecastWeights {
		for _, x := range r.Ranked {
			if x.ID == w.ID {
				near(t, w.Probability, x.Posterior/mass)
			}
		}
	}
}
func TestDirectionAndMagnitudeEquation(t *testing.T) {
	p := problem()
	r := infer(t, p, testfixture.Model())
	for _, x := range r.Ranked {
		switch x.ID {
		case "S":
			near(t, x.Impact, .25/(1+1e-12))
		case "C":
			near(t, x.Impact, 0)
		case "D":
			near(t, x.Impact, .25/(.6+1e-12))
		}
	}
}
func TestConfidenceIsRankMarginNotPosterior(t *testing.T) {
	r := infer(t, problem(), testfixture.Model())
	near(t, *r.Confidence, mon(t, problem()).Quality.Score*(r.Ranked[0].Score-r.Ranked[1].Score))
	if r.Status != "expert_review" || !r.ForecastAllowed || !r.RequiresExpert {
		t.Fatal(r)
	}
}
func TestEmptyStructuralCandidatesRequestsReview(t *testing.T) {
	m := testfixture.Model()
	m.Edges = nil
	for i := range m.Hypotheses {
		m.Hypotheses[i].Strength = [4]float64{}
	}
	r := infer(t, problem(), m)
	if r.Status != "expert_review" || r.Reason != "NO_STRUCTURAL_CANDIDATES" || r.Ran || r.ForecastAllowed || r.Leading != nil {
		t.Fatal(r)
	}
}
func TestNoRelevantHypothesisNoForecast(t *testing.T) {
	m := testfixture.Model()
	m.Relevance = 1
	r := infer(t, problem(), m)
	if r.Reason != "NO_RELEVANT_HYPOTHESES" || r.ForecastAllowed || !r.RequiresExpert || !r.Ran {
		t.Fatal(r)
	}
}
func TestSingleCandidateConfidence(t *testing.T) {
	m := testfixture.Model()
	m.Hypotheses = m.Hypotheses[1:2]
	m.Hypotheses[0].Prior = 1
	r := infer(t, problem(), m)
	if len(r.Ranked) != 1 {
		t.Fatal(r)
	}
	near(t, *r.Confidence, mon(t, problem()).Quality.Score*r.Ranked[0].Score)
}
func TestDeterministicTieBreaking(t *testing.T) {
	m := testfixture.Model()
	m.ScoreWeights = [3]float64{1, 0, 0}
	for i := range m.Hypotheses {
		m.Hypotheses[i].Likelihood = [3]float64{.5, .5, .5}
	}
	r := infer(t, problem(), m)
	want := []string{"C", "D", "DS", "S"}
	for i, x := range r.Ranked {
		if x.ID != want[i] {
			t.Fatal(r)
		}
	}
	near(t, *r.Confidence, 0)
}
func TestModelOrderDoesNotChangeOutput(t *testing.T) {
	m := testfixture.Model()
	a := infer(t, problem(), m)
	for i, j := 0, len(m.Hypotheses)-1; i < j; i, j = i+1, j-1 {
		m.Hypotheses[i], m.Hypotheses[j] = m.Hypotheses[j], m.Hypotheses[i]
	}
	b := infer(t, problem(), m)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("input order leaks into rank")
	}
}
func TestInvalidModelsRejected(t *testing.T) {
	changes := []func(*diagnostics.Model){func(m *diagnostics.Model) { m.Provenance.Description = "" }, func(m *diagnostics.Model) { m.Hypotheses[0].Likelihood[0] = 0 }, func(m *diagnostics.Model) { m.Hypotheses[0].Likelihood[0] = 1 }, func(m *diagnostics.Model) { m.Hypotheses[0].Prior = -1 }, func(m *diagnostics.Model) { m.Edges = append(m.Edges, m.Edges[0]) }, func(m *diagnostics.Model) { m.Hypotheses[0].Strength[1] = 1 }, func(m *diagnostics.Model) { m.ScoreWeights[0] = 2 }, func(m *diagnostics.Model) { m.Epsilon = math.NaN() }, func(m *diagnostics.Model) { m.Hypotheses[0].Direction[0] = 2 }}
	for i, change := range changes {
		m := testfixture.Model()
		change(&m)
		if m.Validate() == nil {
			t.Fatal("invalid model", i)
		}
	}
}
func TestInconsistentMonitorResultRejected(t *testing.T) {
	p := problem()
	r := mon(t, p)
	r.Event = nil
	if _, e := diagnostics.Diagnose(p, r, testfixture.Model()); e == nil {
		t.Fatal("missing event")
	}
	r = mon(t, p)
	r.Day++
	if _, e := diagnostics.Diagnose(p, r, testfixture.Model()); e == nil {
		t.Fatal("wrong day")
	}
}
func TestExtremeLikelihoodsRemainFinite(t *testing.T) {
	m := testfixture.Model()
	for i := range m.Hypotheses {
		m.Hypotheses[i].Likelihood = [3]float64{1e-200, 1e-200, 1e-200}
	}
	p := problem()
	p.Readings[observation.DemandA] = observation.Read(60, 0)
	p.Readings[observation.RawArrival] = observation.Read(40, 0)
	p.Readings[observation.Capacity] = observation.Read(70, 0)
	r := infer(t, p, m)
	for _, x := range r.Ranked {
		near(t, x.Posterior, .25)
	}
}

func TestGraphFilteringUsesPathsAndAllSignalKinds(t *testing.T) {
	m := testfixture.Model()
	p := problem()
	mr := mon(t, p)
	// Only K0 is signalled. D reaches it through an intermediate resource.
	m.Edges = []diagnostics.Edge{{From: "D", To: "resource"}, {From: "resource", To: "K0"}, {From: "S", To: "K1"}, {From: "C", To: "K2"}}
	for i := range m.Hypotheses {
		m.Hypotheses[i].Strength = [4]float64{}
	}
	mr.Signals = &monitor.Signals{EWMA: [4]bool{true, false, false, false}}
	r, e := diagnostics.Diagnose(p, mr, m)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(r.Candidates, []string{"D", "DS"}) {
		t.Fatal(r.Candidates)
	}
	near(t, r.Ranked[0].Prior, .5)
}
func TestStructuralNormalisationUsesWholeGraph(t *testing.T) {
	m := testfixture.Model()
	p := problem()
	mr := mon(t, p)
	mr.Signals = &monitor.Signals{Threshold: [4]bool{true, false, false, false}}
	m.Edges = []diagnostics.Edge{{From: "D", To: "K0"}, {From: "S", To: "K1"}, {From: "C", To: "K1"}, {From: "C", To: "K2"}, {From: "C", To: "K3"}}
	for i := range m.Hypotheses {
		m.Hypotheses[i].Strength = [4]float64{}
	}
	r, e := diagnostics.Diagnose(p, mr, m)
	if e != nil {
		t.Fatal(e)
	}
	for _, x := range r.Ranked {
		near(t, x.Structural, 1/(3+1e-12))
	}
}
