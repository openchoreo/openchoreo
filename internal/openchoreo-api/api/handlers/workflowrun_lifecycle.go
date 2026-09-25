// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	svcerrors "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	workflowrunsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// ResumeWorkflowRun resumes a workflow run paused at a suspend step.
func (h *Handler) ResumeWorkflowRun(
	ctx context.Context,
	request gen.ResumeWorkflowRunRequestObject,
) (gen.ResumeWorkflowRunResponseObject, error) {
	h.logger.Info("ResumeWorkflowRun called", "namespace", request.NamespaceName, "runName", request.RunName)

	wfRun, err := h.services.WorkflowRunService.ResumeWorkflowRun(ctx, request.NamespaceName, request.RunName)
	if err != nil {
		if errors.Is(err, workflowrunsvc.ErrWorkflowRunNotFound) {
			return gen.ResumeWorkflowRun404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRun")}, nil
		}
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.ResumeWorkflowRun403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, workflowrunsvc.ErrWorkflowRunNotSuspended) || errors.Is(err, workflowrunsvc.ErrWorkflowRunNotStarted) {
			return gen.ResumeWorkflowRun409JSONResponse{ConflictJSONResponse: conflict(err.Error())}, nil
		}
		h.logger.Error("Failed to resume workflow run", "error", err)
		return gen.ResumeWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{Namespace: request.NamespaceName, UID: string(wfRun.UID), Name: wfRun.Name})

	genRun, err := convert[openchoreov1alpha1.WorkflowRun, gen.WorkflowRun](*wfRun)
	if err != nil {
		h.logger.Error("Failed to convert workflow run", "error", err)
		return gen.ResumeWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.ResumeWorkflowRun200JSONResponse(genRun), nil
}

// StopWorkflowRun stops a running workflow run.
func (h *Handler) StopWorkflowRun(
	ctx context.Context,
	request gen.StopWorkflowRunRequestObject,
) (gen.StopWorkflowRunResponseObject, error) {
	h.logger.Info("StopWorkflowRun called", "namespace", request.NamespaceName, "runName", request.RunName)

	reason := ""
	if request.Body != nil && request.Body.Reason != nil {
		reason = *request.Body.Reason
	}

	wfRun, err := h.services.WorkflowRunService.StopWorkflowRun(ctx, request.NamespaceName, request.RunName, reason)
	if err != nil {
		if errors.Is(err, workflowrunsvc.ErrWorkflowRunNotFound) {
			return gen.StopWorkflowRun404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRun")}, nil
		}
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.StopWorkflowRun403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, workflowrunsvc.ErrWorkflowRunCompleted) || errors.Is(err, workflowrunsvc.ErrWorkflowRunNotStarted) {
			return gen.StopWorkflowRun409JSONResponse{ConflictJSONResponse: conflict(err.Error())}, nil
		}
		h.logger.Error("Failed to stop workflow run", "error", err)
		return gen.StopWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{Namespace: request.NamespaceName, UID: string(wfRun.UID), Name: wfRun.Name})

	genRun, err := convert[openchoreov1alpha1.WorkflowRun, gen.WorkflowRun](*wfRun)
	if err != nil {
		h.logger.Error("Failed to convert workflow run", "error", err)
		return gen.StopWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.StopWorkflowRun200JSONResponse(genRun), nil
}
