package factorial

import (
	"compress/gzip"
	"dissertation.local/sppr-reconstruction/analysispipe"
	"dissertation.local/sppr-reconstruction/closedrun"
	"dissertation.local/sppr-reconstruction/control"
	"dissertation.local/sppr-reconstruction/controltrace"
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/execution"
	"dissertation.local/sppr-reconstruction/forecast"
	"dissertation.local/sppr-reconstruction/forecastpipe"
	"dissertation.local/sppr-reconstruction/plant"
	"dissertation.local/sppr-reconstruction/sensor"
	"dissertation.local/sppr-reconstruction/simenv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type Shadow struct {
	Policy         core.Policy `json:"policy"`
	Reason         string      `json:"reason"`
	Recommendation *string     `json:"recommendation_id"`
	Eligible       []string    `json:"eligible_ids"`
	Route          string      `json:"route"`
}
type Compact struct {
	Day            int                  `json:"day"`
	Input          control.Input        `json:"input"`
	Execution      execution.Resolution `json:"execution"`
	Analysis       analysispipe.Result  `json:"analysis"`
	ForecastStatus string               `json:"forecast_status"`
	ForecastReason string               `json:"forecast_reason"`
	Estimate       *forecast.Estimate   `json:"observation_estimate"`
	Weights        []float64            `json:"ordered_scenario_weights"`
	Actions        []plant.Action       `json:"actions"`
	Selection      control.Selection    `json:"selection"`
	Shadows        []Shadow             `json:"same_information_choices"`
	FullOutputHash string               `json:"complete_controller_output_hash"`
	StateHash      string               `json:"monitor_state_hash"`
	Previous       string               `json:"previous_hash"`
	Hash           string               `json:"hash"`
}
type Completion struct {
	Schema     string `json:"schema"`
	Notice     string `json:"notice"`
	ConfigHash string `json:"config_hash"`
	TapeHash   string `json:"exogenous_tape_hash"`
	LastHash   string `json:"last_hash"`
	Records    int    `json:"records"`
}

func equal(a, b any) bool {
	x, e := control.Hash(a)
	y, f := control.Hash(b)
	return e == nil && f == nil && x == y
}
func compactHash(c Compact) (string, error) { c.Hash = ""; return control.Hash(c) }
func Condense(in control.Input, out control.Output, state any, ex execution.Resolution, c control.Config, previous string) (Compact, error) {
	r := Compact{Day: in.Forecast.Packet.Day, Input: in, Execution: ex, Analysis: out.Pipeline.Analysis, ForecastStatus: out.Pipeline.Forecast.Status, ForecastReason: out.Pipeline.Forecast.Reason, Estimate: out.Pipeline.Forecast.Admission.Estimate, Selection: out.Selection, Previous: previous, Shadows: []Shadow{}, Weights: []float64{}, Actions: []plant.Action{}}
	var e error
	if r.FullOutputHash, e = control.Hash(out); e != nil {
		return r, e
	}
	if r.StateHash, e = control.Hash(state); e != nil {
		return r, e
	}
	if f := out.Pipeline.Forecast; f.Ensemble != nil && f.Status == "ready" {
		paths := append([]forecast.ScenarioPath{}, f.Ensemble.Paths...)
		sort.Slice(paths, func(i, j int) bool { return paths[i].ID < paths[j].ID })
		for _, p := range paths {
			r.Weights = append(r.Weights, p.Probability)
		}
		for _, a := range f.Alternatives {
			r.Actions = append(r.Actions, control.CloneAction(a.Action))
		}
		for _, policy := range core.FactorialPolicies() {
			cfg := control.CloneConfig(c)
			cfg.Policy = policy
			s, err := control.Select(in, out.Pipeline, cfg)
			if err != nil {
				return r, err
			}
			sh := Shadow{Policy: policy, Reason: s.Reason, Route: s.Route, Eligible: []string{}}
			if s.Decision != nil {
				sh.Recommendation = s.Decision.RecommendationID
				sh.Eligible = s.Decision.EligibleIDs
			}
			r.Shadows = append(r.Shadows, sh)
			if policy == c.Policy && !equal(s, out.Selection) {
				return r, fmt.Errorf("own shadow differs from actual selection")
			}
		}
	}
	r.Hash, e = compactHash(r)
	return r, e
}
func initializeSummary() closedrun.Summary {
	return closedrun.Summary{Notice: "DEV-F06 development synthetic episode, research execution without human review; NOT final efficacy/industrial evidence", Controller: controltrace.Summary{Routes: map[string]int{}, Reasons: map[string]int{}, Actions: map[string]int{}}}
}
func accumulate(s *closedrun.Summary, r closedrun.PhysicalRecord, c Compact) {
	a := &s.Controller
	a.Records++
	a.Routes[c.Selection.Route]++
	a.Reasons[c.Selection.Reason]++
	a.Actions[r.Execution.Action.ID]++
	if c.ForecastStatus == "ready" {
		a.Forecasts++
	}
	if c.Selection.Recommendation != nil {
		a.Recommendations++
	}
	if c.Selection.Assignment != nil {
		a.Assignments++
	}
	if r.Execution.AppliedAssignmentID != nil {
		a.AppliedAssignments++
		if r.Execution.Requested.ExecutionMode == control.ResearchNoReview && r.Execution.Requested.Route != "auto" {
			a.ResearchBypasses++
		}
	}
	if r.Execution.Action.ID != "u0" {
		a.Corrections++
	}
	a.LastHash = c.Hash
	a.Pending = control.CloneAssignment(c.Selection.Assignment)
	s.TotalLoss += r.Loss
	s.TotalKPILoss += r.PeriodLoss
	s.TotalActionCost += r.Cost
	lo, hi := [4]float64{.95, .95, .7, 3}, [4]float64{1, 1, .9, 7}
	for j, v := range r.Result.KPI {
		if v < lo[j] || v > hi[j] {
			s.KPIViolationDays[j]++
		}
	}
	for j := 0; j < 2; j++ {
		s.TotalDemand[j] += r.Tape.Forcing.Demand[j]
		s.TotalShipped[j] += r.Result.Shipped[j]
	}
	s.FinalBacklog = r.Result.Next.Backlog
	s.FinalRaw = r.Result.Next.Raw
}

// Run retains observations and predictive loss distributions, not every forecast
// balance step. Complete controller outputs are regenerated and checked by hash.
// Atomic directory publication ensures failures cannot be included as complete.
func Run(c closedrun.Config, dir string) (s closedrun.Summary, err error) {
	s = initializeSummary()
	if err = c.Validate(); err != nil {
		return s, err
	}
	if _, e := os.Stat(dir); !os.IsNotExist(e) {
		return s, fmt.Errorf("destination must not exist")
	}
	if err = os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return s, err
	}
	tmp, e := os.MkdirTemp(filepath.Dir(dir), ".factorial-partial-")
	if e != nil {
		return s, e
	}
	defer os.RemoveAll(tmp)
	if err = closedrun.Save(filepath.Join(tmp, "experiment_config.json"), c); err != nil {
		return s, err
	}
	tape, e := simenv.Tape(c.Environment)
	if e != nil {
		return s, e
	}
	for _, x := range c.Dropouts {
		for _, field := range x.Fields {
			d := tape[x.Day].Distortions[field]
			d.Missing = true
			tape[x.Day].Distortions[field] = d
		}
	}
	th, e := control.Hash(tape)
	if e != nil {
		return s, e
	}
	cfgHash, e := control.Hash(c)
	if e != nil {
		return s, e
	}
	cf, e := os.Create(filepath.Join(tmp, "compact_controller.jsonl.gz"))
	if e != nil {
		return s, e
	}
	defer cf.Close()
	cg := gzip.NewWriter(cf)
	defer cg.Close()
	ce := json.NewEncoder(cg)
	pf, e := os.Create(filepath.Join(tmp, "physical_evaluation.jsonl.gz"))
	if e != nil {
		return s, e
	}
	defer pf.Close()
	pg := gzip.NewWriter(pf)
	defer pg.Close()
	pe := json.NewEncoder(pg)
	controller, e := control.New(c.Controller, 0)
	if e != nil {
		return s, e
	}
	channel, e := sensor.New(0)
	if e != nil {
		return s, e
	}
	reg := closedrun.InitialRegister(c)
	truth := c.Environment.Initial
	var pending *control.Assignment
	previous := cfgHash
	for day, td := range tape {
		ex, e := execution.Resolve(day, pending, reg)
		if e != nil {
			return s, e
		}
		res, e := plant.Step(truth, td.Forcing, ex.Action, c.Environment.Parameters)
		if e != nil {
			return s, e
		}
		if [2]float64(res.Next.Plan) != ex.After.Plan.Base {
			return s, fmt.Errorf("physical/registered plan mismatch")
		}
		fresh, e := sensor.ClosedDay(day, truth, td.Forcing, ex.Action, res, ex.After.Plan.Version, ex.After.Limits.Version)
		if e != nil {
			return s, e
		}
		pkt, e := channel.Transmit(fresh, td.Distortions)
		if e != nil {
			return s, e
		}
		if e = execution.VerifyObservedContext(pkt, ex); e != nil {
			return s, e
		}
		in := control.Input{CaseID: fmt.Sprintf("%s/%d", c.Environment.EpisodeID, day), Forecast: forecastpipe.Input{Packet: pkt, Plan: ex.After.Plan, ForecastSeed: c.ForecastSeed}, Limits: control.CloneLimits(ex.After.Limits)}
		out, e := controller.Process(in)
		if e != nil {
			return s, fmt.Errorf("day %d: %w", day, e)
		}
		row, e := Condense(in, out, controller.Snapshot(), ex, c.Controller, previous)
		if e != nil {
			return s, e
		}
		if e = ce.Encode(row); e != nil {
			return s, e
		}
		loss, e := plant.PeriodLoss(res.KPI)
		if e != nil {
			return s, e
		}
		pr := closedrun.PhysicalRecord{Day: day, Tape: td, Before: truth, Execution: ex, Result: res, PeriodLoss: loss, Cost: ex.Action.Cost, Loss: loss + ex.Action.Cost}
		if e = pe.Encode(pr); e != nil {
			return s, e
		}
		if ex.AppliedAssignmentID != nil {
			checks, e := control.CheckAction(ex.Action, reg.Limits)
			if e != nil {
				return s, e
			}
			for _, v := range checks {
				if v.Residual > 0 {
					s.AssignedHardViolations++
				}
			}
		}
		accumulate(&s, pr, row)
		previous = row.Hash
		truth = res.Next
		reg = ex.After
		pending = control.CloneAssignment(out.Selection.Assignment)
	}
	for _, close := range []func() error{cg.Close, cf.Close, pg.Close, pf.Close} {
		if e = close(); e != nil {
			return s, e
		}
	}
	if e = closedrun.Save(filepath.Join(tmp, "summary.json"), s); e != nil {
		return s, e
	}
	done := Completion{Schema, "Synthetic development; compact trace requires seeds+code to regenerate full predictions", cfgHash, th, previous, c.Environment.Days}
	if e = closedrun.Save(filepath.Join(tmp, "COMPLETED.json"), done); e != nil {
		return s, e
	}
	return s, os.Rename(tmp, dir)
}
func ReadJSON(path string, out any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, out)
}

// Replay reads no physical file and calls no environment generator. It restores
// the controller solely from the published config and compact observed inputs.
func Replay(dir string) (int, error) {
	var c closedrun.Config
	var done Completion
	if e := ReadJSON(filepath.Join(dir, "experiment_config.json"), &c); e != nil {
		return 0, e
	}
	if e := c.Validate(); e != nil {
		return 0, e
	}
	if e := ReadJSON(filepath.Join(dir, "COMPLETED.json"), &done); e != nil {
		return 0, e
	}
	hash, e := control.Hash(c)
	if e != nil || hash != done.ConfigHash || done.Schema != Schema || done.Records != c.Environment.Days {
		return 0, fmt.Errorf("invalid completion/config")
	}
	f, e := os.Open(filepath.Join(dir, "compact_controller.jsonl.gz"))
	if e != nil {
		return 0, e
	}
	defer f.Close()
	g, e := gzip.NewReader(f)
	if e != nil {
		return 0, e
	}
	defer g.Close()
	dec := json.NewDecoder(g)
	ctrl, e := control.New(c.Controller, 0)
	if e != nil {
		return 0, e
	}
	reg := closedrun.InitialRegister(c)
	var pending *control.Assignment
	for day := 0; day < c.Environment.Days; day++ {
		var row Compact
		if e = dec.Decode(&row); e != nil {
			return day, e
		}
		if row.Day != day || row.Previous != hash || row.Input.CaseID != fmt.Sprintf("%s/%d", c.Environment.EpisodeID, day) || row.Input.Forecast.ForecastSeed != c.ForecastSeed {
			return day, fmt.Errorf("sequence/seed mismatch")
		}
		ex, e := execution.Resolve(day, pending, reg)
		if e != nil {
			return day, e
		}
		if !equal(ex, row.Execution) || !equal(ex.After.Plan, row.Input.Forecast.Plan) || !equal(ex.After.Limits, row.Input.Limits) {
			return day, fmt.Errorf("execution/registry mismatch")
		}
		if e = execution.VerifyObservedContext(row.Input.Forecast.Packet, ex); e != nil {
			return day, e
		}
		out, e := ctrl.Process(row.Input)
		if e != nil {
			return day, e
		}
		got, e := Condense(row.Input, out, ctrl.Snapshot(), ex, c.Controller, hash)
		if e != nil {
			return day, e
		}
		if !equal(got, row) {
			return day, fmt.Errorf("recomputed controller differs at %d", day)
		}
		hash = row.Hash
		reg = ex.After
		pending = control.CloneAssignment(out.Selection.Assignment)
	}
	var extra any
	if e = dec.Decode(&extra); e != io.EOF {
		return done.Records, fmt.Errorf("extra/malformed records")
	}
	if hash != done.LastHash {
		return done.Records, fmt.Errorf("last hash mismatch")
	}
	return done.Records, nil
}
