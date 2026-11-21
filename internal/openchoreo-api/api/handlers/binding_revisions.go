// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/bindingrevision"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// rollbackOutcome classifies a rollback or revision deletion service error for
// both binding kinds.
type rollbackOutcome int

const (
	rollbackInternalError rollbackOutcome = iota
	rollbackBadRequest
	rollbackForbidden
	rollbackNotFound
	rollbackConflict
)

// classifyRollbackError maps a rollback service error to its HTTP outcome and,
// for 404 and 409, the error body. bindingNotFound is the service's binding
// not-found error.
func classifyRollbackError(err, bindingNotFound error) (rollbackOutcome, gen.ErrorResponse, string) {
	switch {
	case errors.Is(err, services.ErrForbidden):
		return rollbackForbidden, gen.ErrorResponse{}, ""
	case errors.Is(err, bindingNotFound):
		return rollbackNotFound, gen.ErrorResponse{Code: gen.NOTFOUND, Error: "Binding not found"}, ""
	case errors.Is(err, bindingrevision.ErrRevisionNotFound):
		return rollbackNotFound, gen.ErrorResponse{Code: gen.NOTFOUND, Error: err.Error()}, ""
	case errors.Is(err, bindingrevision.ErrReleaseDeleted):
		return rollbackConflict, gen.ErrorResponse{Code: gen.RELEASEDELETED, Error: err.Error()}, ""
	case errors.Is(err, bindingrevision.ErrConcurrentModification):
		return rollbackConflict, gen.ErrorResponse{Code: gen.CONCURRENTMODIFICATION,
			Error: err.Error() + "; retry the rollback"}, ""
	case errors.Is(err, bindingrevision.ErrNoPreviousRevision):
		return rollbackConflict, gen.ErrorResponse{Code: gen.NOPREVIOUSREVISION, Error: sentence(err.Error())}, ""
	case errors.Is(err, bindingrevision.ErrAutoDeployEnabled):
		return rollbackConflict, gen.ErrorResponse{Code: gen.AUTODEPLOYENABLED, Error: sentence(err.Error())}, ""
	case errors.Is(err, bindingrevision.ErrGitOpsManaged):
		return rollbackConflict, gen.ErrorResponse{Code: gen.GITOPSMANAGED, Error: sentence(err.Error())}, ""
	}
	if validationErr, ok := errors.AsType[*services.ValidationError](err); ok {
		return rollbackBadRequest, gen.ErrorResponse{}, validationErr.Msg
	}
	return rollbackInternalError, gen.ErrorResponse{}, ""
}

// toRollbackRequest converts a rollback request body. An omitted revision is 0,
// the revision that was active before the current one; an explicit revision
// must be at least 1. It returns a message for a 400 when the body is invalid.
func toRollbackRequest(body *gen.RollbackRequest) (bindingrevision.RollbackRequest, string) {
	if body == nil {
		return bindingrevision.RollbackRequest{}, "Request body is required"
	}
	var req bindingrevision.RollbackRequest
	if body.Revision != nil {
		if *body.Revision < 1 {
			return req, "revision must be at least 1, or omitted for the previously active revision"
		}
		req.Revision = *body.Revision
	}
	if body.DryRun != nil {
		req.DryRun = *body.DryRun
	}
	if body.Message != nil {
		msg, err := bindingrevision.NormalizeMessage(*body.Message)
		if err != nil {
			return req, err.Error()
		}
		req.Message = msg
	}
	return req, ""
}

// toRollbackResponse converts a service rollback result to the API response.
func toRollbackResponse(result *bindingrevision.RollbackResult) gen.RollbackResponse {
	resp := gen.RollbackResponse{
		RevisionName:   result.RevisionName,
		Revision:       result.Revision,
		TargetRevision: result.TargetRevision,
		RevertedFrom:   result.RevertedFrom,
		DryRun:         result.DryRun,
		Changes:        make([]gen.RollbackSpecChange, 0, len(result.Changes)),
		Warnings:       make([]gen.RollbackWarning, 0, len(result.Warnings)),
	}
	if result.RevisionDisplayName != "" {
		resp.RevisionDisplayName = &result.RevisionDisplayName
	}
	for _, c := range result.Changes {
		resp.Changes = append(resp.Changes, gen.RollbackSpecChange{Field: c.Field, Current: c.Current, Target: c.Target})
	}
	for _, w := range result.Warnings {
		resp.Warnings = append(resp.Warnings, gen.RollbackWarning{Code: gen.RollbackWarningCode(w.Code), Message: w.Message})
	}
	return resp
}

// setDryRunAuditResource names the object -- the binding of a rollback, the
// revision of a revision deletion -- in the audit record of a route that takes
// dryRun. Every call is audited, dry runs included, so the record carries
// dryRun to tell a preview, which changes nothing, from an applied change. It
// is set before the service runs so that a failed or denied call is marked too.
// setRollbackAuditResource is setDryRunAuditResource for a rollback, which also
// records the rollback's message when one is given.
func setRollbackAuditResource(ctx context.Context, namespace, binding string, req bindingrevision.RollbackRequest) {
	metadata := map[string]any{"dryRun": req.DryRun}
	if req.Message != "" {
		metadata["message"] = req.Message
	}
	audit.SetResource(ctx, &audit.Resource{Namespace: namespace, Name: binding, Metadata: metadata})
}

func setDryRunAuditResource(ctx context.Context, namespace, name string, dryRun bool) {
	audit.SetResource(ctx, &audit.Resource{
		Namespace: namespace,
		Name:      name,
		Metadata:  map[string]any{"dryRun": dryRun},
	})
}

// classifyDeleteRevisionError maps a revision deletion service error to its
// HTTP outcome and, for 404 and 409, the error body. bindingNotFound is the
// service's binding not-found error.
func classifyDeleteRevisionError(err, bindingNotFound error) (rollbackOutcome, gen.ErrorResponse) {
	switch {
	case errors.Is(err, services.ErrForbidden):
		return rollbackForbidden, gen.ErrorResponse{}
	case errors.Is(err, bindingNotFound):
		return rollbackNotFound, gen.ErrorResponse{Code: gen.NOTFOUND, Error: "Binding not found"}
	case errors.Is(err, bindingrevision.ErrRevisionNotFound):
		return rollbackNotFound, gen.ErrorResponse{Code: gen.NOTFOUND, Error: sentence(err.Error())}
	case errors.Is(err, bindingrevision.ErrRevisionProtected):
		return rollbackConflict, gen.ErrorResponse{Code: gen.REVISIONPROTECTED, Error: sentence(err.Error())}
	case errors.Is(err, bindingrevision.ErrConcurrentModification):
		return rollbackConflict, gen.ErrorResponse{Code: gen.CONCURRENTMODIFICATION, Error: sentence(err.Error())}
	}
	return rollbackInternalError, gen.ErrorResponse{}
}

// toDeleteRevisionResponse converts a service revision deletion result to the API response.
func toDeleteRevisionResponse(result *bindingrevision.DeleteResult) gen.DeleteBindingRevisionResponse {
	resp := gen.DeleteBindingRevisionResponse{
		RevisionName: result.RevisionName,
		Revision:     result.Revision,
		Binding:      result.Binding,
		Environment:  result.Environment,
		DryRun:       result.DryRun,
	}
	if result.RevisionDisplayName != "" {
		resp.RevisionDisplayName = &result.RevisionDisplayName
	}
	return resp
}

// sentence capitalizes an error message for an API response.
func sentence(msg string) string {
	if msg == "" {
		return msg
	}
	return strings.ToUpper(msg[:1]) + msg[1:]
}
