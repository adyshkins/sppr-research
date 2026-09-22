package cvar

import "testing"

func TestCalculateRisk(t *testing.T) {
	result, err := CalculateRisk([]ScenarioLoss{
		{ScenarioName: "low", Probability: 0.5, Breakdown: LossBreakdown{TotalLoss: 10}},
		{ScenarioName: "mid", Probability: 0.3, Breakdown: LossBreakdown{TotalLoss: 20}},
		{ScenarioName: "high", Probability: 0.2, Breakdown: LossBreakdown{TotalLoss: 50}},
	}, 0.8)
	if err != nil {
		t.Fatalf("calculate risk: %v", err)
	}

	assertFloatEqual(t, result.ExpectedLoss, 21)
	assertFloatEqual(t, result.VaRValue, 20)
	assertFloatEqual(t, result.CVaRValue, 32)
	assertFloatEqual(t, result.TailProbability, 0.2)
	if result.ScenarioLosses[0].ScenarioName != "low" {
		t.Fatalf("expected scenario losses to be sorted")
	}
}

func TestCalculateRiskNormalizesProbabilities(t *testing.T) {
	result, err := CalculateRisk([]ScenarioLoss{
		{ScenarioName: "a", Probability: 2, Breakdown: LossBreakdown{TotalLoss: 10}},
		{ScenarioName: "b", Probability: 2, Breakdown: LossBreakdown{TotalLoss: 30}},
	}, 0.5)
	if err != nil {
		t.Fatalf("calculate risk: %v", err)
	}

	assertFloatEqual(t, result.ScenarioLosses[0].Probability, 0.5)
	assertFloatEqual(t, result.ScenarioLosses[1].Probability, 0.5)
	assertFloatEqual(t, result.ExpectedLoss, 20)
}

func TestCalculateRiskErrors(t *testing.T) {
	valid := []ScenarioLoss{{ScenarioName: "a", Probability: 1, Breakdown: LossBreakdown{TotalLoss: 10}}}

	tests := []struct {
		name           string
		scenarioLosses []ScenarioLoss
		alpha          float64
	}{
		{"alpha less than zero", valid, 0},
		{"alpha greater than one", valid, 1},
		{"empty scenario losses", nil, 0.5},
		{"negative probability", []ScenarioLoss{{Probability: -1}}, 0.5},
		{"zero probability sum", []ScenarioLoss{{Probability: 0}, {Probability: 0}}, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CalculateRisk(tt.scenarioLosses, tt.alpha)
			if err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}
