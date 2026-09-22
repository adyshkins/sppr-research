package cvar

import "testing"

func TestRankActionsSortsByCVaR(t *testing.T) {
	results, err := RankActions(
		[]Action{
			{Code: "expensive", StabilityPenalty: 10},
			{Code: "cheap", StabilityPenalty: 1},
		},
		[]Scenario{
			{Name: "base", Probability: 1, KPIValues: map[string]float64{"kpi": 100}},
		},
		[]KPIConfig{{Code: "kpi", Target: 100, Weight: 1, UnderPenalty: 1, OverPenalty: 1}},
		0.9,
	)
	if err != nil {
		t.Fatalf("rank actions: %v", err)
	}

	if results[0].Action.Code != "cheap" {
		t.Fatalf("expected cheap action first, got %q", results[0].Action.Code)
	}
}

func TestSortActionRiskResultsUsesExpectedLossAsTieBreaker(t *testing.T) {
	results := []ActionRiskResult{
		{Action: Action{Code: "higher_expected"}, Risk: RiskResult{CVaRValue: 10, ExpectedLoss: 5}},
		{Action: Action{Code: "lower_expected"}, Risk: RiskResult{CVaRValue: 10, ExpectedLoss: 3}},
	}

	sortActionRiskResults(results)

	if results[0].Action.Code != "lower_expected" {
		t.Fatalf("expected lower expected loss first, got %q", results[0].Action.Code)
	}
}

func TestSortActionRiskResultsPrioritizesNoCriticalViolation(t *testing.T) {
	results := []ActionRiskResult{
		{Action: Action{Code: "critical"}, HasCriticalConstraintViolation: true, Risk: RiskResult{CVaRValue: 1, ExpectedLoss: 1}},
		{Action: Action{Code: "safe"}, HasCriticalConstraintViolation: false, Risk: RiskResult{CVaRValue: 100, ExpectedLoss: 100}},
	}

	sortActionRiskResults(results)

	if results[0].Action.Code != "safe" {
		t.Fatalf("expected safe action first, got %q", results[0].Action.Code)
	}
}

func TestEvaluateActionDetectsCriticalConstraintViolation(t *testing.T) {
	result, err := EvaluateAction(
		Action{Code: "a"},
		[]Scenario{
			{Name: "critical", Probability: 1, KPIValues: map[string]float64{"kpi": 100}, Constraints: map[string]float64{"capacity": 1.1}},
		},
		[]KPIConfig{{Code: "kpi", Target: 100, Weight: 1, UnderPenalty: 1, OverPenalty: 1}},
		0.9,
	)
	if err != nil {
		t.Fatalf("evaluate action: %v", err)
	}
	if !result.HasCriticalConstraintViolation {
		t.Fatalf("expected critical constraint violation")
	}
}

func TestRankActionsErrors(t *testing.T) {
	scenarios := []Scenario{{Name: "base", Probability: 1, KPIValues: map[string]float64{"kpi": 100}}}
	kpis := []KPIConfig{{Code: "kpi", Target: 100, Weight: 1, UnderPenalty: 1, OverPenalty: 1}}

	if _, err := RankActions(nil, scenarios, kpis, 0.9); err == nil {
		t.Fatalf("expected empty actions error")
	}
	if _, err := RankActions([]Action{{Code: "a"}}, nil, kpis, 0.9); err == nil {
		t.Fatalf("expected empty scenarios error")
	}
}
