package recovery

import (
	"dissertation.local/sppr-reconstruction/core"
	"math"
	"math/rand"
	"reflect"
	"testing"
)

func dec() core.Decision {
	d := core.Decision{CaseID: "fixture", Status: core.Deviation, Reason: "RISK_EMPTY", Policy: core.FactorialPolicies()[3], AdmissibleIDs: []string{"u0", "u1", "u2"}, EligibleIDs: []string{}, RiskIDs: []string{}}
	for i, id := range d.AdmissibleIDs {
		risk := core.Risk{Expected: []float64{2, 1, 1.2}[i], CVaR: []float64{4, 5, 3}[i]}
		ok := false
		d.Evaluations = append(d.Evaluations, core.Evaluation{ID: id, HardAdmissible: true, Risk: &risk, RiskAdmissible: &ok, ActionCost: float64(i) * .1})
	}
	return d
}
func TestMeanAndTailAreDistinctRules(t *testing.T) {
	d := dec()
	a, e := Rank(d, 1, Mean)
	if e != nil || a[0].ID != "u1" {
		t.Fatal(e, a)
	}
	b, e := Rank(d, 1, Tail)
	if e != nil || b[0].ID != "u2" || b[0].Excess != 2 {
		t.Fatal(e, b)
	}
}
func TestRefusalIsNotConvertedToRiskAdmissibility(t *testing.T) {
	d := dec()
	before, _ := Rank(d, 1, Hold)
	_, e := Rank(d, 1, Tail)
	after, _ := Rank(d, 1, Hold)
	if e != nil || !reflect.DeepEqual(before, after) || d.RecommendationID != nil || d.Reason != "RISK_EMPTY" {
		t.Fatal("mutated primary refusal")
	}
}
func TestHardRejectedCandidateCannotRecover(t *testing.T) {
	d := dec()
	d.Evaluations = append(d.Evaluations, core.Evaluation{ID: "forbidden", HardAdmissible: false, FailedConstraints: []string{"budget"}, Risk: &core.Risk{Expected: 0, CVaR: 0}})
	r, e := Rank(d, 1, Tail)
	if e != nil || len(r) != 3 || r[0].ID == "forbidden" {
		t.Fatal(e)
	}
}
func TestOnlyRiskEmptyCanUseException(t *testing.T) {
	for _, s := range []string{"ADM_EMPTY", "DATA_CHECK", "NO_DEVIATION", "RECOMMENDATION"} {
		d := dec()
		d.Reason = s
		if _, e := Rank(d, 1, Tail); e == nil {
			t.Fatal(s)
		}
	}
}
func TestMalformedScoresRejected(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), -1, 1} {
		d := dec()
		d.Evaluations[0].Risk.CVaR = v
		if _, e := Rank(d, 1, Tail); e == nil {
			t.Fatal(v)
		}
	}
}
func TestWrongEligibilityRejected(t *testing.T) {
	d := dec()
	yes := true
	d.Evaluations[0].RiskAdmissible = &yes
	if _, e := Rank(d, 1, Tail); e == nil {
		t.Fatal("contradiction")
	}
}
func TestDuplicateOrMismatchedIDsRejected(t *testing.T) {
	d := dec()
	d.AdmissibleIDs[0] = "not_there"
	if _, e := Rank(d, 1, Tail); e == nil {
		t.Fatal("set mismatch")
	}
	d = dec()
	d.Evaluations[0].ID = "u1"
	if _, e := Rank(d, 1, Tail); e == nil {
		t.Fatal("duplicate")
	}
}
func TestTieBreakIndependentOfInputOrder(t *testing.T) {
	d := dec()
	for i := range d.Evaluations {
		d.Evaluations[i].Risk = &core.Risk{Expected: 2, CVaR: 3}
		d.Evaluations[i].ActionCost = .1
	}
	r, e := Rank(d, 1, Tail)
	if e != nil || r[0].ID != "u0" {
		t.Fatal(e)
	}
	d.Evaluations[0], d.Evaluations[2] = d.Evaluations[2], d.Evaluations[0]
	s, e := Rank(d, 1, Tail)
	if e != nil || !reflect.DeepEqual(r, s) {
		t.Fatal("order dependent")
	}
}
func TestFiniteSetLexicographicRuleProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(805))
	for n := 0; n < 500; n++ {
		d := dec()
		for i := range d.Evaluations {
			d.Evaluations[i].Risk = &core.Risk{Expected: rng.Float64() * 4, CVaR: 1 + rng.Float64()*10}
		}
		r, e := Rank(d, 1, Tail)
		if e != nil {
			t.Fatal(e)
		}
		for _, v := range r {
			if v.CVaR < r[0].CVaR {
				t.Fatal("not minimal")
			}
		}
		m, e := Rank(d, 1, Mean)
		if e != nil {
			t.Fatal(e)
		}
		for _, v := range m {
			if v.Expected < m[0].Expected {
				t.Fatal("not minimal mean")
			}
		}
	}
}
func TestRecoveryConfigMustBeExplicit(t *testing.T) {
	for _, c := range []Config{{"", false}, {"safe", true}, {Hold, true}} {
		if c.Validate() == nil {
			t.Fatal(c)
		}
	}
	if (Config{Tail, false}).Validate() != nil {
		t.Fatal("proposal without permission should be allowed")
	}
}
