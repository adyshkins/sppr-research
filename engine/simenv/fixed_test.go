package simenv

import (
	"dissertation.local/sppr-reconstruction/observation"
	"fmt"
	"reflect"
	"testing"
)

func records(t *testing.T, c Config) []Record {
	t.Helper()
	xs := []Record{}
	if e := RunFixed(c, func(r Record) error { xs = append(xs, r); return nil }); e != nil {
		t.Fatal(e)
	}
	return xs
}
func TestSourceEventDayIndexing(t *testing.T) {
	c, _ := DefaultConfig("test", "S", 1)
	c.EnvironmentSD = 0
	c.ObservationSD = 0
	c.MissingProbability = 0
	rs := records(t, c)
	if rs[18].Active || !rs[19].Active || !rs[23].Active || rs[24].Active || rs[19].CalendarDay != 20 {
		t.Fatal("source day indexing")
	}
	if *rs[19].Packet.Readings[observation.RawArrival].Value != 27 {
		t.Fatal("source severity")
	}
}
func TestGeneratorRepeatable(t *testing.T) {
	c, _ := DefaultConfig("test", "D", 1)
	if !reflect.DeepEqual(records(t, c), records(t, c)) {
		t.Fatal("not deterministic")
	}
}
func TestObservationNoiseDoesNotAlterEnvironment(t *testing.T) {
	c, _ := DefaultConfig("test", "DS", 1)
	a := records(t, c)
	c.ObservationSD = .2
	c.MissingProbability = .4
	b := records(t, c)
	for i := range a {
		if a[i].TrueKPI != b[i].TrueKPI {
			t.Fatal("observation affected fixed environment")
		}
	}
}
func TestDropoutDoesNotShiftNoise(t *testing.T) {
	c, _ := DefaultConfig("test", "D", 1)
	c.MissingProbability = 0
	a := records(t, c)
	c.MissingProbability = .5
	b := records(t, c)
	for i := range a {
		for _, f := range observation.Fields() {
			if b[i].Packet.Readings[f].Present && !reflect.DeepEqual(a[i].Packet.Readings[f], b[i].Packet.Readings[f]) {
				t.Fatal("dropout shifted noise")
			}
		}
	}
}
func TestAllMissingStillGeneratesPhysicalDays(t *testing.T) {
	c, _ := DefaultConfig("test", "S", 1)
	c.MissingProbability = 1
	rs := records(t, c)
	if len(rs) != 60 {
		t.Fatal("lost days")
	}
	for _, r := range rs {
		for _, v := range r.Packet.Readings {
			if v.Present {
				t.Fatal("not all missing")
			}
		}
	}
}
func TestInvalidEnvironment(t *testing.T) {
	c, _ := DefaultConfig("test", "S", 1)
	c.Event.EndDay = 60
	if e := c.Validate(); e == nil {
		t.Fatal("invalid window")
	}
	if _, e := EventFor("OOD"); e == nil {
		t.Fatal("unknown label accepted implicitly")
	}
}
func TestSinkFailurePropagates(t *testing.T) {
	c, _ := DefaultConfig("test", "S", 1)
	want := fmt.Errorf("sink")
	if e := RunFixed(c, func(r Record) error { return want }); e != want {
		t.Fatal("swallowed sink error")
	}
}
func TestNoEventBaseline(t *testing.T) {
	c, _ := DefaultConfig("test", "", 1)
	c.EnvironmentSD = 0
	c.ObservationSD = 0
	c.MissingProbability = 0
	for _, r := range records(t, c) {
		if r.Active || r.TrueKPI != [4]float64{1, 1, .8, 5} {
			t.Fatal("baseline inconsistent")
		}
	}
}
