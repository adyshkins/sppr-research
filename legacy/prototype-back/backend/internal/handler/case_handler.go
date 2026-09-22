package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"sppr-prototype/backend/internal/db"
)

type CaseHandler struct {
	queries *db.Queries
}

func NewCaseHandler(queries *db.Queries) *CaseHandler {
	return &CaseHandler{queries: queries}
}

type CaseResponse struct {
	ID               string     `json:"id"`
	Status           string     `json:"status"`
	RiskLevel        string     `json:"risk_level,omitempty"`
	SelectedActionID string     `json:"selected_action_id,omitempty"`
	CreatedAt        *time.Time `json:"created_at,omitempty"`
	UpdatedAt        *time.Time `json:"updated_at,omitempty"`
}

type DecisionTraceResponse struct {
	ID             string          `json:"id"`
	CaseID         string          `json:"case_id"`
	Step           string          `json:"step"`
	Timestamp      *time.Time      `json:"timestamp,omitempty"`
	DataSnapshot   json.RawMessage `json:"data_snapshot"`
	ModelVersion   string          `json:"model_version,omitempty"`
	Parameters     json.RawMessage `json:"parameters"`
	Risk           json.RawMessage `json:"risk"`
	ExpertDecision json.RawMessage `json:"expert_decision"`
	FinalActionID  string          `json:"final_action_id,omitempty"`
	CreatedAt      *time.Time      `json:"created_at,omitempty"`
}

type DiagnosticResponse struct {
	ID         string          `json:"id"`
	CaseID     string          `json:"case_id"`
	Hypothesis string          `json:"hypothesis"`
	Confidence *float64        `json:"confidence,omitempty"`
	Details    json.RawMessage `json:"details"`
	CreatedAt  *time.Time      `json:"created_at,omitempty"`
}

type ScenarioResponse struct {
	ID          string          `json:"id"`
	CaseID      string          `json:"case_id"`
	Name        string          `json:"name"`
	Probability *float64        `json:"probability,omitempty"`
	LossVector  json.RawMessage `json:"loss_vector"`
	CreatedAt   *time.Time      `json:"created_at,omitempty"`
}

type CorrectiveActionResponse struct {
	ID                  string          `json:"id"`
	CaseID              string          `json:"case_id"`
	Code                string          `json:"code,omitempty"`
	Description         string          `json:"description"`
	ActionType          string          `json:"action_type"`
	ExpectedLoss        *float64        `json:"expected_loss,omitempty"`
	VaRValue            *float64        `json:"var_value,omitempty"`
	CVaRValue           *float64        `json:"cvar_value,omitempty"`
	ConstraintsViolated json.RawMessage `json:"constraints_violated"`
	IsSelected          bool            `json:"is_selected"`
	CreatedAt           *time.Time      `json:"created_at,omitempty"`
}

func (h *CaseHandler) List(c *gin.Context) {
	status := c.Query("status")

	var (
		cases []db.Case
		err   error
	)
	if status != "" {
		cases, err = h.queries.ListCasesByStatus(c.Request.Context(), status)
	} else {
		cases, err = h.queries.ListCases(c.Request.Context())
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list cases")
		return
	}

	response := make([]CaseResponse, 0, len(cases))
	for _, item := range cases {
		response = append(response, caseResponse(item))
	}

	c.JSON(http.StatusOK, response)
}

func (h *CaseHandler) GetByID(c *gin.Context) {
	id, ok := parsePathUUID(c)
	if !ok {
		return
	}

	item, err := h.queries.GetCase(c.Request.Context(), uuidToPgtype(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(c, http.StatusNotFound, "case not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to get case")
		return
	}

	c.JSON(http.StatusOK, caseResponse(item))
}

func (h *CaseHandler) Trace(c *gin.Context) {
	id, ok := parsePathUUID(c)
	if !ok {
		return
	}

	items, err := h.queries.ListDecisionTraceByCase(c.Request.Context(), uuidToPgtype(id))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list case trace")
		return
	}

	response := make([]DecisionTraceResponse, 0, len(items))
	for _, item := range items {
		response = append(response, decisionTraceResponse(item))
	}

	c.JSON(http.StatusOK, response)
}

func (h *CaseHandler) Diagnostics(c *gin.Context) {
	id, ok := parsePathUUID(c)
	if !ok {
		return
	}

	items, err := h.queries.ListDiagnosticsByCase(c.Request.Context(), uuidToPgtype(id))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list case diagnostics")
		return
	}

	response := make([]DiagnosticResponse, 0, len(items))
	for _, item := range items {
		response = append(response, diagnosticResponse(item))
	}

	c.JSON(http.StatusOK, response)
}

func (h *CaseHandler) Scenarios(c *gin.Context) {
	id, ok := parsePathUUID(c)
	if !ok {
		return
	}

	items, err := h.queries.ListScenariosByCase(c.Request.Context(), uuidToPgtype(id))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list case scenarios")
		return
	}

	response := make([]ScenarioResponse, 0, len(items))
	for _, item := range items {
		response = append(response, scenarioResponse(item))
	}

	c.JSON(http.StatusOK, response)
}

func (h *CaseHandler) Actions(c *gin.Context) {
	id, ok := parsePathUUID(c)
	if !ok {
		return
	}

	items, err := h.queries.ListCorrectiveActionsByCase(c.Request.Context(), uuidToPgtype(id))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list case actions")
		return
	}

	response := make([]CorrectiveActionResponse, 0, len(items))
	for _, item := range items {
		response = append(response, correctiveActionResponse(item))
	}

	c.JSON(http.StatusOK, response)
}

func parsePathUUID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid case id")
		return uuid.Nil, false
	}

	return id, true
}

func caseResponse(item db.Case) CaseResponse {
	return CaseResponse{
		ID:               uuidString(item.ID),
		Status:           item.Status,
		RiskLevel:        textString(item.RiskLevel),
		SelectedActionID: optionalUUIDString(item.SelectedActionID),
		CreatedAt:        timePtr(item.CreatedAt),
		UpdatedAt:        timePtr(item.UpdatedAt),
	}
}

func decisionTraceResponse(item db.DecisionTrace) DecisionTraceResponse {
	return DecisionTraceResponse{
		ID:             uuidString(item.ID),
		CaseID:         uuidString(item.CaseID),
		Step:           item.Step,
		Timestamp:      timePtr(item.Timestamp),
		DataSnapshot:   rawJSON(item.DataSnapshot),
		ModelVersion:   textString(item.ModelVersion),
		Parameters:     rawJSON(item.Parameters),
		Risk:           rawJSON(item.Risk),
		ExpertDecision: rawJSON(item.ExpertDecision),
		FinalActionID:  optionalUUIDString(item.FinalActionID),
		CreatedAt:      timePtr(item.CreatedAt),
	}
}

func diagnosticResponse(item db.Diagnostic) DiagnosticResponse {
	return DiagnosticResponse{
		ID:         uuidString(item.ID),
		CaseID:     uuidString(item.CaseID),
		Hypothesis: item.Hypothesis,
		Confidence: numericFloatPtr(item.Confidence),
		Details:    rawJSON(item.Details),
		CreatedAt:  timePtr(item.CreatedAt),
	}
}

func scenarioResponse(item db.Scenario) ScenarioResponse {
	return ScenarioResponse{
		ID:          uuidString(item.ID),
		CaseID:      uuidString(item.CaseID),
		Name:        item.Name,
		Probability: numericFloatPtr(item.Probability),
		LossVector:  rawJSON(item.LossVector),
		CreatedAt:   timePtr(item.CreatedAt),
	}
}

func correctiveActionResponse(item db.CorrectiveAction) CorrectiveActionResponse {
	return CorrectiveActionResponse{
		ID:                  uuidString(item.ID),
		CaseID:              uuidString(item.CaseID),
		Code:                textString(item.Code),
		Description:         item.Description,
		ActionType:          item.ActionType,
		ExpectedLoss:        numericFloatPtr(item.ExpectedLoss),
		VaRValue:            numericFloatPtr(item.VarValue),
		CVaRValue:           numericFloatPtr(item.CvarValue),
		ConstraintsViolated: rawJSON(item.ConstraintsViolated),
		IsSelected:          item.IsSelected,
		CreatedAt:           timePtr(item.CreatedAt),
	}
}

func uuidToPgtype(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{
		Bytes: id,
		Valid: true,
	}
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}

	return uuid.UUID(id.Bytes).String()
}

func optionalUUIDString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}

	return uuid.UUID(id.Bytes).String()
}

func textString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}

	return value.String
}

func timePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}

	return &value.Time
}

func rawJSON(data []byte) json.RawMessage {
	if len(data) == 0 {
		return json.RawMessage("{}")
	}

	return json.RawMessage(data)
}

func numericFloatPtr(value pgtype.Numeric) *float64 {
	if !value.Valid {
		return nil
	}

	floatValue, err := value.Float64Value()
	if err != nil || !floatValue.Valid {
		return nil
	}

	return &floatValue.Float64
}
