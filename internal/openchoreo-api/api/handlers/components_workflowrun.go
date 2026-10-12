// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	svcerrors "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	componentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/component"
	workflowrunsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// CreateComponentWorkflowRun starts a run of the component's workflow, applying the release
// line's workflow parameter overrides when the component uses release lines.
func (h *Handler) CreateComponentWorkflowRun(
	ctx context.Context,
	request gen.CreateComponentWorkflowRunRequestObject,
) (gen.CreateComponentWorkflowRunResponseObject, error) {
	h.logger.Info("CreateComponentWorkflowRun called", "namespaceName", request.NamespaceName, "component", request.ComponentName)

	var releaseLine, commit string
	if request.Body != nil {
		if request.Body.ReleaseLine != nil {
			releaseLine = *request.Body.ReleaseLine
		}
		if request.Body.Commit != nil {
			commit = *request.Body.Commit
		}
	}

	triggered, err := h.services.WorkflowRunService.TriggerWorkflow(ctx, request.NamespaceName, "", request.ComponentName, commit, releaseLine)
	if err != nil {
		switch {
		case errors.Is(err, svcerrors.ErrForbidden):
			return gen.CreateComponentWorkflowRun403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		case errors.Is(err, componentsvc.ErrComponentNotFound):
			return gen.CreateComponentWorkflowRun404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		case errors.Is(err, workflowrunsvc.ErrReleaseLineNotFound):
			return gen.CreateComponentWorkflowRun404JSONResponse{NotFoundJSONResponse: notFound("ReleaseLine")}, nil
		case errors.Is(err, workflowrunsvc.ErrReleaseLineRequired),
			errors.Is(err, workflowrunsvc.ErrReleaseLinesNotEnabled),
			errors.Is(err, workflowrunsvc.ErrInvalidCommitSHA):
			return gen.CreateComponentWorkflowRun400JSONResponse{BadRequestJSONResponse: badRequest(err.Error())}, nil
		}
		h.logger.Error("Failed to trigger component workflow run", "error", err)
		return gen.CreateComponentWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	wfRun, err := h.services.WorkflowRunService.GetWorkflowRun(ctx, request.NamespaceName, triggered.Name)
	if err != nil {
		h.logger.Error("Failed to get triggered workflow run", "error", err, "workflowRun", triggered.Name)
		return gen.CreateComponentWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{Namespace: request.NamespaceName, UID: string(wfRun.UID), Name: wfRun.Name})

	genWfRun, err := convert[openchoreov1alpha1.WorkflowRun, gen.WorkflowRun](*wfRun)
	if err != nil {
		h.logger.Error("Failed to convert workflow run", "error", err)
		return gen.CreateComponentWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.CreateComponentWorkflowRun201JSONResponse(genWfRun), nil
}
