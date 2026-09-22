package forecasttrace

import (
	"compress/gzip"
	"dissertation.local/sppr-reconstruction/analysispipe"
	"dissertation.local/sppr-reconstruction/forecast"
	"dissertation.local/sppr-reconstruction/forecastpipe"
	"dissertation.local/sppr-reconstruction/internal/testfixture"
	"dissertation.local/sppr-reconstruction/observation"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func tiny() (forecastpipe.Config, []Episode) {
	c := forecastpipe.Config{Analysis: testfixture.Config(analysispipe.EvidenceRequired), Forecast: forecast.DefaultConfig()}
	c.Forecast.Horizon = 2
	c.Forecast.PathsPerHypothesis = 2
	ep := Episode{Spec: EpisodeSpec{"technical", 0, 3}, Inputs: []forecastpipe.Input{}}
	for day := 0; day < 3; day++ {
		p := testfixture.Packet(day)
		p.Readings[observation.RawEnd] = observation.Read(90, 0)
		ep.Inputs = append(ep.Inputs, forecastpipe.Input{Packet: p, Plan: forecast.KnownPlan{Base: [2]float64{40, 25}, Version: p.Context.PlanVersion, ConstraintVersion: p.Context.ConstraintVersion, SnapshotDay: day}, ForecastSeed: 5})
	}
	return c, []Episode{ep}
}
func load(t *testing.T, path string) (Header, []Record, Footer) {
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
	var h Header
	var end Footer
	if e = d.Decode(&h); e != nil {
		t.Fatal(e)
	}
	rs := []Record{}
	for _, ep := range h.Episodes {
		for i := 0; i < ep.Days; i++ {
			var r Record
			if e = d.Decode(&r); e != nil {
				t.Fatal(e)
			}
			rs = append(rs, r)
		}
	}
	if e = d.Decode(&end); e != nil {
		t.Fatal(e)
	}
	return h, rs, end
}
func save(t *testing.T, path string, h Header, rs []Record, end Footer) {
	t.Helper()
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	g := gzip.NewWriter(f)
	enc := json.NewEncoder(g)
	enc.Encode(h)
	for _, r := range rs {
		enc.Encode(r)
	}
	enc.Encode(end)
	g.Close()
	f.Close()
}
func rehash(h *Header, rs []Record, end *Footer) {
	h.Hash, _ = headerHash(*h)
	prev := h.Hash
	for i := range rs {
		rs[i].Previous = prev
		rs[i].Hash, _ = recordHash(rs[i])
		prev = rs[i].Hash
	}
	end.LastHash = prev
}
func TestForecastReplay(t *testing.T) {
	c, eps := tiny()
	path := filepath.Join(t.TempDir(), "trace.gz")
	a, e := Write(path, c, eps)
	if e != nil {
		t.Fatal(e)
	}
	b, e := Replay(path)
	if e != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("replay", e)
	}
	if a.Records != 3 || a.Forecasts != 3 {
		t.Fatal("counts")
	}
}
func TestForgedResultWithRehashedChainDetected(t *testing.T) {
	c, eps := tiny()
	path := filepath.Join(t.TempDir(), "trace.gz")
	if _, e := Write(path, c, eps); e != nil {
		t.Fatal(e)
	}
	h, rs, end := load(t, path)
	rs[0].Payload.Result.Forecast.Alternatives[0].Risk.CVaR += 1
	rehash(&h, rs, &end)
	save(t, path, h, rs, end)
	if _, e := Replay(path); e == nil {
		t.Fatal("forged forecast accepted")
	}
}
func TestReorderedRecordsDetected(t *testing.T) {
	c, eps := tiny()
	path := filepath.Join(t.TempDir(), "trace.gz")
	Write(path, c, eps)
	h, rs, end := load(t, path)
	rs[0], rs[1] = rs[1], rs[0]
	rehash(&h, rs, &end)
	save(t, path, h, rs, end)
	if _, e := Replay(path); e == nil {
		t.Fatal("order accepted")
	}
}
func TestHeaderParameterTamperDetected(t *testing.T) {
	c, eps := tiny()
	path := filepath.Join(t.TempDir(), "trace.gz")
	Write(path, c, eps)
	h, rs, end := load(t, path)
	h.Config.Forecast.Discount = .5
	rehash(&h, rs, &end)
	save(t, path, h, rs, end)
	if _, e := Replay(path); e == nil {
		t.Fatal("changed configuration accepted")
	}
}
func TestTruncatedCompressedTraceDetected(t *testing.T) {
	c, eps := tiny()
	path := filepath.Join(t.TempDir(), "trace.gz")
	Write(path, c, eps)
	b, _ := os.ReadFile(path)
	os.WriteFile(path, b[:len(b)-15], 0644)
	if _, e := Replay(path); e == nil {
		t.Fatal("truncated trace accepted")
	}
}
func TestIncorrectFooterDetected(t *testing.T) {
	c, eps := tiny()
	path := filepath.Join(t.TempDir(), "trace.gz")
	Write(path, c, eps)
	h, rs, end := load(t, path)
	end.Records--
	save(t, path, h, rs, end)
	if _, e := Replay(path); e == nil {
		t.Fatal("bad footer accepted")
	}
}
func TestAtomicFailedWrite(t *testing.T) {
	c, eps := tiny()
	path := filepath.Join(t.TempDir(), "trace.gz")
	os.WriteFile(path, []byte("original"), 0644)
	eps[0].Inputs[1].Plan.Version = ""
	if _, e := Write(path, c, eps); e == nil {
		t.Fatal("bad input accepted")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "original" {
		t.Fatal("failed write overwrote prior artifact")
	}
}
func TestBadEpisodeSpecs(t *testing.T) {
	c, eps := tiny()
	eps[0].Spec.Days = 2
	if _, e := Write(filepath.Join(t.TempDir(), "x.gz"), c, eps); e == nil {
		t.Fatal("incomplete episode accepted")
	}
}
func TestFailedNewTraceLeavesNoCompletedFile(t *testing.T) {
	c, eps := tiny()
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.gz")
	eps[0].Inputs[1].Plan.Version = ""
	if _, e := Write(path, c, eps); e == nil {
		t.Fatal("bad input accepted")
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("partial file survived")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("partial temporary file survived")
	}
}
