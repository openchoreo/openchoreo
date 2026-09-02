// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"fmt"
	"log/slog"

	apiaudit "github.com/openchoreo/openchoreo/internal/openchoreo-api/audit"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// ExecRoutePattern and WirelogsRoutePattern are the exact
// net/http.ServeMux registration patterns for these two routes — the single
// source main.go's topMux.Handle calls and this file's audit pattern map
// both use, so route registration and audit coverage can't drift apart.
const (
	ExecRoutePattern     = "/exec/"
	WirelogsRoutePattern = "GET /api/v1/namespaces/{namespace}/environments/{environment}/wirelogs"
)

// NewExecWirelogsAuditMiddleware returns the audit middleware for both
// routes, built from a hand-declared route map rather than BuildPatternMap:
// neither route has an operationId in a spec to cross-reference (see
// audit.NewMiddlewareForRoutes). Their Operations are looked up from
// apiaudit.GetOperations() by ID rather than redeclared here — see
// apiaudit's nonSpecOperationDefs — so they stay selectable via
// audit.policies (config/audit.go validates selectors against that same
// table) and appear in the coverage matrix, the same as every spec-derived
// operation. One shared instance is enough: its Handler looks up r.Pattern
// per request, and each route's pattern is distinct.
func NewExecWirelogsAuditMiddleware(logger *slog.Logger, emitter *audit.Emitter, enabled bool) (*audit.Middleware, error) {
	var execOp, wirelogsOp *audit.Operation
	for _, op := range apiaudit.GetOperations() {
		switch op.ID {
		case "Exec":
			o := op
			execOp = &o
		case "Wirelogs":
			o := op
			wirelogsOp = &o
		}
	}
	if execOp == nil {
		return nil, fmt.Errorf("audit: apiaudit.GetOperations() is missing the Exec operation")
	}
	if wirelogsOp == nil {
		return nil, fmt.Errorf("audit: apiaudit.GetOperations() is missing the Wirelogs operation")
	}

	patternMap := map[string]*audit.Operation{
		ExecRoutePattern:     execOp,
		WirelogsRoutePattern: wirelogsOp,
	}
	return audit.NewMiddlewareForRoutes(logger, patternMap, emitter, enabled), nil
}
