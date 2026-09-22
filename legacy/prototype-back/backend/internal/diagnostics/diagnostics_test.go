package diagnostics

import "testing"

func TestDiagnoseDemandSpike(t *testing.T) {
	result := mustDiagnose(t, EventInput{EventID: "e1", PlanDemand: 100, FactDemand: 120})

	requireHypothesis(t, result, "demand_spike")
	if result.TopCause == nil {
		t.Fatalf("expected top cause")
	}
}

func TestDiagnoseCapacityOverload(t *testing.T) {
	result := mustDiagnose(t, EventInput{EventID: "e1", PlanDemand: 100, FactDemand: 100, CapacityLoad: 0.92})

	requireHypothesis(t, result, "capacity_overload")
}

func TestDiagnoseForecastError(t *testing.T) {
	result := mustDiagnose(t, EventInput{EventID: "e1", PlanDemand: 100, FactDemand: 100, ForecastError: 0.2})

	requireHypothesis(t, result, "forecast_error")
}

func TestDiagnoseCampaignEffect(t *testing.T) {
	result := mustDiagnose(t, EventInput{EventID: "e1", PlanDemand: 100, FactDemand: 100, CampaignFlag: true})

	requireHypothesis(t, result, "campaign_effect")
}

func TestDiagnoseNormalVariation(t *testing.T) {
	result := mustDiagnose(t, EventInput{EventID: "e1", PlanDemand: 100, FactDemand: 104, CapacityLoad: 0.8, ForecastError: 0.1})

	if len(result.Hypotheses) != 1 {
		t.Fatalf("expected one hypothesis, got %d", len(result.Hypotheses))
	}
	requireHypothesis(t, result, "normal_variation")
}

func TestDiagnoseInvalidPlanDemand(t *testing.T) {
	_, err := Diagnose(EventInput{PlanDemand: 0})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestDiagnoseConfidenceRange(t *testing.T) {
	result := mustDiagnose(t, EventInput{
		EventID:       "e1",
		PlanDemand:    100,
		FactDemand:    250,
		CapacityLoad:  1.2,
		ForecastError: 0.8,
		CampaignFlag:  true,
	})

	for _, hypothesis := range result.Hypotheses {
		if hypothesis.Confidence < 0 || hypothesis.Confidence > 1 {
			t.Fatalf("confidence out of range for %s: %v", hypothesis.Code, hypothesis.Confidence)
		}
	}
}

func mustDiagnose(t *testing.T, input EventInput) DiagnosticResult {
	t.Helper()

	result, err := Diagnose(input)
	if err != nil {
		t.Fatalf("diagnose: %v", err)
	}
	if result.TopCause == nil {
		t.Fatalf("expected top cause")
	}

	return result
}

func requireHypothesis(t *testing.T, result DiagnosticResult, code string) {
	t.Helper()

	for _, hypothesis := range result.Hypotheses {
		if hypothesis.Code == code {
			return
		}
	}

	t.Fatalf("expected hypothesis %q", code)
}
