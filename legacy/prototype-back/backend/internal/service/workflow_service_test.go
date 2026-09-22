package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"sppr-prototype/backend/internal/db"
	"sppr-prototype/backend/internal/fsm"
	"sppr-prototype/backend/internal/protocol"
)

func TestWorkflowServiceProcessEventSuccess(t *testing.T) {
	repo := newFakeWorkflowRepository()
	svc := NewWorkflowService(repo)

	result, err := svc.ProcessEvent(context.Background(), validWorkflowInput())
	if err != nil {
		t.Fatalf("process event: %v", err)
	}

	if result.CaseID == uuid.Nil {
		t.Fatalf("expected case id")
	}
	if result.FinalStatus != string(fsm.StatusActionSelected) {
		t.Fatalf("expected final status %q, got %q", fsm.StatusActionSelected, result.FinalStatus)
	}
	if result.TopCauseCode == "" {
		t.Fatalf("expected top cause code")
	}
	if result.SelectedActionCode == "" {
		t.Fatalf("expected selected action code")
	}
	if result.CVaRValue <= 0 {
		t.Fatalf("expected positive cvar, got %v", result.CVaRValue)
	}

	wantStatuses := []string{
		string(fsm.StatusRegistered),
		string(fsm.StatusNormalized),
		string(fsm.StatusDeviationConfirmed),
		string(fsm.StatusDiagnosed),
		string(fsm.StatusRiskAssessed),
		string(fsm.StatusAlternativesGenerated),
		string(fsm.StatusActionSelected),
	}
	if len(repo.statusHistory) != len(wantStatuses) {
		t.Fatalf("expected status history %v, got %v", wantStatuses, repo.statusHistory)
	}
	for i, want := range wantStatuses {
		if repo.statusHistory[i] != want {
			t.Fatalf("status %d: expected %q, got %q", i, want, repo.statusHistory[i])
		}
	}

	if !repo.createCaseCalled {
		t.Fatalf("expected CreateCase call")
	}
	if !repo.createEventCalled {
		t.Fatalf("expected CreateEvent call")
	}
	if repo.createDiagnosticCalls == 0 {
		t.Fatalf("expected CreateDiagnostic calls")
	}
	if repo.createScenarioCalls == 0 {
		t.Fatalf("expected CreateScenario calls")
	}
	if repo.createCorrectiveActionCalls == 0 {
		t.Fatalf("expected CreateCorrectiveAction calls")
	}
	if repo.createDecisionTraceCalls == 0 {
		t.Fatalf("expected CreateDecisionTrace calls")
	}
	if repo.updateCaseStatusCalls == 0 {
		t.Fatalf("expected UpdateCaseStatus calls")
	}
	if !repo.updateCaseSelectedActionCalled {
		t.Fatalf("expected UpdateCaseSelectedAction call")
	}

	selectedCount := 0
	for _, action := range repo.correctiveActions {
		if action.IsSelected {
			selectedCount++
		}
	}
	if selectedCount != 1 {
		t.Fatalf("expected one selected action, got %d", selectedCount)
	}

	requireTraceStep(t, repo.traceSteps, string(protocol.StepCaseCreated))
	requireTraceStep(t, repo.traceSteps, string(protocol.StepEventRegistered))
	requireTraceStep(t, repo.traceSteps, string(protocol.StepDiagnosticCreated))
	requireTraceStep(t, repo.traceSteps, string(protocol.StepCVaRCalculated))
	requireTraceStep(t, repo.traceSteps, string(protocol.StepActionGenerated))
	requireTraceStep(t, repo.traceSteps, string(protocol.StepActionSelected))
	requireTraceStep(t, repo.traceSteps, string(protocol.StepStatusChanged))
}

func TestWorkflowServiceValidation(t *testing.T) {
	tests := []struct {
		name  string
		input WorkflowInput
	}{
		{"invalid plan demand", func() WorkflowInput {
			input := validWorkflowInput()
			input.PlanDemand = 0
			return input
		}()},
		{"empty event id", func() WorkflowInput {
			input := validWorkflowInput()
			input.EventID = ""
			return input
		}()},
		{"empty product", func() WorkflowInput {
			input := validWorkflowInput()
			input.Product = ""
			return input
		}()},
		{"negative alpha", func() WorkflowInput {
			input := validWorkflowInput()
			input.Alpha = -0.1
			return input
		}()},
		{"alpha one", func() WorkflowInput {
			input := validWorkflowInput()
			input.Alpha = 1
			return input
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWorkflowService(newFakeWorkflowRepository()).ProcessEvent(context.Background(), tt.input)
			if err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestWorkflowServiceDefaultAlpha(t *testing.T) {
	repo := newFakeWorkflowRepository()
	input := validWorkflowInput()
	input.Alpha = 0

	_, err := NewWorkflowService(repo).ProcessEvent(context.Background(), input)
	if err != nil {
		t.Fatalf("process event: %v", err)
	}

	var found bool
	for _, risk := range repo.traceRisksByStep[string(protocol.StepCVaRCalculated)] {
		var snapshot protocol.RiskSnapshot
		if err := json.Unmarshal(risk, &snapshot); err != nil {
			t.Fatalf("unmarshal risk: %v", err)
		}
		if snapshot.Alpha == defaultWorkflowAlpha {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected cvar trace with default alpha %v", defaultWorkflowAlpha)
	}
}

type fakeWorkflowRepository struct {
	cases             map[uuid.UUID]db.Case
	correctiveActions map[uuid.UUID]db.CorrectiveAction

	createCaseCalled                 bool
	createEventCalled                bool
	createDiagnosticCalls            int
	createScenarioCalls              int
	createCorrectiveActionCalls      int
	createDecisionTraceCalls         int
	updateCaseStatusCalls            int
	updateCaseSelectedActionCalled   bool
	clearSelectedCorrectiveActions   bool
	markCorrectiveActionSelectedCall bool

	statusHistory    []string
	traceSteps       []string
	traceArgs        []db.CreateDecisionTraceParams
	traceRisksByStep map[string][][]byte
}

func newFakeWorkflowRepository() *fakeWorkflowRepository {
	return &fakeWorkflowRepository{
		cases:             make(map[uuid.UUID]db.Case),
		correctiveActions: make(map[uuid.UUID]db.CorrectiveAction),
		traceRisksByStep:  make(map[string][][]byte),
	}
}

func (r *fakeWorkflowRepository) CreateCase(ctx context.Context, arg db.CreateCaseParams) (db.Case, error) {
	r.createCaseCalled = true
	id := uuid.New()
	created := db.Case{
		ID:        uuidToPgtype(id),
		Status:    arg.Status,
		RiskLevel: arg.RiskLevel,
	}
	r.cases[id] = created
	r.statusHistory = append(r.statusHistory, arg.Status)
	return created, nil
}

func (r *fakeWorkflowRepository) GetCase(ctx context.Context, id uuid.UUID) (db.Case, error) {
	return r.cases[id], nil
}

func (r *fakeWorkflowRepository) UpdateCaseStatus(ctx context.Context, arg db.UpdateCaseStatusParams) (db.Case, error) {
	r.updateCaseStatusCalls++
	id, err := uuidFromPgtype(arg.ID)
	if err != nil {
		return db.Case{}, err
	}
	item := r.cases[id]
	item.Status = arg.Status
	r.cases[id] = item
	r.statusHistory = append(r.statusHistory, arg.Status)
	return item, nil
}

func (r *fakeWorkflowRepository) CreateDecisionTrace(ctx context.Context, arg db.CreateDecisionTraceParams) (db.DecisionTrace, error) {
	r.createDecisionTraceCalls++
	r.traceSteps = append(r.traceSteps, arg.Step)
	r.traceArgs = append(r.traceArgs, arg)
	r.traceRisksByStep[arg.Step] = append(r.traceRisksByStep[arg.Step], arg.Risk)
	return db.DecisionTrace{}, nil
}

func (r *fakeWorkflowRepository) CreateEvent(ctx context.Context, arg db.CreateEventParams) (db.Event, error) {
	r.createEventCalled = true
	return db.Event{}, nil
}

func (r *fakeWorkflowRepository) CreateDiagnostic(ctx context.Context, arg db.CreateDiagnosticParams) (db.Diagnostic, error) {
	r.createDiagnosticCalls++
	return db.Diagnostic{}, nil
}

func (r *fakeWorkflowRepository) CreateScenario(ctx context.Context, arg db.CreateScenarioParams) (db.Scenario, error) {
	r.createScenarioCalls++
	return db.Scenario{}, nil
}

func (r *fakeWorkflowRepository) CreateCorrectiveAction(ctx context.Context, arg db.CreateCorrectiveActionParams) (db.CorrectiveAction, error) {
	r.createCorrectiveActionCalls++
	id := uuid.New()
	item := db.CorrectiveAction{
		ID:                  uuidToPgtype(id),
		CaseID:              arg.CaseID,
		Code:                arg.Code,
		Description:         arg.Description,
		ActionType:          arg.ActionType,
		ExpectedLoss:        arg.ExpectedLoss,
		VarValue:            arg.VarValue,
		CvarValue:           arg.CvarValue,
		ConstraintsViolated: arg.ConstraintsViolated,
		IsSelected:          arg.IsSelected,
	}
	r.correctiveActions[id] = item
	return item, nil
}

func (r *fakeWorkflowRepository) ClearSelectedCorrectiveActionsByCase(ctx context.Context, caseID uuid.UUID) error {
	r.clearSelectedCorrectiveActions = true
	for id, action := range r.correctiveActions {
		action.IsSelected = false
		r.correctiveActions[id] = action
	}
	return nil
}

func (r *fakeWorkflowRepository) MarkCorrectiveActionSelected(ctx context.Context, id uuid.UUID) (db.CorrectiveAction, error) {
	r.markCorrectiveActionSelectedCall = true
	action := r.correctiveActions[id]
	action.IsSelected = true
	r.correctiveActions[id] = action
	return action, nil
}

func (r *fakeWorkflowRepository) UpdateCaseSelectedAction(ctx context.Context, arg db.UpdateCaseSelectedActionParams) (db.Case, error) {
	r.updateCaseSelectedActionCalled = true
	id, err := uuidFromPgtype(arg.ID)
	if err != nil {
		return db.Case{}, err
	}
	item := r.cases[id]
	item.SelectedActionID = arg.SelectedActionID
	r.cases[id] = item
	return item, nil
}

func validWorkflowInput() WorkflowInput {
	return WorkflowInput{
		EventID:       "event-1",
		Product:       "product-a",
		PlanDemand:    100,
		FactDemand:    125,
		CapacityLoad:  0.94,
		ForecastError: 0.2,
		CampaignFlag:  true,
		Alpha:         0.9,
	}
}

func requireTraceStep(t *testing.T, steps []string, step string) {
	t.Helper()

	for _, item := range steps {
		if item == step {
			return
		}
	}

	t.Fatalf("expected trace step %q in %v", step, steps)
}

var _ WorkflowRepository = (*fakeWorkflowRepository)(nil)
var _ = pgtype.UUID{}
