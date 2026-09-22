// Package factorial is experiment-side code. It never changes controller v0.5.
package factorial

import (
	"dissertation.local/sppr-reconstruction/closedrun"
	"dissertation.local/sppr-reconstruction/control"
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/forecast"
	"dissertation.local/sppr-reconstruction/forecastpipe"
	"dissertation.local/sppr-reconstruction/settings"
	"dissertation.local/sppr-reconstruction/simenv"
	"fmt"
)

const Schema = "factorial-development-0.6.0"

type Job struct {
	Profile string           `json:"profile"`
	Repeat  int              `json:"repeat"`
	Policy  string           `json:"policy"`
	Config  closedrun.Config `json:"config"`
}

func Configs(m diagnostics.Model, repeats, days int) ([]Job, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if m.Provenance.Kind != "training_fit" {
		return nil, fmt.Errorf("explicit trained model required")
	}
	if repeats < 1 || repeats > 100 || days < 32 || days > 365 {
		return nil, fmt.Errorf("invalid series bounds")
	}
	base := control.Config{Pipeline: forecastpipe.Config{Analysis: settings.Analysis(m), Forecast: forecast.DefaultConfig()}, Choice: core.Config{Horizon: 7, Discount: .97, Alpha: .95, RiskWeight: .35, RiskLimit: 1.25}, Routing: control.DefaultRouting(), ExecutionMode: control.ResearchNoReview}
	out := []Job{}
	for p, label := range []string{"", "S", "C", "D", "DS"} {
		profile := label
		if profile == "" {
			profile = "normal"
		}
		for r := 0; r < repeats; r++ {
			for _, pol := range core.FactorialPolicies() {
				env, e := simenv.DefaultConfig("factorial-development-v06", label, uint64(210001+1000*p+r))
				if e != nil {
					return nil, e
				}
				env.Days = days
				env.ObservationSD = .02
				env.MissingProbability = 0
				env.EpisodeID = fmt.Sprintf("DEV-F06-%s-%02d-%s", profile, r, pol.ID)
				c := closedrun.Config{Notice: "DEV-F06: SYNTHETIC DEVELOPMENT, not final hold-out, no company or human approval", Environment: env, Controller: control.CloneConfig(base), ForecastSeed: uint64(310001 + 1000*p + r), InitialLimits: control.DefaultLimits(), Dropouts: []closedrun.Dropout{}}
				c.Controller.Policy = pol
				if e = c.Validate(); e != nil {
					return nil, e
				}
				out = append(out, Job{profile, r, pol.ID, c})
			}
		}
	}
	return out, nil
}
