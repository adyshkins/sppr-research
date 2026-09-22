// Package recoveryrun is experiment-side code. Physical truth is written to a
// different channel and never supplied to recoverycontrol.
package recoveryrun

import (
	"compress/gzip"
	"dissertation.local/sppr-reconstruction/closedrun"
	"dissertation.local/sppr-reconstruction/control"
	"dissertation.local/sppr-reconstruction/execution"
	"dissertation.local/sppr-reconstruction/factorial"
	"dissertation.local/sppr-reconstruction/forecastpipe"
	"dissertation.local/sppr-reconstruction/plant"
	"dissertation.local/sppr-reconstruction/recovery"
	"dissertation.local/sppr-reconstruction/recoverycontrol"
	"dissertation.local/sppr-reconstruction/sensor"
	"dissertation.local/sppr-reconstruction/simenv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const Schema = "recovery-development-0.8.0"

type Config struct {
	Profile  string           `json:"profile"`
	Repeat   int              `json:"repeat"`
	Arm      string           `json:"arm"`
	Base     closedrun.Config `json:"base"`
	Recovery recovery.Config  `json:"recovery"`
}

func (c Config) Validate() error {
	if c.Profile == "" || c.Arm == "" || c.Repeat < 0 {
		return fmt.Errorf("invalid job identity")
	}
	if e := c.Base.Validate(); e != nil {
		return e
	}
	return c.Recovery.Validate()
}
func (c Config) Controller() recoverycontrol.Config {
	return recoverycontrol.Config{Base: c.Base.Controller, Recovery: c.Recovery}
}

type Record struct {
	Base           factorial.Compact `json:"base"`
	Recovery       recovery.Result   `json:"risk_exception"`
	FullOutputHash string            `json:"full_recovery_controller_hash"`
	Hash           string            `json:"hash"`
}

func Hash(r Record) (string, error) { r.Hash = ""; return control.Hash(r) }
func Condense(in control.Input, o recoverycontrol.Output, state any, ex execution.Resolution, c control.Config, prev string) (Record, error) {
	b, e := factorial.Condense(in, o.Primary, state, ex, c, prev)
	if e != nil {
		return Record{}, e
	}
	r := Record{Base: b, Recovery: o.Recovery}
	r.FullOutputHash, e = control.Hash(o)
	if e != nil {
		return r, e
	}
	r.Hash, e = Hash(r)
	return r, e
}
func (r Record) Assignment() *control.Assignment {
	if r.Base.Selection.Assignment != nil {
		return control.CloneAssignment(r.Base.Selection.Assignment)
	}
	return control.CloneAssignment(r.Recovery.Assignment)
}

type Phase struct {
	Days             int     `json:"days"`
	Loss             float64 `json:"loss"`
	KPILoss          float64 `json:"kpi_loss"`
	Cost             float64 `json:"cost"`
	RiskEmpty        int     `json:"risk_empty"`
	RecoveryProposed int     `json:"recovery_proposed"`
	RecoveryExecuted int     `json:"recovery_executed"`
}
type Summary struct {
	Profile                 string              `json:"profile"`
	Repeat                  int                 `json:"repeat"`
	Arm                     string              `json:"arm"`
	Days                    int                 `json:"days"`
	TotalLoss               float64             `json:"total_loss"`
	KPILoss                 float64             `json:"kpi_loss"`
	Cost                    float64             `json:"action_cost"`
	Demand                  plant.Pair          `json:"new_demand"`
	Shipped                 plant.Pair          `json:"shipped"`
	FinalState              plant.State         `json:"final_state"`
	KPIOutside              [4]int              `json:"kpi_outside_soft_range"`
	Forecasts               int                 `json:"forecasts"`
	Reasons                 map[string]int      `json:"primary_reasons"`
	Actions                 map[string]int      `json:"executed_actions"`
	RecoveryProposals       int                 `json:"recovery_proposals"`
	RecoveryAssignments     int                 `json:"recovery_assignments"`
	RecoveryExecutions      int                 `json:"recovery_executions"`
	RecoveryNoopExecutions  int                 `json:"recovery_noop_executions"`
	PrimaryExecutions       int                 `json:"primary_executions"`
	RiskEmptyFollowedByHold int                 `json:"risk_empty_followed_by_no_assignment"`
	HardViolations          int                 `json:"executed_assignment_hard_permission_violations"`
	Pending                 *control.Assignment `json:"pending_unexecuted_at_end"`
	PendingRecovery         bool                `json:"pending_is_recovery"`
	Phases                  map[string]*Phase   `json:"phases"`
}

func NewSummary(c Config) Summary {
	return Summary{Profile: c.Profile, Repeat: c.Repeat, Arm: c.Arm, Reasons: map[string]int{}, Actions: map[string]int{}, Phases: map[string]*Phase{"pre": {}, "active": {}, "post": {}}}
}
func PhaseFor(c Config, day int) string {
	if c.Base.Environment.Event.Label == "" || day < c.Base.Environment.Event.StartDay {
		return "pre"
	}
	if day <= c.Base.Environment.Event.EndDay {
		return "active"
	}
	return "post"
}
func Accumulate(s *Summary, c Config, r Record, p closedrun.PhysicalRecord, prevRec bool, prevEmpty bool) {
	s.Days++
	s.TotalLoss += p.Loss
	s.KPILoss += p.PeriodLoss
	s.Cost += p.Cost
	for j := 0; j < 2; j++ {
		s.Demand[j] += p.Tape.Forcing.Demand[j]
		s.Shipped[j] += p.Result.Shipped[j]
	}
	s.FinalState = p.Result.Next
	s.Actions[p.Execution.Action.ID]++
	s.Reasons[r.Base.Selection.Reason]++
	if r.Base.ForecastStatus == "ready" {
		s.Forecasts++
	}
	lo, hi := [4]float64{.95, .95, .7, 3}, [4]float64{1, 1, .9, 7}
	for j, v := range p.Result.KPI {
		if v < lo[j] || v > hi[j] {
			s.KPIOutside[j]++
		}
	}
	if r.Recovery.Proposal != nil {
		s.RecoveryProposals++
	}
	if r.Recovery.Assignment != nil {
		s.RecoveryAssignments++
	}
	if p.Execution.AppliedAssignmentID != nil {
		if prevRec {
			s.RecoveryExecutions++
			if p.Execution.Action.ID == "u0" {
				s.RecoveryNoopExecutions++
			}
		} else {
			s.PrimaryExecutions++
		}
	} else if prevEmpty {
		s.RiskEmptyFollowedByHold++
	}
	s.Pending = r.Assignment()
	s.PendingRecovery = r.Recovery.Assignment != nil
	ph := s.Phases[PhaseFor(c, p.Day)]
	ph.Days++
	ph.Loss += p.Loss
	ph.KPILoss += p.PeriodLoss
	ph.Cost += p.Cost
	if r.Recovery.Triggered {
		ph.RiskEmpty++
	}
	if r.Recovery.Proposal != nil {
		ph.RecoveryProposed++
	}
	if p.Execution.AppliedAssignmentID != nil && prevRec {
		ph.RecoveryExecuted++
	}
}

type Completion struct {
	Schema     string `json:"schema"`
	ConfigHash string `json:"config_hash"`
	TapeHash   string `json:"exogenous_tape_hash"`
	LastHash   string `json:"last_hash"`
	Records    int    `json:"records"`
}

func Equal(a, b any) bool {
	x, e := control.Hash(a)
	y, f := control.Hash(b)
	return e == nil && f == nil && x == y
}
func Run(c Config, dir string) (s Summary, err error) {
	s = NewSummary(c)
	if err = c.Validate(); err != nil {
		return s, err
	}
	if _, e := os.Stat(dir); !os.IsNotExist(e) {
		return s, fmt.Errorf("destination exists")
	}
	if err = os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return s, err
	}
	tmp, e := os.MkdirTemp(filepath.Dir(dir), ".recovery-partial-")
	if e != nil {
		return s, e
	}
	defer os.RemoveAll(tmp)
	if err = closedrun.Save(filepath.Join(tmp, "experiment_config.json"), c); err != nil {
		return s, err
	}
	tape, e := simenv.Tape(c.Base.Environment)
	if e != nil {
		return s, e
	}
	for _, x := range c.Base.Dropouts {
		for _, field := range x.Fields {
			v := tape[x.Day].Distortions[field]
			v.Missing = true
			tape[x.Day].Distortions[field] = v
		}
	}
	th, e := control.Hash(tape)
	if e != nil {
		return s, e
	}
	ch, e := control.Hash(c)
	if e != nil {
		return s, e
	}
	cf, e := os.Create(filepath.Join(tmp, "controller.jsonl.gz"))
	if e != nil {
		return s, e
	}
	defer cf.Close()
	cg := gzip.NewWriter(cf)
	defer cg.Close()
	ce := json.NewEncoder(cg)
	pf, e := os.Create(filepath.Join(tmp, "physical.jsonl.gz"))
	if e != nil {
		return s, e
	}
	defer pf.Close()
	pg := gzip.NewWriter(pf)
	defer pg.Close()
	pe := json.NewEncoder(pg)
	controller, e := recoverycontrol.New(c.Controller(), 0)
	if e != nil {
		return s, e
	}
	channel, e := sensor.New(0)
	if e != nil {
		return s, e
	}
	reg := closedrun.InitialRegister(c.Base)
	truth := c.Base.Environment.Initial
	previous := ch
	var pending *control.Assignment
	prevRec, prevEmpty := false, false
	for day, td := range tape {
		ex, e := execution.Resolve(day, pending, reg)
		if e != nil {
			return s, e
		}
		res, e := plant.Step(truth, td.Forcing, ex.Action, c.Base.Environment.Parameters)
		if e != nil {
			return s, e
		}
		if [2]float64(res.Next.Plan) != ex.After.Plan.Base {
			return s, fmt.Errorf("plan mismatch")
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
		in := control.Input{CaseID: fmt.Sprintf("%s/%d", c.Base.Environment.EpisodeID, day), Forecast: forecastpipe.Input{Packet: pkt, Plan: ex.After.Plan, ForecastSeed: c.Base.ForecastSeed}, Limits: control.CloneLimits(ex.After.Limits)}
		out, e := controller.Process(in)
		if e != nil {
			return s, fmt.Errorf("day %d: %w", day, e)
		}
		row, e := Condense(in, out, controller.Snapshot(), ex, c.Base.Controller, previous)
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
					s.HardViolations++
				}
			}
		}
		Accumulate(&s, c, row, pr, prevRec, prevEmpty)
		previous = row.Hash
		truth = res.Next
		reg = ex.After
		pending = out.Assignment()
		prevRec = out.Recovery.Assignment != nil
		prevEmpty = out.Recovery.Triggered
	}
	for _, f := range []func() error{cg.Close, cf.Close, pg.Close, pf.Close} {
		if e = f(); e != nil {
			return s, e
		}
	}
	if e = closedrun.Save(filepath.Join(tmp, "summary.json"), s); e != nil {
		return s, e
	}
	if e = closedrun.Save(filepath.Join(tmp, "COMPLETED.json"), Completion{Schema, ch, th, previous, c.Base.Environment.Days}); e != nil {
		return s, e
	}
	return s, os.Rename(tmp, dir)
}

// Replay uses saved observed inputs only. The physical file can be removed.
func Replay(dir string) (int, error) {
	var c Config
	var done Completion
	if e := factorial.ReadJSON(filepath.Join(dir, "experiment_config.json"), &c); e != nil {
		return 0, e
	}
	if e := c.Validate(); e != nil {
		return 0, e
	}
	if e := factorial.ReadJSON(filepath.Join(dir, "COMPLETED.json"), &done); e != nil {
		return 0, e
	}
	h, e := control.Hash(c)
	if e != nil || h != done.ConfigHash || done.Schema != Schema || done.Records != c.Base.Environment.Days {
		return 0, fmt.Errorf("invalid config/completion")
	}
	f, e := os.Open(filepath.Join(dir, "controller.jsonl.gz"))
	if e != nil {
		return 0, e
	}
	defer f.Close()
	g, e := gzip.NewReader(f)
	if e != nil {
		return 0, e
	}
	defer g.Close()
	d := json.NewDecoder(g)
	ctrl, e := recoverycontrol.New(c.Controller(), 0)
	if e != nil {
		return 0, e
	}
	reg := closedrun.InitialRegister(c.Base)
	var pending *control.Assignment
	for day := 0; day < done.Records; day++ {
		var row Record
		if e = d.Decode(&row); e != nil {
			return day, e
		}
		if row.Base.Day != day || row.Base.Previous != h || row.Base.Input.CaseID != fmt.Sprintf("%s/%d", c.Base.Environment.EpisodeID, day) || row.Base.Input.Forecast.ForecastSeed != c.Base.ForecastSeed {
			return day, fmt.Errorf("sequence/seed mismatch")
		}
		ex, e := execution.Resolve(day, pending, reg)
		if e != nil {
			return day, e
		}
		if !Equal(ex, row.Base.Execution) || !Equal(ex.After.Plan, row.Base.Input.Forecast.Plan) || !Equal(ex.After.Limits, row.Base.Input.Limits) {
			return day, fmt.Errorf("execution/register mismatch")
		}
		if e = execution.VerifyObservedContext(row.Base.Input.Forecast.Packet, ex); e != nil {
			return day, e
		}
		out, e := ctrl.Process(row.Base.Input)
		if e != nil {
			return day, e
		}
		got, e := Condense(row.Base.Input, out, ctrl.Snapshot(), ex, c.Base.Controller, h)
		if e != nil {
			return day, e
		}
		if !Equal(got, row) {
			return day, fmt.Errorf("replay differs on day %d", day)
		}
		h = row.Hash
		reg = ex.After
		pending = out.Assignment()
	}
	var extra any
	if e = d.Decode(&extra); e != io.EOF {
		return done.Records, fmt.Errorf("extra/malformed record")
	}
	if h != done.LastHash {
		return done.Records, fmt.Errorf("last hash mismatch")
	}
	return done.Records, nil
}
