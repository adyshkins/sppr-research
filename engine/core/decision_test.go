package core

import (
	"encoding/json"
	"math"
	"testing"
)

func sampleCase() Case {
	c := Case{ID: "test", InformationVersion: "test-only", Status: Deviation, Config: Config{1, .97, .95, .35, 1.25}, BaselineID: "u0", RequiredConstraints: []string{"resource"},
		Scenarios: []Scenario{{"a", .8}, {"b", .15}, {"c", .05}}}
	for i, id := range []string{"u0", "u1"} {
		v := Candidate{ID: id, Constraints: []Constraint{{"resource", 0}}}
		if i == 1 {
			v.ActionCost = .05
			v.PlanDistance = .1
		}
		for j, s := range c.Scenarios {
			loss := 0.0
			if i == 0 && j == 2 {
				loss = 6
			}
			if i == 1 {
				loss = 1
			}
			v.Trajectories = append(v.Trajectories, Trajectory{s.ID, []float64{loss}})
		}
		c.Candidates = append(c.Candidates, v)
	}
	return c
}
func TestFactorialPolicies(t *testing.T) {
	c := sampleCase()
	ds, err := CompareFactors(c)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"u0", "u1", "u1", "u1"} {
		if ds[i].RecommendationID == nil || *ds[i].RecommendationID != want {
			t.Fatalf("%s wrong selection", ds[i].Policy.ID)
		}
	}
}
func TestStatusGates(t *testing.T) {
	for _, v := range []struct {
		s      Status
		reason string
	}{{Normal, "NO_DEVIATION"}, {DataIncident, "DATA_CHECK"}} {
		c := sampleCase()
		c.Status = v.s
		c.Candidates = nil
		c.Scenarios = nil
		d, e := Decide(c, FactorialPolicies()[3])
		if e != nil {
			t.Fatal(e)
		}
		if d.Reason != v.reason || d.RecommendationID != nil || len(d.Evaluations) != 0 {
			t.Fatal("bad gate")
		}
	}
}
func TestHardEmptyDoesNotExecuteBaseline(t *testing.T) {
	c := sampleCase()
	for i := range c.Candidates {
		c.Candidates[i].Constraints[0].Residual = 1
	}
	ds, err := CompareFactors(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ds {
		if d.Reason != "ADM_EMPTY" || d.RecommendationID != nil {
			t.Fatal("baseline bypassed constraints")
		}
	}
}
func TestRiskEmptyOnlyWhenEnabled(t *testing.T) {
	c := sampleCase()
	c.Config.RiskLimit = .1
	ds, e := CompareFactors(c)
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range ds {
		if d.Policy.RiskConstraint {
			if d.Reason != "RISK_EMPTY" || d.RecommendationID != nil {
				t.Fatal("risk bypass")
			}
		} else if d.RecommendationID == nil {
			t.Fatal("disabled risk was enforced")
		}
	}
}
func TestRiskBoundaryInclusive(t *testing.T) {
	c := sampleCase()
	c.Candidates = c.Candidates[:1]
	for i := range c.Candidates[0].Trajectories {
		c.Candidates[0].Trajectories[i].PeriodLosses = []float64{1.25}
	}
	d, e := Decide(c, FactorialPolicies()[3])
	if e != nil {
		t.Fatal(e)
	}
	if d.RecommendationID == nil {
		t.Fatal("boundary should be admissible")
	}
}
func TestDeterministicTieBreak(t *testing.T) {
	c := sampleCase()
	c.Scenarios = []Scenario{{"a", 1}}
	c.Config.RiskLimit = 2
	baseline := Candidate{ID: "u0", Constraints: []Constraint{{"resource", 1}}}
	c.Candidates = []Candidate{baseline}
	for _, v := range []struct {
		id             string
		cost, distance float64
	}{{"d", .5, 0}, {"c", .25, .5}, {"b", .25, .25}, {"a", .25, .25}} {
		c.Candidates = append(c.Candidates, Candidate{v.id, v.cost, v.distance, []Constraint{{"resource", 0}}, []Trajectory{{"a", []float64{1 - v.cost}}}})
	}
	ds, e := CompareFactors(c)
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range ds {
		if d.RecommendationID == nil || *d.RecommendationID != "a" {
			t.Fatal("wrong tie resolution")
		}
	}
}
func TestPermutationInvariance(t *testing.T) {
	c := sampleCase()
	a, e := CompareFactors(c)
	if e != nil {
		t.Fatal(e)
	}
	c.Candidates[0], c.Candidates[1] = c.Candidates[1], c.Candidates[0]
	c.Scenarios[0], c.Scenarios[2] = c.Scenarios[2], c.Scenarios[0]
	for i := range c.Candidates {
		tr := c.Candidates[i].Trajectories
		tr[0], tr[2] = tr[2], tr[0]
	}
	b, e := CompareFactors(c)
	if e != nil {
		t.Fatal(e)
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatal("order dependent result")
	}
}
func TestMissingConstraintsAreNotAdmissibility(t *testing.T) {
	c := sampleCase()
	c.RequiredConstraints = nil
	if _, e := CompareFactors(c); e == nil {
		t.Fatal("no constraint spec accepted")
	}
	c = sampleCase()
	c.Candidates[0].Constraints = nil
	if _, e := CompareFactors(c); e == nil {
		t.Fatal("missing candidate checks accepted")
	}
}
func TestDecisionRejectsMalformedInputs(t *testing.T) {
	mutations := []func(*Case){
		func(c *Case) { c.Status = "unknown" }, func(c *Case) { c.Candidates[1].ID = "u0" },
		func(c *Case) { c.Scenarios[0].Probability = .2 }, func(c *Case) { c.BaselineID = "absent" },
		func(c *Case) { c.Candidates[0].Trajectories = c.Candidates[0].Trajectories[:1] },
		func(c *Case) { c.Candidates[0].PlanDistance = math.NaN() }, func(c *Case) { c.Config.RiskWeight = 2 },
		func(c *Case) { c.Candidates[0].Constraints[0].Residual = math.NaN() },
		func(c *Case) { c.Candidates[0].Trajectories[0].PeriodLosses = []float64{1, 2} },
		func(c *Case) { c.Scenarios[1].ID = c.Scenarios[0].ID },
	}
	for i, f := range mutations {
		c := sampleCase()
		f(&c)
		if _, e := CompareFactors(c); e == nil {
			t.Fatalf("accepted mutation %d", i)
		}
	}
}
