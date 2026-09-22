// forecastcheck constructs a SYNTHETIC engineering preview and a replay file.
// Environment labels are kept in a separate experiment config, not pipe inputs.
package main

import (
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/forecast"
	"dissertation.local/sppr-reconstruction/forecastpipe"
	"dissertation.local/sppr-reconstruction/forecasttrace"
	"dissertation.local/sppr-reconstruction/settings"
	"dissertation.local/sppr-reconstruction/simenv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func save(path string, x any) error {
	b, e := json.MarshalIndent(x, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
func run(args []string) error {
	fs := flag.NewFlagSet("forecastcheck", flag.ContinueOnError)
	days := fs.Int("days", 28, "engineering preview days (26..60); training stays 60")
	model := fs.String("model", "", "required trained model JSON")
	out := fs.String("out", "results/forecast_v04", "output directory")
	replay := fs.String("replay", "", "replay trace without simulator")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *replay != "" {
		s, e := forecasttrace.Replay(*replay)
		if e != nil {
			return e
		}
		b, _ := json.MarshalIndent(s, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	if *days < 26 || *days > 60 {
		return fmt.Errorf("preview days must be 26..60")
	}
	if entries, e := os.ReadDir(*out); e == nil && len(entries) > 0 {
		return fmt.Errorf("output directory is not empty; choose a new path")
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	if *model == "" {
		return fmt.Errorf("trained model required; no implicit fixture fallback")
	}
	var m diagnostics.Model
	b, e := os.ReadFile(*model)
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, &m); e != nil {
		return e
	}
	if e = m.Validate(); e != nil {
		return e
	}
	if m.Provenance.Kind != "training_fit" {
		return fmt.Errorf("trained model required for this engineering preview")
	}
	c := forecastpipe.Config{Analysis: settings.Analysis(m), Forecast: forecast.DefaultConfig()}
	episodes := []forecasttrace.Episode{}
	specs := []simenv.Config{}
	for i, label := range []string{"", "D", "S", "C", "DS"} {
		ec, e := simenv.DefaultConfig("engineering-preview-v04", label, uint64(81001+i))
		if e != nil {
			return e
		}
		ec.Days = *days
		ec.ObservationSD = .02
		ec.MissingProbability = 0
		ep := forecasttrace.Episode{Spec: forecasttrace.EpisodeSpec{ID: fmt.Sprintf("preview-%d", i), StartDay: 0, Days: ec.Days}, Inputs: []forecastpipe.Input{}}
		e = simenv.RunFixed(ec, func(r simenv.Record) error {
			// Do not forward r.Active, r.Label, r.TrueKPI, event window or environment seed.
			ep.Inputs = append(ep.Inputs, forecastpipe.Input{Packet: r.Packet, Plan: r.Plan, ForecastSeed: uint64(91001 + i)})
			return nil
		})
		if e != nil {
			return e
		}
		episodes = append(episodes, ep)
		specs = append(specs, ec)
	}
	if e = os.MkdirAll(*out, 0755); e != nil {
		return e
	}
	if e = save(filepath.Join(*out, "experiment_config.json"), specs); e != nil {
		return e
	}
	if e = save(filepath.Join(*out, "inference_config.json"), c); e != nil {
		return e
	}
	summary, e := forecasttrace.Write(filepath.Join(*out, "forecast_trace.jsonl.gz"), c, episodes)
	if e != nil {
		return e
	}
	if e = save(filepath.Join(*out, "summary.json"), summary); e != nil {
		return e
	}
	b, _ = json.MarshalIndent(summary, "", "  ")
	fmt.Println(string(b))
	return nil
}
func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
