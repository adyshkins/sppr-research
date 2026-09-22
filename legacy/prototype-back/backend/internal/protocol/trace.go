package protocol

import (
	"encoding/json"
	"time"
)

type Step string

const (
	StepCaseCreated            Step = "case_created"
	StepEventRegistered        Step = "event_registered"
	StepStatusChanged          Step = "status_changed"
	StepDataNormalized         Step = "data_normalized"
	StepDeviationConfirmed     Step = "deviation_confirmed"
	StepDiagnosticCreated      Step = "diagnostic_created"
	StepRiskAssessed           Step = "risk_assessed"
	StepCVaRCalculated         Step = "cvar_calculated"
	StepActionGenerated        Step = "action_generated"
	StepActionSelected         Step = "action_selected"
	StepSentToExpert           Step = "sent_to_expert"
	StepExpertDecisionRecorded Step = "expert_decision_recorded"
	StepCaseExecuted           Step = "case_executed"
	StepCaseArchived           Step = "case_archived"
	StepExperimentRecorded     Step = "experiment_recorded"
)

type RiskSnapshot struct {
	Alpha           float64 `json:"alpha,omitempty"`
	TailProbability float64 `json:"tail_probability,omitempty"`
	VaRValue        float64 `json:"var_value,omitempty"`
	CVaRValue       float64 `json:"cvar_value,omitempty"`
}

type ExpertDecision struct {
	Outcome  string `json:"outcome,omitempty"`
	Comment  string `json:"comment,omitempty"`
	ExpertID string `json:"expert_id,omitempty"`
}

type TracePayload struct {
	Step              Step             `json:"step"`
	Timestamp         time.Time        `json:"timestamp"`
	DataSnapshot      map[string]any   `json:"data_snapshot,omitempty"`
	ModelVersion      string           `json:"model_version,omitempty"`
	Parameters        map[string]any   `json:"parameters,omitempty"`
	Risk              *RiskSnapshot    `json:"risk,omitempty"`
	CorrectiveActions []map[string]any `json:"corrective_actions,omitempty"`
	SelectedAction    map[string]any   `json:"selected_action,omitempty"`
	ExpertDecision    *ExpertDecision  `json:"expert_decision,omitempty"`
}

func NewStatusChangedTrace(from string, to string, event string) TracePayload {
	return TracePayload{
		Step:      StepStatusChanged,
		Timestamp: time.Now().UTC(),
		DataSnapshot: map[string]any{
			"from_status": from,
			"to_status":   to,
			"event":       event,
		},
	}
}

func MarshalTracePayload(payload TracePayload) ([]byte, error) {
	return json.Marshal(payload)
}
