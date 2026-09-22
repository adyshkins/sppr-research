package recoveryrun

import (
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/factorial"
	"dissertation.local/sppr-reconstruction/recovery"
	"fmt"
)

// Plan fixes all factors before execution. B0 is a strong unrestricted expected-
// loss benchmark, H the unchanged E+R11, EM/ET explicit exceptional proposals.
func Plan(m diagnostics.Model, repeats, days int) ([]Config, error) {
	src, e := factorial.Configs(m, repeats, days)
	if e != nil {
		return nil, e
	}
	out := []Config{}
	for _, j := range src {
		if j.Policy != "R11" {
			continue
		}
		p := map[string]int{"normal": 0, "S": 1, "C": 2, "D": 3, "DS": 4}[j.Profile]
		for _, arm := range []string{"B0", "H", "EM", "ET"} {
			b := j.Config
			b.Environment.Dataset = "recovery-development-v08"
			b.Environment.Seed = uint64(810001 + 1000*p + j.Repeat)
			b.ForecastSeed = uint64(910001 + 1000*p + j.Repeat)
			b.Environment.EpisodeID = fmt.Sprintf("DEV-R08-%s-%02d-%s", j.Profile, j.Repeat, arm)
			b.Notice = "DEV-R08 synthetic development: E initializer, contingency NOT risk certified, NO industrial or human trial"
			rc := recovery.Config{Policy: recovery.Hold}
			switch arm {
			case "B0":
				b.Controller.Policy = core.FactorialPolicies()[0]
			case "EM":
				rc = recovery.Config{Policy: recovery.Mean, PermitResearchAssignment: true}
			case "ET":
				rc = recovery.Config{Policy: recovery.Tail, PermitResearchAssignment: true}
			}
			c := Config{Profile: j.Profile, Repeat: j.Repeat, Arm: arm, Base: b, Recovery: rc}
			if e = c.Validate(); e != nil {
				return nil, e
			}
			out = append(out, c)
		}
	}
	return out, nil
}

// CompatibilityPlan re-executes previously disclosed E/R11 and E/R00 points;
// these are regressions against existing v0.7 tables, not new experiments.
func CompatibilityPlan(m diagnostics.Model) ([]Config, error) {
	a, e := Plan(m, 1, 60)
	if e != nil {
		return nil, e
	}
	out := []Config{}
	for _, c := range a {
		if c.Arm != "B0" && c.Arm != "H" {
			continue
		}
		p := map[string]int{"normal": 0, "S": 1, "C": 2, "D": 3, "DS": 4}[c.Profile]
		c.Base.Environment.Dataset = "initialization-development-v07"
		c.Base.Environment.Seed = uint64(610001 + 1000*p)
		c.Base.ForecastSeed = uint64(710001 + 1000*p)
		c.Base.Environment.EpisodeID = fmt.Sprintf("compat-E-%s-%s", c.Profile, c.Arm)
		c.Base.Notice = "REPLAY OF PUBLISHED V07 SEEDS: regression only, not new evidence"
		out = append(out, c)
	}
	return out, nil
}
