// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"errors"

	"github.com/openchoreo/openchoreo/internal/observer/api/handlers"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// AuditLogsQueryArgs is the decoded argument set of the query_audit_logs tool.
//
// Record-derived filters are flattened from the API's nested groups onto their
// full path with the dot replaced, so actor.id becomes actor_id.
//
// Keep ResourceNames an array. mcpaudit reads a "resource_name" argument into
// the audit record's hierarchy expecting the scalar name of one target
// resource; making this one scalar would stamp a filter value onto every record
// the tool emits.
type AuditLogsQueryArgs struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`

	ActorIDs          []string `json:"actor_id"`
	ActorTypes        []string `json:"actor_type"`
	ActorIssuers      []string `json:"actor_issuer"`
	ActorSessionIDs   []string `json:"actor_session_id"`
	ActorEntitlements []string `json:"actor_entitlements"`

	ResourceTypes        []string `json:"resource_type"`
	ResourceNamespaces   []string `json:"resource_namespace"`
	ResourceEnvironments []string `json:"resource_environment"`
	ResourceProjects     []string `json:"resource_project"`
	ResourceComponents   []string `json:"resource_component"`
	ResourceResources    []string `json:"resource_resource"`
	ResourceNames        []string `json:"resource_name"`

	Actions      []string `json:"action"`
	Categories   []string `json:"category"`
	Results      []string `json:"result"`
	Producers    []string `json:"producer"`
	Surfaces     []string `json:"surface"`
	OperationIDs []string `json:"operation_id"`
	RequestIDs   []string `json:"request_id"`
	EventIDs     []string `json:"event_id"`
	SourceIPs    []string `json:"source_ip"`
	UserAgents   []string `json:"user_agent"`

	SearchPhrase string `json:"search_phrase"`
	Limit        int    `json:"limit"`
	SortOrder    string `json:"sort_order"`

	IncludeTimeline  bool   `json:"include_timeline"`
	TimelineInterval string `json:"timeline_interval"`
}

// QueryAuditLogs queries the audit trail.
func (h *MCPHandler) QueryAuditLogs(ctx context.Context, args AuditLogsQueryArgs) (any, error) {
	req := &types.AuditLogsQueryRequest{
		StartTime: args.StartTime,
		EndTime:   args.EndTime,
		Actor: types.AuditLogsActorFilter{
			IDs:          args.ActorIDs,
			Types:        args.ActorTypes,
			Issuers:      args.ActorIssuers,
			SessionIDs:   args.ActorSessionIDs,
			Entitlements: args.ActorEntitlements,
		},
		Resource: types.AuditLogsResourceFilter{
			Types:        args.ResourceTypes,
			Namespaces:   args.ResourceNamespaces,
			Environments: args.ResourceEnvironments,
			Projects:     args.ResourceProjects,
			Components:   args.ResourceComponents,
			Resources:    args.ResourceResources,
			Names:        args.ResourceNames,
		},
		Actions:          args.Actions,
		Categories:       args.Categories,
		Results:          args.Results,
		Producers:        args.Producers,
		Surfaces:         args.Surfaces,
		OperationIDs:     args.OperationIDs,
		RequestIDs:       args.RequestIDs,
		EventIDs:         args.EventIDs,
		SourceIPs:        args.SourceIPs,
		UserAgents:       args.UserAgents,
		SearchPhrase:     args.SearchPhrase,
		Limit:            args.Limit,
		SortOrder:        args.SortOrder,
		IncludeTimeline:  args.IncludeTimeline,
		TimelineInterval: args.TimelineInterval,
	}
	// The REST endpoint's own validator, so a query it would reject with a 400 is
	// not accepted here instead. Also applies the limit and sort order defaults.
	if err := handlers.ValidateAuditLogsQueryRequest(req); err != nil {
		return nil, err
	}

	resp, err := h.auditLogsService.QueryAuditLogs(ctx, req)
	if err != nil {
		recordAuthzResult(ctx, err)
		return nil, err
	}
	return resp, nil
}

// recordAuthzResult tells the audit middleware that err was an authorization
// outcome rather than a plain failure.
//
// Observer enforces authz in the service decorators, inside the tool handler,
// and the MCP SDK folds a handler-returned error into CallToolResult.IsError
// before the middleware can read its identity. Without this every refused read
// of the trail is recorded as "failure", invisible to a result=denied query.
func recordAuthzResult(ctx context.Context, err error) {
	switch {
	case errors.Is(err, observerAuthz.ErrAuthzForbidden):
		audit.SetResult(ctx, audit.ResultDenied)
	case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
		audit.SetResult(ctx, audit.ResultUnauthenticated)
	}
}
