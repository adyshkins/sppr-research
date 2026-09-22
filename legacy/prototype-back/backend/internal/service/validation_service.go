package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"sppr-prototype/backend/internal/db"
	"sppr-prototype/backend/internal/fsm"
	"sppr-prototype/backend/internal/protocol"
)

const (
	ExpertOutcomeApproved = "approved"
	ExpertOutcomeRejected = "rejected"
	ExpertOutcomeReturned = "returned"

	defaultExpertID = "demo-expert"
)

type ExpertDecisionInput struct {
	Outcome  string
	Comment  string
	ExpertID string
}

type ValidationResult struct {
	CaseID   uuid.UUID
	Status   string
	Outcome  string
	Comment  string
	ExpertID string
}

type ValidationService struct {
	repo        WorkflowRepository
	caseService *CaseService
}

func NewValidationService(repo WorkflowRepository) *ValidationService {
	return &ValidationService{
		repo:        repo,
		caseService: NewCaseService(repo),
	}
}

func (s *ValidationService) SubmitForValidation(ctx context.Context, caseID uuid.UUID) (db.Case, error) {
	return s.caseService.AdvanceStatus(ctx, caseID, fsm.EventSendToExpert)
}

func (s *ValidationService) RecordExpertDecision(ctx context.Context, caseID uuid.UUID, input ExpertDecisionInput) (ValidationResult, error) {
	event, err := eventForExpertOutcome(input.Outcome)
	if err != nil {
		return ValidationResult{}, err
	}
	if input.ExpertID == "" {
		input.ExpertID = defaultExpertID
	}

	updatedCase, err := s.caseService.AdvanceStatus(ctx, caseID, event)
	if err != nil {
		return ValidationResult{}, err
	}

	if err := s.createExpertDecisionTrace(ctx, caseID, updatedCase.Status, input); err != nil {
		return ValidationResult{}, err
	}

	return ValidationResult{
		CaseID:   caseID,
		Status:   updatedCase.Status,
		Outcome:  input.Outcome,
		Comment:  input.Comment,
		ExpertID: input.ExpertID,
	}, nil
}

func (s *ValidationService) Execute(ctx context.Context, caseID uuid.UUID) (db.Case, error) {
	return s.caseService.AdvanceStatus(ctx, caseID, fsm.EventExecute)
}

func (s *ValidationService) Archive(ctx context.Context, caseID uuid.UUID) (db.Case, error) {
	return s.caseService.AdvanceStatus(ctx, caseID, fsm.EventArchive)
}

func (s *ValidationService) createExpertDecisionTrace(ctx context.Context, caseID uuid.UUID, status string, input ExpertDecisionInput) error {
	return s.createTrace(ctx, caseID, protocol.TracePayload{
		Step:      protocol.StepExpertDecisionRecorded,
		Timestamp: nowUTC(),
		DataSnapshot: map[string]any{
			"case_id":               caseID.String(),
			"status_after_decision": status,
		},
		ExpertDecision: &protocol.ExpertDecision{
			Outcome:  input.Outcome,
			Comment:  input.Comment,
			ExpertID: input.ExpertID,
		},
	})
}

func (s *ValidationService) createTrace(ctx context.Context, caseID uuid.UUID, payload protocol.TracePayload) error {
	workflowService := WorkflowService{repo: s.repo}
	return workflowService.createTrace(ctx, caseID, payload)
}

func eventForExpertOutcome(outcome string) (fsm.Event, error) {
	switch outcome {
	case ExpertOutcomeApproved:
		return fsm.EventApproveByExpert, nil
	case ExpertOutcomeRejected:
		return fsm.EventRejectByExpert, nil
	case ExpertOutcomeReturned:
		return fsm.EventReturnByExpert, nil
	default:
		return "", errors.New("unknown expert decision outcome")
	}
}
