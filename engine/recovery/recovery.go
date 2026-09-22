// Package recovery defines explicit, NON risk-certified contingency proposals.
// It never changes the primary RISK_EMPTY status or raises the configured limit.
package recovery

import (
	"dissertation.local/sppr-reconstruction/control"
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/plant"
	"fmt"
	"sort"
)

const Hold = "hold_base_plan"
const Mean = "min_expected_exception"
const Tail = "min_cvar_excess_exception"
const Version = "explicit-risk-empty-response-0.8.0"

type Config struct {
	Policy                   string `json:"policy"`
	PermitResearchAssignment bool   `json:"permit_research_assignment_without_human"`
}

func (c Config) Validate() error {
	if c.Policy != Hold && c.Policy != Mean && c.Policy != Tail {
		return fmt.Errorf("unknown recovery policy")
	}
	if c.Policy == Hold && c.PermitResearchAssignment {
		return fmt.Errorf("hold policy cannot authorize contingency actions")
	}
	return nil
}

type Candidate struct {
	ID       string  `json:"id"`
	Expected float64 `json:"expected"`
	CVaR     float64 `json:"cvar"`
	Excess   float64 `json:"risk_limit_excess"`
	Cost     float64 `json:"action_cost"`
	Distance float64 `json:"plan_distance"`
}
type Result struct {
	Version       string              `json:"version"`
	Policy        string              `json:"policy"`
	Triggered     bool                `json:"triggered"`
	Status        string              `json:"status"`
	Limit         float64             `json:"unchanged_risk_limit"`
	RiskCertified bool                `json:"risk_certified"`
	Candidates    []Candidate         `json:"hard_admissible_candidates"`
	Selected      *Candidate          `json:"selected"`
	Baseline      *Candidate          `json:"baseline_if_hard_admissible"`
	Proposal      *plant.Action       `json:"contingency_proposal_not_primary_recommendation"`
	Route         string              `json:"required_route"`
	Assignment    *control.Assignment `json:"research_exception_assignment"`
}

// Rank validates that the input is truly a risk-infeasible decision, not a
// malformed case, missing data, or physical/administrative infeasibility.
func Rank(d core.Decision, limit float64, policy string) ([]Candidate, error) {
	if policy != Hold && policy != Mean && policy != Tail {
		return nil, fmt.Errorf("bad policy")
	}
	if !observation.FiniteNonnegative(limit) || d.Reason != "RISK_EMPTY" || d.Status != core.Deviation || !d.Policy.RiskConstraint || d.RecommendationID != nil || len(d.EligibleIDs) != 0 || len(d.RiskIDs) != 0 || len(d.AdmissibleIDs) == 0 {
		return nil, fmt.Errorf("not a well-formed risk-empty decision")
	}
	rows := []Candidate{}
	seen := map[string]bool{}
	hard := map[string]bool{}
	for _, e := range d.Evaluations {
		if e.ID == "" || seen[e.ID] {
			return nil, fmt.Errorf("duplicate or empty ID")
		}
		seen[e.ID] = true
		if !e.HardAdmissible {
			continue
		}
		if len(e.FailedConstraints) != 0 || e.Risk == nil || e.RiskAdmissible == nil || *e.RiskAdmissible || e.Objective != nil {
			return nil, fmt.Errorf("inconsistent hard/risk eligibility")
		}
		for _, v := range []float64{e.Risk.Expected, e.Risk.CVaR, e.ActionCost, e.PlanDistance} {
			if !observation.FiniteNonnegative(v) {
				return nil, fmt.Errorf("nonfinite contingency score")
			}
		}
		if e.Risk.CVaR <= limit {
			return nil, fmt.Errorf("risk-admissible action mislabeled empty")
		}
		rows = append(rows, Candidate{e.ID, e.Risk.Expected, e.Risk.CVaR, e.Risk.CVaR - limit, e.ActionCost, e.PlanDistance})
		hard[e.ID] = true
	}
	if len(rows) != len(d.AdmissibleIDs) {
		return nil, fmt.Errorf("hard-set mismatch")
	}
	used := map[string]bool{}
	for _, id := range d.AdmissibleIDs {
		if !hard[id] || used[id] {
			return nil, fmt.Errorf("invalid admissible IDs")
		}
		used[id] = true
	}
	// Exact deterministic ordering. No tunable tolerance or post-hoc scalar penalty.
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if policy == Tail && a.Excess != b.Excess {
			return a.Excess < b.Excess
		}
		if a.Expected != b.Expected {
			return a.Expected < b.Expected
		}
		if a.Cost != b.Cost {
			return a.Cost < b.Cost
		}
		if a.Distance != b.Distance {
			return a.Distance < b.Distance
		}
		return a.ID < b.ID
	})
	return rows, nil
}

// Propose never overwrites s. An exceptional action is visibly outside the risk
// limit, requires committee review, and can execute only under an explicit
// experiment-only authorization AND the existing research execution mode.
func Propose(in control.Input, s control.Selection, actions []plant.Action, cc control.Config, c Config) (Result, error) {
	r := Result{Version: Version, Policy: c.Policy, Status: "not_triggered", Limit: cc.Choice.RiskLimit, Candidates: []Candidate{}}
	if err := c.Validate(); err != nil {
		return r, err
	}
	if err := cc.Validate(); err != nil {
		return r, err
	}
	if s.Reason != "RISK_EMPTY" {
		return r, nil
	}
	if s.Decision == nil || s.Recommendation != nil || s.Assignment != nil || len(s.PermissionReasons) != 0 {
		return r, fmt.Errorf("malformed primary refusal")
	}
	rows, err := Rank(*s.Decision, r.Limit, c.Policy)
	if err != nil {
		return r, err
	}
	r.Triggered = true
	r.Candidates = rows
	r.Status = "risk_empty_continue_base_not_certified"
	for _, v := range rows {
		if v.ID == "u0" {
			x := v
			r.Baseline = &x
		}
	}
	if c.Policy == Hold {
		return r, nil
	}
	selected := rows[0]
	r.Selected = &selected
	r.Route = "committee"
	r.Status = "exception_proposed_requires_separate_review"
	found := false
	for _, a := range actions {
		if a.ID == selected.ID {
			if found {
				return r, fmt.Errorf("duplicate proposal action")
			}
			found = true
			checks, e := control.CheckAction(a, in.Limits)
			if e != nil {
				return r, e
			}
			for _, x := range checks {
				if x.Residual > 0 {
					return r, fmt.Errorf("proposed action fails current hard permission")
				}
			}
			x := control.CloneAction(a)
			r.Proposal = &x
		}
	}
	if !found {
		return r, fmt.Errorf("contingency action missing")
	}
	if !c.PermitResearchAssignment || cc.ExecutionMode != control.ResearchNoReview {
		return r, nil
	}
	r.Assignment = &control.Assignment{ID: in.CaseID + "/risk-exception-next-day", ExecuteDay: s.Basis.ClosedDay + 1, Action: control.CloneAction(*r.Proposal), Basis: s.Basis, Route: "committee", ExecutionMode: control.ResearchNoReview}
	r.Status = "research_exception_assigned_LIMIT_STILL_EXCEEDED_NO_HUMAN_APPROVAL"
	return r, r.Assignment.Validate()
}
