// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/service"
)

// CreateAlertRule handles POST /api/v1alpha1/alerts/sources/{sourceType}/rules
func (h *Handler) CreateAlertRule(w http.ResponseWriter, r *http.Request) {
	sourceType := r.PathValue("sourceType")

	if err := validateSourceType(sourceType); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "INVALID_SOURCE_TYPE", err.Error())
		return
	}

	var req gen.AlertRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "INVALID_REQUEST_BODY", "invalid request body: "+err.Error())
		return
	}

	if err := validateAlertRuleRequest(req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if req.Source == nil || req.Source.Type == nil || string(*req.Source.Type) != sourceType {
		bodyType := ""
		if req.Source != nil && req.Source.Type != nil {
			bodyType = string(*req.Source.Type)
		}
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "SOURCE_TYPE_MISMATCH",
			fmt.Sprintf("path sourceType %q does not match body source.type %q", sourceType, bodyType))
		return
	}

	resp, err := h.alertService.CreateAlertRule(r.Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrAlertRuleAlreadyExists) {
			h.writeErrorResponse(w, http.StatusConflict, gen.Conflict, "ALREADY_EXISTS", err.Error())
			return
		}
		h.logger.Error("Failed to create alert rule", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "CREATE_FAILED", "failed to create alert rule: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusCreated, resp)
}

// GetAlertRule handles GET /api/v1alpha1/alerts/sources/{sourceType}/rules/{ruleName}
func (h *Handler) GetAlertRule(w http.ResponseWriter, r *http.Request) {
	sourceType := r.PathValue("sourceType")
	ruleName := r.PathValue("ruleName")

	if err := validateSourceType(sourceType); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "INVALID_SOURCE_TYPE", err.Error())
		return
	}

	if ruleName == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "INVALID_RULE_NAME", "ruleName path parameter is required")
		return
	}

	resp, err := h.alertService.GetAlertRule(r.Context(), ruleName, sourceType)
	if err != nil {
		if errors.Is(err, service.ErrAlertRuleNotFound) {
			h.writeErrorResponse(w, http.StatusNotFound, gen.NotFound, "NOT_FOUND", err.Error())
			return
		}
		h.logger.Error("Failed to get alert rule", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "GET_FAILED", "failed to get alert rule: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// UpdateAlertRule handles PUT /api/v1alpha1/alerts/sources/{sourceType}/rules/{ruleName}
func (h *Handler) UpdateAlertRule(w http.ResponseWriter, r *http.Request) {
	sourceType := r.PathValue("sourceType")
	ruleName := r.PathValue("ruleName")

	if err := validateSourceType(sourceType); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "INVALID_SOURCE_TYPE", err.Error())
		return
	}

	if ruleName == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "INVALID_RULE_NAME", "ruleName path parameter is required")
		return
	}

	var req gen.AlertRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "INVALID_REQUEST_BODY", "invalid request body: "+err.Error())
		return
	}

	if err := validateAlertRuleRequest(req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if req.Source == nil || req.Source.Type == nil || string(*req.Source.Type) != sourceType {
		bodyType := ""
		if req.Source != nil && req.Source.Type != nil {
			bodyType = string(*req.Source.Type)
		}
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "SOURCE_TYPE_MISMATCH",
			fmt.Sprintf("path sourceType %q does not match body source.type %q", sourceType, bodyType))
		return
	}

	resp, err := h.alertService.UpdateAlertRule(r.Context(), ruleName, req)
	if err != nil {
		if errors.Is(err, service.ErrAlertRuleNotFound) {
			h.writeErrorResponse(w, http.StatusNotFound, gen.NotFound, "NOT_FOUND", err.Error())
			return
		}
		h.logger.Error("Failed to update alert rule", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "UPDATE_FAILED", "failed to update alert rule: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// DeleteAlertRule handles DELETE /api/v1alpha1/alerts/sources/{sourceType}/rules/{ruleName}
func (h *Handler) DeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	sourceType := r.PathValue("sourceType")
	ruleName := r.PathValue("ruleName")

	if err := validateSourceType(sourceType); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "INVALID_SOURCE_TYPE", err.Error())
		return
	}

	if ruleName == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "INVALID_RULE_NAME", "ruleName path parameter is required")
		return
	}

	resp, err := h.alertService.DeleteAlertRule(r.Context(), ruleName, sourceType)
	if err != nil {
		if errors.Is(err, service.ErrAlertRuleNotFound) {
			h.writeErrorResponse(w, http.StatusNotFound, gen.NotFound, "NOT_FOUND", err.Error())
			return
		}
		h.logger.Error("Failed to delete alert rule", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "DELETE_FAILED", "failed to delete alert rule: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// HandleAlertWebhook handles POST /api/v1alpha1/alerts/webhook
func (h *Handler) HandleAlertWebhook(w http.ResponseWriter, r *http.Request) {
	var req gen.AlertWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "INVALID_REQUEST_BODY", "invalid request body: "+err.Error())
		return
	}

	if req.RuleName == nil || *req.RuleName == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "MISSING_RULE_NAME", "ruleName is required")
		return
	}
	if req.RuleNamespace == nil || *req.RuleNamespace == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "MISSING_RULE_NAMESPACE", "ruleNamespace is required")
		return
	}

	resp, err := h.alertService.HandleAlertWebhook(r.Context(), req)
	if err != nil {
		h.logger.Error("Failed to handle alert webhook", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "WEBHOOK_FAILED", "failed to handle alert webhook: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, resp)
}
