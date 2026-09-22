package core

import (
	"fmt"
	"sort"
)

// Status is an INPUT from a monitor. This module does not invent
// observations, calculate quality, or infer these statuses by itself.
type Status string

const (
	Normal       Status = "normal"
	Deviation    Status = "deviation"
	DataIncident Status = "data_quality_incident"
)

type Config struct {
	Horizon    int     `json:"horizon"`
	Discount   float64 `json:"discount"`
	Alpha      float64 `json:"alpha"`
	RiskWeight float64 `json:"risk_weight"`
	RiskLimit  float64 `json:"risk_limit"`
}

func (c Config) Validate() error {
	if c.Horizon < 1 || !finite(c.Discount) || c.Discount <= 0 || c.Discount > 1 ||
		!finite(c.Alpha) || c.Alpha <= 0 || c.Alpha >= 1 ||
		!finite(c.RiskWeight) || c.RiskWeight < 0 || c.RiskWeight > 1 || !nonnegative(c.RiskLimit) {
		return fmt.Errorf("invalid decision configuration")
	}
	return nil
}

type Policy struct {
	ID              string `json:"id"`
	RiskInObjective bool   `json:"risk_in_objective"`
	RiskConstraint  bool   `json:"risk_constraint"`
}

// FactorialPolicies is the NEW 2x2 comparison, not a recovered old experiment.
func FactorialPolicies() []Policy {
	return []Policy{{"R00", false, false}, {"R10", true, false}, {"R01", false, true}, {"R11", true, true}}
}
func (p Policy) Validate() error {
	for _, v := range FactorialPolicies() {
		if p == v {
			return nil
		}
	}
	return fmt.Errorf("unknown policy or inconsistent factor code")
}

type Scenario struct {
	ID          string  `json:"id"`
	Probability float64 `json:"probability"`
}
type Constraint struct {
	ID       string  `json:"id"`
	Residual float64 `json:"residual"`
}
type Trajectory struct {
	ScenarioID   string    `json:"scenario_id"`
	PeriodLosses []float64 `json:"period_losses"`
}
type Candidate struct {
	ID           string       `json:"id"`
	ActionCost   float64      `json:"action_cost"`
	PlanDistance float64      `json:"plan_distance"`
	Constraints  []Constraint `json:"constraints"`
	Trajectories []Trajectory `json:"trajectories"`
}

// Constraint residuals are supplied by the caller. The dissertation does not
// supply the lost code's complete numerical g_j set. Requiring a named, nonempty
// constraint set prevents missing checks being treated as implicit admissibility.
type Case struct {
	ID                  string      `json:"id"`
	InformationVersion  string      `json:"information_version"`
	Status              Status      `json:"status"`
	Config              Config      `json:"config"`
	BaselineID          string      `json:"baseline_id"`
	RequiredConstraints []string    `json:"required_constraints"`
	Scenarios           []Scenario  `json:"scenarios"`
	Candidates          []Candidate `json:"candidates"`
}

type Evaluation struct {
	ID                string    `json:"id"`
	ActionCost        float64   `json:"action_cost"`
	PlanDistance      float64   `json:"plan_distance"`
	HardAdmissible    bool      `json:"hard_admissible"`
	FailedConstraints []string  `json:"failed_constraints"`
	ScenarioLosses    []float64 `json:"scenario_losses,omitempty"`
	Risk              *Risk     `json:"risk"`
	RiskAdmissible    *bool     `json:"risk_admissible"`
	Objective         *float64  `json:"objective"`
}

// A recommendation is not an executed action. This output deliberately has no
// final_action field; expert routing and plant execution are separate stages.
type Decision struct {
	CaseID           string       `json:"case_id"`
	Policy           Policy       `json:"policy"`
	Status           Status       `json:"status"`
	Reason           string       `json:"reason"`
	AdmissibleIDs    []string     `json:"admissible_ids"`
	RiskIDs          []string     `json:"risk_ids"`
	EligibleIDs      []string     `json:"eligible_ids"`
	Evaluations      []Evaluation `json:"evaluations"`
	RecommendationID *string      `json:"recommendation_id"`
}

// Decide implements algorithm 3.5 with separate optional risk factors.
// Malformed inputs are errors, not a scientific NoSolution outcome.
func Decide(in Case, policy Policy) (Decision, error) {
	d := Decision{CaseID: in.ID, Policy: policy, Status: in.Status,
		AdmissibleIDs: []string{}, RiskIDs: []string{}, EligibleIDs: []string{}, Evaluations: []Evaluation{}}
	if in.ID == "" || in.InformationVersion == "" {
		return d, fmt.Errorf("case and information version required")
	}
	if err := in.Config.Validate(); err != nil {
		return d, err
	}
	if err := policy.Validate(); err != nil {
		return d, err
	}
	switch in.Status {
	case DataIncident:
		d.Reason = "DATA_CHECK"
		return d, nil
	case Normal:
		d.Reason = "NO_DEVIATION"
		return d, nil
	case Deviation:
	default:
		return d, fmt.Errorf("unknown status %q", in.Status)
	}
	if in.BaselineID == "" || len(in.Candidates) == 0 {
		return d, fmt.Errorf("explicit baseline candidate required")
	}
	if len(in.RequiredConstraints) == 0 {
		return d, fmt.Errorf("explicit nonempty constraint specification required")
	}
	required := map[string]bool{}
	for _, id := range in.RequiredConstraints {
		if id == "" || required[id] {
			return d, fmt.Errorf("duplicate/empty required constraint")
		}
		required[id] = true
	}
	scenarios := append([]Scenario(nil), in.Scenarios...)
	sort.Slice(scenarios, func(i, j int) bool { return scenarios[i].ID < scenarios[j].ID })
	weights := make([]float64, len(scenarios))
	sid := map[string]bool{}
	for i, s := range scenarios {
		if s.ID == "" || sid[s.ID] {
			return d, fmt.Errorf("duplicate/empty scenario")
		}
		sid[s.ID] = true
		weights[i] = s.Probability
	}
	if _, err := normalizedWeights(weights); err != nil {
		return d, err
	}
	candidates := append([]Candidate(nil), in.Candidates...)
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	seen := map[string]bool{}
	baselineFound := false
	var best *Evaluation
	for _, u := range candidates {
		if u.ID == "" || seen[u.ID] || !nonnegative(u.ActionCost) || !nonnegative(u.PlanDistance) {
			return d, fmt.Errorf("invalid candidate %q", u.ID)
		}
		seen[u.ID] = true
		baselineFound = baselineFound || u.ID == in.BaselineID
		e := Evaluation{ID: u.ID, ActionCost: u.ActionCost, PlanDistance: u.PlanDistance, HardAdmissible: true, FailedConstraints: []string{}}
		checks := map[string]bool{}
		for _, g := range u.Constraints {
			if !required[g.ID] || checks[g.ID] || !finite(g.Residual) {
				return d, fmt.Errorf("invalid constraint for %s", u.ID)
			}
			checks[g.ID] = true
			if g.Residual > 0 {
				e.HardAdmissible = false
				e.FailedConstraints = append(e.FailedConstraints, g.ID)
			}
		}
		if len(checks) != len(required) {
			return d, fmt.Errorf("missing constraint for %s", u.ID)
		}
		sort.Strings(e.FailedConstraints)
		if !e.HardAdmissible {
			d.Evaluations = append(d.Evaluations, e)
			continue
		}
		trajectories := map[string][]float64{}
		for _, tr := range u.Trajectories {
			if !sid[tr.ScenarioID] || trajectories[tr.ScenarioID] != nil || len(tr.PeriodLosses) != in.Config.Horizon {
				return d, fmt.Errorf("invalid trajectory for %s", u.ID)
			}
			trajectories[tr.ScenarioID] = tr.PeriodLosses
		}
		if len(trajectories) != len(scenarios) {
			return d, fmt.Errorf("incomplete scenario ensemble for %s", u.ID)
		}
		losses := make([]float64, len(scenarios))
		for i, s := range scenarios {
			z, err := HorizonLoss(trajectories[s.ID], in.Config.Discount, u.ActionCost)
			if err != nil {
				return d, err
			}
			losses[i] = z
		}
		r, err := EvaluateRisk(losses, weights, in.Config.Alpha)
		if err != nil {
			return d, err
		}
		riskOK := r.CVaR <= in.Config.RiskLimit
		e.Risk = &r
		e.RiskAdmissible = &riskOK
		e.ScenarioLosses = losses
		d.AdmissibleIDs = append(d.AdmissibleIDs, u.ID)
		if riskOK {
			d.RiskIDs = append(d.RiskIDs, u.ID)
		}
		eligible := !policy.RiskConstraint || riskOK
		if eligible {
			lambda := 0.0
			if policy.RiskInObjective {
				lambda = in.Config.RiskWeight
			}
			objective := (1-lambda)*r.Expected + lambda*r.CVaR
			if !finite(objective) {
				return d, fmt.Errorf("objective overflow")
			}
			e.Objective = &objective
			d.EligibleIDs = append(d.EligibleIDs, u.ID)
			if best == nil || better(e, *best) {
				copyE := e
				best = &copyE
			}
		}
		d.Evaluations = append(d.Evaluations, e)
	}
	if !baselineFound {
		return d, fmt.Errorf("baseline ID is not in the candidate set")
	}
	if len(d.AdmissibleIDs) == 0 {
		d.Reason = "ADM_EMPTY"
		return d, nil
	}
	if len(d.EligibleIDs) == 0 {
		d.Reason = "RISK_EMPTY"
		return d, nil
	}
	id := best.ID
	d.RecommendationID = &id
	d.Reason = "RECOMMENDATION"
	return d, nil
}
func better(a, b Evaluation) bool {
	if *a.Objective != *b.Objective {
		return *a.Objective < *b.Objective
	}
	if a.ActionCost != b.ActionCost {
		return a.ActionCost < b.ActionCost
	}
	if a.PlanDistance != b.PlanDistance {
		return a.PlanDistance < b.PlanDistance
	}
	return a.ID < b.ID
}
func CompareFactors(in Case) ([]Decision, error) {
	out := make([]Decision, 0, 4)
	for _, p := range FactorialPolicies() {
		d, err := Decide(in, p)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}
