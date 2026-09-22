package cvar

type Scenario struct {
	Name        string
	Probability float64
	KPIValues   map[string]float64
	Constraints map[string]float64
}

type KPIConfig struct {
	Code         string
	Target       float64
	LowerBound   *float64
	UpperBound   *float64
	Weight       float64
	UnderPenalty float64
	OverPenalty  float64
}

type Action struct {
	Code             string
	Description      string
	ActionType       string
	StabilityPenalty float64
	Metadata         map[string]any
}

type LossBreakdown struct {
	KPIDeviationLoss float64
	ConstraintLoss   float64
	StabilityLoss    float64
	TotalLoss        float64
}

type ScenarioLoss struct {
	ScenarioName string
	Probability  float64
	Breakdown    LossBreakdown
}

type RiskResult struct {
	Alpha           float64
	TailProbability float64
	VaRValue        float64
	CVaRValue       float64
	ExpectedLoss    float64
	ScenarioLosses  []ScenarioLoss
}

type ActionRiskResult struct {
	Action                         Action
	Risk                           RiskResult
	HasCriticalConstraintViolation bool
}
