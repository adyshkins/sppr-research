// factorcheck is the declared DEV-F06 experiment runner, not a production system.
package main

import (
	"dissertation.local/sppr-reconstruction/closedrun"
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/factorial"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func run(args []string) error {
	fs := flag.NewFlagSet("factorcheck", flag.ContinueOnError)
	model := fs.String("init-model", "", "create NEW frozen plan from this fitted diagnostic model")
	plan := fs.String("plan", "results/factorial_v06/plan.json", "explicit plan JSON")
	repeats := fs.Int("repeats", 12, "replications per profile when creating plan")
	days := fs.Int("days", 60, "episode length when creating plan")
	job := fs.Int("job", -1, "zero-based job index; result goes to out")
	out := fs.String("out", "", "NEW output directory")
	replay := fs.String("replay", "", "compact controller replay, no environment")
	ref := fs.String("reference", "", "source episode for predeclared independent continuations")
	day := fs.Int("ref-day", -1, "closed day of predeclared checkpoint")
	seed := fs.Uint64("ref-seed", 0, "independent evaluator seed")
	paths := fs.Int("ref-paths", 2048, "independent continuations")
	verify := fs.String("verify-reference", "", "regenerate saved reference result")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	modes := 0
	for _, s := range []string{*model, *replay, *ref, *verify} {
		if s != "" {
			modes++
		}
	}
	if *job >= 0 {
		modes++
	}
	if modes != 1 {
		return fmt.Errorf("choose exactly one execution mode")
	}
	if *model != "" {
		var m diagnostics.Model
		if e := factorial.ReadJSON(*model, &m); e != nil {
			return e
		}
		jobs, e := factorial.Configs(m, *repeats, *days)
		if e != nil {
			return e
		}
		if _, e = os.Stat(*plan); !os.IsNotExist(e) {
			return fmt.Errorf("plan already exists")
		}
		if e = os.MkdirAll(filepath.Dir(*plan), 0755); e != nil {
			return e
		}
		if e = closedrun.Save(*plan, jobs); e != nil {
			return e
		}
		fmt.Printf("Plan saved: %d jobs, %d days each; no trajectories executed\n", len(jobs), *days)
		return nil
	}
	if *replay != "" {
		n, e := factorial.Replay(*replay)
		if e == nil {
			fmt.Printf("Replay passed: %d observed controller records\n", n)
		}
		return e
	}
	if *verify != "" {
		return factorial.VerifyReference(*verify)
	}
	if *out == "" {
		return fmt.Errorf("new output path required")
	}
	if *ref != "" {
		return factorial.ReferenceFromEpisode(*ref, *day, *out, *seed, *paths)
	}
	var jobs []factorial.Job
	if e := factorial.ReadJSON(*plan, &jobs); e != nil {
		return e
	}
	if *job < 0 || *job >= len(jobs) {
		return fmt.Errorf("job out of bounds")
	}
	j := jobs[*job]
	if j.Policy != j.Config.Controller.Policy.ID {
		return fmt.Errorf("policy identity mismatch")
	}
	s, e := factorial.Run(j.Config, *out)
	if e == nil {
		fmt.Printf("%s: days=%d loss=%.10g corrections=%d risk_empty=%d\n", j.Config.Environment.EpisodeID, s.Controller.Records, s.TotalLoss, s.Controller.Corrections, s.Controller.Reasons["RISK_EMPTY"])
	}
	return e
}
func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
