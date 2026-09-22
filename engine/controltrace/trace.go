// Package controltrace verifies A3-A6 and command timing from saved observations.
// No simenv, sensor, training labels, true state or future tape are imported.
package controltrace

import (
	"compress/gzip"
	"dissertation.local/sppr-reconstruction/control"
	"dissertation.local/sppr-reconstruction/execution"
	"dissertation.local/sppr-reconstruction/monitor"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const Schema = "sppr.closed-controller-trace.v05"

type Spec struct {
	ID           string             `json:"id"`
	Days         int                `json:"days"`
	ForecastSeed uint64             `json:"forecast_seed"`
	Initial      execution.Register `json:"initial_register"`
	Config       control.Config     `json:"config"`
}

func (s Spec) Validate() error {
	if s.ID == "" || s.Days < 1 || s.Days > 365 {
		return fmt.Errorf("invalid trace spec")
	}
	if e := s.Initial.Validate(0); e != nil {
		return e
	}
	return s.Config.Validate()
}

type Header struct {
	Type   string `json:"type"`
	Schema string `json:"schema"`
	Notice string `json:"notice"`
	Spec   Spec   `json:"spec"`
	Hash   string `json:"hash"`
}
type Payload struct {
	Index     int                  `json:"index"`
	Input     control.Input        `json:"input"`
	Output    control.Output       `json:"output"`
	State     monitor.State        `json:"monitor_state"`
	Execution execution.Resolution `json:"execution"`
}
type Record struct {
	Type     string  `json:"type"`
	Previous string  `json:"previous_hash"`
	Payload  Payload `json:"payload"`
	Hash     string  `json:"hash"`
}
type Footer struct {
	Type     string              `json:"type"`
	Records  int                 `json:"records"`
	LastHash string              `json:"last_hash"`
	Pending  *control.Assignment `json:"terminal_unexecuted_assignment"`
}
type Summary struct {
	Records            int                 `json:"records"`
	Forecasts          int                 `json:"forecasts"`
	Recommendations    int                 `json:"recommendations"`
	Assignments        int                 `json:"assignments"`
	AppliedAssignments int                 `json:"applied_assignments_including_u0"`
	Corrections        int                 `json:"executed_non_u0_actions"`
	ResearchBypasses   int                 `json:"applied_research_route_bypasses"`
	Routes             map[string]int      `json:"route_counts"`
	Reasons            map[string]int      `json:"selection_reason_counts"`
	Actions            map[string]int      `json:"executed_action_counts"`
	LastHash           string              `json:"last_hash"`
	Pending            *control.Assignment `json:"terminal_unexecuted_assignment"`
}

func newSummary() Summary {
	return Summary{Routes: map[string]int{}, Reasons: map[string]int{}, Actions: map[string]int{}}
}
func collect(s *Summary, p Payload) {
	s.Records++
	s.Routes[p.Output.Selection.Route]++
	s.Reasons[p.Output.Selection.Reason]++
	s.Actions[p.Execution.Action.ID]++
	if p.Output.Pipeline.Forecast.Status == "ready" {
		s.Forecasts++
	}
	if p.Output.Selection.Recommendation != nil {
		s.Recommendations++
	}
	if p.Output.Selection.Assignment != nil {
		s.Assignments++
	}
	if p.Execution.AppliedAssignmentID != nil {
		s.AppliedAssignments++
		if p.Execution.Requested.ExecutionMode == control.ResearchNoReview && p.Execution.Requested.Route != "auto" {
			s.ResearchBypasses++
		}
	}
	if p.Execution.Action.ID != "u0" {
		s.Corrections++
	}
}
func headerHash(h Header) (string, error) { h.Hash = ""; return control.Hash(h) }
func recordHash(r Record) (string, error) { r.Hash = ""; return control.Hash(r) }
func equal(a, b any) bool {
	x, e := control.Hash(a)
	y, f := control.Hash(b)
	return e == nil && f == nil && x == y
}

type chain struct {
	spec     Spec
	register execution.Register
	pending  *control.Assignment
	next     int
}

func (c *chain) check(p Payload) error {
	if p.Index != c.next || p.Input.CaseID != fmt.Sprintf("%s/%d", c.spec.ID, c.next) || p.Input.Forecast.Packet.Day != c.next || p.Input.Forecast.ForecastSeed != c.spec.ForecastSeed {
		return fmt.Errorf("trace sequence/seed/identity mismatch")
	}
	r, e := execution.Resolve(c.next, c.pending, c.register)
	if e != nil {
		return e
	}
	if !equal(r, p.Execution) {
		return fmt.Errorf("execution differs from previous assignment/register at day %d", c.next)
	}
	if !equal(r.After.Plan, p.Input.Forecast.Plan) || !equal(r.After.Limits, p.Input.Limits) {
		return fmt.Errorf("post-execution register mismatch")
	}
	if e := execution.VerifyObservedContext(p.Input.Forecast.Packet, r); e != nil {
		return e
	}
	return nil
}
func (c *chain) commit(p Payload) {
	c.pending = control.CloneAssignment(p.Output.Selection.Assignment)
	c.register = execution.CloneRegister(p.Execution.After)
	c.next++
}

type Writer struct {
	path, tmp string
	file      *os.File
	gz        *gzip.Writer
	enc       *json.Encoder
	previous  string
	chain     chain
	summary   Summary
	closed    bool
}

func NewWriter(path string, spec Spec) (*Writer, error) {
	if e := spec.Validate(); e != nil {
		return nil, e
	}
	if _, e := os.Lstat(path); e == nil {
		return nil, fmt.Errorf("trace already exists")
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return nil, e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".control-partial-*")
	if e != nil {
		return nil, e
	}
	h := Header{Type: "header", Schema: Schema, Notice: "NEW synthetic engineering closed-loop trace; research bypass is not human approval; hashes identify content, not scientific truth", Spec: spec}
	h.Hash, e = headerHash(h)
	if e != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, e
	}
	w := &Writer{path: path, tmp: f.Name(), file: f, gz: gzip.NewWriter(f), previous: h.Hash, chain: chain{spec: spec, register: execution.CloneRegister(spec.Initial)}, summary: newSummary()}
	w.enc = json.NewEncoder(w.gz)
	if e = w.enc.Encode(h); e != nil {
		w.Abort()
		return nil, e
	}
	return w, nil
}
func (w *Writer) Append(p Payload) error {
	if w.closed || w.chain.next >= w.chain.spec.Days {
		return fmt.Errorf("closed/full trace")
	}
	if e := w.chain.check(p); e != nil {
		return e
	}
	r := Record{Type: "record", Previous: w.previous, Payload: p}
	var e error
	r.Hash, e = recordHash(r)
	if e != nil {
		return e
	}
	if e = w.enc.Encode(r); e != nil {
		return e
	}
	w.previous = r.Hash
	w.chain.commit(p)
	collect(&w.summary, p)
	return nil
}
func (w *Writer) Abort() {
	if !w.closed {
		w.gz.Close()
		w.file.Close()
		os.Remove(w.tmp)
		w.closed = true
	}
}
func (w *Writer) Close() (Summary, error) {
	if w.closed || w.chain.next != w.chain.spec.Days {
		return w.summary, fmt.Errorf("trace incomplete or closed")
	}
	f := Footer{"footer", w.chain.next, w.previous, control.CloneAssignment(w.chain.pending)}
	if e := w.enc.Encode(f); e != nil {
		return w.summary, e
	}
	if e := w.gz.Close(); e != nil {
		return w.summary, e
	}
	if e := w.file.Close(); e != nil {
		return w.summary, e
	}
	if e := os.Rename(w.tmp, w.path); e != nil {
		return w.summary, e
	}
	w.closed = true
	w.summary.LastHash = w.previous
	w.summary.Pending = control.CloneAssignment(w.chain.pending)
	return w.summary, nil
}
func Replay(path string) (Summary, error) {
	s := newSummary()
	f, e := os.Open(path)
	if e != nil {
		return s, e
	}
	defer f.Close()
	g, e := gzip.NewReader(f)
	if e != nil {
		return s, e
	}
	defer g.Close()
	d := json.NewDecoder(g)
	d.DisallowUnknownFields()
	var h Header
	if e = d.Decode(&h); e != nil {
		return s, e
	}
	if h.Type != "header" || h.Schema != Schema || h.Notice == "" {
		return s, fmt.Errorf("invalid trace header")
	}
	if e = h.Spec.Validate(); e != nil {
		return s, e
	}
	hs, e := headerHash(h)
	if e != nil || hs != h.Hash {
		return s, fmt.Errorf("header hash mismatch")
	}
	c, e := control.New(h.Spec.Config, 0)
	if e != nil {
		return s, e
	}
	chain := chain{spec: h.Spec, register: execution.CloneRegister(h.Spec.Initial)}
	prev := h.Hash
	for i := 0; i < h.Spec.Days; i++ {
		var r Record
		if e = d.Decode(&r); e != nil {
			return s, e
		}
		if r.Type != "record" || r.Previous != prev {
			return s, fmt.Errorf("broken trace chain")
		}
		hs, e = recordHash(r)
		if e != nil || hs != r.Hash {
			return s, fmt.Errorf("record hash mismatch")
		}
		if e = chain.check(r.Payload); e != nil {
			return s, e
		}
		o, e := c.Process(r.Payload.Input)
		if e != nil {
			return s, e
		}
		if !equal(o, r.Payload.Output) || !equal(c.Snapshot(), r.Payload.State) {
			return s, fmt.Errorf("recomputed controller mismatch at day %d", i)
		}
		chain.commit(r.Payload)
		collect(&s, r.Payload)
		prev = r.Hash
	}
	var end Footer
	if e = d.Decode(&end); e != nil {
		return s, e
	}
	if end.Type != "footer" || end.Records != s.Records || end.LastHash != prev || !equal(end.Pending, chain.pending) {
		return s, fmt.Errorf("invalid footer or terminal assignment")
	}
	var extra any
	if e = d.Decode(&extra); e != io.EOF {
		return s, fmt.Errorf("trailing data or invalid gzip trailer: %v", e)
	}
	s.LastHash = prev
	s.Pending = control.CloneAssignment(chain.pending)
	return s, nil
}
