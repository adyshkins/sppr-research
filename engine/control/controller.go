package control

import (
	"crypto/sha256"
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/forecastpipe"
	"dissertation.local/sppr-reconstruction/monitor"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/plant"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
)

const HonorRoutes = "honor_routes_no_human_approval"
const ResearchNoReview = "simulation_no_review_not_human_approval"

type Routing struct {
	ExpertRisk        float64 `json:"expert_risk"`
	CommitteeRisk     float64 `json:"committee_risk"`
	ExpertDistance    float64 `json:"expert_plan_distance"`
	CommitteeDistance float64 `json:"committee_plan_distance"`
}

func DefaultRouting() Routing { return Routing{.35, .75, .10, .25} }
func (r Routing) Validate() error {
	for _, v := range []float64{r.ExpertRisk, r.CommitteeRisk, r.ExpertDistance, r.CommitteeDistance} {
		if !observation.FiniteNonnegative(v) {
			return fmt.Errorf("bad route threshold")
		}
	}
	if r.ExpertRisk >= r.CommitteeRisk || r.ExpertDistance >= r.CommitteeDistance {
		return fmt.Errorf("unordered route thresholds")
	}
	return nil
}
func (r Routing) Route(risk, distance float64, expert bool) (string, error) {
	if e := r.Validate(); e != nil {
		return "", e
	}
	if !observation.FiniteNonnegative(risk) || !observation.FiniteNonnegative(distance) {
		return "", fmt.Errorf("invalid routing input")
	}
	if risk >= r.CommitteeRisk || distance >= r.CommitteeDistance {
		return "committee", nil
	}
	if expert || risk >= r.ExpertRisk || distance >= r.ExpertDistance {
		return "expert", nil
	}
	return "auto", nil
}

type Config struct {
	Pipeline      forecastpipe.Config `json:"pipeline"`
	Choice        core.Config         `json:"choice"`
	Policy        core.Policy         `json:"risk_policy"`
	Routing       Routing             `json:"routing"`
	ExecutionMode string              `json:"execution_mode"`
}

func (c Config) Validate() error {
	if e := c.Pipeline.Validate(); e != nil {
		return e
	}
	if e := c.Choice.Validate(); e != nil {
		return e
	}
	if e := c.Policy.Validate(); e != nil {
		return e
	}
	if e := c.Routing.Validate(); e != nil {
		return e
	}
	if c.Choice.Horizon != c.Pipeline.Forecast.Horizon || c.Choice.Discount != c.Pipeline.Forecast.Discount || c.Choice.Alpha != c.Pipeline.Forecast.Alpha {
		return fmt.Errorf("choice/forecast configuration mismatch")
	}
	if c.ExecutionMode != HonorRoutes && c.ExecutionMode != ResearchNoReview {
		return fmt.Errorf("unknown execution mode")
	}
	return nil
}
func CloneConfig(c Config) Config { c.Pipeline = forecastpipe.CloneConfig(c.Pipeline); return c }

type Input struct {
	CaseID   string             `json:"case_id"`
	Forecast forecastpipe.Input `json:"forecast_input"`
	Limits   Limits             `json:"known_permissions"`
}
type Basis struct {
	ClosedDay         int    `json:"closed_day"`
	InformationHash   string `json:"information_hash"`
	PlanVersion       string `json:"plan_version"`
	ConstraintVersion string `json:"constraint_version"`
}
type Assignment struct {
	ID            string       `json:"id"`
	ExecuteDay    int          `json:"execute_day"`
	Action        plant.Action `json:"action"`
	Basis         Basis        `json:"basis"`
	Route         string       `json:"original_route"`
	ExecutionMode string       `json:"execution_mode"`
}

func CloneAction(a plant.Action) plant.Action {
	if a.PermanentPlan != nil {
		v := *a.PermanentPlan
		a.PermanentPlan = &v
	}
	return a
}
func CloneAssignment(a *Assignment) *Assignment {
	if a == nil {
		return nil
	}
	v := *a
	v.Action = CloneAction(v.Action)
	return &v
}
func (a Assignment) Validate() error {
	if a.ID == "" || a.Basis.ClosedDay < 0 || a.ExecuteDay != a.Basis.ClosedDay+1 || a.Basis.PlanVersion == "" || a.Basis.ConstraintVersion == "" {
		return fmt.Errorf("invalid assignment identity/time")
	}
	b, e := hex.DecodeString(a.Basis.InformationHash)
	if e != nil || len(b) != 32 {
		return fmt.Errorf("invalid assignment information hash")
	}
	if a.Route != "auto" && a.Route != "expert" && a.Route != "committee" {
		return fmt.Errorf("invalid assignment route")
	}
	if a.ExecutionMode != HonorRoutes && a.ExecutionMode != ResearchNoReview {
		return fmt.Errorf("invalid assignment mode")
	}
	if a.ExecutionMode == HonorRoutes && a.Route != "auto" {
		return fmt.Errorf("unapproved review route cannot be assigned")
	}
	d, e := plant.FixedAction(a.Action.ID, a.Action.PermanentPlan)
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(a.Action, d) {
		return fmt.Errorf("malformed assigned action")
	}
	return nil
}

type Selection struct {
	Basis             Basis                        `json:"basis"`
	PermissionReasons []string                     `json:"permission_reasons"`
	Decision          *core.Decision               `json:"decision"`
	HardChecks        map[string][]core.Constraint `json:"hard_checks"`
	Recommendation    *plant.Action                `json:"recommendation"`
	Route             string                       `json:"route"`
	Reason            string                       `json:"reason"`
	Assignment        *Assignment                  `json:"assignment"`
	AssignmentReason  string                       `json:"assignment_reason"`
}
type Output struct {
	Version   string              `json:"version"`
	Pipeline  forecastpipe.Result `json:"pipeline"`
	Selection Selection           `json:"selection"`
}

func Hash(v any) (string, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return "", e
	}
	s := sha256.Sum256(b)
	return fmt.Sprintf("%x", s[:]), nil
}

// Select accepts generated A3-A5 results, never environment state/labels/tape.
func Select(in Input, out forecastpipe.Result, c Config) (Selection, error) {
	s := Selection{PermissionReasons: []string{}, HardChecks: map[string][]core.Constraint{}, AssignmentReason: "NO_NEW_ACTION_CONTINUE_BASE_PLAN_NOT_CERTIFIED_SAFE"}
	if e := c.Validate(); e != nil {
		return s, e
	}
	if in.CaseID == "" {
		return s, fmt.Errorf("case ID required")
	}
	if e := in.Limits.Validate(); e != nil {
		return s, e
	}
	day := in.Forecast.Packet.Day
	hash, e := Hash(in)
	if e != nil {
		return s, e
	}
	s.Basis = Basis{day, hash, in.Forecast.Plan.Version, in.Forecast.Plan.ConstraintVersion}
	if out.Analysis.Day != day {
		return s, fmt.Errorf("analysis day mismatch")
	}
	switch out.Analysis.Monitor.Status {
	case core.Normal:
		s.Route = "normal"
		s.Reason = "NO_DEVIATION"
		return s, nil
	case core.DataIncident:
		s.Route = "data_check"
		s.Reason = "MONITOR_INFORMATION_UNAVAILABLE"
		return s, nil
	case core.Deviation:
	default:
		return s, fmt.Errorf("unknown monitor status")
	}
	f := out.Forecast
	if f.Status != "ready" {
		s.Route = "data_check"
		if f.Status == "expert" || f.Status == "no_candidates" {
			s.Route = "expert"
		}
		// Diagnostic no-candidate / low applicability can have status review.
		if f.Status == "review" || f.Status == "expert_review" {
			s.Route = "expert"
		}
		s.Reason = f.Reason
		return s, nil
	}
	s.PermissionReasons, e = LimitsReasons(in.Limits, in.Forecast.Plan, day)
	if e != nil {
		return s, e
	}
	if len(s.PermissionReasons) > 0 {
		s.Route = "data_check"
		s.Reason = "PERMISSION_INFORMATION_INSUFFICIENT"
		return s, nil
	}
	if !f.Admission.Passed || f.Admission.Estimate == nil || f.Ensemble == nil || f.Ensemble.ClosedDay != day || len(f.Alternatives) != 8 {
		return s, fmt.Errorf("incomplete ready forecast")
	}
	ci := core.Case{ID: in.CaseID, InformationVersion: hash, Status: core.Deviation, Config: c.Choice, BaselineID: "u0", RequiredConstraints: ConstraintIDs()}
	for _, p := range f.Ensemble.Paths {
		ci.Scenarios = append(ci.Scenarios, core.Scenario{ID: p.ID, Probability: p.Probability})
	}
	actions := map[string]plant.Action{}
	pred := map[string]core.Risk{}
	for _, a := range f.Alternatives {
		id := a.Action.ID
		if _, ok := actions[id]; ok {
			return s, fmt.Errorf("duplicate alternative")
		}
		actions[id] = CloneAction(a.Action)
		pred[id] = a.Risk
		checks, e := CheckAction(a.Action, in.Limits)
		if e != nil {
			return s, e
		}
		s.HardChecks[id] = checks
		distance, e := PlanDistance(a.Action, in.Forecast.Plan)
		if e != nil {
			return s, e
		}
		candidate := core.Candidate{ID: id, ActionCost: a.Action.Cost, PlanDistance: distance, Constraints: checks}
		for _, tr := range a.Trajectories {
			candidate.Trajectories = append(candidate.Trajectories, core.Trajectory{ScenarioID: tr.ScenarioID, PeriodLosses: tr.PeriodLosses})
		}
		ci.Candidates = append(ci.Candidates, candidate)
	}
	d, e := core.Decide(ci, c.Policy)
	if e != nil {
		return s, e
	}
	s.Decision = &d
	s.Reason = d.Reason
	for _, ev := range d.Evaluations {
		if ev.Risk != nil {
			p := pred[ev.ID]
			if math.Abs(ev.Risk.CVaR-p.CVaR) > 1e-10*math.Max(1, math.Abs(p.CVaR)) || math.Abs(ev.Risk.Expected-p.Expected) > 1e-10*math.Max(1, math.Abs(p.Expected)) {
				return s, fmt.Errorf("forecast/selection risk inconsistency")
			}
		}
	}
	if d.RecommendationID == nil {
		s.Route = "no_solution"
		return s, nil
	}
	a := actions[*d.RecommendationID]
	s.Recommendation = &a
	distance, _ := PlanDistance(a, in.Forecast.Plan)
	s.Route, e = c.Routing.Route(pred[a.ID].CVaR, distance, f.RequiresExpert)
	if e != nil {
		return s, e
	}
	if s.Route != "auto" && c.ExecutionMode == HonorRoutes {
		s.AssignmentReason = "REVIEW_REQUIRED_NO_HUMAN_APPROVAL_NO_ASSIGNMENT"
		return s, nil
	}
	s.Assignment = &Assignment{ID: in.CaseID + "/next-day", ExecuteDay: day + 1, Action: CloneAction(a), Basis: s.Basis, Route: s.Route, ExecutionMode: c.ExecutionMode}
	s.AssignmentReason = "AUTO_ASSIGNED_FOR_NEXT_DAY"
	if c.ExecutionMode == ResearchNoReview && s.Route != "auto" {
		s.AssignmentReason = "RESEARCH_NO_REVIEW_BYPASS_NOT_EXPERT_APPROVAL"
	}
	return s, s.Assignment.Validate()
}

type Controller struct {
	config   Config
	pipeline *forecastpipe.Pipeline
}

func New(c Config, startDay int) (*Controller, error) {
	if e := c.Validate(); e != nil {
		return nil, e
	}
	p, e := forecastpipe.New(c.Pipeline, startDay)
	if e != nil {
		return nil, e
	}
	return &Controller{CloneConfig(c), p}, nil
}
func (c *Controller) Snapshot() monitor.State { return c.pipeline.Snapshot() }
func (c *Controller) Process(in Input) (Output, error) {
	out := Output{Version: Version}
	if in.CaseID == "" {
		return out, fmt.Errorf("case ID required")
	}
	if e := in.Limits.Validate(); e != nil {
		return out, e
	}
	p := c.pipeline.Fork()
	var e error
	out.Pipeline, e = p.Process(in.Forecast)
	if e != nil {
		return out, e
	}
	out.Selection, e = Select(in, out.Pipeline, c.config)
	if e != nil {
		return out, e
	}
	c.pipeline = p
	return out, nil
}
