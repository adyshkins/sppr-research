package diagnostics

type EventInput struct {
	EventID       string
	Product       string
	PlanDemand    float64
	FactDemand    float64
	CapacityLoad  float64
	ForecastError float64
	CampaignFlag  bool
}

type Hypothesis struct {
	Code        string
	Description string
	Confidence  float64
	Details     map[string]any
}

type DiagnosticResult struct {
	EventID    string
	Hypotheses []Hypothesis
	TopCause   *Hypothesis
}
