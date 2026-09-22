// Package forecastpipe joins A3/A4/A5. There is no selection or execution here.
package forecastpipe

import (
	"dissertation.local/sppr-reconstruction/analysispipe"
	"dissertation.local/sppr-reconstruction/forecast"
	"dissertation.local/sppr-reconstruction/monitor"
	"dissertation.local/sppr-reconstruction/observation"
	"fmt"
)

const Version = "analysis-forecast-pipeline-0.4.0"

type Config struct {
	Analysis analysispipe.Config `json:"analysis"`
	Forecast forecast.Config     `json:"forecast"`
}

func (c Config) Validate() error {
	if err := c.Analysis.Validate(); err != nil {
		return err
	}
	if err := c.Forecast.Validate(); err != nil {
		return err
	}
	m := c.Analysis.Diagnosis
	f := c.Forecast
	if m.BaseDemand != [2]float64(f.Parameters.BaseDemand) || m.BaseRaw != f.BaseRaw || m.BaseCapacity != f.BaseCapacity || m.FeatureRatios != f.FeatureRatios || c.Analysis.Monitor.Quality != f.Quality {
		return fmt.Errorf("inconsistent versioned baselines or quality parameters")
	}
	return nil
}
func CloneConfig(c Config) Config {
	return Config{analysispipe.CloneConfig(c.Analysis), forecast.CloneConfig(c.Forecast)}
}

type Input struct {
	Packet       observation.Packet `json:"observation"`
	Plan         forecast.KnownPlan `json:"known_plan"`
	ForecastSeed uint64             `json:"forecast_seed"`
}
type Result struct {
	Version  string              `json:"version"`
	Analysis analysispipe.Result `json:"analysis"`
	Forecast forecast.Result     `json:"forecast"`
}
type Pipeline struct {
	config   Config
	analysis *analysispipe.Pipeline
}

func New(c Config, startDay int) (*Pipeline, error) {
	if e := c.Validate(); e != nil {
		return nil, e
	}
	a, e := analysispipe.New(c.Analysis, startDay)
	if e != nil {
		return nil, e
	}
	return &Pipeline{CloneConfig(c), a}, nil
}
func (p *Pipeline) Snapshot() monitor.State { return p.analysis.Snapshot() }
func (p *Pipeline) Process(in Input) (Result, error) {
	r := Result{Version: Version}
	a := p.analysis.Fork()
	var e error
	r.Analysis, e = a.Process(in.Packet)
	if e != nil {
		return r, e
	}
	r.Forecast, e = forecast.Build(in.Packet, in.Plan, r.Analysis.Diagnosis, p.config.Forecast, in.ForecastSeed)
	if e != nil {
		return r, e
	}
	p.analysis = a
	return r, nil
}
