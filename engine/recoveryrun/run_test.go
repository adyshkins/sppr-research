package recoveryrun

import (
	"compress/gzip"
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/factorial"
	"dissertation.local/sppr-reconstruction/recovery"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func setup(t *testing.T) Config {
	t.Helper()
	var m diagnostics.Model
	if e := factorial.ReadJSON("../results/fit_v04/diagnostic_model.json", &m); e != nil {
		t.Fatal(e)
	}
	jobs, e := Plan(m, 1, 32)
	if e != nil {
		t.Fatal(e)
	}
	c := jobs[7]
	c.Base.Controller.Pipeline.Forecast.PathsPerHypothesis = 2
	c.Base.Controller.Pipeline.Forecast.Horizon = 3
	c.Base.Controller.Choice.Horizon = 3
	c.Base.Controller.Choice.RiskLimit = 0
	return c
}
func TestPlanUsesMatchedExternalSeedsWithoutIdenticalStateAssumption(t *testing.T) {
	c := setup(t)
	var m diagnostics.Model
	_ = factorial.ReadJSON("../results/fit_v04/diagnostic_model.json", &m)
	jobs, e := Plan(m, 2, 32)
	if e != nil || len(jobs) != 40 {
		t.Fatal(e)
	}
	for i := 0; i < len(jobs); i += 4 {
		for j := 1; j < 4; j++ {
			if jobs[i].Base.Environment.Seed != jobs[i+j].Base.Environment.Seed || jobs[i].Base.ForecastSeed != jobs[i+j].Base.ForecastSeed {
				t.Fatal("unmatched")
			}
		}
	}
	if c.Arm != "ET" {
		t.Fatal("fixture")
	}
}
func TestClosedLoopRecoveryReplayWithoutPhysicalFile(t *testing.T) {
	c := setup(t)
	d := filepath.Join(t.TempDir(), "episode")
	s, e := Run(c, d)
	if e != nil || s.Days != 32 || s.RecoveryExecutions == 0 || s.HardViolations != 0 {
		t.Fatal(e, s)
	}
	if e = os.Remove(filepath.Join(d, "physical.jsonl.gz")); e != nil {
		t.Fatal(e)
	}
	if n, e := Replay(d); e != nil || n != 32 {
		t.Fatal(e, n)
	}
}
func TestRegenerationIsBitwiseDeterministic(t *testing.T) {
	c := setup(t)
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	if _, e := Run(c, a); e != nil {
		t.Fatal(e)
	}
	if _, e := Run(c, b); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"experiment_config.json", "controller.jsonl.gz", "physical.jsonl.gz", "summary.json", "COMPLETED.json"} {
		x, _ := os.ReadFile(filepath.Join(a, name))
		y, _ := os.ReadFile(filepath.Join(b, name))
		if string(x) != string(y) {
			t.Fatal("diff", name)
		}
	}
}
func TestHoldRetainsRiskEmptyWithoutRecovery(t *testing.T) {
	c := setup(t)
	c.Recovery = recovery.Config{Policy: recovery.Hold}
	s, e := Run(c, filepath.Join(t.TempDir(), "e"))
	if e != nil || s.Reasons["RISK_EMPTY"] == 0 || s.RecoveryExecutions != 0 || s.RecoveryProposals != 0 {
		t.Fatal(e, s)
	}
}
func TestNoImplicitOverrideWithoutAuthorization(t *testing.T) {
	c := setup(t)
	c.Recovery.PermitResearchAssignment = false
	s, e := Run(c, filepath.Join(t.TempDir(), "e"))
	if e != nil || s.RecoveryProposals == 0 || s.RecoveryExecutions != 0 {
		t.Fatal(e, s)
	}
}
func TestExistingDestinationRejected(t *testing.T) {
	c := setup(t)
	d := t.TempDir()
	if _, e := Run(c, d); e == nil {
		t.Fatal("overwrote directory")
	}
}
func TestReplayRejectsTamperedRecordEvenWithRehashedRecord(t *testing.T) {
	c := setup(t)
	d := filepath.Join(t.TempDir(), "e")
	if _, e := Run(c, d); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(d, "controller.jsonl.gz")
	f, _ := os.Open(path)
	g, _ := gzip.NewReader(f)
	dec := json.NewDecoder(g)
	rows := []Record{}
	for {
		var r Record
		e := dec.Decode(&r)
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		rows = append(rows, r)
	}
	g.Close()
	f.Close()
	rows[20].Recovery.RiskCertified = true
	rows[20].Hash, _ = Hash(rows[20])
	f, _ = os.Create(path)
	gw := gzip.NewWriter(f)
	enc := json.NewEncoder(gw)
	for _, r := range rows {
		if e := enc.Encode(r); e != nil {
			t.Fatal(e)
		}
	}
	gw.Close()
	f.Close()
	if _, e := Replay(d); e == nil {
		t.Fatal("tamper accepted")
	}
}
func TestReferenceSkipsPreselectedNonRiskEmpty(t *testing.T) {
	c := setup(t)
	c.Base.Controller.Policy = core.FactorialPolicies()[0]
	d := filepath.Join(t.TempDir(), "e")
	if _, e := Run(c, d); e != nil {
		t.Fatal(e)
	}
	out := filepath.Join(t.TempDir(), "ref")
	if e := ReferenceFromEpisode(d, 0, out, 18, 16); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(out, "SKIPPED.json")); e != nil {
		t.Fatal(e)
	}
}
