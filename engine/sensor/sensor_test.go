package sensor

import (
	"bytes"
	"dissertation.local/sppr-reconstruction/monitor"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/plant"
	"encoding/json"
	"reflect"
	"testing"
)

func closed(t *testing.T, day int) observation.Packet {
	t.Helper()
	s := plant.DissertationInitialState()
	w := plant.Exogenous{Demand: plant.Pair{40, 25}, Capacity: 100, RawArrival: 90}
	a, _ := plant.FixedAction("u0", nil)
	r, e := plant.Step(s, w, a, plant.DissertationParameters())
	if e != nil {
		t.Fatal(e)
	}
	p, e := ClosedDay(day, s, w, a, r, "plan1", "constraints1")
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func channel(t *testing.T) *Channel {
	t.Helper()
	c, e := New(0)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func TestPerfectChannelMatchesPlantKPI(t *testing.T) {
	state := plant.DissertationInitialState()
	ch := channel(t)
	m, _ := monitor.New(monitor.DefaultConfig(), 0)
	for day := 0; day < 120; day++ {
		a, _ := plant.FixedAction("u0", nil)
		w := plant.Exogenous{Demand: plant.Pair{40 + float64(day%3), 25}, Capacity: 100, RawArrival: 90}
		r, e := plant.Step(state, w, a, plant.DissertationParameters())
		if e != nil {
			t.Fatal(e)
		}
		p, e := ClosedDay(day, state, w, a, r, "p", "c")
		if e != nil {
			t.Fatal(e)
		}
		p, e = ch.Transmit(p, nil)
		if e != nil {
			t.Fatal(e)
		}
		mr, e := m.Observe(p)
		if e != nil || mr.KPI == nil || *mr.KPI != r.KPI {
			t.Fatalf("day %d: %+v %+v %v", day, mr, r.KPI, e)
		}
		state = r.Next
	}
}
func TestDelayRetrievesSampleWithoutResampling(t *testing.T) {
	c := channel(t)
	p := closed(t, 0)
	p.Readings[observation.RawEnd] = observation.Read(100, 0)
	first, e := c.Transmit(p, map[observation.Field]Distortion{observation.RawEnd: {RelativeError: .1}})
	if e != nil {
		t.Fatal(e)
	}
	second, e := c.Transmit(closed(t, 1), map[observation.Field]Distortion{observation.RawEnd: {LagDays: 1, RelativeError: .9}})
	if e != nil {
		t.Fatal(e)
	}
	a, b := first.Readings[observation.RawEnd], second.Readings[observation.RawEnd]
	if *a.Value != *b.Value || b.AgeDays != 1 {
		t.Fatalf("%+v %+v", a, b)
	}
}
func TestChannelBlackoutAndUnavailableDelay(t *testing.T) {
	c := channel(t)
	d := map[observation.Field]Distortion{}
	for _, f := range observation.Fields() {
		d[f] = Distortion{Missing: true}
	}
	p, e := c.Transmit(closed(t, 0), d)
	if e != nil {
		t.Fatal(e)
	}
	q, _ := observation.EvaluateQuality(p, observation.DissertationQualityConfig())
	if q.Score != 0 {
		t.Fatal(q)
	}
	p, e = c.Transmit(closed(t, 1), map[observation.Field]Distortion{observation.RawEnd: {LagDays: 3}})
	if e != nil || p.Readings[observation.RawEnd].Present {
		t.Fatal("future/absent history fabricated")
	}
}
func TestNoiseIsNotClippedAndReturnedPacketIndependent(t *testing.T) {
	c := channel(t)
	p, e := c.Transmit(closed(t, 0), map[observation.Field]Distortion{observation.RawEnd: {RelativeError: -2}})
	if e != nil || *p.Readings[observation.RawEnd].Value != -450 {
		t.Fatal("silent clipping")
	}
	*p.Readings[observation.RawEnd].Value = 999
	p, e = c.Transmit(closed(t, 1), map[observation.Field]Distortion{observation.RawEnd: {LagDays: 1}})
	if e != nil || *p.Readings[observation.RawEnd].Value != -450 {
		t.Fatal("state alias")
	}
}
func TestInvalidDistortionDoesNotConsumeDay(t *testing.T) {
	c := channel(t)
	if _, e := c.Transmit(closed(t, 0), map[observation.Field]Distortion{"future": {}}); e == nil {
		t.Fatal("unknown channel")
	}
	if _, e := c.Transmit(closed(t, 0), nil); e != nil {
		t.Fatal(e)
	}
}
func TestTrueKPIIsNotUsedByAdapter(t *testing.T) {
	s := plant.DissertationInitialState()
	w := plant.Exogenous{Demand: plant.Pair{40, 25}, Capacity: 100, RawArrival: 90}
	a, _ := plant.FixedAction("u0", nil)
	r, _ := plant.Step(s, w, a, plant.DissertationParameters())
	p, _ := ClosedDay(0, s, w, a, r, "p", "c")
	r.KPI = [4]float64{777, 888, 999, 1000}
	q, _ := ClosedDay(0, s, w, a, r, "p", "c")
	if !reflect.DeepEqual(p, q) {
		t.Fatal("true KPI leaked")
	}
	raw, _ := json.Marshal(p)
	for _, key := range []string{`"true_kpi"`, `"scenario"`, `"cause"`, `"future"`} {
		if bytes.Contains(raw, []byte(key)) {
			t.Fatal("forbidden packet field")
		}
	}
}
func TestDistortedRawChangesObservedNotTrueKPI(t *testing.T) {
	ch := channel(t)
	m, _ := monitor.New(monitor.DefaultConfig(), 0)
	p, e := ch.Transmit(closed(t, 0), map[observation.Field]Distortion{observation.RawEnd: {RelativeError: -.5}})
	if e != nil {
		t.Fatal(e)
	}
	r, e := m.Observe(p)
	if e != nil || r.KPI == nil || r.KPI[3] != 2.5 {
		t.Fatalf("monitor saw hidden true raw instead: %+v %v", r, e)
	}
}
