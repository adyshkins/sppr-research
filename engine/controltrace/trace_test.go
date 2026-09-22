package controltrace_test

import (
	"compress/gzip"
	"dissertation.local/sppr-reconstruction/analysispipe"
	"dissertation.local/sppr-reconstruction/closedrun"
	"dissertation.local/sppr-reconstruction/control"
	"dissertation.local/sppr-reconstruction/controltrace"
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/forecast"
	"dissertation.local/sppr-reconstruction/forecastpipe"
	"dissertation.local/sppr-reconstruction/internal/testfixture"
	"dissertation.local/sppr-reconstruction/simenv"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T) (string, closedrun.Summary) {
	t.Helper()
	ec, _ := simenv.DefaultConfig("trace-test", "", 92)
	ec.Days = 4
	ec.Initial.Raw = 90
	ec.MissingProbability = 0
	ec.ObservationSD = 0
	ec.EnvironmentSD = 0
	f := forecast.DefaultConfig()
	f.Horizon = 2
	f.PathsPerHypothesis = 1
	c := closedrun.Config{Notice: "technical fixture not efficacy", Environment: ec, Controller: control.Config{Pipeline: forecastpipe.Config{Analysis: testfixture.Config(analysispipe.EvidenceRequired), Forecast: f}, Choice: core.Config{Horizon: 2, Discount: .97, Alpha: .95, RiskWeight: .35, RiskLimit: 1.25}, Policy: core.FactorialPolicies()[0], Routing: control.DefaultRouting(), ExecutionMode: control.ResearchNoReview}, ForecastSeed: 18, InitialLimits: control.DefaultLimits(), Dropouts: []closedrun.Dropout{}}
	dir := filepath.Join(t.TempDir(), "run")
	s, e := closedrun.Run(c, dir)
	if e != nil {
		t.Fatal(e)
	}
	return filepath.Join(dir, "controller_trace.jsonl.gz"), s
}
func load(t *testing.T, path string) (controltrace.Header, []controltrace.Record, controltrace.Footer) {
	t.Helper()
	f, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	g, e := gzip.NewReader(f)
	if e != nil {
		t.Fatal(e)
	}
	defer g.Close()
	d := json.NewDecoder(g)
	var h controltrace.Header
	if e = d.Decode(&h); e != nil {
		t.Fatal(e)
	}
	rs := make([]controltrace.Record, h.Spec.Days)
	for i := range rs {
		if e = d.Decode(&rs[i]); e != nil {
			t.Fatal(e)
		}
	}
	var end controltrace.Footer
	if e = d.Decode(&end); e != nil {
		t.Fatal(e)
	}
	return h, rs, end
}
func save(t *testing.T, path string, h controltrace.Header, rs []controltrace.Record, end controltrace.Footer, seal, footer, extra bool) {
	t.Helper()
	if seal {
		h.Hash = ""
		h.Hash, _ = control.Hash(h)
		prev := h.Hash
		for i := range rs {
			rs[i].Previous = prev
			rs[i].Hash = ""
			rs[i].Hash, _ = control.Hash(rs[i])
			prev = rs[i].Hash
		}
		end.LastHash = prev
	}
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	g := gzip.NewWriter(f)
	d := json.NewEncoder(g)
	d.Encode(h)
	for _, r := range rs {
		d.Encode(r)
	}
	if footer {
		d.Encode(end)
	}
	if extra {
		d.Encode(map[string]string{"extra": "not allowed"})
	}
	if e = g.Close(); e != nil {
		t.Fatal(e)
	}
	f.Close()
}
func TestFullControllerReplayMatches(t *testing.T) {
	p, s := fixture(t)
	r, e := controltrace.Replay(p)
	a, _ := control.Hash(r)
	b, _ := control.Hash(s.Controller)
	if e != nil || a != b {
		t.Fatal(e, r)
	}
}
func TestControllerReplayRejectsModifiedOutputEvenRehashed(t *testing.T) {
	p, _ := fixture(t)
	h, r, f := load(t, p)
	r[0].Payload.Output.Selection.Reason = "fabricated"
	save(t, p, h, r, f, true, true, false)
	if _, e := controltrace.Replay(p); e == nil {
		t.Fatal("tamper accepted")
	}
}
func TestControllerReplayRejectsActuationTamperEvenRehashed(t *testing.T) {
	p, _ := fixture(t)
	h, r, f := load(t, p)
	r[1].Payload.Execution.Action.Cost += 1
	save(t, p, h, r, f, true, true, false)
	if _, e := controltrace.Replay(p); e == nil {
		t.Fatal("execution tamper accepted")
	}
}
func TestControllerReplayRejectsRecordReordering(t *testing.T) {
	p, _ := fixture(t)
	h, r, f := load(t, p)
	r[0], r[1] = r[1], r[0]
	save(t, p, h, r, f, true, true, false)
	if _, e := controltrace.Replay(p); e == nil {
		t.Fatal("reorder")
	}
}
func TestControllerReplayRejectsMissingRecord(t *testing.T) {
	p, _ := fixture(t)
	h, r, f := load(t, p)
	r = r[:len(r)-1]
	save(t, p, h, r, f, true, true, false)
	if _, e := controltrace.Replay(p); e == nil {
		t.Fatal("truncated")
	}
}
func TestControllerReplayRejectsMissingFooter(t *testing.T) {
	p, _ := fixture(t)
	h, r, f := load(t, p)
	save(t, p, h, r, f, false, false, false)
	if _, e := controltrace.Replay(p); e == nil {
		t.Fatal("no footer")
	}
}
func TestControllerReplayRejectsTrailingData(t *testing.T) {
	p, _ := fixture(t)
	h, r, f := load(t, p)
	save(t, p, h, r, f, false, true, true)
	if _, e := controltrace.Replay(p); e == nil {
		t.Fatal("extra accepted")
	}
}
func TestControllerReplayRejectsChangedSeed(t *testing.T) {
	p, _ := fixture(t)
	h, r, f := load(t, p)
	r[0].Payload.Input.Forecast.ForecastSeed++
	save(t, p, h, r, f, true, true, false)
	if _, e := controltrace.Replay(p); e == nil {
		t.Fatal("seed mismatch")
	}
}
func TestControllerReplayRejectsRegisterChange(t *testing.T) {
	p, _ := fixture(t)
	h, r, f := load(t, p)
	r[1].Payload.Input.Limits.RemainingBudget += 1
	save(t, p, h, r, f, true, true, false)
	if _, e := controltrace.Replay(p); e == nil {
		t.Fatal("budget fabrication")
	}
}
func TestControllerReplayRejectsTerminalAssignmentChange(t *testing.T) {
	p, _ := fixture(t)
	h, r, f := load(t, p)
	f.Pending = &control.Assignment{ID: "invented"}
	save(t, p, h, r, f, false, true, false)
	if _, e := controltrace.Replay(p); e == nil {
		t.Fatal("terminal command")
	}
}
func TestControllerReplayRejectsCorruptHash(t *testing.T) {
	p, _ := fixture(t)
	h, r, f := load(t, p)
	r[0].Hash = "bad"
	save(t, p, h, r, f, false, true, false)
	if _, e := controltrace.Replay(p); e == nil {
		t.Fatal("bad hash")
	}
}
func TestTraceRefusesOverwriteAndBadSpec(t *testing.T) {
	p, _ := fixture(t)
	h, _, _ := load(t, p)
	if _, e := controltrace.NewWriter(p, h.Spec); e == nil {
		t.Fatal("overwrite")
	}
	h.Spec.Days = 0
	if _, e := controltrace.NewWriter(filepath.Join(t.TempDir(), "new.gz"), h.Spec); e == nil {
		t.Fatal("bad spec")
	}
}
func TestTraceCannotCommitPartialEpisode(t *testing.T) {
	p, _ := fixture(t)
	h, r, _ := load(t, p)
	dest := filepath.Join(t.TempDir(), "partial.gz")
	w, e := controltrace.NewWriter(dest, h.Spec)
	if e != nil {
		t.Fatal(e)
	}
	defer w.Abort()
	if e = w.Append(r[0].Payload); e != nil {
		t.Fatal(e)
	}
	if _, e = w.Close(); e == nil {
		t.Fatal("partial close")
	}
	w.Abort()
	if _, e = os.Stat(dest); !os.IsNotExist(e) {
		t.Fatal("partial published")
	}
}
func TestTraceRejectsDuplicateDay(t *testing.T) {
	p, _ := fixture(t)
	h, r, _ := load(t, p)
	w, e := controltrace.NewWriter(filepath.Join(t.TempDir(), "dup.gz"), h.Spec)
	if e != nil {
		t.Fatal(e)
	}
	defer w.Abort()
	if e = w.Append(r[0].Payload); e != nil {
		t.Fatal(e)
	}
	if e = w.Append(r[0].Payload); e == nil {
		t.Fatal("duplicate execution")
	}
}
