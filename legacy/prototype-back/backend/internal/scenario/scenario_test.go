package scenario

import (
	"math"
	"testing"
)

func TestGenerateScenarioSet(t *testing.T) {
	set, err := GenerateScenarioSet(validInput())
	if err != nil {
		t.Fatalf("generate scenario set: %v", err)
	}

	if len(set.Scenarios) != 4 {
		t.Fatalf("expected 4 scenarios, got %d", len(set.Scenarios))
	}

	var probabilitySum float64
	for _, scenario := range set.Scenarios {
		probabilitySum += scenario.Probability
		for _, code := range []string{"service_level", "cost_index", "capacity_load"} {
			if _, ok := scenario.KPIValues[code]; !ok {
				t.Fatalf("scenario %s missing kpi %s", scenario.Name, code)
			}
		}
		if _, ok := scenario.Constraints["capacity_over"]; !ok {
			t.Fatalf("scenario %s missing capacity_over constraint", scenario.Name)
		}
	}
	if math.Abs(probabilitySum-1) > 1e-9 {
		t.Fatalf("expected probability sum 1, got %v", probabilitySum)
	}

	requireKPIConfig(t, set, "service_level")
	requireKPIConfig(t, set, "cost_index")
	requireKPIConfig(t, set, "capacity_load")

	if len(set.Actions) != 4 {
		t.Fatalf("expected 4 actions, got %d", len(set.Actions))
	}
}

func TestGenerateScenarioSetInvalidPlanDemand(t *testing.T) {
	input := validInput()
	input.PlanDemand = 0

	_, err := GenerateScenarioSet(input)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func requireKPIConfig(t *testing.T, set ScenarioSet, code string) {
	t.Helper()

	for _, config := range set.KPIConfig {
		if config.Code == code {
			return
		}
	}

	t.Fatalf("expected kpi config %s", code)
}

func validInput() GenerationInput {
	return GenerationInput{
		EventID:       "event-1",
		Product:       "product-a",
		PlanDemand:    100,
		FactDemand:    120,
		CapacityLoad:  0.91,
		ForecastError: 0.18,
		CampaignFlag:  true,
		CauseCode:     "demand_spike",
	}
}
