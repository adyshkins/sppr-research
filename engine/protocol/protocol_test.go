package protocol

import (
	"encoding/json"
	"os"
	"testing"
)

func fixtureSuite(t *testing.T) Suite {
	t.Helper()
	data, e := os.ReadFile("../fixtures/cases.json")
	if e != nil {
		t.Fatal(e)
	}
	var s Suite
	if e = json.Unmarshal(data, &s); e != nil {
		t.Fatal(e)
	}
	return s
}
func fixtureBundle(t *testing.T) Bundle {
	t.Helper()
	b, e := Build(fixtureSuite(t))
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestBundleRoundTripAndReplay(t *testing.T) {
	b := fixtureBundle(t)
	j, e := json.Marshal(b)
	if e != nil {
		t.Fatal(e)
	}
	var copyB Bundle
	if e = json.Unmarshal(j, &copyB); e != nil {
		t.Fatal(e)
	}
	if e = Verify(copyB); e != nil {
		t.Fatal(e)
	}
	if b.Count != 6 {
		t.Fatal("unexpected fixture count")
	}
}
func TestRepeatBuildIdentical(t *testing.T) {
	a := fixtureBundle(t)
	b := fixtureBundle(t)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatal("non-deterministic static core")
	}
}
func TestRejectsCorruptionTruncationAndOrdering(t *testing.T) {
	mutations := []func(*Bundle){
		func(b *Bundle) { b.Records[0].Payload.Input.Config.Alpha = .9 },
		func(b *Bundle) { b.Records = b.Records[:len(b.Records)-1] },
		func(b *Bundle) { b.Records[0], b.Records[1] = b.Records[1], b.Records[0] },
		func(b *Bundle) { b.FinalHash = "wrong" }, func(b *Bundle) { b.CodeVersion = "old" },
		func(b *Bundle) { b.Records[0].Payload.PreviousHash = "wrong" },
	}
	for i, f := range mutations {
		b := fixtureBundle(t)
		f(&b)
		if e := Verify(b); e == nil {
			t.Fatalf("accepted corruption %d", i)
		}
	}
}
func TestReplayRejectsWrongDecisionEvenWithUpdatedHash(t *testing.T) {
	s := fixtureSuite(t)
	s.Cases = s.Cases[:1]
	b, e := Build(s)
	if e != nil {
		t.Fatal(e)
	}
	wrong := "u1"
	b.Records[0].Payload.Decisions[0].RecommendationID = &wrong
	b.Records[0].Hash, e = hash(b.Records[0].Payload)
	if e != nil {
		t.Fatal(e)
	}
	b.FinalHash = b.Records[0].Hash
	if e := Verify(b); e == nil {
		t.Fatal("accepted wrong decision with recomputed hash")
	}
}
func TestRejectsMissingEvidenceClassAndEmptySuite(t *testing.T) {
	s := fixtureSuite(t)
	s.EvidenceClass = ""
	if _, e := Build(s); e == nil {
		t.Fatal("no evidence class")
	}
	s = fixtureSuite(t)
	s.Cases = nil
	if _, e := Build(s); e == nil {
		t.Fatal("empty suite")
	}
}
