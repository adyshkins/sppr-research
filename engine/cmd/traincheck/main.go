// traincheck generates or verifies a separately versioned synthetic fit.
package main

import (
	"dissertation.local/sppr-reconstruction/training"
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func run(args []string) error {
	fs := flag.NewFlagSet("traincheck", flag.ContinueOnError)
	out := fs.String("out", "results/fit_v04", "output directory")
	verify := fs.String("verify", "", "verify a saved directory by recomputation")
	config := fs.String("config", "", "explicit setup JSON, otherwise declared v04 defaults")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *verify != "" {
		if *config != "" {
			return fmt.Errorf("verify and config cannot be combined")
		}
		if e := training.Verify(*verify); e != nil {
			return e
		}
		fmt.Println("verified: synthetic observations, fit, independent development validation, hashes")
		return nil
	}
	s := training.DefaultSetup()
	if *config != "" {
		b, e := os.ReadFile(*config)
		if e != nil {
			return e
		}
		if e = json.Unmarshal(b, &s); e != nil {
			return e
		}
	}
	a, e := training.Run(s, *out)
	if e != nil {
		return e
	}
	fmt.Printf("NEW synthetic fit: %d episodes / %d days; retained %d; development validation %d episodes / %d days\n", a.TrainingEpisodes, a.TrainingDays, a.RetainedExamples, a.ValidationEpisodes, a.ValidationDays)
	return nil
}
func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
