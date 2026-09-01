// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/api/internalgen"
	"github.com/openchoreo/openchoreo/internal/observer/service"
)

// Compile-time check that InternalHandler implements the generated internal
// strict server interface. This is what makes the internal spec authoritative:
// adding, removing or reshaping an operation in observer-internal-api.yaml
// becomes a build error here rather than a silent routing divergence.
var _ internalgen.StrictServerInterface = (*InternalHandler)(nil)

// errNilServiceResponse guards a service returning (nil, nil), which the
// AlertRuleService contract does not permit. The strict response types are value
// types, so dereferencing without this check would panic. Before the migration
// the same case wrote a 200 with a literal `null` body; a 500 is both safer and
// more honest, and no caller can reach it while the contract holds.
var errNilServiceResponse = errors.New("alert service returned no response and no error")

// CreateAlertRule handles POST /api/v1alpha1/alerts/sources/{sourceType}/rules
func (h *InternalHandler) CreateAlertRule(
	ctx context.Context,
	request internalgen.CreateAlertRuleRequestObject,
) (internalgen.CreateAlertRuleResponseObject, error) {
	if err := validateSourceType(request.SourceType); err != nil {
		return internalgen.CreateAlertRule400JSONResponse(
			errorPayload(gen.BadRequest, "INVALID_SOURCE_TYPE", err.Error())), nil
	}

	if request.Body == nil {
		return internalgen.CreateAlertRule400JSONResponse(
			errorPayload(gen.BadRequest, "INVALID_REQUEST_BODY", "request body is required")), nil
	}
	// The generated request-body type is an alias of internalgen.AlertRuleRequest,
	// so this is the same type the service layer already takes.
	req := *request.Body

	if err := validateAlertRuleRequest(req); err != nil {
		return internalgen.CreateAlertRule400JSONResponse(
			errorPayload(gen.BadRequest, "VALIDATION_ERROR", err.Error())), nil
	}

	if string(req.Source.Type) != request.SourceType {
		return internalgen.CreateAlertRule400JSONResponse(
			errorPayload(gen.BadRequest, "SOURCE_TYPE_MISMATCH",
				fmt.Sprintf("path sourceType %q does not match body source.type %q",
					request.SourceType, string(req.Source.Type)))), nil
	}

	resp, err := h.alertService.CreateAlertRule(ctx, req)
	if err != nil {
		if errors.Is(err, service.ErrAlertRuleAlreadyExists) {
			return internalgen.CreateAlertRule409JSONResponse(
				errorPayload(gen.Conflict, "ALREADY_EXISTS", err.Error())), nil
		}
		h.logger.Error("Failed to create alert rule", "error", err)
		return internalgen.CreateAlertRule500JSONResponse(
			errorPayload(gen.InternalServerError, "CREATE_FAILED",
				"failed to create alert rule: "+err.Error())), nil
	}

	if resp == nil {
		h.logger.Error("Failed to create alert rule", "error", errNilServiceResponse)
		return internalgen.CreateAlertRule500JSONResponse(
			errorPayload(gen.InternalServerError, "CREATE_FAILED",
				"failed to create alert rule: "+errNilServiceResponse.Error())), nil
	}

	return internalgen.CreateAlertRule201JSONResponse(*resp), nil
}

// GetAlertRule handles GET /api/v1alpha1/alerts/sources/{sourceType}/rules/{ruleName}
func (h *InternalHandler) GetAlertRule(
	ctx context.Context,
	request internalgen.GetAlertRuleRequestObject,
) (internalgen.GetAlertRuleResponseObject, error) {
	if err := validateSourceType(request.SourceType); err != nil {
		return internalgen.GetAlertRule400JSONResponse(
			errorPayload(gen.BadRequest, "INVALID_SOURCE_TYPE", err.Error())), nil
	}

	if request.RuleName == "" {
		return internalgen.GetAlertRule400JSONResponse(
			errorPayload(gen.BadRequest, "INVALID_RULE_NAME",
				"ruleName path parameter is required")), nil
	}

	resp, err := h.alertService.GetAlertRule(ctx, request.RuleName, request.SourceType)
	if err != nil {
		if errors.Is(err, service.ErrAlertRuleNotFound) {
			return internalgen.GetAlertRule404JSONResponse(
				errorPayload(gen.NotFound, "NOT_FOUND", err.Error())), nil
		}
		h.logger.Error("Failed to get alert rule", "error", err)
		return internalgen.GetAlertRule500JSONResponse(
			errorPayload(gen.InternalServerError, "GET_FAILED",
				"failed to get alert rule: "+err.Error())), nil
	}

	if resp == nil {
		h.logger.Error("Failed to get alert rule", "error", errNilServiceResponse)
		return internalgen.GetAlertRule500JSONResponse(
			errorPayload(gen.InternalServerError, "GET_FAILED",
				"failed to get alert rule: "+errNilServiceResponse.Error())), nil
	}

	return internalgen.GetAlertRule200JSONResponse(*resp), nil
}

// UpdateAlertRule handles PUT /api/v1alpha1/alerts/sources/{sourceType}/rules/{ruleName}
func (h *InternalHandler) UpdateAlertRule(
	ctx context.Context,
	request internalgen.UpdateAlertRuleRequestObject,
) (internalgen.UpdateAlertRuleResponseObject, error) {
	if err := validateSourceType(request.SourceType); err != nil {
		return internalgen.UpdateAlertRule400JSONResponse(
			errorPayload(gen.BadRequest, "INVALID_SOURCE_TYPE", err.Error())), nil
	}

	if request.RuleName == "" {
		return internalgen.UpdateAlertRule400JSONResponse(
			errorPayload(gen.BadRequest, "INVALID_RULE_NAME",
				"ruleName path parameter is required")), nil
	}

	if request.Body == nil {
		return internalgen.UpdateAlertRule400JSONResponse(
			errorPayload(gen.BadRequest, "INVALID_REQUEST_BODY", "request body is required")), nil
	}
	// The generated request-body type is an alias of internalgen.AlertRuleRequest,
	// so this is the same type the service layer already takes.
	req := *request.Body

	if err := validateAlertRuleRequest(req); err != nil {
		return internalgen.UpdateAlertRule400JSONResponse(
			errorPayload(gen.BadRequest, "VALIDATION_ERROR", err.Error())), nil
	}

	if string(req.Source.Type) != request.SourceType {
		return internalgen.UpdateAlertRule400JSONResponse(
			errorPayload(gen.BadRequest, "SOURCE_TYPE_MISMATCH",
				fmt.Sprintf("path sourceType %q does not match body source.type %q",
					request.SourceType, string(req.Source.Type)))), nil
	}

	resp, err := h.alertService.UpdateAlertRule(ctx, request.RuleName, req)
	if err != nil {
		if errors.Is(err, service.ErrAlertRuleNotFound) {
			return internalgen.UpdateAlertRule404JSONResponse(
				errorPayload(gen.NotFound, "NOT_FOUND", err.Error())), nil
		}
		h.logger.Error("Failed to update alert rule", "error", err)
		return internalgen.UpdateAlertRule500JSONResponse(
			errorPayload(gen.InternalServerError, "UPDATE_FAILED",
				"failed to update alert rule: "+err.Error())), nil
	}

	if resp == nil {
		h.logger.Error("Failed to update alert rule", "error", errNilServiceResponse)
		return internalgen.UpdateAlertRule500JSONResponse(
			errorPayload(gen.InternalServerError, "UPDATE_FAILED",
				"failed to update alert rule: "+errNilServiceResponse.Error())), nil
	}

	return internalgen.UpdateAlertRule200JSONResponse(*resp), nil
}

// DeleteAlertRule handles DELETE /api/v1alpha1/alerts/sources/{sourceType}/rules/{ruleName}
func (h *InternalHandler) DeleteAlertRule(
	ctx context.Context,
	request internalgen.DeleteAlertRuleRequestObject,
) (internalgen.DeleteAlertRuleResponseObject, error) {
	if err := validateSourceType(request.SourceType); err != nil {
		return internalgen.DeleteAlertRule400JSONResponse(
			errorPayload(gen.BadRequest, "INVALID_SOURCE_TYPE", err.Error())), nil
	}

	if request.RuleName == "" {
		return internalgen.DeleteAlertRule400JSONResponse(
			errorPayload(gen.BadRequest, "INVALID_RULE_NAME",
				"ruleName path parameter is required")), nil
	}

	resp, err := h.alertService.DeleteAlertRule(ctx, request.RuleName, request.SourceType)
	if err != nil {
		if errors.Is(err, service.ErrAlertRuleNotFound) {
			return internalgen.DeleteAlertRule404JSONResponse(
				errorPayload(gen.NotFound, "NOT_FOUND", err.Error())), nil
		}
		h.logger.Error("Failed to delete alert rule", "error", err)
		return internalgen.DeleteAlertRule500JSONResponse(
			errorPayload(gen.InternalServerError, "DELETE_FAILED",
				"failed to delete alert rule: "+err.Error())), nil
	}

	if resp == nil {
		h.logger.Error("Failed to delete alert rule", "error", errNilServiceResponse)
		return internalgen.DeleteAlertRule500JSONResponse(
			errorPayload(gen.InternalServerError, "DELETE_FAILED",
				"failed to delete alert rule: "+errNilServiceResponse.Error())), nil
	}

	return internalgen.DeleteAlertRule200JSONResponse(*resp), nil
}

// HandleAlertWebhook handles POST /api/v1alpha1/alerts/webhook
func (h *InternalHandler) HandleAlertWebhook(
	ctx context.Context,
	request internalgen.HandleAlertWebhookRequestObject,
) (internalgen.HandleAlertWebhookResponseObject, error) {
	if request.Body == nil {
		return internalgen.HandleAlertWebhook400JSONResponse(
			errorPayload(gen.BadRequest, "INVALID_REQUEST_BODY", "request body is required")), nil
	}
	// Alias of internalgen.AlertWebhookRequest — see CreateAlertRule.
	req := *request.Body

	if req.RuleName == nil || *req.RuleName == "" {
		return internalgen.HandleAlertWebhook400JSONResponse(
			errorPayload(gen.BadRequest, "MISSING_RULE_NAME", "ruleName is required")), nil
	}
	if req.RuleNamespace == nil || *req.RuleNamespace == "" {
		return internalgen.HandleAlertWebhook400JSONResponse(
			errorPayload(gen.BadRequest, "MISSING_RULE_NAMESPACE", "ruleNamespace is required")), nil
	}

	resp, err := h.alertService.HandleAlertWebhook(ctx, req)
	if err != nil {
		h.logger.Error("Failed to handle alert webhook", "error", err)
		return internalgen.HandleAlertWebhook500JSONResponse(
			errorPayload(gen.InternalServerError, "WEBHOOK_FAILED",
				"failed to handle alert webhook: "+err.Error())), nil
	}

	if resp == nil {
		h.logger.Error("Failed to handle alert webhook", "error", errNilServiceResponse)
		return internalgen.HandleAlertWebhook500JSONResponse(
			errorPayload(gen.InternalServerError, "WEBHOOK_FAILED",
				"failed to handle alert webhook: "+errNilServiceResponse.Error())), nil
	}

	return internalgen.HandleAlertWebhook200JSONResponse(*resp), nil
}
