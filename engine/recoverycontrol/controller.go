// Package recoverycontrol accepts observations only; no simulator/tape import.
package recoverycontrol

import (
	"dissertation.local/sppr-reconstruction/analysispipe"
	"dissertation.local/sppr-reconstruction/control"
	"dissertation.local/sppr-reconstruction/evidenceinit"
	"dissertation.local/sppr-reconstruction/forecastpipe"
	"dissertation.local/sppr-reconstruction/monitor"
	"dissertation.local/sppr-reconstruction/plant"
	"dissertation.local/sppr-reconstruction/recovery"
)

const Version = "observation-only-recovery-controller-0.8.0"

type Config struct {
	Base     control.Config  `json:"base"`
	Recovery recovery.Config `json:"recovery"`
}

func (c Config) Validate() error {
	if e := c.Base.Validate(); e != nil {
		return e
	}
	return c.Recovery.Validate()
}

type Output struct {
	Version  string          `json:"version"`
	Primary  control.Output  `json:"primary_unchanged_status"`
	Recovery recovery.Result `json:"risk_exception"`
}

func (o Output) Assignment() *control.Assignment {
	if o.Primary.Selection.Assignment != nil {
		return control.CloneAssignment(o.Primary.Selection.Assignment)
	}
	return control.CloneAssignment(o.Recovery.Assignment)
}

type Controller struct {
	config   Config
	analysis *analysispipe.Pipeline
}

func New(c Config, start int) (*Controller, error) {
	if e := c.Validate(); e != nil {
		return nil, e
	}
	a, e := analysispipe.New(c.Base.Pipeline.Analysis, start)
	if e != nil {
		return nil, e
	}
	c.Base = control.CloneConfig(c.Base)
	return &Controller{c, a}, nil
}
func (c *Controller) Snapshot() monitor.State { return c.analysis.Snapshot() }
func (c *Controller) Process(in control.Input) (Output, error) {
	out := Output{Version: Version}
	next := c.analysis.Fork()
	a, e := next.Process(in.Forecast.Packet)
	if e != nil {
		return out, e
	}
	f, e := evidenceinit.Build(in.Forecast.Packet, in.Forecast.Plan, a.Diagnosis, c.config.Base.Pipeline.Forecast, in.Forecast.ForecastSeed)
	if e != nil {
		return out, e
	}
	p := forecastpipe.Result{Version: Version, Analysis: a, Forecast: f}
	s, e := control.Select(in, p, c.config.Base)
	if e != nil {
		return out, e
	}
	out.Primary = control.Output{Version: Version, Pipeline: p, Selection: s}
	actions := []plant.Action{}
	for _, x := range f.Alternatives {
		actions = append(actions, x.Action)
	}
	out.Recovery, e = recovery.Propose(in, s, actions, c.config.Base, c.config.Recovery)
	if e != nil {
		return out, e
	}
	c.analysis = next
	return out, nil
}
