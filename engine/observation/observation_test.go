package observation

import (
	"encoding/json"
	"math"
	"testing"
)

func basePacket() Packet {
	p := Packet{Schema: Schema, Day: 0, Context: Context{[2]float64{40, 25}, 0, "u0", "plan-1", "constraints-1"}, Readings: map[Field]Measurement{}}
	xs := [15]float64{40, 25, 100, 90, 40, 25, 40, 25, 450, 20, 12.5, 0, 0, 0, 0}
	for i, f := range Fields() {
		p.Readings[f] = Read(xs[i], 0)
	}
	return p
}
func values(p Packet) map[Field]float64 {
	x := map[Field]float64{}
	for _, f := range Fields() {
		x[f] = *p.Readings[f].Value
	}
	return x
}
func TestQualityHealthy(t *testing.T) {
	q, e := EvaluateQuality(basePacket(), DissertationQualityConfig())
	if e != nil || q.Score != 1 || !q.GatePassed || len(q.Checks) != 19 || q.ValidCount != 15 {
		t.Fatalf("%+v %v", q, e)
	}
}
func TestQualityTotalMissing(t *testing.T) {
	p := basePacket()
	for _, f := range Fields() {
		p.Readings[f] = Missing()
	}
	q, e := EvaluateQuality(p, DissertationQualityConfig())
	if e != nil || q.Score != 0 || q.GatePassed {
		t.Fatalf("%+v %v", q, e)
	}
}
func TestQualityFreshnessDenominator(t *testing.T) {
	p := basePacket()
	p.Readings[RawEnd] = Missing()
	q, e := EvaluateQuality(p, DissertationQualityConfig())
	if e != nil || q.Freshness != 1 || q.Completeness != 14.0/15 || q.Consistency != 18.0/19 {
		t.Fatalf("%+v %v", q, e)
	}
}
func TestQualityAllStalePassesAtBoundary(t *testing.T) {
	p := basePacket()
	for f, m := range p.Readings {
		m.AgeDays = 1
		p.Readings[f] = m
	}
	q, e := EvaluateQuality(p, DissertationQualityConfig())
	if e != nil || q.Freshness != 0 || q.Score != .7 || !q.GatePassed {
		t.Fatalf("%+v %v", q, e)
	}
}
func TestQualityInvalidFreshCompensationIsDocumented(t *testing.T) {
	p := basePacket()
	for _, f := range Fields() {
		p.Readings[f] = Read(-1, 0)
	}
	q, e := EvaluateQuality(p, DissertationQualityConfig())
	if e != nil || q.ValidCount != 0 || q.Consistency != 0 || q.Score != .7 || !q.GatePassed {
		t.Fatalf("expected additive-gate counterexample: %+v %v", q, e)
	}
}
func TestQualityCrossChecksIndependent(t *testing.T) {
	p := basePacket()
	p.Readings[OutputA] = Read(200, 0)
	p.Readings[ShippedA] = Read(49, 0)
	p.Readings[ShippedB] = Read(31, 0)
	p.Readings[Capacity] = Read(121, 0)
	q, e := EvaluateQuality(p, DissertationQualityConfig())
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range q.Checks[15:] {
		if c.Passed {
			t.Fatal(c)
		}
	}
}
func TestCrossToleranceBoundary(t *testing.T) {
	p := basePacket()
	p.Readings[ShippedA] = Read(48, 0)
	p.Readings[Capacity] = Read(120, 0)
	q, e := EvaluateQuality(p, DissertationQualityConfig())
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range q.Checks {
		if !c.Passed {
			t.Fatal(c)
		}
	}
}
func TestQualityOvertimeUsesExecutedContext(t *testing.T) {
	p := basePacket()
	p.Context.Overtime = .2
	p.Readings[OutputA] = Read(104, 0)
	q, e := EvaluateQuality(p, DissertationQualityConfig())
	if e != nil || !q.Checks[15].Passed {
		t.Fatalf("%+v %v", q, e)
	}
	p.Context.Overtime = 0
	q, _ = EvaluateQuality(p, DissertationQualityConfig())
	if q.Checks[15].Passed {
		t.Fatal("overtime ignored")
	}
}
func TestKPIBaselineAndBacklogTiming(t *testing.T) {
	p := basePacket()
	v := values(p)
	k, e := ComputeKPI(v, p.Context, DissertationQualityConfig())
	if e != nil || k != [4]float64{1, 1, .8, 5} {
		t.Fatalf("%v %v", k, e)
	}
	v[BacklogBeforeA] = 35
	v[BacklogAfterA] = 123
	k, e = ComputeKPI(v, p.Context, DissertationQualityConfig())
	if e != nil || k[0] != .65 {
		t.Fatalf("must use pre-period backlog, not post-period %v %v", k, e)
	}
}
func TestKPINoSilentClipping(t *testing.T) {
	p := basePacket()
	v := values(p)
	v[ShippedA] = 45
	k, e := ComputeKPI(v, p.Context, DissertationQualityConfig())
	if e != nil || k[0] <= 1 {
		t.Fatalf("sensor ratio must not be silently clipped %v %v", k, e)
	}
}
func TestKPIZeroDenominators(t *testing.T) {
	p := basePacket()
	v := values(p)
	for f := range v {
		v[f] = 0
	}
	ctx := p.Context
	ctx.ExecutedPlan = [2]float64{}
	k, e := ComputeKPI(v, ctx, DissertationQualityConfig())
	if e != nil || k != [4]float64{1, 1, 0, 0} {
		t.Fatalf("%v %v", k, e)
	}
	v[OutputA] = 1
	if _, e = ComputeKPI(v, ctx, DissertationQualityConfig()); e == nil {
		t.Fatal("undefined zero-plan KPI")
	}
	ctx.ExecutedPlan = [2]float64{1, 0}
	if _, e = ComputeKPI(v, ctx, DissertationQualityConfig()); e == nil {
		t.Fatal("undefined zero-capacity KPI")
	}
	v[OutputA] = 0
	v[ShippedA] = 1
	if _, e = ComputeKPI(v, ctx, DissertationQualityConfig()); e == nil {
		t.Fatal("undefined zero-demand KPI")
	}
}
func TestKPIOverflow(t *testing.T) {
	p := basePacket()
	v := values(p)
	v[DemandA] = math.MaxFloat64
	v[DemandB] = math.MaxFloat64
	if _, e := ComputeKPI(v, p.Context, DissertationQualityConfig()); e == nil {
		t.Fatal("overflow accepted")
	}
}
func TestFaultSerialization(t *testing.T) {
	for _, x := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		m := Read(x, 0)
		if m.Valid() || m.Validate() != nil || m.Fault != "non_finite" {
			t.Fatal(m)
		}
		if _, e := json.Marshal(m); e != nil {
			t.Fatal(e)
		}
	}
}
func TestMalformedReadingsRejected(t *testing.T) {
	x := 1.0
	bad := []Measurement{{Present: false, Value: &x}, {Present: true}, {Present: true, Value: &x, AgeDays: -1}, {Present: true, Fault: "unknown"}, {Present: true, Value: &x, Fault: "sensor_error"}}
	for _, m := range bad {
		if m.Validate() == nil {
			t.Fatalf("accepted %+v", m)
		}
	}
}
func TestPacketRequiresExplicitMissing(t *testing.T) {
	p := basePacket()
	delete(p.Readings, RawEnd)
	if p.Validate() == nil {
		t.Fatal("silently missing key")
	}
	p.Readings["truth"] = Read(3, 0)
	if p.Validate() == nil {
		t.Fatal("unknown channel accepted")
	}
}
func TestCloneNoAliasing(t *testing.T) {
	p := basePacket()
	q := Clone(p)
	*q.Readings[RawEnd].Value = 123
	if *p.Readings[RawEnd].Value != 450 {
		t.Fatal("aliased value")
	}
}
func TestInvalidQualityConfiguration(t *testing.T) {
	c := DissertationQualityConfig()
	c.Minimum = 0
	if c.Validate() == nil {
		t.Fatal("zero gate permits totally missing packet")
	}
	c = DissertationQualityConfig()
	c.Weights[0] = 1
	if c.Validate() == nil {
		t.Fatal("weights")
	}
}
