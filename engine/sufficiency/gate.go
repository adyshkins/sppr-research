// Package sufficiency is a NEW v0.3 information-admission rule, not a recovered
// rule of the lost experiment. It checks observability requirements of each KPI;
// it does not establish correctness/truth of the measurements.
package sufficiency

import (
	"dissertation.local/sppr-reconstruction/monitor"
	"dissertation.local/sppr-reconstruction/observation"
	"fmt"
)

const Version = "information-sufficiency-0.3.0"
const Strict = "fresh_required"
const BoundedRawHistory = "bounded_raw_stock_history"

type Config struct {
	Policy string `json:"policy"`
	// Only raw_end (stock) may be historical in the OPTIONAL policy. Flows and
	// backlog paired with today's shipments must belong to the current day.
	RawStockMaxAge int `json:"raw_stock_max_age"`
}

func StrictConfig() Config { return Config{Policy: Strict} }
func (c Config) Validate() error {
	switch c.Policy {
	case Strict:
		if c.RawStockMaxAge != 0 {
			return fmt.Errorf("strict policy permits no history")
		}
	case BoundedRawHistory:
		if c.RawStockMaxAge < 1 {
			return fmt.Errorf("bounded history needs positive maximum age")
		}
	default:
		return fmt.Errorf("unknown sufficiency policy")
	}
	return nil
}

type Evidence struct {
	Field    observation.Field `json:"field"`
	Source   string            `json:"source"`
	Age      *int              `json:"age_days"`
	Accepted bool              `json:"accepted"`
	Reason   string            `json:"reason"`
}
type Requirement struct {
	KPI          string     `json:"kpi"`
	Passed       bool       `json:"passed"`
	Evidence     []Evidence `json:"evidence"`
	FailedChecks []string   `json:"failed_checks"`
}
type Result struct {
	Version      string        `json:"version"`
	Policy       Config        `json:"policy"`
	Passed       bool          `json:"passed"`
	UsesHistory  bool          `json:"uses_history"`
	Requirements []Requirement `json:"requirements"`
}

func Assess(p observation.Packet, qcfg observation.QualityConfig, state monitor.State, cfg Config) (Result, error) {
	r := Result{Version: Version, Policy: cfg, Passed: true, Requirements: []Requirement{}}
	if err := cfg.Validate(); err != nil {
		return r, err
	}
	if err := p.Validate(); err != nil {
		return r, err
	}
	if p.Day != state.NextDay {
		return r, fmt.Errorf("state/packet day mismatch")
	}
	q, err := observation.EvaluateQuality(p, qcfg)
	if err != nil {
		return r, err
	}
	checks := map[string]bool{}
	for _, c := range q.Checks {
		checks[c.ID] = c.Passed
	}
	type req struct {
		name   string
		fields []observation.Field
		checks []string
	}
	requirements := []req{
		{"service", []observation.Field{observation.DemandA, observation.DemandB, observation.BacklogBeforeA, observation.BacklogBeforeB, observation.ShippedA, observation.ShippedB}, []string{"shipment_demand_a", "shipment_demand_b"}},
		{"plan_execution", []observation.Field{observation.OutputA, observation.OutputB, observation.Capacity}, []string{"capacity_use", "capacity_upper"}},
		{"capacity_use", []observation.Field{observation.OutputA, observation.OutputB, observation.Capacity}, []string{"capacity_use", "capacity_upper"}},
		{"raw_days", []observation.Field{observation.RawEnd}, nil},
	}
	for _, x := range requirements {
		item := Requirement{KPI: x.name, Passed: true, Evidence: []Evidence{}, FailedChecks: []string{}}
		for _, f := range x.fields {
			e := Evidence{Field: f, Source: "unavailable", Reason: "NO_VALID_MEASUREMENT"}
			m := p.Readings[f]
			maxAge := 0
			if f == observation.RawEnd && cfg.Policy == BoundedRawHistory {
				maxAge = cfg.RawStockMaxAge
			}
			if m.Valid() {
				age := m.AgeDays
				e.Age = &age
				e.Source = "measurement"
				if age <= maxAge {
					e.Accepted = true
					e.Reason = "CURRENT_MEASUREMENT"
					if age > 0 {
						e.Reason = "BOUNDED_STOCK_ESTIMATE"
						r.UsesHistory = true
					}
				} else {
					e.Reason = "STALE_MEASUREMENT"
				}
			} else if f == observation.RawEnd && cfg.Policy == BoundedRawHistory {
				old, ok := state.Cache[f]
				if ok {
					age := p.Day - old.SampleDay
					e.Age = &age
					e.Source = "accepted_history"
					if age < 0 || old.DeliveryDay > p.Day || !observation.FiniteNonnegative(old.Value) {
						return r, fmt.Errorf("invalid measurement cache")
					}
					if age <= maxAge {
						e.Accepted = true
						e.Reason = "BOUNDED_STOCK_ESTIMATE"
						r.UsesHistory = true
					} else {
						e.Reason = "HISTORY_EXPIRED"
					}
				}
			}
			// No initial constant can be evidence, even if numerically plausible.
			if !e.Accepted {
				item.Passed = false
			}
			item.Evidence = append(item.Evidence, e)
		}
		for _, id := range x.checks {
			if !checks[id] {
				item.Passed = false
				item.FailedChecks = append(item.FailedChecks, id)
			}
		}
		if !item.Passed {
			r.Passed = false
		}
		r.Requirements = append(r.Requirements, item)
	}
	return r, nil
}
