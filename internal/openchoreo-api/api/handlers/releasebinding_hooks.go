// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// ListReleaseBindingHooks returns the deployment gate (hook status) of a release binding.
func (h *Handler) ListReleaseBindingHooks(
	ctx context.Context,
	request gen.ListReleaseBindingHooksRequestObject,
) (gen.ListReleaseBindingHooksResponseObject, error) {
	h.logger.Debug("ListReleaseBindingHooks called", "namespaceName", request.NamespaceName, "releaseBindingName", request.ReleaseBindingName)

	gate, err := h.services.ReleaseBindingService.ListHooks(ctx, request.NamespaceName, request.ReleaseBindingName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListReleaseBindingHooks403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, releasebindingsvc.ErrReleaseBindingNotFound) {
			return gen.ListReleaseBindingHooks404JSONResponse{NotFoundJSONResponse: notFound("ReleaseBinding")}, nil
		}
		h.logger.Error("Failed to list release binding hooks", "error", err)
		return gen.ListReleaseBindingHooks500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genGate, err := convert[openchoreov1alpha1.DeploymentGateStatus, gen.DeploymentGateStatus](*gate)
	if err != nil {
		h.logger.Error("Failed to convert deployment gate", "error", err)
		return gen.ListReleaseBindingHooks500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	// The CRD type omits empty lists from JSON; the API contract is "empty list, not absent".
	if genGate.PreDeploy == nil {
		genGate.PreDeploy = &[]gen.DeploymentHookStatus{}
	}
	if genGate.PostDeploy == nil {
		genGate.PostDeploy = &[]gen.DeploymentHookStatus{}
	}
	return gen.ListReleaseBindingHooks200JSONResponse(genGate), nil
}

// RetryReleaseBindingHook asks the controller to re-run one hook of the gate.
func (h *Handler) RetryReleaseBindingHook(
	ctx context.Context,
	request gen.RetryReleaseBindingHookRequestObject,
) (gen.RetryReleaseBindingHookResponseObject, error) {
	h.logger.Info("RetryReleaseBindingHook called", "namespaceName", request.NamespaceName,
		"releaseBindingName", request.ReleaseBindingName, "hookName", request.HookName)

	if request.Body == nil {
		return gen.RetryReleaseBindingHook400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	updated, err := h.services.ReleaseBindingService.RetryHook(ctx, request.NamespaceName, request.ReleaseBindingName,
		string(request.Body.Phase), request.HookName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.RetryReleaseBindingHook403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, releasebindingsvc.ErrReleaseBindingNotFound) {
			return gen.RetryReleaseBindingHook404JSONResponse{NotFoundJSONResponse: notFound("ReleaseBinding")}, nil
		}
		if errors.Is(err, releasebindingsvc.ErrHookNotFound) {
			return gen.RetryReleaseBindingHook404JSONResponse{NotFoundJSONResponse: notFound("Hook")}, nil
		}
		if validationErr, ok := errors.AsType[*services.ValidationError](err); ok {
			return gen.RetryReleaseBindingHook400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to retry release binding hook", "error", err)
		return gen.RetryReleaseBindingHook500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{Namespace: request.NamespaceName, UID: string(updated.UID), Name: updated.Name})

	genRB, err := convert[openchoreov1alpha1.ReleaseBinding, gen.ReleaseBinding](*updated)
	if err != nil {
		h.logger.Error("Failed to convert release binding", "error", err)
		return gen.RetryReleaseBindingHook500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.RetryReleaseBindingHook200JSONResponse(genRB), nil
}

// AcknowledgeReleaseBindingGate acknowledges an Alert post-deploy hook failure.
func (h *Handler) AcknowledgeReleaseBindingGate(
	ctx context.Context,
	request gen.AcknowledgeReleaseBindingGateRequestObject,
) (gen.AcknowledgeReleaseBindingGateResponseObject, error) {
	h.logger.Info("AcknowledgeReleaseBindingGate called", "namespaceName", request.NamespaceName, "releaseBindingName", request.ReleaseBindingName)

	if request.Body == nil {
		return gen.AcknowledgeReleaseBindingGate400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	updated, err := h.services.ReleaseBindingService.AcknowledgeGate(ctx, request.NamespaceName, request.ReleaseBindingName, request.Body.Key)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.AcknowledgeReleaseBindingGate403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, releasebindingsvc.ErrReleaseBindingNotFound) {
			return gen.AcknowledgeReleaseBindingGate404JSONResponse{NotFoundJSONResponse: notFound("ReleaseBinding")}, nil
		}
		if errors.Is(err, releasebindingsvc.ErrGateKeyMismatch) {
			return gen.AcknowledgeReleaseBindingGate400JSONResponse{BadRequestJSONResponse: badRequest(err.Error())}, nil
		}
		if validationErr, ok := errors.AsType[*services.ValidationError](err); ok {
			return gen.AcknowledgeReleaseBindingGate400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to acknowledge release binding gate", "error", err)
		return gen.AcknowledgeReleaseBindingGate500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{Namespace: request.NamespaceName, UID: string(updated.UID), Name: updated.Name})

	genRB, err := convert[openchoreov1alpha1.ReleaseBinding, gen.ReleaseBinding](*updated)
	if err != nil {
		h.logger.Error("Failed to convert release binding", "error", err)
		return gen.AcknowledgeReleaseBindingGate500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.AcknowledgeReleaseBindingGate200JSONResponse(genRB), nil
}
