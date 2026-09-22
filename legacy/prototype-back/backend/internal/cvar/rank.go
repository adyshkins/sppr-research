package cvar

import (
	"errors"
	"sort"
)

func EvaluateAction(action Action, scenarios []Scenario, kpis []KPIConfig, alpha float64) (ActionRiskResult, error) {
	scenarioLosses := make([]ScenarioLoss, 0, len(scenarios))
	hasCriticalConstraintViolation := false

	for _, scenario := range scenarios {
		for _, value := range scenario.Constraints {
			if value > 1 {
				hasCriticalConstraintViolation = true
				break
			}
		}

		scenarioLoss, err := CalculateScenarioLoss(scenario, kpis, action)
		if err != nil {
			return ActionRiskResult{}, err
		}
		scenarioLosses = append(scenarioLosses, scenarioLoss)
	}

	risk, err := CalculateRisk(scenarioLosses, alpha)
	if err != nil {
		return ActionRiskResult{}, err
	}

	return ActionRiskResult{
		Action:                         action,
		Risk:                           risk,
		HasCriticalConstraintViolation: hasCriticalConstraintViolation,
	}, nil
}

func RankActions(actions []Action, scenarios []Scenario, kpis []KPIConfig, alpha float64) ([]ActionRiskResult, error) {
	if len(actions) == 0 {
		return nil, errors.New("actions must not be empty")
	}
	if len(scenarios) == 0 {
		return nil, errors.New("scenarios must not be empty")
	}

	results := make([]ActionRiskResult, 0, len(actions))
	for _, action := range actions {
		result, err := EvaluateAction(action, scenarios, kpis, alpha)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	sortActionRiskResults(results)

	return results, nil
}

func sortActionRiskResults(results []ActionRiskResult) {
	sort.SliceStable(results, func(i, j int) bool {
		left := results[i]
		right := results[j]

		if left.HasCriticalConstraintViolation != right.HasCriticalConstraintViolation {
			return !left.HasCriticalConstraintViolation
		}
		if left.Risk.CVaRValue != right.Risk.CVaRValue {
			return left.Risk.CVaRValue < right.Risk.CVaRValue
		}

		return left.Risk.ExpectedLoss < right.Risk.ExpectedLoss
	})
}
