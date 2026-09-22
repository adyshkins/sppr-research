package diagnostics

import (
	"errors"
	"math"
)

func Diagnose(input EventInput) (DiagnosticResult, error) {
	if input.PlanDemand <= 0 {
		return DiagnosticResult{}, errors.New("plan demand must be greater than 0")
	}

	demandDeviationRate := (input.FactDemand - input.PlanDemand) / input.PlanDemand

	hypotheses := make([]Hypothesis, 0, 4)
	if demandDeviationRate >= 0.15 {
		hypotheses = append(hypotheses, Hypothesis{
			Code:        "demand_spike",
			Description: "Существенное превышение фактического спроса над плановым",
			Confidence:  clamp01(math.Min(0.95, 0.5+demandDeviationRate)),
			Details: map[string]any{
				"demand_deviation_rate": demandDeviationRate,
			},
		})
	}

	if input.CapacityLoad >= 0.9 {
		hypotheses = append(hypotheses, Hypothesis{
			Code:        "capacity_overload",
			Description: "Высокая загрузка производственных мощностей",
			Confidence:  clamp01(math.Min(0.95, input.CapacityLoad)),
			Details: map[string]any{
				"capacity_load": input.CapacityLoad,
			},
		})
	}

	if input.ForecastError >= 0.15 {
		hypotheses = append(hypotheses, Hypothesis{
			Code:        "forecast_error",
			Description: "Повышенная ошибка прогноза",
			Confidence:  clamp01(math.Min(0.95, 0.5+input.ForecastError)),
			Details: map[string]any{
				"forecast_error": input.ForecastError,
			},
		})
	}

	if input.CampaignFlag {
		hypotheses = append(hypotheses, Hypothesis{
			Code:        "campaign_effect",
			Description: "Возможное влияние маркетинговой или внешней кампании на спрос",
			Confidence:  0.7,
			Details: map[string]any{
				"campaign_flag": input.CampaignFlag,
			},
		})
	}

	if len(hypotheses) == 0 {
		hypotheses = append(hypotheses, Hypothesis{
			Code:        "normal_variation",
			Description: "Отклонение находится в зоне допустимой вариативности",
			Confidence:  0.5,
			Details:     map[string]any{},
		})
	}

	topCause := hypotheses[0]
	for _, hypothesis := range hypotheses[1:] {
		if hypothesis.Confidence > topCause.Confidence {
			topCause = hypothesis
		}
	}

	return DiagnosticResult{
		EventID:    input.EventID,
		Hypotheses: hypotheses,
		TopCause:   &topCause,
	}, nil
}

func clamp01(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}
