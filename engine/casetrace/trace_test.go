package casetrace

import (
	"dissertation.local/sppr-reconstruction/analysispipe"
	"dissertation.local/sppr-reconstruction/internal/testfixture"
	"encoding/json"
	"testing"
)

func trace(t *testing.T) Bundle {
	t.Helper()
	b, e := Build(Suite{Evidence, 0, testfixture.Config(analysispipe.EvidenceRequired), testfixture.Packets()})
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func rehash(t *testing.T, b *Bundle) {
	t.Helper()
	var e error
	b.HeaderHash, e = digest(b.Header)
	if e != nil {
		t.Fatal(e)
	}
	prev := b.HeaderHash
	for i := range b.Records {
		b.Records[i].Payload.PreviousHash = prev
		b.Records[i].Hash, e = digest(b.Records[i].Payload)
		if e != nil {
			t.Fatal(e)
		}
		prev = b.Records[i].Hash
	}
	b.FinalHash = prev
}
func TestRoundTripAndReplay(t *testing.T) {
	b := trace(t)
	raw, e := json.Marshal(b)
	if e != nil {
		t.Fatal(e)
	}
	var out Bundle
	if e = json.Unmarshal(raw, &out); e != nil {
		t.Fatal(e)
	}
	if e = Verify(out); e != nil {
		t.Fatal(e)
	}
}
func TestTruncationDetected(t *testing.T) {
	b := trace(t)
	b.Records = b.Records[:len(b.Records)-1]
	if Verify(b) == nil {
		t.Fatal("truncation accepted")
	}
}
func TestOrderAndDayDetected(t *testing.T) {
	b := trace(t)
	b.Records[1], b.Records[2] = b.Records[2], b.Records[1]
	rehash(t, &b)
	if Verify(b) == nil {
		t.Fatal("permutation accepted")
	}
}
func TestHeaderMutationDetected(t *testing.T) {
	b := trace(t)
	b.Header.Config.Admission.RawStockMaxAge = 2
	if Verify(b) == nil {
		t.Fatal("header mutation accepted")
	}
}
func TestForgedRankingWithUpdatedHashesDetected(t *testing.T) {
	b := trace(t)
	i := 8
	if len(b.Records[i].Payload.Result.Diagnosis.Ranked) == 0 {
		t.Fatal("fixture lacks diagnosis")
	}
	b.Records[i].Payload.Result.Diagnosis.Ranked[0].Posterior = .001
	rehash(t, &b)
	if Verify(b) == nil {
		t.Fatal("forged posterior accepted")
	}
}
func TestForgedStateWithUpdatedHashesDetected(t *testing.T) {
	b := trace(t)
	b.Records[1].Payload.After.Statistics.AcceptedSteps += 1
	rehash(t, &b)
	if Verify(b) == nil {
		t.Fatal("forged state accepted")
	}
}
func TestChangedValidPolicyNeedsNewResults(t *testing.T) {
	b := trace(t)
	b.Header.Config.Mode = analysispipe.AggregateOnly
	rehash(t, &b)
	if Verify(b) == nil {
		t.Fatal("policy changed without recomputation accepted")
	}
}
func TestFinalHashDetected(t *testing.T) {
	b := trace(t)
	b.FinalHash = "bad"
	if Verify(b) == nil {
		t.Fatal("bad final")
	}
}
func TestEmptyFixtureAndWrongEvidenceRejected(t *testing.T) {
	s := Suite{Evidence, 0, testfixture.Config(analysispipe.EvidenceRequired), nil}
	if _, e := Build(s); e == nil {
		t.Fatal("empty accepted")
	}
	s.Packets = testfixture.Packets()
	s.EvidenceClass = "real_company"
	if _, e := Build(s); e == nil {
		t.Fatal("engineering command mislabelled")
	}
}
func TestTraceOwnsConfigAndInputs(t *testing.T) {
	s := Suite{Evidence, 0, testfixture.Config(analysispipe.EvidenceRequired), testfixture.Packets()}
	b, e := Build(s)
	if e != nil {
		t.Fatal(e)
	}
	s.Config.Diagnosis.Hypotheses[0].Causes[0] = "mutated"
	s.Packets[0].Readings = nil
	if e = Verify(b); e != nil {
		t.Fatal("input alias", e)
	}
}
