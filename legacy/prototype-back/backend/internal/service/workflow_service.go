package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"sppr-prototype/backend/internal/cvar"
	"sppr-prototype/backend/internal/db"
	"sppr-prototype/backend/internal/diagnostics"
	"sppr-prototype/backend/internal/fsm"
	"sppr-prototype/backend/internal/protocol"
	"sppr-prototype/backend/internal/scenario"
)

const defaultWorkflowAlpha = 0.95

type WorkflowInput struct {
	EventID       string
	Product       string
	PlanDemand    float64
	FactDemand    float64
	CapacityLoad  float64
	ForecastError float64
	CampaignFlag  bool
	Alpha         float64
}

type WorkflowResult struct {
	CaseID             uuid.UUID
	FinalStatus        string
	TopCauseCode       string
	SelectedActionCode string
	ExpectedLoss       float64
	VaRValue           float64
	CVaRValue          float64
}

type WorkflowRepository interface {
	CaseRepository

	CreateCase(ctx context.Context, arg db.CreateCaseParams) (db.Case, error)
	CreateEvent(ctx context.Context, arg db.CreateEventParams) (db.Event, error)
	CreateDiagnostic(ctx context.Context, arg db.CreateDiagnosticParams) (db.Diagnostic, error)
	CreateScenario(ctx context.Context, arg db.CreateScenarioParams) (db.Scenario, error)
	CreateCorrectiveAction(ctx context.Context, arg db.CreateCorrectiveActionParams) (db.CorrectiveAction, error)
	ClearSelectedCorrectiveActionsByCase(ctx context.Context, caseID uuid.UUID) error
	MarkCorrectiveActionSelected(ctx context.Context, id uuid.UUID) (db.CorrectiveAction, error)
	UpdateCaseSelectedAction(ctx context.Context, arg db.UpdateCaseSelectedActionParams) (db.Case, error)
}

type WorkflowService struct {
	repo        WorkflowRepository
	caseService *CaseService
}

func NewWorkflowService(repo WorkflowRepository) *WorkflowService {
	return &WorkflowService{
		repo:        repo,
		caseService: NewCaseService(repo),
	}
}

func (s *WorkflowService) ProcessEvent(ctx context.Context, input WorkflowInput) (WorkflowResult, error) {
	alpha, err := validateWorkflowInput(input)
	if err != nil {
		return WorkflowResult{}, err
	}

	createdCase, err := s.repo.CreateCase(ctx, db.CreateCaseParams{
		Status:    string(fsm.StatusRegistered),
		RiskLevel: textFromString("unknown"),
	})
	if err != nil {
		return WorkflowResult{}, err
	}

	caseID, err := uuidFromPgtype(createdCase.ID)
	if err != nil {
		return WorkflowResult{}, err
	}

	if err := s.createTrace(ctx, caseID, protocol.TracePayload{
		Step:      protocol.StepCaseCreated,
		Timestamp: nowUTC(),
		DataSnapshot: map[string]any{
			"event_id": input.EventID,
			"product":  input.Product,
			"status":   string(fsm.StatusRegistered),
		},
	}); err != nil {
		return WorkflowResult{}, err
	}

	rawData, err := json.Marshal(input)
	if err != nil {
		return WorkflowResult{}, err
	}
	if _, err := s.repo.CreateEvent(ctx, db.CreateEventParams{
		CaseID:     uuidToPgtype(caseID),
		OccurredAt: timestamptzNow(),
		Source:     "manual",
		RawData:    rawData,
	}); err != nil {
		return WorkflowResult{}, err
	}

	if err := s.createTrace(ctx, caseID, protocol.TracePayload{
		Step:      protocol.StepEventRegistered,
		Timestamp: nowUTC(),
		DataSnapshot: map[string]any{
			"event_id": input.EventID,
			"source":   "manual",
		},
	}); err != nil {
		return WorkflowResult{}, err
	}

	if _, err := s.caseService.AdvanceStatus(ctx, caseID, fsm.EventNormalize); err != nil {
		return WorkflowResult{}, err
	}
	if _, err := s.caseService.AdvanceStatus(ctx, caseID, fsm.EventConfirmDeviation); err != nil {
		return WorkflowResult{}, err
	}

	diagnosticResult, err := diagnostics.Diagnose(diagnostics.EventInput{
		EventID:       input.EventID,
		Product:       input.Product,
		PlanDemand:    input.PlanDemand,
		FactDemand:    input.FactDemand,
		CapacityLoad:  input.CapacityLoad,
		ForecastError: input.ForecastError,
		CampaignFlag:  input.CampaignFlag,
	})
	if err != nil {
		return WorkflowResult{}, err
	}
	if diagnosticResult.TopCause == nil {
		return WorkflowResult{}, errors.New("top cause is not available")
	}

	for _, hypothesis := range diagnosticResult.Hypotheses {
		confidence, err := numericFromFloat64(hypothesis.Confidence)
		if err != nil {
			return WorkflowResult{}, err
		}
		details, err := json.Marshal(hypothesis.Details)
		if err != nil {
			return WorkflowResult{}, err
		}
		if _, err := s.repo.CreateDiagnostic(ctx, db.CreateDiagnosticParams{
			CaseID:     uuidToPgtype(caseID),
			Hypothesis: hypothesis.Code,
			Confidence: confidence,
			Details:    details,
		}); err != nil {
			return WorkflowResult{}, err
		}
	}

	if err := s.createTrace(ctx, caseID, protocol.TracePayload{
		Step:      protocol.StepDiagnosticCreated,
		Timestamp: nowUTC(),
		DataSnapshot: map[string]any{
			"top_cause_code": diagnosticResult.TopCause.Code,
			"hypotheses":     len(diagnosticResult.Hypotheses),
		},
	}); err != nil {
		return WorkflowResult{}, err
	}

	if _, err := s.caseService.AdvanceStatus(ctx, caseID, fsm.EventDiagnose); err != nil {
		return WorkflowResult{}, err
	}

	scenarioSet, err := scenario.GenerateScenarioSet(scenario.GenerationInput{
		EventID:       input.EventID,
		Product:       input.Product,
		PlanDemand:    input.PlanDemand,
		FactDemand:    input.FactDemand,
		CapacityLoad:  input.CapacityLoad,
		ForecastError: input.ForecastError,
		CampaignFlag:  input.CampaignFlag,
		CauseCode:     diagnosticResult.TopCause.Code,
	})
	if err != nil {
		return WorkflowResult{}, err
	}

	for _, generatedScenario := range scenarioSet.Scenarios {
		probability, err := numericFromFloat64(generatedScenario.Probability)
		if err != nil {
			return WorkflowResult{}, err
		}
		lossVector, err := json.Marshal(map[string]any{
			"kpi_values":  generatedScenario.KPIValues,
			"constraints": generatedScenario.Constraints,
		})
		if err != nil {
			return WorkflowResult{}, err
		}
		if _, err := s.repo.CreateScenario(ctx, db.CreateScenarioParams{
			CaseID:      uuidToPgtype(caseID),
			Name:        generatedScenario.Name,
			Probability: probability,
			LossVector:  lossVector,
		}); err != nil {
			return WorkflowResult{}, err
		}
	}

	actionResults, err := cvar.RankActions(scenarioSet.Actions, scenarioSet.Scenarios, scenarioSet.KPIConfig, alpha)
	if err != nil {
		return WorkflowResult{}, err
	}
	if len(actionResults) == 0 {
		return WorkflowResult{}, errors.New("action risk results are empty")
	}
	bestAction := actionResults[0]

	if err := s.createTrace(ctx, caseID, protocol.TracePayload{
		Step:      protocol.StepCVaRCalculated,
		Timestamp: nowUTC(),
		DataSnapshot: map[string]any{
			"expected_loss":        bestAction.Risk.ExpectedLoss,
			"selected_action_code": bestAction.Action.Code,
		},
		Risk: &protocol.RiskSnapshot{
			Alpha:           bestAction.Risk.Alpha,
			TailProbability: bestAction.Risk.TailProbability,
			VaRValue:        bestAction.Risk.VaRValue,
			CVaRValue:       bestAction.Risk.CVaRValue,
		},
	}); err != nil {
		return WorkflowResult{}, err
	}

	if _, err := s.caseService.AdvanceStatus(ctx, caseID, fsm.EventAssessRisk); err != nil {
		return WorkflowResult{}, err
	}

	actionIDs := make(map[string]uuid.UUID, len(actionResults))
	for _, actionResult := range actionResults {
		expectedLoss, err := numericFromFloat64(actionResult.Risk.ExpectedLoss)
		if err != nil {
			return WorkflowResult{}, err
		}
		varValue, err := numericFromFloat64(actionResult.Risk.VaRValue)
		if err != nil {
			return WorkflowResult{}, err
		}
		cvarValue, err := numericFromFloat64(actionResult.Risk.CVaRValue)
		if err != nil {
			return WorkflowResult{}, err
		}
		constraintsViolated, err := json.Marshal(map[string]any{
			"has_critical_constraint_violation": actionResult.HasCriticalConstraintViolation,
		})
		if err != nil {
			return WorkflowResult{}, err
		}
		createdAction, err := s.repo.CreateCorrectiveAction(ctx, db.CreateCorrectiveActionParams{
			CaseID:              uuidToPgtype(caseID),
			Code:                textFromString(actionResult.Action.Code),
			Description:         actionResult.Action.Description,
			ActionType:          actionResult.Action.ActionType,
			ExpectedLoss:        expectedLoss,
			VarValue:            varValue,
			CvarValue:           cvarValue,
			ConstraintsViolated: constraintsViolated,
			IsSelected:          false,
		})
		if err != nil {
			return WorkflowResult{}, err
		}
		actionID, err := uuidFromPgtype(createdAction.ID)
		if err != nil {
			return WorkflowResult{}, err
		}
		actionIDs[actionResult.Action.Code] = actionID
	}

	if err := s.createTrace(ctx, caseID, protocol.TracePayload{
		Step:      protocol.StepActionGenerated,
		Timestamp: nowUTC(),
		DataSnapshot: map[string]any{
			"actions_count": len(actionResults),
		},
	}); err != nil {
		return WorkflowResult{}, err
	}

	if _, err := s.caseService.AdvanceStatus(ctx, caseID, fsm.EventGenerateAlternatives); err != nil {
		return WorkflowResult{}, err
	}

	selectedActionID, ok := actionIDs[bestAction.Action.Code]
	if !ok {
		return WorkflowResult{}, errors.New("selected action id is not available")
	}
	if err := s.repo.ClearSelectedCorrectiveActionsByCase(ctx, caseID); err != nil {
		return WorkflowResult{}, err
	}
	selectedAction, err := s.repo.MarkCorrectiveActionSelected(ctx, selectedActionID)
	if err != nil {
		return WorkflowResult{}, err
	}
	if _, err := s.repo.UpdateCaseSelectedAction(ctx, db.UpdateCaseSelectedActionParams{
		ID:               uuidToPgtype(caseID),
		SelectedActionID: selectedAction.ID,
	}); err != nil {
		return WorkflowResult{}, err
	}

	if err := s.createTrace(ctx, caseID, protocol.TracePayload{
		Step:      protocol.StepActionSelected,
		Timestamp: nowUTC(),
		DataSnapshot: map[string]any{
			"selected_action_code": bestAction.Action.Code,
			"selected_action_id":   selectedActionID.String(),
		},
	}); err != nil {
		return WorkflowResult{}, err
	}

	finalCase, err := s.caseService.AdvanceStatus(ctx, caseID, fsm.EventSelectAction)
	if err != nil {
		return WorkflowResult{}, err
	}

	return WorkflowResult{
		CaseID:             caseID,
		FinalStatus:        finalCase.Status,
		TopCauseCode:       diagnosticResult.TopCause.Code,
		SelectedActionCode: bestAction.Action.Code,
		ExpectedLoss:       bestAction.Risk.ExpectedLoss,
		VaRValue:           bestAction.Risk.VaRValue,
		CVaRValue:          bestAction.Risk.CVaRValue,
	}, nil
}

func validateWorkflowInput(input WorkflowInput) (float64, error) {
	if input.EventID == "" {
		return 0, errors.New("event id must not be empty")
	}
	if input.Product == "" {
		return 0, errors.New("product must not be empty")
	}
	if input.PlanDemand <= 0 {
		return 0, errors.New("plan demand must be greater than 0")
	}

	alpha := input.Alpha
	if alpha == 0 {
		alpha = defaultWorkflowAlpha
	}
	if alpha < 0 || alpha >= 1 {
		return 0, errors.New("alpha must be greater than or equal to 0 and less than 1")
	}

	return alpha, nil
}

func (s *WorkflowService) createTrace(ctx context.Context, caseID uuid.UUID, payload protocol.TracePayload) error {
	data, err := protocol.MarshalTracePayload(payload)
	if err != nil {
		return err
	}
	dataSnapshot, err := json.Marshal(payload.DataSnapshot)
	if err != nil {
		return err
	}

	risk := []byte("{}")
	if payload.Risk != nil {
		risk, err = json.Marshal(payload.Risk)
		if err != nil {
			return err
		}
	}
	expertDecision := []byte("{}")
	if payload.ExpertDecision != nil {
		expertDecision, err = json.Marshal(payload.ExpertDecision)
		if err != nil {
			return err
		}
	}

	_, err = s.repo.CreateDecisionTrace(ctx, db.CreateDecisionTraceParams{
		CaseID:         uuidToPgtype(caseID),
		Step:           string(payload.Step),
		DataSnapshot:   dataSnapshot,
		ModelVersion:   pgtype.Text{},
		Parameters:     data,
		Risk:           risk,
		ExpertDecision: expertDecision,
		FinalActionID:  pgtype.UUID{},
	})

	return err
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
