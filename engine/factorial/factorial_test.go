package factorial

import (
	"compress/gzip"
	"dissertation.local/sppr-reconstruction/closedrun"
	"dissertation.local/sppr-reconstruction/control"
	"dissertation.local/sppr-reconstruction/controltrace"
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/plant"
	"dissertation.local/sppr-reconstruction/simenv"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func jobs(t *testing.T) []Job {
	t.Helper()
	var m diagnostics.Model
	if e := ReadJSON("../results/fit_v04/diagnostic_model.json", &m); e != nil {
		t.Fatal(e)
	}
	j, e := Configs(m, 1, 32)
	if e != nil {
		t.Fatal(e)
	}
	return j
}
func decoder(t *testing.T, path string) *json.Decoder {
	t.Helper()
	f, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { f.Close() })
	g, e := gzip.NewReader(f)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { g.Close() })
	return json.NewDecoder(g)
}
func writeRows(t *testing.T, path string, rows []Compact) {
	t.Helper()
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	g := gzip.NewWriter(f)
	d := json.NewEncoder(g)
	for _, r := range rows {
		if e = d.Encode(r); e != nil {
			t.Fatal(e)
		}
	}
	if e = g.Close(); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
}
func loadRows(t *testing.T, path string) []Compact {
	t.Helper()
	d := decoder(t, path)
	r := []Compact{}
	for {
		var x Compact
		e := d.Decode(&x)
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		r = append(r, x)
	}
	return r
}
func TestFactorialConfigsOnlyChangeRiskFactors(t *testing.T) {
	j := jobs(t)
	if len(j) != 20 {
		t.Fatal(len(j))
	}
	for i := 0; i < len(j); i += 4 {
		base := control.CloneConfig(j[i].Config.Controller)
		for k := 0; k < 4; k++ {
			q := j[i+k]
			got := control.CloneConfig(q.Config.Controller)
			got.Policy = base.Policy
			if !equal(got, base) {
				t.Fatal("other component changed")
			}
			if q.Policy != core.FactorialPolicies()[k].ID || q.Config.ForecastSeed != j[i].Config.ForecastSeed || q.Config.Environment.Seed != j[i].Config.Environment.Seed {
				t.Fatal("pairing mismatch")
			}
		}
	}
}
func TestFactorialTapeIdenticalAcrossModes(t *testing.T) {
	j := jobs(t)
	for i := 0; i < len(j); i += 4 {
		a, e := simenv.Tape(j[i].Config.Environment)
		if e != nil {
			t.Fatal(e)
		}
		for k := 1; k < 4; k++ {
			b, e := simenv.Tape(j[i+k].Config.Environment)
			if e != nil || !equal(a, b) {
				t.Fatal("unmatched tape")
			}
		}
	}
}
func TestFactorialInvalidBoundsAndUnfittedModel(t *testing.T) {
	var m diagnostics.Model
	if e := ReadJSON("../results/fit_v04/diagnostic_model.json", &m); e != nil {
		t.Fatal(e)
	}
	for _, v := range [][2]int{{0, 60}, {101, 60}, {1, 31}, {1, 366}} {
		if _, e := Configs(m, v[0], v[1]); e == nil {
			t.Fatal("accepted bad bounds")
		}
	}
	m.Provenance.Kind = "fixture"
	if _, e := Configs(m, 1, 32); e == nil {
		t.Fatal("accepted non-trained model")
	}
}
func TestCompactMatchesLegacyPhysicalAndFullController(t *testing.T) {
	c := jobs(t)[4].Config
	tmp := t.TempDir()
	old, new := filepath.Join(tmp, "old"), filepath.Join(tmp, "new")
	s, e := closedrun.Run(c, old)
	if e != nil {
		t.Fatal(e)
	}
	got, e := Run(c, new)
	if e != nil {
		t.Fatal(e)
	}
	s.Notice = got.Notice
	s.Controller.LastHash = got.Controller.LastHash
	if !equal(s, got) {
		t.Fatal("metrics differ from legacy")
	}
	a, _ := os.ReadFile(filepath.Join(old, "physical_evaluation.jsonl.gz"))
	b, _ := os.ReadFile(filepath.Join(new, "physical_evaluation.jsonl.gz"))
	if string(a) != string(b) {
		t.Fatal("physical bytes differ")
	}
	d := decoder(t, filepath.Join(old, "controller_trace.jsonl.gz"))
	var h controltrace.Header
	if e = d.Decode(&h); e != nil {
		t.Fatal(e)
	}
	rows := loadRows(t, filepath.Join(new, "compact_controller.jsonl.gz"))
	for i, r := range rows {
		var legacy controltrace.Record
		if e = d.Decode(&legacy); e != nil {
			t.Fatal(e)
		}
		if !equal(legacy.Payload.Input, r.Input) || !equal(legacy.Payload.Output.Selection, r.Selection) {
			t.Fatal("controller differs", i)
		}
		oh, _ := control.Hash(legacy.Payload.Output)
		mh, _ := control.Hash(legacy.Payload.State)
		if oh != r.FullOutputHash || mh != r.StateHash {
			t.Fatal("full-output hash mismatch")
		}
	}
	n, e := Replay(new)
	if e != nil || n != 32 {
		t.Fatal(n, e)
	}
}
func TestReplayWithoutPhysicalFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ep")
	_, e := Run(jobs(t)[4].Config, dir)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(filepath.Join(dir, "physical_evaluation.jsonl.gz")); e != nil {
		t.Fatal(e)
	}
	if n, e := Replay(dir); e != nil || n != 32 {
		t.Fatal(n, e)
	}
}
func TestCompactTamperDetectedAfterRehash(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ep")
	if _, e := Run(jobs(t)[4].Config, dir); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "compact_controller.jsonl.gz")
	rows := loadRows(t, path)
	rows[21].Selection.Reason = "forged"
	for i := 21; i < len(rows); i++ {
		if i > 21 {
			rows[i].Previous = rows[i-1].Hash
		}
		rows[i].Hash, _ = compactHash(rows[i])
	}
	writeRows(t, path, rows)
	if _, e := Replay(dir); e == nil {
		t.Fatal("accepted forged recomputed hash")
	}
}
func TestCompactTruncationAndDuplicateRejected(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ep")
	if _, e := Run(jobs(t)[0].Config, dir); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "compact_controller.jsonl.gz")
	rows := loadRows(t, path)
	writeRows(t, path, rows[:31])
	if _, e := Replay(dir); e == nil {
		t.Fatal("accepted truncation")
	}
	writeRows(t, path, append(rows, rows[31]))
	if _, e := Replay(dir); e == nil {
		t.Fatal("accepted duplicate")
	}
}
func TestCompactNoOverwriteAndNoInvalidPublication(t *testing.T) {
	tmp := t.TempDir()
	c := jobs(t)[0].Config
	if _, e := Run(c, tmp); e == nil {
		t.Fatal("overwrote directory")
	}
	c.ForecastSeed = 1
	c.Controller.Choice.Alpha = 2
	p := filepath.Join(tmp, "bad")
	if _, e := Run(c, p); e == nil {
		t.Fatal("invalid config")
	}
	if _, e := os.Stat(p); !os.IsNotExist(e) {
		t.Fatal("published invalid result")
	}
}
func refspec(t *testing.T) ReferenceSpec {
	t.Helper()
	c := jobs(t)[4].Config
	as := []plant.Action{}
	for _, id := range []string{"u0", "u1", "u3", "u6"} {
		p := plant.Pair{30, 20}
		a, e := plant.FixedAction(id, &p)
		if e != nil {
			t.Fatal(e)
		}
		as = append(as, a)
	}
	return ReferenceSpec{"technical test fixture", string(make([]byte, 64)), 19, c.Environment.Initial, c.Environment, as, 7, .97, .95, 32, 777}
}
func TestReferenceDeterministic(t *testing.T) {
	s := refspec(t)
	a, e := Reference(s)
	if e != nil {
		t.Fatal(e)
	}
	b, e := Reference(s)
	if e != nil || !equal(a, b) {
		t.Fatal("reference not deterministic", e)
	}
}
func TestReferenceActionOrderDoesNotShiftNoise(t *testing.T) {
	s := refspec(t)
	a, e := Reference(s)
	if e != nil {
		t.Fatal(e)
	}
	s.Actions[0], s.Actions[3] = s.Actions[3], s.Actions[0]
	b, e := Reference(s)
	if e != nil || !equal(a, b) {
		t.Fatal("actions changed continuation noise")
	}
}
func TestReferenceIndependentSeedAffectsContinuations(t *testing.T) {
	s := refspec(t)
	a, _ := Reference(s)
	s.Seed++
	b, e := Reference(s)
	if e != nil || a.ForcingHash == b.ForcingHash {
		t.Fatal("seed not used")
	}
}
func TestReferenceOneTimeActionCostAndPermanentPlan(t *testing.T) {
	s := refspec(t)
	s.Environment.EnvironmentSD = 0
	s.TrueStart.Raw = 450
	s.TrueStart.Backlog = plant.Pair{20, 10}
	r, e := Reference(s)
	if e != nil {
		t.Fatal(e)
	}
	noop, _ := plant.FixedAction("u0", nil)
	for _, act := range s.Actions {
		state := s.TrueStart
		ls := []float64{}
		for h := 0; h < s.Horizon; h++ {
			day := s.ClosedDay + 1 + h
			ratio := 1.
			if day <= s.Environment.Event.EndDay {
				ratio = .3
			}
			w := plant.Exogenous{Demand: plant.Pair{40, 25}, Capacity: 100, RawArrival: 90 * ratio}
			a := noop
			if h == 0 {
				a = act
			}
			p, e := plant.Step(state, w, a, s.Environment.Parameters)
			if e != nil {
				t.Fatal(e)
			}
			state = p.Next
			l, _ := plant.PeriodLoss(p.KPI)
			ls = append(ls, l)
		}
		z, _ := core.HorizonLoss(ls, s.Discount, act.Cost)
		for _, v := range r.Losses[act.ID] {
			if v != z {
				t.Fatal("horizon/cost differs", act.ID, v, z)
			}
		}
		if act.ID == "u6" && state.Plan != *act.PermanentPlan {
			t.Fatal("permanent plan not retained")
		}
	}
}
func TestReferenceStartsAfterClosedDay(t *testing.T) {
	s := refspec(t)
	s.Environment.EnvironmentSD = 0
	s.Environment.Event.EndDay = s.ClosedDay
	a, e := Reference(s)
	if e != nil {
		t.Fatal(e)
	}
	s.Environment.Event.Label = ""
	b, e := Reference(s)
	if e != nil || !equal(a, b) {
		t.Fatal("reused already closed day")
	}
}
func TestReferenceRejectsMalformedInputs(t *testing.T) {
	base := refspec(t)
	cases := []ReferenceSpec{}
	s := base
	s.Paths = 3
	cases = append(cases, s)
	s = base
	s.Alpha = 1
	cases = append(cases, s)
	s = base
	s.Actions = append(append([]plant.Action{}, base.Actions...), base.Actions[0])
	cases = append(cases, s)
	s = base
	s.TrueStart.Raw = -1
	cases = append(cases, s)
	s = base
	s.ClosedDay = -1
	cases = append(cases, s)
	for _, x := range cases {
		if _, e := Reference(x); e == nil {
			t.Fatal("accepted malformed reference")
		}
	}
}
func TestReferencePreselectedCheckpointNotReplaced(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "ep")
	if _, e := Run(jobs(t)[0].Config, dir); e != nil {
		t.Fatal(e)
	}
	out := filepath.Join(tmp, "ref")
	if e := ReferenceFromEpisode(dir, 0, out, 77, 32); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(out, "SKIPPED.json")); e != nil {
		t.Fatal("did not record skipped checkpoint")
	}
}
func TestReferencePublishedFromRecordedCaseAndRegenerated(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "ep")
	if _, e := Run(jobs(t)[4].Config, dir); e != nil {
		t.Fatal(e)
	}
	rows := loadRows(t, filepath.Join(dir, "compact_controller.jsonl.gz"))
	day := -1
	for _, r := range rows {
		if r.ForecastStatus == "ready" {
			day = r.Day
			break
		}
	}
	if day < 0 {
		t.Fatal("fixture has no ready case")
	}
	out := filepath.Join(tmp, "ref")
	if e := ReferenceFromEpisode(dir, day, out, 888, 32); e != nil {
		t.Fatal(e)
	}
	if e := VerifyReference(out); e != nil {
		t.Fatal(e)
	}
}
