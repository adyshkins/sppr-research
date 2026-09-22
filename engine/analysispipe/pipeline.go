// Package analysispipe connects information admission, monitoring and diagnostic
// ranking. It does NOT forecast, select or execute corrective actions.
package analysispipe

import (
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/monitor"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/sufficiency"
	"fmt"
)

const Version = "analysis-pipeline-0.3.0"
const AggregateOnly = "aggregate_only_v0_2"
const EvidenceRequired = "evidence_required_v0_3"

type Config struct {
	Mode      string             `json:"mode"`
	Monitor   monitor.Config     `json:"monitor"`
	Admission sufficiency.Config `json:"admission"`
	Diagnosis diagnostics.Model  `json:"diagnosis"`
}

func (c Config) Validate() error {
	if c.Mode != AggregateOnly && c.Mode != EvidenceRequired {
		return fmt.Errorf("unknown admission mode")
	}
	if e := c.Monitor.Validate(); e != nil {
		return e
	}
	if e := c.Admission.Validate(); e != nil {
		return e
	}
	if e := c.Diagnosis.Validate(); e != nil {
		return e
	}
	if c.Diagnosis.KPIThreshold != c.Monitor.Threshold {
		return fmt.Errorf("diagnostic and monitoring deviation scales must match")
	}
	return nil
}
func CloneConfig(c Config) Config {
	n := c
	n.Diagnosis = diagnostics.CloneModel(c.Diagnosis)
	n.Monitor.InitialValues = map[observation.Field]float64{}
	for k, v := range c.Monitor.InitialValues {
		n.Monitor.InitialValues[k] = v
	}
	return n
}

type Result struct {
	Day              int                `json:"day"`
	AdmissionApplied bool               `json:"admission_applied"`
	Admission        sufficiency.Result `json:"admission"`
	Monitor          monitor.Result     `json:"monitor"`
	Diagnosis        diagnostics.Result `json:"diagnosis"`
}
type Pipeline struct {
	config  Config
	monitor *monitor.Monitor
}

func New(c Config, startDay int) (*Pipeline, error) {
	if e := c.Validate(); e != nil {
		return nil, e
	}
	m, e := monitor.New(c.Monitor, startDay)
	if e != nil {
		return nil, e
	}
	return &Pipeline{CloneConfig(c), m}, nil
}
func (p *Pipeline) Snapshot() monitor.State { return p.monitor.Snapshot() }
func (p *Pipeline) Process(in observation.Packet) (Result, error) {
	r := Result{Day: in.Day, AdmissionApplied: p.config.Mode == EvidenceRequired}
	admission, e := sufficiency.Assess(in, p.config.Monitor.Quality, p.monitor.Snapshot(), p.config.Admission)
	if e != nil {
		return r, e
	}
	r.Admission = admission
	next := p.monitor.Fork()
	q, e := observation.EvaluateQuality(in, p.config.Monitor.Quality)
	if e != nil {
		return r, e
	}
	if r.AdmissionApplied && q.GatePassed && !admission.Passed {
		r.Monitor, e = next.Block(in, "INSUFFICIENT_INFORMATION")
	} else {
		r.Monitor, e = next.Observe(in)
	}
	if e != nil {
		return r, e
	}
	r.Diagnosis, e = diagnostics.Diagnose(in, r.Monitor, p.config.Diagnosis)
	if e != nil {
		return r, e
	}
	p.monitor = next // atomic state commit only after all stages complete
	return r, nil
}
