package cvar

import (
	"fmt"
	"math"
)

const constraintLossMultiplier = 100.0

func CalculateScenarioLoss(scenario Scenario, kpis []KPIConfig, action Action) (ScenarioLoss, error) {
	var kpiDeviationLoss float64

	for _, kpi := range kpis {
		actual, ok := scenario.KPIValues[kpi.Code]
		if !ok {
			return ScenarioLoss{}, fmt.Errorf("kpi %q is missing in scenario %q", kpi.Code, scenario.Name)
		}

		deviation := actual - kpi.Target
		switch {
		case actual < kpi.Target:
			kpiDeviationLoss += math.Abs(deviation) * kpi.Weight * kpi.UnderPenalty
		case actual > kpi.Target:
			kpiDeviationLoss += math.Abs(deviation) * kpi.Weight * kpi.OverPenalty
		}
	}

	var constraintLoss float64
	for _, value := range scenario.Constraints {
		if value > 0 {
			constraintLoss += value * constraintLossMultiplier
		}
	}

	stabilityLoss := action.StabilityPenalty
	totalLoss := kpiDeviationLoss + constraintLoss + stabilityLoss

	return ScenarioLoss{
		ScenarioName: scenario.Name,
		Probability:  scenario.Probability,
		Breakdown: LossBreakdown{
			KPIDeviationLoss: kpiDeviationLoss,
			ConstraintLoss:   constraintLoss,
			StabilityLoss:    stabilityLoss,
			TotalLoss:        totalLoss,
		},
	}, nil
}
