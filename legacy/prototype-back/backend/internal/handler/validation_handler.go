package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"sppr-prototype/backend/internal/service"
)

type ValidationHandler struct {
	service *service.ValidationService
}

func NewValidationHandler(service *service.ValidationService) *ValidationHandler {
	return &ValidationHandler{service: service}
}

type ExpertDecisionRequest struct {
	Outcome  string `json:"outcome" binding:"required"`
	Comment  string `json:"comment"`
	ExpertID string `json:"expert_id"`
}

type ValidationResponse struct {
	CaseID   string `json:"case_id"`
	Status   string `json:"status"`
	Outcome  string `json:"outcome,omitempty"`
	Comment  string `json:"comment,omitempty"`
	ExpertID string `json:"expert_id,omitempty"`
}

func (h *ValidationHandler) Submit(c *gin.Context) {
	caseID, ok := parsePathUUID(c)
	if !ok {
		return
	}

	updatedCase, err := h.service.SubmitForValidation(c.Request.Context(), caseID)
	if err != nil {
		respondError(c, http.StatusBadRequest, "failed to submit case for validation")
		return
	}

	c.JSON(http.StatusOK, ValidationResponse{
		CaseID: caseID.String(),
		Status: updatedCase.Status,
	})
}

func (h *ValidationHandler) Decide(c *gin.Context) {
	caseID, ok := parsePathUUID(c)
	if !ok {
		return
	}

	var req ExpertDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid expert decision request")
		return
	}

	result, err := h.service.RecordExpertDecision(c.Request.Context(), caseID, service.ExpertDecisionInput{
		Outcome:  req.Outcome,
		Comment:  req.Comment,
		ExpertID: req.ExpertID,
	})
	if err != nil {
		respondError(c, http.StatusBadRequest, "failed to record expert decision")
		return
	}

	c.JSON(http.StatusOK, ValidationResponse{
		CaseID:   result.CaseID.String(),
		Status:   result.Status,
		Outcome:  result.Outcome,
		Comment:  result.Comment,
		ExpertID: result.ExpertID,
	})
}

func (h *ValidationHandler) Execute(c *gin.Context) {
	caseID, ok := parsePathUUID(c)
	if !ok {
		return
	}

	updatedCase, err := h.service.Execute(c.Request.Context(), caseID)
	if err != nil {
		respondError(c, http.StatusBadRequest, "failed to execute case")
		return
	}

	c.JSON(http.StatusOK, ValidationResponse{
		CaseID: caseID.String(),
		Status: updatedCase.Status,
	})
}

func (h *ValidationHandler) Archive(c *gin.Context) {
	caseID, ok := parsePathUUID(c)
	if !ok {
		return
	}

	updatedCase, err := h.service.Archive(c.Request.Context(), caseID)
	if err != nil {
		respondError(c, http.StatusBadRequest, "failed to archive case")
		return
	}

	c.JSON(http.StatusOK, ValidationResponse{
		CaseID: caseID.String(),
		Status: updatedCase.Status,
	})
}
