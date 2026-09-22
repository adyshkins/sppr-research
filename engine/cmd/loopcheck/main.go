// loopcheck runs explicitly SYNTHETIC engineering trajectories, not a final trial.
package main

import (
	"bytes"
	"dissertation.local/sppr-reconstruction/closedrun"
	"dissertation.local/sppr-reconstruction/control"
	"dissertation.local/sppr-reconstruction/controltrace"
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/forecast"
	"dissertation.local/sppr-reconstruction/forecastpipe"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/settings"
	"dissertation.local/sppr-reconstruction/simenv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func decodeStrict(b []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return e
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return fmt.Errorf("trailing or invalid JSON data: %v", e)
	}
	return nil
}
func engineeringConfigs(m diagnostics.Model, days int) ([]closedrun.Config, error) {
	if e := m.Validate(); e != nil {
		return nil, e
	}
	if m.Provenance.Kind != "training_fit" {
		return nil, fmt.Errorf("trained model required, no test-fixture fallback")
	}
	if days < 26 || days > 60 {
		return nil, fmt.Errorf("engineering days must be 26..60")
	}
	base := control.Config{Pipeline: forecastpipe.Config{Analysis: settings.Analysis(m), Forecast: forecast.DefaultConfig()}, Choice: core.Config{Horizon: 7, Discount: .97, Alpha: .95, RiskWeight: .35, RiskLimit: 1.25}, Policy: core.FactorialPolicies()[0], Routing: control.DefaultRouting(), ExecutionMode: control.ResearchNoReview}
	out := []closedrun.Config{}
	for i, label := range []string{"", "S", "C", "D", "DS"} {
		env, e := simenv.DefaultConfig("engineering-feedback-v05", label, uint64(110001+i))
		if e != nil {
			return nil, e
		}
		env.Days = days
		env.ObservationSD = .02
		env.MissingProbability = 0
		env.EpisodeID = fmt.Sprintf("engineering-%d-R00-no-review", i)
		out = append(out, closedrun.Config{Notice: "NEW engineering fixture selected before execution; not industrial data, not final comparison. Optional research_no_review explicitly bypasses review, never imitates approval.", Environment: env, Controller: control.CloneConfig(base), ForecastSeed: uint64(120001 + i), InitialLimits: control.DefaultLimits(), Dropouts: []closedrun.Dropout{}})
	}
	ds := out[4]
	a := ds
	a.Environment.EpisodeID = "engineering-DS-R00-honor-routes"
	a.Controller = control.CloneConfig(base)
	a.Controller.ExecutionMode = control.HonorRoutes
	out = append(out, a)
	b := ds
	b.Environment.EpisodeID = "engineering-DS-R11-no-review"
	b.Controller = control.CloneConfig(base)
	b.Controller.Policy = core.FactorialPolicies()[3]
	out = append(out, b)
	d := ds
	d.Environment.EpisodeID = "engineering-DS-R00-dropout"
	fields := observation.Fields()
	d.Dropouts = []closedrun.Dropout{{Day: 22, Fields: append([]observation.Field{}, fields[:]...)}, {Day: 23, Fields: append([]observation.Field{}, fields[:]...)}}
	out = append(out, d)
	z := ds
	z.Environment.EpisodeID = "engineering-DS-R00-no-permissions"
	z.InitialLimits = control.CloneLimits(ds.InitialLimits)
	z.InitialLimits.Version = "declared-deny-all-fixture-v05"
	z.InitialLimits.AllowedIDs = []string{}
	out = append(out, z)
	return out, nil
}
func run(args []string) error {
	fs := flag.NewFlagSet("loopcheck", flag.ContinueOnError)
	model := fs.String("model", "", "trained model for declared engineering suite")
	config := fs.String("config", "", "explicit single-episode config JSON")
	out := fs.String("out", "results/closed_v05", "NEW output directory")
	days := fs.Int("days", 32, "engineering episode days, 26..60")
	replay := fs.String("replay", "", "replay one controller trace without environment")
	verify := fs.String("verify", "", "rerun saved suite into a temporary directory; compare all saved outputs")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	modes := 0
	for _, s := range []string{*model, *config, *replay, *verify} {
		if s != "" {
			modes++
		}
	}
	if modes != 1 {
		return fmt.Errorf("choose exactly one of model/config/replay/verify")
	}
	if *replay != "" {
		s, e := controltrace.Replay(*replay)
		if e != nil {
			return e
		}
		b, _ := json.MarshalIndent(s, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	if *verify != "" {
		return verifySuite(*verify)
	}
	if es, e := os.ReadDir(*out); e == nil && len(es) > 0 {
		return fmt.Errorf("output directory must be new or empty")
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	var cs []closedrun.Config
	if *config != "" {
		b, e := os.ReadFile(*config)
		if e != nil {
			return e
		}
		var c closedrun.Config
		if e = decodeStrict(b, &c); e != nil {
			return e
		}
		cs = []closedrun.Config{c}
	} else {
		b, e := os.ReadFile(*model)
		if e != nil {
			return e
		}
		var m diagnostics.Model
		if e = decodeStrict(b, &m); e != nil {
			return e
		}
		cs, e = engineeringConfigs(m, *days)
		if e != nil {
			return e
		}
	}
	if e := os.MkdirAll(*out, 0755); e != nil {
		return e
	}
	if e := closedrun.Save(filepath.Join(*out, "suite_config.json"), cs); e != nil {
		return e
	}
	sums := map[string]closedrun.Summary{}
	for i, c := range cs {
		dir := filepath.Join(*out, fmt.Sprintf("episode_%02d", i))
		s, e := closedrun.Run(c, dir)
		if e != nil {
			return fmt.Errorf("episode %d: %w", i, e)
		}
		rs, e := controltrace.Replay(filepath.Join(dir, "controller_trace.jsonl.gz"))
		if e != nil {
			return e
		}
		a, _ := control.Hash(rs)
		b, _ := control.Hash(s.Controller)
		if a != b {
			return fmt.Errorf("controller replay summary mismatch")
		}
		sums[filepath.Base(dir)] = s
		fmt.Printf("%s: days=%d forecasts=%d corrections=%d replay=ok\n", c.Environment.EpisodeID, s.Controller.Records, s.Controller.Forecasts, s.Controller.Corrections)
	}
	return closedrun.Save(filepath.Join(*out, "summary.json"), sums)
}
func verifySuite(dir string) error {
	b, e := os.ReadFile(filepath.Join(dir, "suite_config.json"))
	if e != nil {
		return e
	}
	var cs []closedrun.Config
	if e = decodeStrict(b, &cs); e != nil {
		return e
	}
	if len(cs) < 1 || len(cs) > 100 {
		return fmt.Errorf("invalid suite size")
	}
	tmp, e := os.MkdirTemp("", "sppr-v05-rerun-*")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	sums := map[string]closedrun.Summary{}
	for i, c := range cs {
		name := fmt.Sprintf("episode_%02d", i)
		original := filepath.Join(dir, name)
		rerun := filepath.Join(tmp, name)
		s, e := closedrun.Run(c, rerun)
		if e != nil {
			return e
		}
		sums[name] = s
		for _, file := range []string{"controller_trace.jsonl.gz", "physical_evaluation.jsonl.gz", "summary.json", "experiment_config.json"} {
			a, e := os.ReadFile(filepath.Join(original, file))
			if e != nil {
				return e
			}
			b, e := os.ReadFile(filepath.Join(rerun, file))
			if e != nil {
				return e
			}
			if string(a) != string(b) {
				return fmt.Errorf("regenerated file mismatch %s/%s", name, file)
			}
		}
		r, e := controltrace.Replay(filepath.Join(original, "controller_trace.jsonl.gz"))
		if e != nil {
			return e
		}
		h, _ := control.Hash(r)
		q, _ := control.Hash(s.Controller)
		if h != q {
			return fmt.Errorf("replay mismatch")
		}
	}
	expected, e := os.ReadFile(filepath.Join(dir, "summary.json"))
	if e != nil {
		return e
	}
	var prev map[string]closedrun.Summary
	if e = decodeStrict(expected, &prev); e != nil {
		return e
	}
	a, _ := control.Hash(prev)
	bhash, _ := control.Hash(sums)
	if a != bhash {
		return fmt.Errorf("suite summary mismatch")
	}
	fmt.Printf("Regenerated and replayed %d engineering episodes; observations, actions, physical records and summaries match.\n", len(cs))
	return nil
}
func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
