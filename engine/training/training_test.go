package training

import (
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/settings"
	"dissertation.local/sppr-reconstruction/simenv"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func small() Setup {
	s := Setup{Version: Version, Notice: "test-only", Template: settings.DiagnosticTemplate(), Laplace: 1}
	for i, label := range []string{"D", "S", "C", "DS"} {
		for j := 0; j < 2; j++ {
			c, _ := simenv.DefaultConfig("test-fit", label, uint64(1+i+10*j))
			c.Days = 3
			c.Event.StartDay = 0
			c.Event.EndDay = 1
			c.EnvironmentSD = 0
			c.ObservationSD = 0
			c.MissingProbability = 0
			if j == 0 {
				s.Training = append(s.Training, c)
			} else {
				s.Validation = append(s.Validation, c)
			}
		}
	}
	return s
}
func TestSetupDisjointSeeds(t *testing.T) {
	s := small()
	s.Validation[0].Seed = s.Training[0].Seed
	if e := s.Validate(); e == nil {
		t.Fatal("overlap")
	}
}
func TestFitFromExplicitObservedCounts(t *testing.T) {
	s := small()
	a, e := Run(s, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if a.RetainedExamples != 8 || a.TrainingDays != 12 || a.ValidationDays != 12 {
		t.Fatal("counts")
	}
	for _, h := range a.Model.Hypotheses {
		if h.Prior != .25 {
			t.Fatal("prior changed")
		}
		for j, p := range h.Likelihood {
			want := .25
			if (h.ID == "D" || h.ID == "DS") && j == 0 || (h.ID == "S" || h.ID == "DS") && j == 1 || h.ID == "C" && j == 2 {
				want = .75
			}
			if p != want {
				t.Fatal(h.ID, j, p, want)
			}
		}
	}
}
func TestValidationCannotChangeFit(t *testing.T) {
	s := small()
	a, e := Run(s, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	for i := range s.Validation {
		s.Validation[i].Seed += 100
		s.Validation[i].ObservationSD = .3
		s.Validation[i].MissingProbability = .3
	}
	b, e := Run(s, t.TempDir())
	if e != nil || !reflect.DeepEqual(a.Model, b.Model) {
		t.Fatal("validation contaminated training")
	}
}
func TestFitNoDataFailsRatherThanInventing(t *testing.T) {
	s := small()
	for i := range s.Training {
		s.Training[i].MissingProbability = 1
	}
	if _, e := Run(s, t.TempDir()); e == nil {
		t.Fatal("invented fit")
	}
}
func TestUnavailableFeatureNotFalse(t *testing.T) {
	c := small().Training[0]
	var rec simenv.Record
	if e := simenv.RunFixed(c, func(r simenv.Record) error {
		if r.Day == 0 {
			rec = r
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	rec.Packet.Readings[observation.DemandA] = observation.Missing()
	a, x, e := Extract(rec, settings.DiagnosticTemplate())
	if e != nil || a.Included || x != nil || a.Features[0].Value != nil {
		t.Fatal("missing became false")
	}
}
func TestAuditRecomputation(t *testing.T) {
	dir := t.TempDir()
	if _, e := Run(small(), dir); e != nil {
		t.Fatal(e)
	}
	if e := Verify(dir); e != nil {
		t.Fatal(e)
	}
}
func TestAuditTamperDetected(t *testing.T) {
	dir := t.TempDir()
	if _, e := Run(small(), dir); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "retained_examples.json"), []byte("[]"), 0644); e != nil {
		t.Fatal(e)
	}
	if e := Verify(dir); e == nil {
		t.Fatal("tampered input accepted")
	}
}
func TestPriorAndGraphNotFitted(t *testing.T) {
	s := small()
	a, e := Run(s, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if a.Model.GraphVersion != s.Template.GraphVersion || !reflect.DeepEqual(a.Model.Edges, s.Template.Edges) || a.Model.Provenance.Kind != "training_fit" {
		t.Fatal("graph/provenance")
	}
}
func TestRatioUndefinedOnNoCases(t *testing.T) {
	if ratio(0, 0) != nil {
		t.Fatal("undefined accuracy fabricated")
	}
	x := ratio(1, 2)
	if x == nil || *x != .5 {
		t.Fatal("ratio")
	}
}
func TestAvailableRequiresAllFeatures(t *testing.T) {
	if Available([]diagnostics.Feature{}) {
		t.Fatal("empty features")
	}
}
func TestExistingResultsNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	if _, e := Run(small(), dir); e != nil {
		t.Fatal(e)
	}
	before, e := Sum(filepath.Join(dir, "summary.json"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e := Run(small(), dir); e == nil {
		t.Fatal("overwrote completed run")
	}
	after, _ := Sum(filepath.Join(dir, "summary.json"))
	if before != after {
		t.Fatal("prior artifact changed")
	}
}
