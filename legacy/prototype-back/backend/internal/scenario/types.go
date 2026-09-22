package scenario

import "sppr-prototype/backend/internal/cvar"

type GenerationInput struct {
	EventID       string
	Product       string
	PlanDemand    float64
	FactDemand    float64
	CapacityLoad  float64
	ForecastError float64
	CampaignFlag  bool
	CauseCode     string
}

type ScenarioSet struct {
	Scenarios []cvar.Scenario
	KPIConfig []cvar.KPIConfig
	Actions   []cvar.Action
}
