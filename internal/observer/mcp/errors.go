// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"errors"
	"fmt"

	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
)

// invalidArgumentError marks an error as the caller's own mistake: a missing or
// malformed argument, described only in terms of what the caller sent. Its
// message is safe to return as it stands, and is what lets the caller correct
// the call, so toolError passes it through where it would otherwise replace it.
type invalidArgumentError struct {
	err error
}

func (e *invalidArgumentError) Error() string { return e.err.Error() }
func (e *invalidArgumentError) Unwrap() error { return e.err }

// invalidArgument marks err as caller-safe. It returns nil for a nil err, so a
// validator's result can be wrapped without checking it first.
func invalidArgument(err error) error {
	if err == nil {
		return nil
	}
	return &invalidArgumentError{err: err}
}

// Service errors whose message describes the caller's request rather than the
// observer, and so reaches the caller unchanged. The REST handlers return the
// same text in their 400 responses.
var callerSafeServiceErrors = []error{
	service.ErrMetricsInvalidRequest,
	service.ErrRuntimeTopologyInvalidRequest,
	service.ErrFinOpsInvalidRequest,
	service.ErrTracesInvalidRequest,
}

// Sentinels that report a capability the installation does not have. Only the
// sentinel's own text is returned, never whatever it was wrapped in.
var unsupportedErrors = []error{
	service.ErrEventsNotImplemented,
	service.ErrPlatformLogsNotSupported,
	service.ErrPlatformLogFilterValuesNotSupported,
	service.ErrAuditLogsNotSupported,
	service.ErrAuditLogFilterValuesNotSupported,
}

// toolError turns an error from a tool into the message the MCP caller sees.
//
// The SDK renders a handler's error to the caller verbatim, and the service layer
// wraps whatever went wrong underneath it: adapter response bodies, resolver
// paths, other subsystems' sentinels. The REST handlers map those errors to fixed
// messages; this is the same mapping for the MCP surface, so no tool can hand
// internal detail to a caller. The full error is logged here instead.
func (h *MCPHandler) toolError(tool string, err error) error {
	if err == nil {
		return nil
	}

	var invalid *invalidArgumentError
	if errors.As(err, &invalid) {
		return errors.New(err.Error())
	}
	for _, safe := range callerSafeServiceErrors {
		if errors.Is(err, safe) {
			return errors.New(err.Error())
		}
	}

	switch {
	case errors.Is(err, observerAuthz.ErrAuthzForbidden):
		return errors.New("access denied")
	case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
		return errors.New("unauthorized")
	case errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable),
		errors.Is(err, observerAuthz.ErrAuthzTimeout):
		h.logger.Error("MCP tool authorization unavailable", "tool", tool, "error", err)
		return errors.New("authorization service temporarily unavailable")
	case errors.Is(err, service.ErrScopeNotFound),
		errors.Is(err, service.ErrResourceNotFound):
		return errors.New("one or more resources in the search scope were not found")
	case errors.Is(err, service.ErrScopeResolutionFailed):
		h.logger.Error("MCP tool could not resolve its search scope", "tool", tool, "error", err)
		return errors.New("failed to resolve search scope")
	case errors.Is(err, service.ErrSpanNotFound):
		return errors.New("span not found")
	}

	for _, unsupported := range unsupportedErrors {
		if errors.Is(err, unsupported) {
			return errors.New(unsupported.Error())
		}
	}

	h.logger.Error("MCP tool call failed", "tool", tool, "error", err)
	return fmt.Errorf("%s failed due to an internal error", tool)
}
