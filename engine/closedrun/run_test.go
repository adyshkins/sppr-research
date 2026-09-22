package closedrun

import (
	"compress/gzip"
	"dissertation.local/sppr-reconstruction/analysispipe"
	"dissertation.local/sppr-reconstruction/control"
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/forecast"
	"dissertation.local/sppr-reconstruction/forecastpipe"
	"dissertation.local/sppr-reconstruction/internal/testfixture"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/simenv"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func config() Config {
	e, _ := simenv.DefaultConfig("loop-unit-test", "", 51)
	e.Days = 6
	e.Initial.Raw = 90
	e.MissingProbability = 0
	e.ObservationSD = 0
	e.EnvironmentSD = 0
	f := forecast.DefaultConfig()
	f.Horizon = 2
	f.PathsPerHypothesis = 1
	return Config{Notice: "engineering fixture", Environment: e, Controller: control.Config{Pipeline: forecastpipe.Config{Analysis: testfixture.Config(analysispipe.EvidenceRequired), Forecast: f}, Choice: core.Config{Horizon: 2, Discount: .97, Alpha: .95, RiskWeight: .35, RiskLimit: 1.25}, Policy: core.FactorialPolicies()[0], Routing: control.DefaultRouting(), ExecutionMode: control.ResearchNoReview}, ForecastSeed: 123, InitialLimits: control.DefaultLimits(), Dropouts: []Dropout{}}
}
func records(t *testing.T, dir string, n int) []PhysicalRecord {
	t.Helper()
	f, e := os.Open(filepath.Join(dir, "physical_evaluation.jsonl.gz"))
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	g, e := gzip.NewReader(f)
	if e != nil {
		t.Fatal(e)
	}
	defer g.Close()
	d := json.NewDecoder(g)
	rs := make([]PhysicalRecord, n)
	for i := range rs {
		if e = d.Decode(&rs[i]); e != nil {
			t.Fatal(e)
		}
	}
	return rs
}
func TestClosedLoopReallyChangesPlant(t *testing.T) {
	c := config()
	// A deliberately restricted permission fixture forces an observable action,
	// testing coupling rather than claiming its optimality.
	c.InitialLimits.AllowedIDs = []string{"u3"}
	aDir := filepath.Join(t.TempDir(), "a")
	a, e := Run(c, aDir)
	if e != nil {
		t.Fatal(e)
	}
	c.Controller.ExecutionMode = control.HonorRoutes
	bDir := filepath.Join(t.TempDir(), "b")
	b, e := Run(c, bDir)
	if e != nil {
		t.Fatal(e)
	}
	if a.Controller.Corrections == 0 || b.Controller.Corrections != 0 || a.FinalRaw == b.FinalRaw {
		t.Fatal("feedback missing", a.Controller, b.Controller)
	}
	ra := records(t, aDir, c.Environment.Days)
	rb := records(t, bDir, c.Environment.Days)
	for i := range ra {
		if !reflect.DeepEqual(ra[i].Tape, rb[i].Tape) {
			t.Fatal("exogenous tape shifted")
		}
	}
}
func TestTerminalAssignmentIsNotExecutedOrCharged(t *testing.T) {
	c := config()
	c.Environment.Days = 1
	dir := t.TempDir()
	s, e := Run(c, dir)
	if e != nil || s.TotalActionCost != 0 || s.Controller.Corrections != 0 || s.Controller.Pending == nil {
		t.Fatal(e, s)
	}
}
func TestCostSumUsesActualExecution(t *testing.T) {
	c := config()
	dir := t.TempDir()
	s, e := Run(c, dir)
	if e != nil {
		t.Fatal(e)
	}
	sum := 0.
	for _, r := range records(t, dir, c.Environment.Days) {
		sum += r.Execution.Action.Cost
		if r.Cost != r.Result.ActionCost || r.Loss != r.PeriodLoss+r.Cost {
			t.Fatal("double count")
		}
	}
	if sum != s.TotalActionCost {
		t.Fatal("cost summary")
	}
}
func TestDropoutsDoNotEraseAlreadyExecutedActions(t *testing.T) {
	c := config()
	fields := observation.Fields()
	c.Dropouts = []Dropout{{Day: 1, Fields: fields[:]}}
	dir := t.TempDir()
	s, e := Run(c, dir)
	if e != nil {
		t.Fatal(e)
	}
	r := records(t, dir, c.Environment.Days)
	if s.Controller.Routes["data_check"] != 1 || r[1].Execution.AppliedAssignmentID == nil || r[2].Execution.AppliedAssignmentID != nil {
		t.Fatal("closed-day dropout incorrectly changed prior execution")
	}
}
func TestNoSolutionContinuesBaseButNotSafety(t *testing.T) {
	c := config()
	c.InitialLimits.AllowedIDs = []string{}
	dir := t.TempDir()
	s, e := Run(c, dir)
	if e != nil || s.Controller.Reasons["ADM_EMPTY"] == 0 || s.Controller.Corrections != 0 || s.TotalKPILoss <= 0 {
		t.Fatal(e, s)
	}
}
func TestPhysicalBalancesAndTiming(t *testing.T) {
	c := config()
	dir := t.TempDir()
	_, e := Run(c, dir)
	if e != nil {
		t.Fatal(e)
	}
	rs := records(t, dir, c.Environment.Days)
	for i, r := range rs {
		if i == 0 {
			if r.Execution.AppliedAssignmentID != nil {
				t.Fatal("initial intervention")
			}
		} else {
			if r.Before != rs[i-1].Result.Next {
				t.Fatal("state discontinuity")
			}
		}
		if r.Execution.AppliedAssignmentID != nil && r.Execution.Requested.Basis.ClosedDay != i-1 {
			t.Fatal("lookahead")
		}
		if r.Result.Next.Raw < 0 || r.Result.Scale < 0 || r.Result.Scale > 1 {
			t.Fatal("balance")
		}
	}
}
func TestConfigAndFaultScheduleValidated(t *testing.T) {
	c := config()
	c.Controller.Pipeline.Forecast.BaseRaw = 99
	if c.Validate() == nil {
		t.Fatal("mismatch")
	}
	c = config()
	c.Dropouts = []Dropout{{Day: 0, Fields: []observation.Field{"bad"}}}
	if c.Validate() == nil {
		t.Fatal("bad channel")
	}
	c = config()
	c.Dropouts = []Dropout{{Day: 1, Fields: []observation.Field{observation.RawEnd}}, {Day: 1, Fields: []observation.Field{observation.RawEnd}}}
	if c.Validate() == nil {
		t.Fatal("duplicate day")
	}
}
func TestRunRefusesOverwrite(t *testing.T) {
	c := config()
	dir := t.TempDir()
	if _, e := Run(c, dir); e != nil {
		t.Fatal(e)
	}
	if _, e := Run(c, dir); e == nil {
		t.Fatal("overwrites run")
	}
}
func TestRunIsDeterministic(t *testing.T) {
	c := config()
	a, e := Run(c, t.TempDir())
	b, f := Run(c, t.TempDir())
	if e != nil || f != nil || !reflect.DeepEqual(a, b) {
		t.Fatal(e, f)
	}
}
