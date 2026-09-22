package sufficiency_test

import (
	"dissertation.local/sppr-reconstruction/internal/testfixture"
	"dissertation.local/sppr-reconstruction/monitor"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/sufficiency"
	"fmt"
	"testing"
)

func assess(t *testing.T, p observation.Packet, c sufficiency.Config) sufficiency.Result {
	t.Helper()
	r, e := sufficiency.Assess(p, monitor.DefaultConfig().Quality, monitor.State{NextDay: p.Day}, c)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestCurrentPacketAdmitted(t *testing.T) {
	r := assess(t, testfixture.Packet(0), sufficiency.StrictConfig())
	if !r.Passed || r.UsesHistory || len(r.Requirements) != 4 {
		t.Fatal(r)
	}
}
func TestNoInitialValuesCountAsEvidence(t *testing.T) {
	p := testfixture.Packet(0)
	for f := range p.Readings {
		p.Readings[f] = observation.Read(-1, 0)
	}
	q, e := observation.EvaluateQuality(p, monitor.DefaultConfig().Quality)
	if e != nil || !q.GatePassed || q.Score != .7 {
		t.Fatal(q, e)
	}
	r := assess(t, p, sufficiency.StrictConfig())
	if r.Passed {
		t.Fatal("all-invalid packet admitted")
	}
}
func TestAllStaleNotCurrentEvidence(t *testing.T) {
	p := testfixture.Packet(5)
	for f, m := range p.Readings {
		p.Readings[f] = observation.Read(*m.Value, 1)
	}
	if assess(t, p, sufficiency.StrictConfig()).Passed {
		t.Fatal("stale bundle admitted")
	}
}
func TestEveryRequiredMissingSubset(t *testing.T) {
	fields := []observation.Field{observation.DemandA, observation.DemandB, observation.BacklogBeforeA, observation.BacklogBeforeB, observation.ShippedA, observation.ShippedB, observation.OutputA, observation.OutputB, observation.Capacity, observation.RawEnd}
	for mask := 0; mask < 1<<len(fields); mask++ {
		p := testfixture.Packet(0)
		for i, f := range fields {
			if mask&(1<<i) != 0 {
				p.Readings[f] = observation.Missing()
			}
		}
		r := assess(t, p, sufficiency.StrictConfig())
		if r.Passed != (mask == 0) {
			t.Fatalf("required availability mask %d", mask)
		}
	}
}
func TestOptionalChannelsDoNotInventMandatoryRequirement(t *testing.T) {
	p := testfixture.Packet(0)
	for _, f := range []observation.Field{observation.RawArrival, observation.FinishedA, observation.FinishedB, observation.BacklogAfterA, observation.BacklogAfterB} {
		p.Readings[f] = observation.Missing()
	}
	r := assess(t, p, sufficiency.StrictConfig())
	if !r.Passed {
		t.Fatal(r)
	}
}
func TestCrossFieldContradictionBlocks(t *testing.T) {
	for _, f := range []observation.Field{observation.ShippedA, observation.ShippedB, observation.OutputA, observation.Capacity} {
		t.Run(string(f), func(t *testing.T) {
			p := testfixture.Packet(0)
			p.Readings[f] = observation.Read(1000, 0)
			if assess(t, p, sufficiency.StrictConfig()).Passed {
				t.Fatal("contradictory packet admitted")
			}
		})
	}
}
func TestNoHistoryByDefault(t *testing.T) {
	p := testfixture.Packet(1)
	p.Readings[observation.RawEnd] = observation.Missing()
	state := monitor.State{NextDay: 1, Cache: map[observation.Field]monitor.Cached{observation.RawEnd: {Value: 450, SampleDay: 0, DeliveryDay: 0}}}
	r, e := sufficiency.Assess(p, monitor.DefaultConfig().Quality, state, sufficiency.StrictConfig())
	if e != nil || r.Passed {
		t.Fatal(r, e)
	}
}
func TestOptionalRawHistoryAge(t *testing.T) {
	for _, age := range []int{1, 2} {
		p := testfixture.Packet(age)
		p.Readings[observation.RawEnd] = observation.Missing()
		state := monitor.State{NextDay: age, Cache: map[observation.Field]monitor.Cached{observation.RawEnd: {Value: 450, SampleDay: 0, DeliveryDay: 0}}}
		r, e := sufficiency.Assess(p, monitor.DefaultConfig().Quality, state, sufficiency.Config{Policy: sufficiency.BoundedRawHistory, RawStockMaxAge: 1})
		if e != nil || r.Passed != (age == 1) {
			t.Fatal(r, e)
		}
	}
}
func TestHistoryNeverUsedForFlows(t *testing.T) {
	p := testfixture.Packet(1)
	p.Readings[observation.DemandA] = observation.Missing()
	state := monitor.State{NextDay: 1, Cache: map[observation.Field]monitor.Cached{observation.DemandA: {Value: 40, SampleDay: 0, DeliveryDay: 0}}}
	r, e := sufficiency.Assess(p, monitor.DefaultConfig().Quality, state, sufficiency.Config{Policy: sufficiency.BoundedRawHistory, RawStockMaxAge: 1})
	if e != nil || r.Passed {
		t.Fatal(r, e)
	}
}
func TestHistoricalTimestampNotDeliveryTimestamp(t *testing.T) {
	p := testfixture.Packet(5)
	p.Readings[observation.RawEnd] = observation.Missing()
	state := monitor.State{NextDay: 5, Cache: map[observation.Field]monitor.Cached{observation.RawEnd: {Value: 450, SampleDay: 0, DeliveryDay: 4}}}
	r, e := sufficiency.Assess(p, monitor.DefaultConfig().Quality, state, sufficiency.Config{Policy: sufficiency.BoundedRawHistory, RawStockMaxAge: 1})
	if e != nil || r.Passed {
		t.Fatal(r, e)
	}
}
func TestStaleMeasurementNotReplacedByDifferentSourceSilently(t *testing.T) {
	p := testfixture.Packet(3)
	p.Readings[observation.RawEnd] = observation.Read(900, 2)
	state := monitor.State{NextDay: 3, Cache: map[observation.Field]monitor.Cached{observation.RawEnd: {Value: 450, SampleDay: 2, DeliveryDay: 2}}}
	r, e := sufficiency.Assess(p, monitor.DefaultConfig().Quality, state, sufficiency.Config{Policy: sufficiency.BoundedRawHistory, RawStockMaxAge: 1})
	if e != nil || r.Passed {
		t.Fatal(r, e)
	}
}
func TestInvalidSufficiencyConfiguration(t *testing.T) {
	for i, c := range []sufficiency.Config{{}, {Policy: "unknown"}, {Policy: sufficiency.Strict, RawStockMaxAge: 1}, {Policy: sufficiency.BoundedRawHistory, RawStockMaxAge: 0}} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if c.Validate() == nil {
				t.Fatal(c)
			}
		})
	}
}
func TestMalformedPacketAndStateRejected(t *testing.T) {
	p := testfixture.Packet(0)
	_, e := sufficiency.Assess(p, monitor.DefaultConfig().Quality, monitor.State{NextDay: 1}, sufficiency.StrictConfig())
	if e == nil {
		t.Fatal("day mismatch")
	}
	delete(p.Readings, observation.RawEnd)
	_, e = sufficiency.Assess(p, monitor.DefaultConfig().Quality, monitor.State{}, sufficiency.StrictConfig())
	if e == nil {
		t.Fatal("invalid schema")
	}
}
