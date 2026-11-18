// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	endpointsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpoint"
	endpointreleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointreleasebinding"
	rolloutsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointrollout"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// rolloutResult is the outcome of a rollout call, mapped to each operation's
// response types.
type rolloutResult struct {
	status     *gen.EndpointRolloutStatus
	badRequest string
	notFound   string
	conflict   string
	forbidden  bool
}

func (h *Handler) rolloutResult(ctx context.Context, namespaceName, endpointName string, status *rolloutsvc.Status, err error) rolloutResult {
	var validationErr *services.ValidationError
	var conflictErr *rolloutsvc.ConflictError
	switch {
	case err == nil:
	case errors.Is(err, services.ErrForbidden):
		return rolloutResult{forbidden: true}
	case errors.Is(err, endpointsvc.ErrEndpointNotFound):
		return rolloutResult{notFound: "Endpoint"}
	case errors.Is(err, rolloutsvc.ErrEnvironmentNotBound), errors.Is(err, endpointreleasebindingsvc.ErrEndpointReleaseBindingNotFound):
		return rolloutResult{notFound: "EndpointReleaseBinding"}
	case errors.Is(err, rolloutsvc.ErrNoRollout):
		return rolloutResult{conflict: err.Error()}
	case errors.As(err, &conflictErr):
		return rolloutResult{conflict: conflictErr.Msg}
	case errors.As(err, &validationErr):
		return rolloutResult{badRequest: validationErr.Msg}
	default:
		h.logger.Error("Endpoint rollout call failed", "endpoint", endpointName, "error", err)
		return rolloutResult{}
	}
	out, err := convert[rolloutsvc.Status, gen.EndpointRolloutStatus](*status)
	if err != nil {
		h.logger.Error("Failed to convert endpoint rollout", "error", err)
		return rolloutResult{}
	}
	audit.SetResource(ctx, &audit.Resource{Namespace: namespaceName, Name: endpointName})
	return rolloutResult{status: &out}
}

// GetEndpointRollout returns an Endpoint's rollout plan and where each environment stands.
func (h *Handler) GetEndpointRollout(ctx context.Context, request gen.GetEndpointRolloutRequestObject) (gen.GetEndpointRolloutResponseObject, error) {
	status, err := h.services.EndpointRolloutService.Get(ctx, request.NamespaceName, request.EndpointName)
	r := h.rolloutResult(ctx, request.NamespaceName, request.EndpointName, status, err)
	switch {
	case r.status != nil:
		return gen.GetEndpointRollout200JSONResponse(*r.status), nil
	case r.forbidden:
		return gen.GetEndpointRollout403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	case r.notFound != "":
		return gen.GetEndpointRollout404JSONResponse{NotFoundJSONResponse: notFound(r.notFound)}, nil
	}
	return gen.GetEndpointRollout500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
}

// StartEndpointRollout writes an Endpoint's rollout plan.
func (h *Handler) StartEndpointRollout(ctx context.Context, request gen.StartEndpointRolloutRequestObject) (gen.StartEndpointRolloutResponseObject, error) {
	if request.Body == nil {
		return gen.StartEndpointRollout400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}
	req, err := convert[gen.EndpointRolloutStartRequest, rolloutsvc.StartRequest](*request.Body)
	if err != nil {
		return gen.StartEndpointRollout400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	status, err := h.services.EndpointRolloutService.Start(ctx, request.NamespaceName, request.EndpointName, req)
	r := h.rolloutResult(ctx, request.NamespaceName, request.EndpointName, status, err)
	switch {
	case r.status != nil:
		return gen.StartEndpointRollout200JSONResponse(*r.status), nil
	case r.badRequest != "":
		return gen.StartEndpointRollout400JSONResponse{BadRequestJSONResponse: badRequest(r.badRequest)}, nil
	case r.forbidden:
		return gen.StartEndpointRollout403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	case r.notFound != "":
		return gen.StartEndpointRollout404JSONResponse{NotFoundJSONResponse: notFound(r.notFound)}, nil
	case r.conflict != "":
		return gen.StartEndpointRollout409JSONResponse{ConflictJSONResponse: conflict(r.conflict)}, nil
	}
	return gen.StartEndpointRollout500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
}

// StepEndpointRollout moves an environment to the next stage, or to the one given.
func (h *Handler) StepEndpointRollout(ctx context.Context, request gen.StepEndpointRolloutRequestObject) (gen.StepEndpointRolloutResponseObject, error) {
	if request.Body == nil {
		return gen.StepEndpointRollout400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}
	req, err := convert[gen.EndpointRolloutStepRequest, rolloutsvc.StepRequest](*request.Body)
	if err != nil {
		return gen.StepEndpointRollout400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	status, err := h.services.EndpointRolloutService.Step(ctx, request.NamespaceName, request.EndpointName, req)
	r := h.rolloutResult(ctx, request.NamespaceName, request.EndpointName, status, err)
	switch {
	case r.status != nil:
		return gen.StepEndpointRollout200JSONResponse(*r.status), nil
	case r.badRequest != "":
		return gen.StepEndpointRollout400JSONResponse{BadRequestJSONResponse: badRequest(r.badRequest)}, nil
	case r.forbidden:
		return gen.StepEndpointRollout403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	case r.notFound != "":
		return gen.StepEndpointRollout404JSONResponse{NotFoundJSONResponse: notFound(r.notFound)}, nil
	case r.conflict != "":
		return gen.StepEndpointRollout409JSONResponse{ConflictJSONResponse: conflict(r.conflict)}, nil
	}
	return gen.StepEndpointRollout500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
}

// AbortEndpointRollout returns an environment's weighted rollout traffic to the
// stable backend; requests a candidate's match selects still reach it.
func (h *Handler) AbortEndpointRollout(ctx context.Context, request gen.AbortEndpointRolloutRequestObject) (gen.AbortEndpointRolloutResponseObject, error) {
	if request.Body == nil {
		return gen.AbortEndpointRollout400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}
	req, err := convert[gen.EndpointRolloutAbortRequest, rolloutsvc.AbortRequest](*request.Body)
	if err != nil {
		return gen.AbortEndpointRollout400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	status, err := h.services.EndpointRolloutService.Abort(ctx, request.NamespaceName, request.EndpointName, req)
	r := h.rolloutResult(ctx, request.NamespaceName, request.EndpointName, status, err)
	switch {
	case r.status != nil:
		return gen.AbortEndpointRollout200JSONResponse(*r.status), nil
	case r.badRequest != "":
		return gen.AbortEndpointRollout400JSONResponse{BadRequestJSONResponse: badRequest(r.badRequest)}, nil
	case r.forbidden:
		return gen.AbortEndpointRollout403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	case r.notFound != "":
		return gen.AbortEndpointRollout404JSONResponse{NotFoundJSONResponse: notFound(r.notFound)}, nil
	case r.conflict != "":
		return gen.AbortEndpointRollout409JSONResponse{ConflictJSONResponse: conflict(r.conflict)}, nil
	}
	return gen.AbortEndpointRollout500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
}

// FinishEndpointRollout ends the rollout keeping one backend.
func (h *Handler) FinishEndpointRollout(ctx context.Context, request gen.FinishEndpointRolloutRequestObject) (gen.FinishEndpointRolloutResponseObject, error) {
	if request.Body == nil {
		return gen.FinishEndpointRollout400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}
	req, err := convert[gen.EndpointRolloutFinishRequest, rolloutsvc.FinishRequest](*request.Body)
	if err != nil {
		return gen.FinishEndpointRollout400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	status, err := h.services.EndpointRolloutService.Finish(ctx, request.NamespaceName, request.EndpointName, req)
	r := h.rolloutResult(ctx, request.NamespaceName, request.EndpointName, status, err)
	switch {
	case r.status != nil:
		return gen.FinishEndpointRollout200JSONResponse(*r.status), nil
	case r.badRequest != "":
		return gen.FinishEndpointRollout400JSONResponse{BadRequestJSONResponse: badRequest(r.badRequest)}, nil
	case r.forbidden:
		return gen.FinishEndpointRollout403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	case r.notFound != "":
		return gen.FinishEndpointRollout404JSONResponse{NotFoundJSONResponse: notFound(r.notFound)}, nil
	case r.conflict != "":
		return gen.FinishEndpointRollout409JSONResponse{ConflictJSONResponse: conflict(r.conflict)}, nil
	}
	return gen.FinishEndpointRollout500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
}
