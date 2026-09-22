package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"sppr-prototype/backend/internal/db"
	"sppr-prototype/backend/internal/fsm"
	"sppr-prototype/backend/internal/protocol"
)

func TestValidationServiceSubmitForValidation(t *testing.T) {
	repo, caseID := seededValidationRepo(string(fsm.StatusActionSelected))
	svc := NewValidationService(repo)

	got, err := svc.SubmitForValidation(context.Background(), caseID)
	if err != nil {
		t.Fatalf("submit for validation: %v", err)
	}
	if got.Status != string(fsm.StatusExpertValidation) {
		t.Fatalf("expected status %q, got %q", fsm.StatusExpertValidation, got.Status)
	}
}

func TestValidationServiceRecordExpertDecisionApproved(t *testing.T) {
	repo, caseID := seededValidationRepo(string(fsm.StatusExpertValidation))
	svc := NewValidationService(repo)

	result, err := svc.RecordExpertDecision(context.Background(), caseID, ExpertDecisionInput{
		Outcome:  ExpertOutcomeApproved,
		Comment:  "approved by expert",
		ExpertID: "expert-1",
	})
	if err != nil {
		t.Fatalf("record expert decision: %v", err)
	}

	if result.Status != string(fsm.StatusExpertApproved) {
		t.Fatalf("expected status %q, got %q", fsm.StatusExpertApproved, result.Status)
	}
	assertExpertTrace(t, repo, ExpertOutcomeApproved, "approved by expert", "expert-1")
}

func TestValidationServiceRecordExpertDecisionRejected(t *testing.T) {
	repo, caseID := seededValidationRepo(string(fsm.StatusExpertValidation))

	result, err := NewValidationService(repo).RecordExpertDecision(context.Background(), caseID, ExpertDecisionInput{
		Outcome: ExpertOutcomeRejected,
	})
	if err != nil {
		t.Fatalf("record expert decision: %v", err)
	}
	if result.Status != string(fsm.StatusExpertRejected) {
		t.Fatalf("expected status %q, got %q", fsm.StatusExpertRejected, result.Status)
	}
	assertExpertTrace(t, repo, ExpertOutcomeRejected, "", defaultExpertID)
}

func TestValidationServiceRecordExpertDecisionReturned(t *testing.T) {
	repo, caseID := seededValidationRepo(string(fsm.StatusExpertValidation))

	result, err := NewValidationService(repo).RecordExpertDecision(context.Background(), caseID, ExpertDecisionInput{
		Outcome: ExpertOutcomeReturned,
	})
	if err != nil {
		t.Fatalf("record expert decision: %v", err)
	}
	if result.Status != string(fsm.StatusExpertReturned) {
		t.Fatalf("expected status %q, got %q", fsm.StatusExpertReturned, result.Status)
	}
	assertExpertTrace(t, repo, ExpertOutcomeReturned, "", defaultExpertID)
}

func TestValidationServiceExecute(t *testing.T) {
	repo, caseID := seededValidationRepo(string(fsm.StatusExpertApproved))

	got, err := NewValidationService(repo).Execute(context.Background(), caseID)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got.Status != string(fsm.StatusExecuted) {
		t.Fatalf("expected status %q, got %q", fsm.StatusExecuted, got.Status)
	}
}

func TestValidationServiceArchiveExecuted(t *testing.T) {
	repo, caseID := seededValidationRepo(string(fsm.StatusExecuted))

	got, err := NewValidationService(repo).Archive(context.Background(), caseID)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if got.Status != string(fsm.StatusArchived) {
		t.Fatalf("expected status %q, got %q", fsm.StatusArchived, got.Status)
	}
}

func TestValidationServiceArchiveRejected(t *testing.T) {
	repo, caseID := seededValidationRepo(string(fsm.StatusExpertRejected))

	got, err := NewValidationService(repo).Archive(context.Background(), caseID)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if got.Status != string(fsm.StatusArchived) {
		t.Fatalf("expected status %q, got %q", fsm.StatusArchived, got.Status)
	}
}

func TestValidationServiceErrors(t *testing.T) {
	tests := []struct {
		name   string
		status string
		run    func(*ValidationService, uuid.UUID) error
	}{
		{
			name:   "execute from expert validation",
			status: string(fsm.StatusExpertValidation),
			run: func(svc *ValidationService, caseID uuid.UUID) error {
				_, err := svc.Execute(context.Background(), caseID)
				return err
			},
		},
		{
			name:   "archive from action selected",
			status: string(fsm.StatusActionSelected),
			run: func(svc *ValidationService, caseID uuid.UUID) error {
				_, err := svc.Archive(context.Background(), caseID)
				return err
			},
		},
		{
			name:   "unknown outcome",
			status: string(fsm.StatusExpertValidation),
			run: func(svc *ValidationService, caseID uuid.UUID) error {
				_, err := svc.RecordExpertDecision(context.Background(), caseID, ExpertDecisionInput{Outcome: "unknown"})
				return err
			},
		},
		{
			name:   "decision outside expert validation",
			status: string(fsm.StatusActionSelected),
			run: func(svc *ValidationService, caseID uuid.UUID) error {
				_, err := svc.RecordExpertDecision(context.Background(), caseID, ExpertDecisionInput{Outcome: ExpertOutcomeApproved})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, caseID := seededValidationRepo(tt.status)
			if err := tt.run(NewValidationService(repo), caseID); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func seededValidationRepo(status string) (*fakeWorkflowRepository, uuid.UUID) {
	repo := newFakeWorkflowRepository()
	caseID := uuid.New()
	repo.cases[caseID] = db.Case{
		ID:     uuidToPgtype(caseID),
		Status: status,
	}
	repo.statusHistory = append(repo.statusHistory, status)
	return repo, caseID
}

func assertExpertTrace(t *testing.T, repo *fakeWorkflowRepository, outcome string, comment string, expertID string) {
	t.Helper()

	for _, arg := range repo.traceArgs {
		if arg.Step != string(protocol.StepExpertDecisionRecorded) {
			continue
		}

		var decision protocol.ExpertDecision
		if err := json.Unmarshal(arg.ExpertDecision, &decision); err != nil {
			t.Fatalf("unmarshal expert decision: %v", err)
		}
		if decision.Outcome != outcome {
			t.Fatalf("expected outcome %q, got %q", outcome, decision.Outcome)
		}
		if decision.Comment != comment {
			t.Fatalf("expected comment %q, got %q", comment, decision.Comment)
		}
		if decision.ExpertID != expertID {
			t.Fatalf("expected expert id %q, got %q", expertID, decision.ExpertID)
		}
		return
	}

	t.Fatalf("expected expert_decision_recorded trace")
}
