package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"sppr-prototype/backend/internal/db"
	"sppr-prototype/backend/internal/fsm"
	"sppr-prototype/backend/internal/protocol"
)

type fakeCaseRepository struct {
	caseByID db.Case

	updateCalled bool
	traceCalled  bool

	updateArg db.UpdateCaseStatusParams
	traceArg  db.CreateDecisionTraceParams
}

func (r *fakeCaseRepository) GetCase(ctx context.Context, id uuid.UUID) (db.Case, error) {
	return r.caseByID, nil
}

func (r *fakeCaseRepository) UpdateCaseStatus(ctx context.Context, arg db.UpdateCaseStatusParams) (db.Case, error) {
	r.updateCalled = true
	r.updateArg = arg
	r.caseByID.Status = arg.Status
	return r.caseByID, nil
}

func (r *fakeCaseRepository) CreateDecisionTrace(ctx context.Context, arg db.CreateDecisionTraceParams) (db.DecisionTrace, error) {
	r.traceCalled = true
	r.traceArg = arg
	return db.DecisionTrace{}, nil
}

func TestCaseServiceAdvanceStatusAllowed(t *testing.T) {
	caseID := uuid.New()
	repo := &fakeCaseRepository{
		caseByID: db.Case{
			ID:     uuidToPgtype(caseID),
			Status: string(fsm.StatusRegistered),
		},
	}
	svc := NewCaseService(repo)

	got, err := svc.AdvanceStatus(context.Background(), caseID, fsm.EventNormalize)
	if err != nil {
		t.Fatalf("advance status: %v", err)
	}

	if got.Status != string(fsm.StatusNormalized) {
		t.Fatalf("expected status %q, got %q", fsm.StatusNormalized, got.Status)
	}
	if !repo.updateCalled {
		t.Fatalf("expected UpdateCaseStatus to be called")
	}
	if repo.updateArg.Status != string(fsm.StatusNormalized) {
		t.Fatalf("expected update status %q, got %q", fsm.StatusNormalized, repo.updateArg.Status)
	}
	if !repo.traceCalled {
		t.Fatalf("expected CreateDecisionTrace to be called")
	}
	if repo.traceArg.Step != string(protocol.StepStatusChanged) {
		t.Fatalf("expected trace step %q, got %q", protocol.StepStatusChanged, repo.traceArg.Step)
	}

	var snapshot map[string]string
	if err := json.Unmarshal(repo.traceArg.DataSnapshot, &snapshot); err != nil {
		t.Fatalf("unmarshal data snapshot: %v", err)
	}
	if snapshot["from_status"] != string(fsm.StatusRegistered) {
		t.Fatalf("unexpected from_status: %q", snapshot["from_status"])
	}
	if snapshot["to_status"] != string(fsm.StatusNormalized) {
		t.Fatalf("unexpected to_status: %q", snapshot["to_status"])
	}
	if snapshot["event"] != string(fsm.EventNormalize) {
		t.Fatalf("unexpected event: %q", snapshot["event"])
	}
}

func TestCaseServiceAdvanceStatusForbidden(t *testing.T) {
	caseID := uuid.New()
	repo := &fakeCaseRepository{
		caseByID: db.Case{
			ID:     uuidToPgtype(caseID),
			Status: string(fsm.StatusRegistered),
		},
	}
	svc := NewCaseService(repo)

	_, err := svc.AdvanceStatus(context.Background(), caseID, fsm.EventExecute)
	if err == nil {
		t.Fatalf("expected error")
	}
	if repo.updateCalled {
		t.Fatalf("expected UpdateCaseStatus not to be called")
	}
	if repo.traceCalled {
		t.Fatalf("expected CreateDecisionTrace not to be called")
	}
}

func TestCaseServiceReturnsGetCaseError(t *testing.T) {
	wantErr := errors.New("get case failed")
	repo := &errorCaseRepository{err: wantErr}
	svc := NewCaseService(repo)

	_, err := svc.AdvanceStatus(context.Background(), uuid.New(), fsm.EventNormalize)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

type errorCaseRepository struct {
	err error
}

func (r *errorCaseRepository) GetCase(ctx context.Context, id uuid.UUID) (db.Case, error) {
	return db.Case{}, r.err
}

func (r *errorCaseRepository) UpdateCaseStatus(ctx context.Context, arg db.UpdateCaseStatusParams) (db.Case, error) {
	return db.Case{}, nil
}

func (r *errorCaseRepository) CreateDecisionTrace(ctx context.Context, arg db.CreateDecisionTraceParams) (db.DecisionTrace, error) {
	return db.DecisionTrace{}, nil
}

func TestUUIDToPgtype(t *testing.T) {
	id := uuid.New()
	got := uuidToPgtype(id)

	if !got.Valid {
		t.Fatalf("expected valid pgtype uuid")
	}
	if got.Bytes != [16]byte(id) {
		t.Fatalf("unexpected uuid bytes")
	}
}
