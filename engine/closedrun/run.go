// Package closedrun is the EXPERIMENT side of the closed loop. Its truth, tape
// and labels are never accepted by the observation-only control.Controller.
package closedrun

import (
	"compress/gzip"
	"dissertation.local/sppr-reconstruction/control"
	"dissertation.local/sppr-reconstruction/controltrace"
	"dissertation.local/sppr-reconstruction/execution"
	"dissertation.local/sppr-reconstruction/forecast"
	"dissertation.local/sppr-reconstruction/forecastpipe"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/plant"
	"dissertation.local/sppr-reconstruction/sensor"
	"dissertation.local/sppr-reconstruction/simenv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Dropout struct {
	Day    int                 `json:"day"`
	Fields []observation.Field `json:"missing_fields"`
}
type Config struct {
	Notice        string         `json:"notice"`
	Environment   simenv.Config  `json:"environment"`
	Controller    control.Config `json:"controller"`
	ForecastSeed  uint64         `json:"forecast_seed"`
	InitialLimits control.Limits `json:"initial_command_permissions"`
	Dropouts      []Dropout      `json:"deterministic_observation_faults"`
}

func (c Config) Validate() error {
	if c.Notice == "" {
		return fmt.Errorf("experiment notice required")
	}
	if e := c.Environment.Validate(); e != nil {
		return e
	}
	if e := c.Controller.Validate(); e != nil {
		return e
	}
	if e := c.InitialLimits.Validate(); e != nil {
		return e
	}
	f := c.Controller.Pipeline.Forecast
	if c.Environment.Parameters != f.Parameters || c.Environment.BaseCapacity != f.BaseCapacity || c.Environment.BaseRaw != f.BaseRaw || c.InitialLimits.SnapshotDay != -1 {
		return fmt.Errorf("environment/model/initial register mismatch")
	}
	seen := map[int]bool{}
	allowed := map[observation.Field]bool{}
	for _, f := range observation.Fields() {
		allowed[f] = true
	}
	for _, x := range c.Dropouts {
		if x.Day < 0 || x.Day >= c.Environment.Days || seen[x.Day] || len(x.Fields) == 0 {
			return fmt.Errorf("invalid fault schedule")
		}
		seen[x.Day] = true
		fields := map[observation.Field]bool{}
		for _, f := range x.Fields {
			if !allowed[f] || fields[f] {
				return fmt.Errorf("invalid fault field")
			}
			fields[f] = true
		}
	}
	return nil
}
func InitialRegister(c Config) execution.Register {
	return execution.Register{Plan: forecast.KnownPlan{Base: [2]float64(c.Environment.Initial.Plan), Version: "registered-initial-plan-v05", ConstraintVersion: c.InitialLimits.Version, SnapshotDay: -1}, Limits: control.CloneLimits(c.InitialLimits)}
}

type PhysicalRecord struct {
	Day        int                  `json:"day"`
	Tape       simenv.TapeDay       `json:"tape_evaluation_only"`
	Before     plant.State          `json:"true_state_before_evaluation_only"`
	Execution  execution.Resolution `json:"execution"`
	Result     plant.Result         `json:"physical_result_evaluation_only"`
	PeriodLoss float64              `json:"period_kpi_loss"`
	Cost       float64              `json:"executed_action_cost"`
	Loss       float64              `json:"physical_loss_plus_action_cost"`
}
type Summary struct {
	Notice                 string               `json:"notice"`
	Controller             controltrace.Summary `json:"controller_summary"`
	TotalKPILoss           float64              `json:"sum_physical_kpi_loss"`
	TotalActionCost        float64              `json:"sum_executed_action_cost"`
	TotalLoss              float64              `json:"undiscounted_episode_loss"`
	TotalDemand            plant.Pair           `json:"total_new_demand"`
	TotalShipped           plant.Pair           `json:"total_shipped"`
	FinalBacklog           plant.Pair           `json:"final_backlog"`
	FinalRaw               float64              `json:"final_raw"`
	KPIViolationDays       [4]int               `json:"physical_kpi_outside_soft_range_days"`
	AssignedHardViolations int                  `json:"assigned_hard_permission_violations"`
}

func Save(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
func Run(c Config, dir string) (s Summary, err error) {
	s.Notice = "Engineering episode, not final efficacy estimate; soft KPI violations are not hard permission violations. No real company or expert."
	if err = c.Validate(); err != nil {
		return s, err
	}
	if entries, e := os.ReadDir(dir); e == nil && len(entries) > 0 {
		return s, fmt.Errorf("output directory not empty")
	} else if e != nil && !os.IsNotExist(e) {
		return s, e
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return s, err
	}
	if err = Save(filepath.Join(dir, "experiment_config.json"), c); err != nil {
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
	register := InitialRegister(c)
	spec := controltrace.Spec{ID: c.Environment.EpisodeID, Days: c.Environment.Days, ForecastSeed: c.ForecastSeed, Initial: register, Config: c.Controller}
	writer, e := controltrace.NewWriter(filepath.Join(dir, "controller_trace.jsonl.gz"), spec)
	if e != nil {
		return s, e
	}
	defer writer.Abort()
	physical, e := os.Create(filepath.Join(dir, "physical_evaluation.jsonl.gz"))
	if e != nil {
		return s, e
	}
	defer physical.Close()
	g := gzip.NewWriter(physical)
	defer g.Close()
	enc := json.NewEncoder(g)
	controller, e := control.New(c.Controller, 0)
	if e != nil {
		return s, e
	}
	channel, e := sensor.New(0)
	if e != nil {
		return s, e
	}
	truth := c.Environment.Initial
	var pending *control.Assignment
	lo, hi := [4]float64{.95, .95, .7, 3}, [4]float64{1, 1, .9, 7}
	for day, td := range tape {
		resolution, e := execution.Resolve(day, pending, register)
		if e != nil {
			return s, e
		}
		result, e := plant.Step(truth, td.Forcing, resolution.Action, c.Environment.Parameters)
		if e != nil {
			return s, e
		}
		if [2]float64(result.Next.Plan) != resolution.After.Plan.Base {
			return s, fmt.Errorf("plant/registered plan mismatch")
		}
		fresh, e := sensor.ClosedDay(day, truth, td.Forcing, resolution.Action, result, resolution.After.Plan.Version, resolution.After.Limits.Version)
		if e != nil {
			return s, e
		}
		packet, e := channel.Transmit(fresh, td.Distortions)
		if e != nil {
			return s, e
		}
		if e = execution.VerifyObservedContext(packet, resolution); e != nil {
			return s, e
		}
		in := control.Input{CaseID: fmt.Sprintf("%s/%d", spec.ID, day), Forecast: forecastpipe.Input{Packet: packet, Plan: resolution.After.Plan, ForecastSeed: c.ForecastSeed}, Limits: control.CloneLimits(resolution.After.Limits)}
		out, e := controller.Process(in)
		if e != nil {
			return s, fmt.Errorf("controller day %d: %w", day, e)
		}
		if e = writer.Append(controltrace.Payload{Index: day, Input: in, Output: out, State: controller.Snapshot(), Execution: resolution}); e != nil {
			return s, e
		}
		l, e := plant.PeriodLoss(result.KPI)
		if e != nil {
			return s, e
		}
		rec := PhysicalRecord{Day: day, Tape: td, Before: truth, Execution: resolution, Result: result, PeriodLoss: l, Cost: resolution.Action.Cost, Loss: l + resolution.Action.Cost}
		if e = enc.Encode(rec); e != nil {
			return s, e
		}
		s.TotalKPILoss += l
		s.TotalActionCost += resolution.Action.Cost
		s.TotalLoss += rec.Loss
		for j, v := range result.KPI {
			if v < lo[j] || v > hi[j] {
				s.KPIViolationDays[j]++
			}
		}
		for j := 0; j < 2; j++ {
			s.TotalDemand[j] += td.Forcing.Demand[j]
			s.TotalShipped[j] += result.Shipped[j]
		}
		if resolution.AppliedAssignmentID != nil {
			checks, _ := control.CheckAction(resolution.Action, register.Limits)
			for _, v := range checks {
				if v.Residual > 0 {
					s.AssignedHardViolations++
				}
			}
		}
		truth = result.Next
		register = resolution.After
		pending = control.CloneAssignment(out.Selection.Assignment)
	}
	s.FinalBacklog = truth.Backlog
	s.FinalRaw = truth.Raw
	if e = g.Close(); e != nil {
		return s, e
	}
	if e = physical.Close(); e != nil {
		return s, e
	}
	s.Controller, e = writer.Close()
	if e != nil {
		return s, e
	}
	if e = Save(filepath.Join(dir, "summary.json"), s); e != nil {
		return s, e
	}
	return s, nil
}
