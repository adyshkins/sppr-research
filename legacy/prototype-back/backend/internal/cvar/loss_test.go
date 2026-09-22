package cvar

import "testing"

func TestCalculateScenarioLoss(t *testing.T) {
	lower := 80.0
	upper := 120.0
	scenario := Scenario{
		Name:        "stress",
		Probability: 1,
		KPIValues: map[string]float64{
			"kpi_under": 90,
			"kpi_over":  130,
			"kpi_equal": 50,
		},
		Constraints: map[string]float64{
			"capacity": 0.5,
			"budget":   -2,
		},
	}
	kpis := []KPIConfig{
		{Code: "kpi_under", Target: 100, LowerBound: &lower, UpperBound: &upper, Weight: 2, UnderPenalty: 3, OverPenalty: 1},
		{Code: "kpi_over", Target: 100, Weight: 1.5, UnderPenalty: 1, OverPenalty: 4},
		{Code: "kpi_equal", Target: 50, Weight: 10, UnderPenalty: 10, OverPenalty: 10},
	}
	action := Action{Code: "a1", StabilityPenalty: 7}

	got, err := CalculateScenarioLoss(scenario, kpis, action)
	if err != nil {
		t.Fatalf("calculate scenario loss: %v", err)
	}

	assertFloatEqual(t, got.Breakdown.KPIDeviationLoss, 240)
	assertFloatEqual(t, got.Breakdown.ConstraintLoss, 50)
	assertFloatEqual(t, got.Breakdown.StabilityLoss, 7)
	assertFloatEqual(t, got.Breakdown.TotalLoss, 297)
}

func TestCalculateScenarioLossMissingKPI(t *testing.T) {
	_, err := CalculateScenarioLoss(
		Scenario{
			Name:        "missing",
			Probability: 1,
			KPIValues:   map[string]float64{},
		},
		[]KPIConfig{{Code: "kpi", Target: 1, Weight: 1, UnderPenalty: 1, OverPenalty: 1}},
		Action{},
	)
	if err == nil {
		t.Fatalf("expected error")
	}
}
