package monitortrace

import (
	"dissertation.local/sppr-reconstruction/monitor"
	"dissertation.local/sppr-reconstruction/observation"
	"encoding/json"
	"testing"
)

func fixture() Suite {
	c := monitor.DefaultConfig()
	s := Suite{EvidenceClass: Evidence, Config: c, Packets: []observation.Packet{}}
	for day := 0; day < 6; day++ {
		p := observation.Packet{Schema: observation.Schema, Day: day, Context: observation.Context{ExecutedPlan: [2]float64{40, 25}, ActionID: "u0", PlanVersion: "p1", ConstraintVersion: "c1"}, Readings: map[observation.Field]observation.Measurement{}}
		for _, f := range observation.Fields() {
			p.Readings[f] = observation.Read(c.InitialValues[f], 0)
			if day == 2 {
				p.Readings[f] = observation.Missing()
			}
		}
		if day == 4 {
			p.Readings[observation.RawEnd] = observation.Read(180, 0)
		}
		s.Packets = append(s.Packets, p)
	}
	return s
}
func built(t *testing.T) Bundle {
	t.Helper()
	b, e := Build(fixture())
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestReplayJSONRoundTrip(t *testing.T) {
	b := built(t)
	raw, e := json.Marshal(b)
	if e != nil {
		t.Fatal(e)
	}
	var c Bundle
	if e = json.Unmarshal(raw, &c); e != nil {
		t.Fatal(e)
	}
	if e = Verify(c); e != nil {
		t.Fatal(e)
	}
}
func TestCorruptResultRejected(t *testing.T) {
	b := built(t)
	b.Records[0].Payload.Result.Reason = "modified"
	if Verify(b) == nil {
		t.Fatal("corruption accepted")
	}
}
func TestRehashedFakeResultRejectedByRecalculation(t *testing.T) {
	b := built(t)
	i := len(b.Records) - 1
	b.Records[i].Payload.Result.Reason = "fake"
	h, e := digest(b.Records[i].Payload)
	if e != nil {
		t.Fatal(e)
	}
	b.Records[i].Hash = h
	b.FinalHash = h
	if Verify(b) == nil {
		t.Fatal("trusted stored results instead of replay")
	}
}
func TestRehashedFakeStateRejected(t *testing.T) {
	b := built(t)
	i := len(b.Records) - 1
	b.Records[i].Payload.After.Statistics.AcceptedSteps = 1000
	h, _ := digest(b.Records[i].Payload)
	b.Records[i].Hash = h
	b.FinalHash = h
	if Verify(b) == nil {
		t.Fatal("trusted stored state")
	}
}
func TestTruncationAndOrderRejected(t *testing.T) {
	b := built(t)
	b.Records = b.Records[:5]
	if Verify(b) == nil {
		t.Fatal("truncation")
	}
	b = built(t)
	b.Records[0], b.Records[1] = b.Records[1], b.Records[0]
	if Verify(b) == nil {
		t.Fatal("reorder")
	}
}
func TestReplayVersionAndConfig(t *testing.T) {
	b := built(t)
	b.CodeVersion = "unknown"
	if Verify(b) == nil {
		t.Fatal("unsupported version")
	}
	b = built(t)
	b.Config.Targets[3] = 99
	if Verify(b) == nil {
		t.Fatal("config mismatch not detected")
	}
}
func TestBuildClonesInputs(t *testing.T) {
	s := fixture()
	b, e := Build(s)
	if e != nil {
		t.Fatal(e)
	}
	s.Config.InitialValues[observation.RawEnd] = 999
	*s.Packets[0].Readings[observation.RawEnd].Value = 999
	if e = Verify(b); e != nil {
		t.Fatalf("alias: %v", e)
	}
}
func TestInvalidSuiteAndChronology(t *testing.T) {
	s := fixture()
	s.EvidenceClass = "industrial_trial"
	if _, e := Build(s); e == nil {
		t.Fatal("false evidence class")
	}
	s = fixture()
	s.Packets[1].Day = 8
	if _, e := Build(s); e == nil {
		t.Fatal("invalid sequence")
	}
}
