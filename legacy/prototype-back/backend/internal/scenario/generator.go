package scenario

import (
	"errors"
	"math"

	"sppr-prototype/backend/internal/cvar"
)

func GenerateScenarioSet(input GenerationInput) (ScenarioSet, error) {
	if input.PlanDemand <= 0 {
		return ScenarioSet{}, errors.New("plan demand must be greater than 0")
	}

	actions, err := GenerateActions(input)
	if err != nil {
		return ScenarioSet{}, err
	}

	return ScenarioSet{
		Scenarios: []cvar.Scenario{
			{
				Name:        "baseline",
				Probability: 0.45,
				KPIValues: map[string]float64{
					"service_level": estimateServiceLevel(input, 0),
					"cost_index":    1.0 + input.ForecastError*0.3,
					"capacity_load": input.CapacityLoad,
				},
				Constraints: map[string]float64{
					"capacity_over": positive(input.CapacityLoad - 1),
				},
			},
			{
				Name:        "demand_growth",
				Probability: 0.25,
				KPIValues: map[string]float64{
					"service_level": estimateServiceLevel(input, 0.08),
					"cost_index":    1.0 + input.ForecastError*0.5 + 0.05,
					"capacity_load": input.CapacityLoad + 0.05,
				},
				Constraints: map[string]float64{
					"capacity_over": positive(input.CapacityLoad + 0.05 - 1),
				},
			},
			{
				Name:        "stress_capacity",
				Probability: 0.20,
				KPIValues: map[string]float64{
					"service_level": estimateServiceLevel(input, 0.12),
					"cost_index":    1.0 + input.ForecastError*0.7 + 0.1,
					"capacity_load": input.CapacityLoad + 0.12,
				},
				Constraints: map[string]float64{
					"capacity_over": positive(input.CapacityLoad + 0.12 - 1),
				},
			},
			{
				Name:        "recovery",
				Probability: 0.10,
				KPIValues: map[string]float64{
					"service_level": math.Min(0.99, estimateServiceLevel(input, -0.05)+0.03),
					"cost_index":    math.Max(0.8, 1.0+input.ForecastError*0.2-0.03),
					"capacity_load": math.Max(0, input.CapacityLoad-0.05),
				},
				Constraints: map[string]float64{
					"capacity_over": positive(input.CapacityLoad - 0.05 - 1),
				},
			},
		},
		KPIConfig: []cvar.KPIConfig{
			{
				Code:         "service_level",
				Target:       0.95,
				Weight:       1.0,
				UnderPenalty: 200,
				OverPenalty:  20,
			},
			{
				Code:         "cost_index",
				Target:       1.0,
				Weight:       0.7,
				UnderPenalty: 30,
				OverPenalty:  100,
			},
			{
				Code:         "capacity_load",
				Target:       0.85,
				Weight:       0.8,
				UnderPenalty: 20,
				OverPenalty:  120,
			},
		},
		Actions: actions,
	}, nil
}

func estimateServiceLevel(input GenerationInput, demandShock float64) float64 {
	demandRatio := input.FactDemand / input.PlanDemand
	base := 0.98 - math.Max(0, demandRatio-1)*0.25 - input.ForecastError*0.2 - demandShock
	if input.CampaignFlag {
		base -= 0.03
	}

	return clamp(base, 0.5, 0.99)
}

func clamp(value, minValue, maxValue float64) float64 {
	return math.Max(minValue, math.Min(maxValue, value))
}

func positive(value float64) float64 {
	return math.Max(0, value)
}
