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
	resourcereleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcereleasebinding"
)

// ListResourceReleaseBindingRevisions returns the revision history of a resource's release bindings.
func (h *Handler) ListResourceReleaseBindingRevisions(
	ctx context.Context,
	request gen.ListResourceReleaseBindingRevisionsRequestObject,
) (gen.ListResourceReleaseBindingRevisionsResponseObject, error) {
	h.logger.Debug("ListResourceReleaseBindingRevisions called", "namespaceName", request.NamespaceName,
		"resource", request.Params.Resource)

	environment := ""
	if request.Params.Environment != nil {
		environment = *request.Params.Environment
	}
	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, nil)

	binding := ""
	if request.Params.Binding != nil {
		binding = *request.Params.Binding
	}

	result, err := h.services.ResourceReleaseBindingService.ListResourceReleaseBindingRevisions(ctx, request.NamespaceName,
		request.Params.Resource, environment, binding, opts)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListResourceReleaseBindingRevisions403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, resourcereleasebindingsvc.ErrResourceNotFound) {
			return gen.ListResourceReleaseBindingRevisions404JSONResponse{NotFoundJSONResponse: notFound("Resource")}, nil
		}
		if validationErr, ok := errors.AsType[*services.ValidationError](err); ok {
			return gen.ListResourceReleaseBindingRevisions400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list resource release binding revisions", "error", err)
		return gen.ListResourceReleaseBindingRevisions500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ResourceReleaseBindingRevision, gen.ResourceReleaseBindingRevision](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert resource release binding revisions", "error", err)
		return gen.ListResourceReleaseBindingRevisions500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.ListResourceReleaseBindingRevisions200JSONResponse{Items: items, Pagination: ToPagination(result)}, nil
}

// GetResourceReleaseBindingRevision returns one resource release binding revision.
func (h *Handler) GetResourceReleaseBindingRevision(
	ctx context.Context,
	request gen.GetResourceReleaseBindingRevisionRequestObject,
) (gen.GetResourceReleaseBindingRevisionResponseObject, error) {
	h.logger.Debug("GetResourceReleaseBindingRevision called", "namespaceName", request.NamespaceName,
		"revision", request.ResourceReleaseBindingRevisionName)

	rev, err := h.services.ResourceReleaseBindingService.GetResourceReleaseBindingRevision(ctx, request.NamespaceName,
		request.ResourceReleaseBindingRevisionName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetResourceReleaseBindingRevision403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, bindingrevision.ErrRevisionNotFound) {
			return gen.GetResourceReleaseBindingRevision404JSONResponse{NotFoundJSONResponse: notFound("Resource release binding revision")}, nil
		}
		h.logger.Error("Failed to get resource release binding revision", "error", err)
		return gen.GetResourceReleaseBindingRevision500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRev, err := convert[openchoreov1alpha1.ResourceReleaseBindingRevision, gen.ResourceReleaseBindingRevision](*rev)
	if err != nil {
		h.logger.Error("Failed to convert resource release binding revision", "error", err)
		return gen.GetResourceReleaseBindingRevision500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.GetResourceReleaseBindingRevision200JSONResponse(genRev), nil
}

// RollbackResourceReleaseBinding restores the binding spec recorded by an earlier revision.
func (h *Handler) RollbackResourceReleaseBinding(
	ctx context.Context,
	request gen.RollbackResourceReleaseBindingRequestObject,
) (gen.RollbackResourceReleaseBindingResponseObject, error) {
	h.logger.Info("RollbackResourceReleaseBinding called", "namespaceName", request.NamespaceName,
		"resourceReleaseBindingName", request.ResourceReleaseBindingName)

	req, invalid := toRollbackRequest(request.Body)
	if invalid != "" {
		return gen.RollbackResourceReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest(invalid)}, nil
	}
	setRollbackAuditResource(ctx, request.NamespaceName, request.ResourceReleaseBindingName, req)

	result, err := h.services.ResourceReleaseBindingService.RollbackResourceReleaseBinding(ctx, request.NamespaceName,
		request.ResourceReleaseBindingName, req)
	if err != nil {
		outcome, body, msg := classifyRollbackError(err, resourcereleasebindingsvc.ErrResourceReleaseBindingNotFound)
		switch outcome {
		case rollbackForbidden:
			return gen.RollbackResourceReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		case rollbackNotFound:
			return gen.RollbackResourceReleaseBinding404JSONResponse{RollbackNotFoundJSONResponse: gen.RollbackNotFoundJSONResponse(body)}, nil
		case rollbackConflict:
			return gen.RollbackResourceReleaseBinding409JSONResponse{RollbackConflictJSONResponse: gen.RollbackConflictJSONResponse(body)}, nil
		case rollbackBadRequest:
			return gen.RollbackResourceReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest(msg)}, nil
		default:
			h.logger.Error("Failed to roll back resource release binding", "error", err)
			return gen.RollbackResourceReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
		}
	}

	return gen.RollbackResourceReleaseBinding200JSONResponse(toRollbackResponse(result)), nil
}

// DeleteResourceReleaseBindingRevision deletes one inactive revision from a binding's history.
func (h *Handler) DeleteResourceReleaseBindingRevision(
	ctx context.Context,
	request gen.DeleteResourceReleaseBindingRevisionRequestObject,
) (gen.DeleteResourceReleaseBindingRevisionResponseObject, error) {
	h.logger.Info("DeleteResourceReleaseBindingRevision called", "namespaceName", request.NamespaceName,
		"revision", request.ResourceReleaseBindingRevisionName)

	req := bindingrevision.DeleteRequest{}
	if request.Params.Binding != nil {
		req.Binding = *request.Params.Binding
	}
	if request.Params.DryRun != nil {
		req.DryRun = *request.Params.DryRun
	}
	setDryRunAuditResource(ctx, request.NamespaceName, request.ResourceReleaseBindingRevisionName, req.DryRun)

	result, err := h.services.ResourceReleaseBindingService.DeleteResourceReleaseBindingRevision(ctx, request.NamespaceName,
		request.ResourceReleaseBindingRevisionName, req)
	if err != nil {
		outcome, body := classifyDeleteRevisionError(err, resourcereleasebindingsvc.ErrResourceReleaseBindingNotFound)
		switch outcome {
		case rollbackForbidden:
			return gen.DeleteResourceReleaseBindingRevision403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		case rollbackNotFound:
			return gen.DeleteResourceReleaseBindingRevision404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(body)}, nil
		case rollbackConflict:
			return gen.DeleteResourceReleaseBindingRevision409JSONResponse{RevisionDeleteConflictJSONResponse: gen.RevisionDeleteConflictJSONResponse(body)}, nil
		default:
			h.logger.Error("Failed to delete resource release binding revision", "error", err)
			return gen.DeleteResourceReleaseBindingRevision500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
		}
	}

	return gen.DeleteResourceReleaseBindingRevision200JSONResponse(toDeleteRevisionResponse(result)), nil
}
