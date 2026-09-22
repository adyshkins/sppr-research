package main

import (
	"dissertation.local/sppr-reconstruction/closedrun"
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/factorial"
	"dissertation.local/sppr-reconstruction/recoveryrun"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func run(args []string) error {
	f := flag.NewFlagSet("recoverycheck", flag.ContinueOnError)
	model := f.String("init-model", "", "trained diagnostic model for plan creation")
	plan := f.String("plan", "", "plan JSON")
	repeats := f.Int("repeats", 12, "paired episodes per profile")
	days := f.Int("days", 60, "days per episode")
	compat := f.Bool("compat", false, "create previously disclosed E regression plan")
	job := f.Int("job", -1, "job index")
	out := f.String("out", "", "new output directory")
	replay := f.String("replay", "", "episode to recompute from observations")
	ref := f.String("reference", "", "source episode for independent evaluator")
	day := f.Int("day", -1, "zero-based reference day")
	seed := f.Uint64("seed", 0, "new reference seed")
	paths := f.Int("paths", 2048, "reference paths")
	if e := f.Parse(args); e != nil {
		return e
	}
	modes := 0
	if *model != "" {
		modes++
	}
	if *job >= 0 {
		modes++
	}
	if *replay != "" {
		modes++
	}
	if *ref != "" {
		modes++
	}
	if modes != 1 {
		return fmt.Errorf("choose exactly one mode")
	}
	if *model != "" {
		var m diagnostics.Model
		if e := factorial.ReadJSON(*model, &m); e != nil {
			return e
		}
		jobs, e := recoveryrun.Plan(m, *repeats, *days)
		if *compat {
			jobs, e = recoveryrun.CompatibilityPlan(m)
		}
		if e != nil {
			return e
		}
		if *plan == "" {
			return fmt.Errorf("plan path required")
		}
		if _, e = os.Stat(*plan); !os.IsNotExist(e) {
			return fmt.Errorf("plan exists")
		}
		if e = os.MkdirAll(filepath.Dir(*plan), 0755); e != nil {
			return e
		}
		if e = closedrun.Save(*plan, jobs); e != nil {
			return e
		}
		fmt.Printf("Created %d jobs; nothing executed\n", len(jobs))
		return nil
	}
	if *replay != "" {
		n, e := recoveryrun.Replay(*replay)
		if e == nil {
			fmt.Printf("Replay passed: %d records\n", n)
		}
		return e
	}
	if *out == "" {
		return fmt.Errorf("output required")
	}
	if *ref != "" {
		return recoveryrun.ReferenceFromEpisode(*ref, *day, *out, *seed, *paths)
	}
	var jobs []recoveryrun.Config
	if e := factorial.ReadJSON(*plan, &jobs); e != nil {
		return e
	}
	if *job < 0 || *job >= len(jobs) {
		return fmt.Errorf("job out of bounds")
	}
	s, e := recoveryrun.Run(jobs[*job], *out)
	if e == nil {
		fmt.Printf("%s/%s/r%d: days=%d loss=%.10g risk_empty=%d recovery_executed=%d\n", s.Profile, s.Arm, s.Repeat, s.Days, s.TotalLoss, s.Reasons["RISK_EMPTY"], s.RecoveryExecutions)
	}
	return e
}
func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
