// Package observation defines the explicit, NEW v0.2 observation contract.
// Fifteen channels and four cross-checks operationalize paragraph P1775;
// their exact identities were not recoverable from the lost source code.
package observation

import (
	"fmt"
	"math"
)

type Field string

const (
	DemandA        Field = "demand_a"
	DemandB        Field = "demand_b"
	Capacity       Field = "capacity_before_overtime"
	RawArrival     Field = "regular_raw_arrival"
	OutputA        Field = "output_a"
	OutputB        Field = "output_b"
	ShippedA       Field = "shipped_a"
	ShippedB       Field = "shipped_b"
	RawEnd         Field = "raw_end"
	FinishedA      Field = "finished_end_a"
	FinishedB      Field = "finished_end_b"
	BacklogBeforeA Field = "backlog_before_a"
	BacklogBeforeB Field = "backlog_before_b"
	BacklogAfterA  Field = "backlog_after_a"
	BacklogAfterB  Field = "backlog_after_b"
)

// Fields returns a value copy, preventing callers from modifying the schema.
func Fields() [15]Field {
	return [15]Field{DemandA, DemandB, Capacity, RawArrival, OutputA, OutputB, ShippedA, ShippedB, RawEnd, FinishedA, FinishedB, BacklogBeforeA, BacklogBeforeB, BacklogAfterA, BacklogAfterB}
}

const FieldCount = 15
const CheckCount = 19
const Schema = "sppr.observation.v2"

type Measurement struct {
	Present bool     `json:"present"`
	Value   *float64 `json:"value"`
	AgeDays int      `json:"age_days"`
	Fault   string   `json:"fault,omitempty"`
}

func Missing() Measurement { return Measurement{} }
func Read(value float64, ageDays int) Measurement {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return Measurement{Present: true, AgeDays: ageDays, Fault: "non_finite"}
	}
	return Measurement{Present: true, Value: &value, AgeDays: ageDays}
}
func (m Measurement) Valid() bool {
	return m.Present && m.Value != nil && m.Fault == "" && FiniteNonnegative(*m.Value)
}

// Malformed serialization is an error. A well-formed missing/invalid sensor
// reading is not an error: it is evaluated by the quality model.
func (m Measurement) Validate() error {
	if m.AgeDays < 0 {
		return fmt.Errorf("negative measurement age")
	}
	if !m.Present {
		if m.Value != nil || m.Fault != "" || m.AgeDays != 0 {
			return fmt.Errorf("missing measurement must have null value and zero age")
		}
		return nil
	}
	if m.Fault != "" {
		if m.Value != nil {
			return fmt.Errorf("fault measurement must have null value")
		}
		if m.Fault != "non_finite" && m.Fault != "sensor_error" {
			return fmt.Errorf("unknown sensor fault %q", m.Fault)
		}
		return nil
	}
	if m.Value == nil || !Finite(*m.Value) {
		return fmt.Errorf("present measurement requires finite numeric value or explicit fault")
	}
	return nil // negative numbers are sensor defects, assessed by quality
}

type Context struct {
	ExecutedPlan      [2]float64 `json:"executed_plan"`
	Overtime          float64    `json:"executed_overtime"`
	ActionID          string     `json:"executed_action_id"`
	PlanVersion       string     `json:"plan_version"`
	ConstraintVersion string     `json:"constraint_version"`
}

func (c Context) Validate() error {
	if c.ActionID == "" || c.PlanVersion == "" || c.ConstraintVersion == "" || !FiniteNonnegative(c.Overtime) {
		return fmt.Errorf("invalid known execution context")
	}
	for _, p := range c.ExecutedPlan {
		if !FiniteNonnegative(p) {
			return fmt.Errorf("invalid executed plan")
		}
	}
	return nil
}

type Packet struct {
	Schema   string                `json:"schema"`
	Day      int                   `json:"day"`
	Context  Context               `json:"context"`
	Readings map[Field]Measurement `json:"readings"`
}

func (p Packet) Validate() error {
	if p.Schema != Schema || p.Day < 0 || len(p.Readings) != FieldCount {
		return fmt.Errorf("invalid observation schema, day, or channel count")
	}
	if err := p.Context.Validate(); err != nil {
		return err
	}
	for _, f := range Fields() {
		m, ok := p.Readings[f]
		if !ok {
			return fmt.Errorf("missing channel key %s; use explicit missing measurement", f)
		}
		if err := m.Validate(); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
	}
	return nil
}
func Clone(p Packet) Packet {
	q := p
	q.Readings = make(map[Field]Measurement, len(p.Readings))
	for k, v := range p.Readings {
		if v.Value != nil {
			x := *v.Value
			v.Value = &x
		}
		q.Readings[k] = v
	}
	return q
}
func Finite(x float64) bool            { return !math.IsNaN(x) && !math.IsInf(x, 0) }
func FiniteNonnegative(x float64) bool { return Finite(x) && x >= 0 }
