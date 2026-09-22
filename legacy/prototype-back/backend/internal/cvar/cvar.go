package cvar

import (
	"errors"
	"sort"
)

func CalculateRisk(scenarioLosses []ScenarioLoss, alpha float64) (RiskResult, error) {
	if alpha <= 0 {
		return RiskResult{}, errors.New("alpha must be greater than 0")
	}
	if alpha >= 1 {
		return RiskResult{}, errors.New("alpha must be less than 1")
	}
	if len(scenarioLosses) == 0 {
		return RiskResult{}, errors.New("scenario losses must not be empty")
	}

	var probabilitySum float64
	for _, scenarioLoss := range scenarioLosses {
		if scenarioLoss.Probability < 0 {
			return RiskResult{}, errors.New("scenario probability must not be negative")
		}
		probabilitySum += scenarioLoss.Probability
	}
	if probabilitySum <= 0 {
		return RiskResult{}, errors.New("sum of scenario probabilities must be greater than 0")
	}

	normalized := make([]ScenarioLoss, len(scenarioLosses))
	for i, scenarioLoss := range scenarioLosses {
		normalized[i] = scenarioLoss
		normalized[i].Probability = scenarioLoss.Probability / probabilitySum
	}

	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].Breakdown.TotalLoss < normalized[j].Breakdown.TotalLoss
	})

	var expectedLoss float64
	for _, scenarioLoss := range normalized {
		expectedLoss += scenarioLoss.Probability * scenarioLoss.Breakdown.TotalLoss
	}

	var cumulativeProbability float64
	var varValue float64
	var varFound bool
	for _, scenarioLoss := range normalized {
		cumulativeProbability += scenarioLoss.Probability
		if cumulativeProbability >= alpha {
			varValue = scenarioLoss.Breakdown.TotalLoss
			varFound = true
			break
		}
	}
	if !varFound {
		varValue = normalized[len(normalized)-1].Breakdown.TotalLoss
	}

	var tailWeightedLoss float64
	var actualTailProbability float64
	for _, scenarioLoss := range normalized {
		if scenarioLoss.Breakdown.TotalLoss >= varValue {
			tailWeightedLoss += scenarioLoss.Probability * scenarioLoss.Breakdown.TotalLoss
			actualTailProbability += scenarioLoss.Probability
		}
	}
	if actualTailProbability <= 0 {
		return RiskResult{}, errors.New("tail probability must be greater than 0")
	}

	return RiskResult{
		Alpha:           alpha,
		TailProbability: 1 - alpha,
		VaRValue:        varValue,
		CVaRValue:       tailWeightedLoss / actualTailProbability,
		ExpectedLoss:    expectedLoss,
		ScenarioLosses:  normalized,
	}, nil
}
