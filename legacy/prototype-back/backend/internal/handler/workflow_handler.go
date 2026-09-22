package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"sppr-prototype/backend/internal/service"
)

type WorkflowHandler struct {
	service *service.WorkflowService
}

func NewWorkflowHandler(service *service.WorkflowService) *WorkflowHandler {
	return &WorkflowHandler{service: service}
}

type ProcessWorkflowRequest struct {
	EventID       string  `json:"event_id" binding:"required"`
	Product       string  `json:"product" binding:"required"`
	PlanDemand    float64 `json:"plan_demand" binding:"gt=0"`
	FactDemand    float64 `json:"fact_demand"`
	CapacityLoad  float64 `json:"capacity_load"`
	ForecastError float64 `json:"forecast_error"`
	CampaignFlag  bool    `json:"campaign_flag"`
	Alpha         float64 `json:"alpha"`
}

type ProcessWorkflowResponse struct {
	CaseID             string  `json:"case_id"`
	FinalStatus        string  `json:"final_status"`
	TopCauseCode       string  `json:"top_cause_code"`
	SelectedActionCode string  `json:"selected_action_code"`
	ExpectedLoss       float64 `json:"expected_loss"`
	VaRValue           float64 `json:"var_value"`
	CVaRValue          float64 `json:"cvar_value"`
}

func (h *WorkflowHandler) Process(c *gin.Context) {
	var req ProcessWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid workflow request")
		return
	}
	if req.Alpha < 0 || req.Alpha >= 1 {
		respondError(c, http.StatusBadRequest, "alpha must be greater than or equal to 0 and less than 1")
		return
	}

	result, err := h.service.ProcessEvent(c.Request.Context(), service.WorkflowInput{
		EventID:       req.EventID,
		Product:       req.Product,
		PlanDemand:    req.PlanDemand,
		FactDemand:    req.FactDemand,
		CapacityLoad:  req.CapacityLoad,
		ForecastError: req.ForecastError,
		CampaignFlag:  req.CampaignFlag,
		Alpha:         req.Alpha,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to process workflow")
		return
	}

	c.JSON(http.StatusCreated, ProcessWorkflowResponse{
		CaseID:             result.CaseID.String(),
		FinalStatus:        result.FinalStatus,
		TopCauseCode:       result.TopCauseCode,
		SelectedActionCode: result.SelectedActionCode,
		ExpectedLoss:       result.ExpectedLoss,
		VaRValue:           result.VaRValue,
		CVaRValue:          result.CVaRValue,
	})
}
