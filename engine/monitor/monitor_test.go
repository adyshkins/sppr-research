package monitor

import (
	"bytes"
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/observation"
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func packet(day int) observation.Packet {
	c := DefaultConfig()
	p := observation.Packet{Schema: observation.Schema, Day: day, Context: observation.Context{ExecutedPlan: [2]float64{40, 25}, ActionID: "u0", PlanVersion: "p1", ConstraintVersion: "c1"}, Readings: map[observation.Field]observation.Measurement{}}
	for _, f := range observation.Fields() {
		p.Readings[f] = observation.Read(c.InitialValues[f], 0)
	}
	return p
}
func makeMonitor(t *testing.T) *Monitor {
	t.Helper()
	m, e := New(DefaultConfig(), 0)
	if e != nil {
		t.Fatal(e)
	}
	return m
}
func mustObserve(t *testing.T, m *Monitor, p observation.Packet) Result {
	t.Helper()
	r, e := m.Observe(p)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func empty(p observation.Packet) observation.Packet {
	for f := range p.Readings {
		p.Readings[f] = observation.Missing()
	}
	return p
}
func shipment(p observation.Packet, ratio float64) observation.Packet {
	p.Readings[observation.ShippedA] = observation.Read(40*ratio, 0)
	p.Readings[observation.ShippedB] = observation.Read(25*ratio, 0)
	return p
}
func TestNormalSequence(t *testing.T) {
	m := makeMonitor(t)
	for i := 0; i < 60; i++ {
		r := mustObserve(t, m, packet(i))
		if r.Status != core.Normal || r.Event == nil || *r.Event || r.KPI == nil || r.Statistics.AcceptedSteps != i+1 {
			t.Fatalf("%+v", r)
		}
	}
}
func TestIncidentNullsAndFrozenState(t *testing.T) {
	m := makeMonitor(t)
	mustObserve(t, m, shipment(packet(0), .8))
	before := m.Snapshot()
	r := mustObserve(t, m, empty(packet(1)))
	after := m.Snapshot()
	if r.Status != core.DataIncident || r.KPI != nil || r.Delta != nil || r.Event != nil || r.Criticality != nil || r.Signals != nil || r.Resolution != nil || r.Computed {
		t.Fatalf("%+v", r)
	}
	if before.Statistics != after.Statistics || !reflect.DeepEqual(before.Cache, after.Cache) {
		t.Fatal("incident polluted state")
	}
	b, _ := json.Marshal(r)
	if !bytes.Contains(b, []byte(`"event":null`)) {
		t.Fatal(string(b))
	}
	r = mustObserve(t, m, packet(2))
	if !r.Resumed {
		t.Fatal("missing recovery transition")
	}
}
func TestImputationUsesLastAdmittedNotRejectedPacket(t *testing.T) {
	m := makeMonitor(t)
	mustObserve(t, m, packet(0))
	p := empty(packet(1))
	p.Readings[observation.RawEnd] = observation.Read(900, 0)
	r := mustObserve(t, m, p)
	if r.Computed {
		t.Fatal("bad packet accepted")
	}
	p = packet(2)
	p.Readings[observation.RawEnd] = observation.Missing()
	r = mustObserve(t, m, p)
	if r.KPI[3] != 5 || len(r.Resolution.Historical) != 1 || *r.Resolution.SourceSampleDays[observation.RawEnd] != 0 {
		t.Fatalf("%+v", r)
	}
}
func TestInitialImputationExplicit(t *testing.T) {
	m := makeMonitor(t)
	p := packet(0)
	p.Readings[observation.RawEnd] = observation.Missing()
	r := mustObserve(t, m, p)
	if len(r.Resolution.Initial) != 1 || r.Resolution.SourceSampleDays[observation.RawEnd] != nil {
		t.Fatalf("%+v", r)
	}
	if _, ok := m.Snapshot().Cache[observation.RawEnd]; ok {
		t.Fatal("initial estimate became measurement")
	}
}
func TestAllInvalidCompensationNotHidden(t *testing.T) {
	m := makeMonitor(t)
	p := packet(0)
	for f := range p.Readings {
		p.Readings[f] = observation.Read(-1, 0)
	}
	r := mustObserve(t, m, p)
	if !r.Computed || r.Status != core.Normal || r.Quality.Score != .7 || len(r.Resolution.Initial) != 15 || len(r.Warnings) == 0 || len(m.Snapshot().Cache) != 0 {
		t.Fatalf("counterexample changed without versioning: %+v", r)
	}
}
func TestCUSUMPositiveAndNegativeSmallShift(t *testing.T) {
	for _, ratio := range []float64{.96, 1.04} {
		m := makeMonitor(t)
		var r Result
		for i := 0; i < 11; i++ {
			r = mustObserve(t, m, shipment(packet(i), ratio))
			if r.Signals.Threshold[0] || r.Signals.EWMA[0] {
				t.Fatal("instant threshold/ewma should stay silent")
			}
			if i < 10 && *r.Event {
				t.Fatal("accumulation threshold crossed too soon")
			}
		}
		if !*r.Event {
			t.Fatal("CUSUM failed")
		}
		if ratio < 1 && !r.Signals.Minus[0] {
			t.Fatal("negative branch")
		}
		if ratio > 1 && !r.Signals.Plus[0] {
			t.Fatal("positive branch")
		}
	}
}
func TestEWMAEquationAndPersistence(t *testing.T) {
	m := makeMonitor(t)
	r := mustObserve(t, m, shipment(packet(0), .5))
	if math.Abs(r.Statistics.EWMA[0]+.1) > 1e-14 {
		t.Fatal(r.Statistics)
	}
	r = mustObserve(t, m, packet(1))
	if r.Signals.Threshold[0] || !r.Signals.EWMA[0] || !*r.Event {
		t.Fatal("statistical persistence lost")
	}
}
func TestThresholdBoundaryWithFloatingArithmetic(t *testing.T) {
	m := makeMonitor(t)
	r := mustObserve(t, m, shipment(packet(0), .95))
	if r.Signals.Threshold[0] {
		t.Fatal("equality misclassified through roundoff")
	}
	m = makeMonitor(t)
	r = mustObserve(t, m, shipment(packet(0), .95-1e-8))
	if !r.Signals.Threshold[0] {
		t.Fatal("meaningful excess hidden")
	}
}
func TestChronologyErrorIsAtomic(t *testing.T) {
	m := makeMonitor(t)
	before := m.Snapshot()
	if _, e := m.Observe(packet(1)); e == nil {
		t.Fatal("skip accepted")
	}
	if !reflect.DeepEqual(before, m.Snapshot()) {
		t.Fatal("mutated on invalid day")
	}
	mustObserve(t, m, packet(0))
	before = m.Snapshot()
	if _, e := m.Observe(packet(0)); e == nil {
		t.Fatal("duplicate accepted")
	}
	if !reflect.DeepEqual(before, m.Snapshot()) {
		t.Fatal("duplicate polluted state")
	}
}
func TestMalformedPacketIsAtomic(t *testing.T) {
	m := makeMonitor(t)
	before := m.Snapshot()
	p := packet(0)
	delete(p.Readings, observation.RawEnd)
	if _, e := m.Observe(p); e == nil {
		t.Fatal("bad schema")
	}
	if !reflect.DeepEqual(before, m.Snapshot()) {
		t.Fatal("bad schema consumed step")
	}
}
func TestSnapshotAndConfigCannotMutateMonitor(t *testing.T) {
	c := DefaultConfig()
	m, e := New(c, 0)
	if e != nil {
		t.Fatal(e)
	}
	c.InitialValues[observation.RawEnd] = 999
	p := packet(0)
	p.Readings[observation.RawEnd] = observation.Missing()
	r := mustObserve(t, m, p)
	if r.KPI[3] != 5 {
		t.Fatal("configuration alias")
	}
	s := m.Snapshot()
	s.Cache[observation.DemandA] = Cached{Value: 1}
	*s.PreviousComputed = false
	if m.Snapshot().Cache[observation.DemandA].Value != 40 || !*m.Snapshot().PreviousComputed {
		t.Fatal("snapshot alias")
	}
}
func TestOlderDelayedValueDoesNotReplaceNewerCache(t *testing.T) {
	m := makeMonitor(t)
	mustObserve(t, m, packet(0))
	p := packet(1)
	p.Readings[observation.RawEnd] = observation.Read(900, 2)
	r := mustObserve(t, m, p)
	if r.KPI[3] != 10 || len(r.Resolution.CacheNotUpdated) != 1 {
		t.Fatal("stale observation semantics changed")
	}
	p = packet(2)
	p.Readings[observation.RawEnd] = observation.Missing()
	r = mustObserve(t, m, p)
	if r.KPI[3] != 5 {
		t.Fatal("old delivery overwrote newer cache")
	}
}
func TestUndefinedKPIBecomesIncidentWithoutStatePollution(t *testing.T) {
	m := makeMonitor(t)
	mustObserve(t, m, packet(0))
	before := m.Snapshot()
	p := packet(1)
	p.Readings[observation.Capacity] = observation.Read(0, 0)
	r := mustObserve(t, m, p)
	if !r.Quality.GatePassed || r.Computed || r.Reason != "KPI_UNDEFINED" || r.KPI != nil || r.Status != core.DataIncident {
		t.Fatalf("%+v", r)
	}
	after := m.Snapshot()
	if before.Statistics != after.Statistics || !reflect.DeepEqual(before.Cache, after.Cache) {
		t.Fatal("undefined KPI corrupted state")
	}
}
func TestCriticalityOnlyForDeviation(t *testing.T) {
	m := makeMonitor(t)
	r := mustObserve(t, m, packet(0))
	if r.Criticality != nil {
		t.Fatal("normal criticality policy")
	}
	r = mustObserve(t, m, shipment(packet(1), .5))
	if r.Criticality == nil || *r.Criticality < 0 || *r.Criticality > 1 {
		t.Fatal("invalid criticality")
	}
}
func TestGateToExistingDecisionCore(t *testing.T) {
	m := makeMonitor(t)
	r := mustObserve(t, m, empty(packet(0)))
	c := core.Case{ID: "incident", InformationVersion: "v2", Status: r.Status, Config: core.Config{Horizon: 7, Discount: .97, Alpha: .95, RiskWeight: .35, RiskLimit: 1.25}}
	ds, e := core.CompareFactors(c)
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range ds {
		if d.RecommendationID != nil || d.Reason != "DATA_CHECK" {
			t.Fatal(d)
		}
	}
}
func TestPrefixDoesNotDependOnFuture(t *testing.T) {
	a := makeMonitor(t)
	b := makeMonitor(t)
	for i := 0; i < 12; i++ {
		pa := shipment(packet(i), .98)
		ra := mustObserve(t, a, pa)
		rb := mustObserve(t, b, observation.Clone(pa))
		if !reflect.DeepEqual(ra, rb) {
			t.Fatal("prefix mismatch")
		}
	}
	old := a.Snapshot()
	mustObserve(t, a, shipment(packet(12), .2))
	if old.NextDay != 12 || b.Snapshot().NextDay != 12 {
		t.Fatal("cross-instance state leakage")
	}
}
func TestInvalidMonitorConfig(t *testing.T) {
	c := DefaultConfig()
	c.EWMAWeight = 0
	if _, e := New(c, 0); e == nil {
		t.Fatal("invalid config")
	}
	if _, e := New(DefaultConfig(), -1); e == nil {
		t.Fatal("invalid start")
	}
}
