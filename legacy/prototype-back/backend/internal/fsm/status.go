package fsm

type Status string

const (
	StatusRegistered            Status = "registered"
	StatusNormalized            Status = "normalized"
	StatusDeviationConfirmed    Status = "deviation_confirmed"
	StatusDiagnosed             Status = "diagnosed"
	StatusRiskAssessed          Status = "risk_assessed"
	StatusAlternativesGenerated Status = "alternatives_generated"
	StatusActionSelected        Status = "action_selected"
	StatusExpertValidation      Status = "expert_validation"
	StatusExpertApproved        Status = "expert_approved"
	StatusExpertRejected        Status = "expert_rejected"
	StatusExpertReturned        Status = "expert_returned"
	StatusExecuted              Status = "executed"
	StatusArchived              Status = "archived"
)
