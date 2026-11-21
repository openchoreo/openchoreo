// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/bindingrevision"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
)

// ListReleaseBindingRevisions returns the revision history of a component's release bindings.
func (h *Handler) ListReleaseBindingRevisions(
	ctx context.Context,
	request gen.ListReleaseBindingRevisionsRequestObject,
) (gen.ListReleaseBindingRevisionsResponseObject, error) {
	h.logger.Debug("ListReleaseBindingRevisions called", "namespaceName", request.NamespaceName,
		"component", request.Params.Component)

	environment := ""
	if request.Params.Environment != nil {
		environment = *request.Params.Environment
	}
	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, nil)

	binding := ""
	if request.Params.Binding != nil {
		binding = *request.Params.Binding
	}

	result, err := h.services.ReleaseBindingService.ListReleaseBindingRevisions(ctx, request.NamespaceName,
		request.Params.Component, environment, binding, opts)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListReleaseBindingRevisions403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, releasebindingsvc.ErrComponentNotFound) {
			return gen.ListReleaseBindingRevisions404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		}
		if validationErr, ok := errors.AsType[*services.ValidationError](err); ok {
			return gen.ListReleaseBindingRevisions400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list release binding revisions", "error", err)
		return gen.ListReleaseBindingRevisions500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ReleaseBindingRevision, gen.ReleaseBindingRevision](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert release binding revisions", "error", err)
		return gen.ListReleaseBindingRevisions500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.ListReleaseBindingRevisions200JSONResponse{Items: items, Pagination: ToPagination(result)}, nil
}

// GetReleaseBindingRevision returns one release binding revision.
func (h *Handler) GetReleaseBindingRevision(
	ctx context.Context,
	request gen.GetReleaseBindingRevisionRequestObject,
) (gen.GetReleaseBindingRevisionResponseObject, error) {
	h.logger.Debug("GetReleaseBindingRevision called", "namespaceName", request.NamespaceName,
		"revision", request.ReleaseBindingRevisionName)

	rev, err := h.services.ReleaseBindingService.GetReleaseBindingRevision(ctx, request.NamespaceName,
		request.ReleaseBindingRevisionName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetReleaseBindingRevision403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, bindingrevision.ErrRevisionNotFound) {
			return gen.GetReleaseBindingRevision404JSONResponse{NotFoundJSONResponse: notFound("Release binding revision")}, nil
		}
		h.logger.Error("Failed to get release binding revision", "error", err)
		return gen.GetReleaseBindingRevision500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRev, err := convert[openchoreov1alpha1.ReleaseBindingRevision, gen.ReleaseBindingRevision](*rev)
	if err != nil {
		h.logger.Error("Failed to convert release binding revision", "error", err)
		return gen.GetReleaseBindingRevision500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.GetReleaseBindingRevision200JSONResponse(genRev), nil
}

// RollbackReleaseBinding restores the binding spec recorded by an earlier revision.
func (h *Handler) RollbackReleaseBinding(
	ctx context.Context,
	request gen.RollbackReleaseBindingRequestObject,
) (gen.RollbackReleaseBindingResponseObject, error) {
	h.logger.Info("RollbackReleaseBinding called", "namespaceName", request.NamespaceName,
		"releaseBindingName", request.ReleaseBindingName)

	req, invalid := toRollbackRequest(request.Body)
	if invalid != "" {
		return gen.RollbackReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest(invalid)}, nil
	}
	setRollbackAuditResource(ctx, request.NamespaceName, request.ReleaseBindingName, req)

	result, err := h.services.ReleaseBindingService.RollbackReleaseBinding(ctx, request.NamespaceName,
		request.ReleaseBindingName, req)
	if err != nil {
		outcome, body, msg := classifyRollbackError(err, releasebindingsvc.ErrReleaseBindingNotFound)
		switch outcome {
		case rollbackForbidden:
			return gen.RollbackReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		case rollbackNotFound:
			return gen.RollbackReleaseBinding404JSONResponse{RollbackNotFoundJSONResponse: gen.RollbackNotFoundJSONResponse(body)}, nil
		case rollbackConflict:
			return gen.RollbackReleaseBinding409JSONResponse{RollbackConflictJSONResponse: gen.RollbackConflictJSONResponse(body)}, nil
		case rollbackBadRequest:
			return gen.RollbackReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest(msg)}, nil
		default:
			h.logger.Error("Failed to roll back release binding", "error", err)
			return gen.RollbackReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
		}
	}

	return gen.RollbackReleaseBinding200JSONResponse(toRollbackResponse(result)), nil
}

// DeleteReleaseBindingRevision deletes one inactive revision from a binding's history.
func (h *Handler) DeleteReleaseBindingRevision(
	ctx context.Context,
	request gen.DeleteReleaseBindingRevisionRequestObject,
) (gen.DeleteReleaseBindingRevisionResponseObject, error) {
	h.logger.Info("DeleteReleaseBindingRevision called", "namespaceName", request.NamespaceName,
		"revision", request.ReleaseBindingRevisionName)

	req := bindingrevision.DeleteRequest{}
	if request.Params.Binding != nil {
		req.Binding = *request.Params.Binding
	}
	if request.Params.DryRun != nil {
		req.DryRun = *request.Params.DryRun
	}
	setDryRunAuditResource(ctx, request.NamespaceName, request.ReleaseBindingRevisionName, req.DryRun)

	result, err := h.services.ReleaseBindingService.DeleteReleaseBindingRevision(ctx, request.NamespaceName,
		request.ReleaseBindingRevisionName, req)
	if err != nil {
		outcome, body := classifyDeleteRevisionError(err, releasebindingsvc.ErrReleaseBindingNotFound)
		switch outcome {
		case rollbackForbidden:
			return gen.DeleteReleaseBindingRevision403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		case rollbackNotFound:
			return gen.DeleteReleaseBindingRevision404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(body)}, nil
		case rollbackConflict:
			return gen.DeleteReleaseBindingRevision409JSONResponse{RevisionDeleteConflictJSONResponse: gen.RevisionDeleteConflictJSONResponse(body)}, nil
		default:
			h.logger.Error("Failed to delete release binding revision", "error", err)
			return gen.DeleteReleaseBindingRevision500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
		}
	}

	return gen.DeleteReleaseBindingRevision200JSONResponse(toDeleteRevisionResponse(result)), nil
}
