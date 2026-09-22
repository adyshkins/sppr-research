package simenv

import (
	"dissertation.local/sppr-reconstruction/plant"
	"dissertation.local/sppr-reconstruction/sensor"
	"reflect"
	"testing"
)

func TestTapeMatchesV04FixedGenerator(t *testing.T) {
	for _, label := range []string{"", "D", "S", "C", "DS"} {
		c, _ := DefaultConfig("tape-equality", label, 37)
		c.Days = 30
		tape, e := Tape(c)
		if e != nil {
			t.Fatal(e)
		}
		channel, _ := sensor.New(0)
		truth := c.Initial
		a, _ := plant.FixedAction("u0", nil)
		i := 0
		e = RunFixed(c, func(old Record) error {
			td := tape[i]
			r, e := plant.Step(truth, td.Forcing, a, c.Parameters)
			if e != nil {
				return e
			}
			fresh, e := sensor.ClosedDay(i, truth, td.Forcing, a, r, "registered-initial-plan-v04", "balance-parameters-v04")
			if e != nil {
				return e
			}
			p, e := channel.Transmit(fresh, td.Distortions)
			if e != nil {
				return e
			}
			if !reflect.DeepEqual(p, old.Packet) || r.KPI != old.TrueKPI || td.Active != old.Active {
				t.Fatalf("tape changed fixed day %d label %s", i, label)
			}
			truth = r.Next
			i++
			return nil
		})
		if e != nil {
			t.Fatal(e)
		}
	}
}
func TestTapeDoesNotShiftWhenDropoutChanges(t *testing.T) {
	c, _ := DefaultConfig("paired", "DS", 20)
	a, _ := Tape(c)
	c.MissingProbability = .5
	b, _ := Tape(c)
	for i := range a {
		if a[i].Forcing != b[i].Forcing {
			t.Fatal("forcing shifted")
		}
		for f, x := range a[i].Distortions {
			if x.RelativeError != b[i].Distortions[f].RelativeError {
				t.Fatal("noise shifted")
			}
		}
	}
}
func TestTapeCopiesAndRejectsInvalidConfig(t *testing.T) {
	c, _ := DefaultConfig("test", "D", 10)
	a, _ := Tape(c)
	b, _ := Tape(c)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("not deterministic")
	}
	c.Days = 0
	if _, e := Tape(c); e == nil {
		t.Fatal("invalid config")
	}
}
