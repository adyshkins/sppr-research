package plant

import (
	"math"
	"math/rand"
	"testing"
)

func near(t *testing.T, a, b float64) {
	t.Helper()
	if math.Abs(a-b) > 1e-10*(1+math.Abs(b)) {
		t.Fatalf("got %.17g want %.17g", a, b)
	}
}
func TestBalancedDeterministic60Steps(t *testing.T) {
	s := DissertationInitialState()
	p := DissertationParameters()
	u, _ := FixedAction("u0", nil)
	for day := 0; day < 60; day++ {
		r, e := Step(s, Exogenous{Pair{40, 25}, 100, 90}, u, p)
		if e != nil {
			t.Fatal(e)
		}
		if r.Next != s {
			t.Fatalf("unbalanced deterministic day %d", day)
		}
		for i, v := range [4]float64{1, 1, .8, 5} {
			near(t, r.KPI[i], v)
		}
		loss, e := PeriodLoss(r.KPI)
		if e != nil {
			t.Fatal(e)
		}
		near(t, loss, 0)
		s = r.Next
	}
}
func TestCapacityAndRawDeficits(t *testing.T) {
	p := DissertationParameters()
	s := DissertationInitialState()
	s.Finished = Pair{}
	u, _ := FixedAction("u0", nil)
	r, e := Step(s, Exogenous{Pair{40, 25}, 40, 90}, u, p)
	if e != nil {
		t.Fatal(e)
	}
	near(t, r.Scale, .5)
	near(t, r.Output[0], 20)
	near(t, r.Output[1], 12.5)
	near(t, r.Next.Raw, 495)
	s.Raw = 0
	r, e = Step(s, Exogenous{Pair{40, 25}, 100, 45}, u, p)
	if e != nil {
		t.Fatal(e)
	}
	near(t, r.Scale, .5)
	near(t, r.Next.Raw, 0)
}
func TestZeroResourcesAndDemand(t *testing.T) {
	p := DissertationParameters()
	u, _ := FixedAction("u0", nil)
	r, e := Step(State{}, Exogenous{}, u, p)
	if e != nil {
		t.Fatal(e)
	}
	if r.KPI != [4]float64{1, 1, 0, 0} {
		t.Fatalf("wrong zero convention %v", r.KPI)
	}
	s := DissertationInitialState()
	s.Raw = 0
	s.Finished = Pair{}
	r, e = Step(s, Exogenous{Pair{40, 25}, 0, 0}, u, p)
	if e != nil {
		t.Fatal(e)
	}
	near(t, r.Scale, 0)
	near(t, r.Next.Backlog[0], 40)
	near(t, r.Next.Backlog[1], 25)
}
func TestTemporaryAndPermanentPlan(t *testing.T) {
	p := DissertationParameters()
	s := DissertationInitialState()
	w := Exogenous{Pair{40, 25}, 100, 90}
	u, _ := FixedAction("u4", nil)
	r, e := Step(s, w, u, p)
	if e != nil {
		t.Fatal(e)
	}
	if r.ExecutedPlan != (Pair{48, 20}) || r.Next.Plan != s.Plan {
		t.Fatal("temporary priority persisted")
	}
	newPlan := Pair{45, 20}
	u, _ = FixedAction("u6", &newPlan)
	newPlan[0] = 999 // Verify snapshot copy.
	r, e = Step(s, w, u, p)
	if e != nil {
		t.Fatal(e)
	}
	if r.Next.Plan != (Pair{45, 20}) {
		t.Fatal("permanent plan not saved")
	}
	near(t, r.ActionCost, .05)
	u, _ = FixedAction("u0", nil)
	r2, e := Step(r.Next, w, u, p)
	if e != nil {
		t.Fatal(e)
	}
	if r2.ExecutedPlan != r.Next.Plan {
		t.Fatal("new plan did not persist")
	}
	near(t, r2.ActionCost, 0)
}
func TestActionsAndReplan(t *testing.T) {
	want := []float64{0, .05, .12, .15, .03, .03, .05, .27}
	p := Pair{40, 25}
	for i, id := range []string{"u0", "u1", "u2", "u3", "u4", "u5", "u6", "u7"} {
		a, e := FixedAction(id, &p)
		if e != nil {
			t.Fatal(e)
		}
		near(t, a.Cost, want[i])
	}
	if _, e := FixedAction("u6", nil); e == nil {
		t.Fatal("u6 invented a snapshot")
	}
	if _, e := FixedAction("unknown", nil); e == nil {
		t.Fatal("unknown action accepted")
	}
	got, e := Replan(Pair{1000, 0}, Pair{}, Pair{}, Pair{40, 25})
	if e != nil {
		t.Fatal(e)
	}
	if got != (Pair{70, 6.25}) {
		t.Fatal("bad bounds")
	}
	got, e = Replan(Pair{40, 25}, Pair{8, 4}, Pair{20, 10}, Pair{40, 25})
	if e != nil {
		t.Fatal(e)
	}
	if got != (Pair{40, 25}) {
		t.Fatal("bad replan equation")
	}
}
func TestMaterialBalanceProperties(t *testing.T) {
	rng := rand.New(rand.NewSource(91426))
	p := DissertationParameters()
	for i := 0; i < 500; i++ {
		pair := func(scale float64) Pair { return Pair{rng.Float64() * scale, rng.Float64() * scale} }
		s := State{rng.Float64() * 500, pair(30), pair(60), pair(70)}
		w := Exogenous{pair(60), rng.Float64() * 120, rng.Float64() * 100}
		a, _ := FixedAction([]string{"u0", "u1", "u2", "u3", "u4", "u5", "u7"}[i%7], nil)
		r, e := Step(s, w, a, p)
		if e != nil {
			t.Fatal(e)
		}
		near(t, r.Next.Raw+r.RawUsed, s.Raw+w.RawArrival+a.ExtraRaw)
		if r.CapacityUsed > r.EffectiveCapacity+1e-9 || r.Next.Raw < 0 {
			t.Fatal("resource bound violated")
		}
		for j := 0; j < 2; j++ {
			near(t, r.Next.Finished[j]+r.Shipped[j], s.Finished[j]+r.Output[j])
			near(t, r.Next.Backlog[j]+r.Shipped[j], s.Backlog[j]+w.Demand[j])
		}
	}
}
func TestPeriodLoss(t *testing.T) {
	for _, v := range []struct {
		k    [4]float64
		loss float64
	}{
		{[4]float64{1, 1, .8, 5}, 0}, {[4]float64{.95, .95, .7, 3}, 0},
		{[4]float64{1, 1, .9, 7}, 0}, {[4]float64{0, 1, .8, 5}, .35},
		{[4]float64{1, 1, .8, 0}, .25}, {[4]float64{1, 1, .8, 10}, .0625},
	} {
		got, e := PeriodLoss(v.k)
		if e != nil {
			t.Fatal(e)
		}
		near(t, got, v.loss)
	}
}
func TestRejectsInvalidPlantInputs(t *testing.T) {
	p := DissertationParameters()
	s := DissertationInitialState()
	u, _ := FixedAction("u0", nil)
	s.Raw = -1
	if _, e := Step(s, Exogenous{Pair{40, 25}, 100, 90}, u, p); e == nil {
		t.Fatal("negative stock accepted")
	}
	if _, e := PeriodLoss([4]float64{1, math.NaN(), 0, 0}); e == nil {
		t.Fatal("NaN KPI accepted")
	}
}
