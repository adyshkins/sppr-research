package evidenceinit

import (
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/forecast"
	"reflect"
	"testing"
)

func fixture(bits int) (forecast.Estimate, diagnostics.Result) {
	x := forecast.Estimate{ClosedDay: 3, Raw: 90, Finished: [2]float64{20, 12.5}, Plan: [2]float64{40, 25}, Demand: [2]float64{40, 25}, Capacity: 100, Arrival: 90, Origin: "current_readings_point_estimate_no_initial_state_sampling"}
	if bits&1 != 0 {
		x.Demand = [2]float64{54, 33.75}
	}
	if bits&2 != 0 {
		x.Arrival = 27
	}
	if bits&4 != 0 {
		x.Capacity = 50
	}
	d := diagnostics.Result{Status: "completed", Ran: true, ForecastAllowed: true, Relevant: []string{"D", "S", "C", "DS"}}
	for i, id := range []string{"demand_high", "supply_low", "capacity_low"} {
		b := bits&(1<<i) != 0
		d.Features = append(d.Features, diagnostics.Feature{ID: id, Value: &b})
	}
	for _, id := range d.Relevant {
		d.Ranked = append(d.Ranked, diagnostics.Ranked{ID: id, Posterior: .25})
		d.ForecastWeights = append(d.ForecastWeights, diagnostics.Weighted{ID: id, Probability: .25})
	}
	return x, d
}
func TestAllFeaturePatternsNeutralizeOnlyAbsentFactors(t *testing.T) {
	for bits := 0; bits < 8; bits++ {
		x, d := fixture(bits)
		c := forecast.DefaultConfig()
		c.PathsPerHypothesis = 3
		e, err := Generate(x, d, c, 31)
		if err != nil {
			t.Fatal(err)
		}
		for _, g := range e.Groups {
			if bits&1 == 0 && g.ActiveMeans.Demand != c.Parameters.BaseDemand {
				t.Fatal("invented demand")
			}
			if bits&2 == 0 && g.ActiveMeans.RawArrival != c.BaseRaw {
				t.Fatal("invented supply")
			}
			if bits&4 == 0 && g.ActiveMeans.Capacity != c.BaseCapacity {
				t.Fatal("invented capacity")
			}
		}
	}
}
func TestAllPresentMatchesLegacyBitwise(t *testing.T) {
	x, d := fixture(7)
	c := forecast.DefaultConfig()
	a, e := Generate(x, d, c, 51)
	if e != nil {
		t.Fatal(e)
	}
	b, e := forecast.Generate(x, d, c, 51)
	if e != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("legacy draw order changed", e)
	}
}
func TestRepeatablePreservesInputs(t *testing.T) {
	x, d := fixture(0)
	c := forecast.DefaultConfig()
	a, e := Generate(x, d, c, 14)
	if e != nil {
		t.Fatal(e)
	}
	b, e := Generate(x, d, c, 14)
	if e != nil || !reflect.DeepEqual(a, b) || x.Raw != 90 {
		t.Fatal(e)
	}
}
func TestAbsentFeaturesAreNotMissingMeasurements(t *testing.T) {
	x, d := fixture(0)
	d.Features[0].Value = nil
	if _, e := Generate(x, d, forecast.DefaultConfig(), 1); e == nil {
		t.Fatal("missing treated as false")
	}
}
func TestNominalForcingDoesNotResetResidualState(t *testing.T) {
	x, d := fixture(0)
	c := forecast.DefaultConfig()
	c.RelativeSD = 0
	c.PathsPerHypothesis = 2
	e, er := Generate(x, d, c, 99)
	if er != nil {
		t.Fatal(er)
	}
	as, _ := forecast.Alternatives(x, c)
	out, er := forecast.Evaluate(x, e, c, as[:1])
	if er != nil || out[0].Risk.Expected <= 0 {
		t.Fatal("residual raw shortage erased", er)
	}
	if out[0].Trajectories[0].FinalPredictedState.Raw != 90 {
		t.Fatal("state reset")
	}
}
func TestInvalidSeedIndependentInputsRejected(t *testing.T) {
	x, d := fixture(0)
	c := forecast.DefaultConfig()
	c.Horizon = 0
	if _, e := Generate(x, d, c, 12); e == nil {
		t.Fatal("invalid config accepted")
	}
	c = forecast.DefaultConfig()
	d.ForecastWeights[0].Probability = .5
	if _, e := Generate(x, d, c, 12); e == nil {
		t.Fatal("invalid weights accepted")
	}
}
