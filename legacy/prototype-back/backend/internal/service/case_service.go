package service

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"sppr-prototype/backend/internal/db"
	"sppr-prototype/backend/internal/fsm"
	"sppr-prototype/backend/internal/protocol"
)

type CaseRepository interface {
	GetCase(ctx context.Context, id uuid.UUID) (db.Case, error)
	UpdateCaseStatus(ctx context.Context, arg db.UpdateCaseStatusParams) (db.Case, error)
	CreateDecisionTrace(ctx context.Context, arg db.CreateDecisionTraceParams) (db.DecisionTrace, error)
}

type CaseService struct {
	repo CaseRepository
}

func NewCaseService(repo CaseRepository) *CaseService {
	return &CaseService{repo: repo}
}

func (s *CaseService) AdvanceStatus(ctx context.Context, caseID uuid.UUID, event fsm.Event) (db.Case, error) {
	currentCase, err := s.repo.GetCase(ctx, caseID)
	if err != nil {
		return db.Case{}, err
	}

	from := fsm.Status(currentCase.Status)
	to, err := fsm.MustTransition(from, event)
	if err != nil {
		return db.Case{}, err
	}

	updatedCase, err := s.repo.UpdateCaseStatus(ctx, db.UpdateCaseStatusParams{
		ID:     uuidToPgtype(caseID),
		Status: string(to),
	})
	if err != nil {
		return db.Case{}, err
	}

	trace := protocol.NewStatusChangedTrace(string(from), string(to), string(event))
	dataSnapshot, err := json.Marshal(trace.DataSnapshot)
	if err != nil {
		return db.Case{}, err
	}

	_, err = s.repo.CreateDecisionTrace(ctx, db.CreateDecisionTraceParams{
		CaseID:         uuidToPgtype(caseID),
		Step:           string(trace.Step),
		DataSnapshot:   dataSnapshot,
		ModelVersion:   pgtype.Text{},
		Parameters:     []byte("{}"),
		Risk:           []byte("{}"),
		ExpertDecision: []byte("{}"),
		FinalActionID:  pgtype.UUID{},
	})
	if err != nil {
		return db.Case{}, err
	}

	return updatedCase, nil
}
