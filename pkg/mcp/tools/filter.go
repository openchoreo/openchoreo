// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

const (
	// methodCallTool is the MCP method name for tool invocation.
	methodCallTool = "tools/call"
	// methodListTools is the MCP method name for listing tools.
	methodListTools = "tools/list"
)

// NewToolFilterMiddleware returns an MCP receiving middleware that filters
// tools/list and tools/call results along two independent axes:
//
//  1. Toolset narrowing — when the client requested a specific subset of
//     toolsets via ?toolsets= on the initialize request, tools/list returns
//     only tools whose registered toolsets intersect the requested set. This
//     filter is purely a tools/list visibility helper; tools/call is not
//     gated by it (clients that bypass tools/list can still call any
//     registered tool).
//
//  2. Authz filtering — when filterByAuthz is true (the default) and a PDP
//     is configured, tools/list hides tools the user lacks permission for and
//     tools/call rejects unauthorized calls. When filterByAuthz is false, or
//     pdp is nil, the MCP server is permissive at the protocol layer; the
//     service layer still enforces authz independently.
//
// The perms map is produced by Register(); it maps tool name to ToolPermission
// (carrying the required authz action). The toolToToolsets map is also
// produced by Register(); it maps each tool name to the set of toolsets it
// belongs to.
func NewToolFilterMiddleware(
	pdp authzcore.PDP,
	perms map[string]ToolPermission,
	toolToToolsets map[string]map[ToolsetType]bool,
) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			switch method {
			case methodListTools:
				return filterListTools(ctx, next, req, pdp, perms, toolToToolsets)
			case methodCallTool:
				return filterCallTool(ctx, next, method, req, pdp, perms)
			default:
				return next(ctx, method, req)
			}
		}
	}
}

// filterListTools calls the next handler, then narrows the returned tool list
// by the per-session toolset request and (when enabled) the user's authz
// capabilities.
func filterListTools(
	ctx context.Context,
	next mcp.MethodHandler,
	req mcp.Request,
	pdp authzcore.PDP,
	perms map[string]ToolPermission,
	toolToToolsets map[string]map[ToolsetType]bool,
) (mcp.Result, error) {
	result, err := next(ctx, methodListTools, req)
	if err != nil {
		return result, err
	}

	listResult, ok := result.(*mcp.ListToolsResult)
	if !ok || listResult == nil {
		// Unexpected type: pass through unchanged.
		return result, nil
	}

	requested, hasRequested := RequestedToolsetsFromContext(ctx)
	authzActive := authzFilteringActive(ctx, pdp)

	if !hasRequested && !authzActive {
		// Nothing to filter on — return the result as-is.
		return listResult, nil
	}

	var profile *authzcore.UserCapabilitiesResponse
	if authzActive {
		subjectCtx, _ := auth.GetSubjectContextFromContext(ctx)
		if subjectCtx == nil {
			// No authenticated user in context — return no tools.
			listResult.Tools = []*mcp.Tool{}
			return listResult, nil
		}
		profile, err = pdp.GetSubjectProfile(ctx, &authzcore.ProfileRequest{
			SubjectContext: authzcore.GetAuthzSubjectContext(subjectCtx),
		})
		if err != nil {
			// On PDP error, be safe: return no tools. The service layer will
			// independently deny any calls the user makes.
			listResult.Tools = []*mcp.Tool{}
			return listResult, nil
		}
	}

	filtered := listResult.Tools[:0:0]
	for _, tool := range listResult.Tools {
		if hasRequested && !toolInRequestedToolsets(tool.Name, toolToToolsets, requested) {
			continue
		}
		if authzActive && !isAllowed(tool.Name, perms, profile) {
			continue
		}
		filtered = append(filtered, tool)
	}
	listResult.Tools = filtered
	return listResult, nil
}

// filterCallTool checks whether the user is permitted to call the requested tool
// before forwarding to the next handler. Authz checks are skipped when the
// per-session filterByAuthz flag is false or no PDP is configured; the service
// layer enforces authz independently in those cases.
func filterCallTool(
	ctx context.Context,
	next mcp.MethodHandler,
	method string,
	req mcp.Request,
	pdp authzcore.PDP,
	perms map[string]ToolPermission,
) (mcp.Result, error) {
	if !authzFilteringActive(ctx, pdp) {
		return next(ctx, method, req)
	}

	toolName := callToolName(req)
	perm, hasPerm := perms[toolName]
	if !hasPerm {
		// No permission entry — allow through (unknown tools pass, service layer will check).
		return next(ctx, method, req)
	}

	subjectCtx, _ := auth.GetSubjectContextFromContext(ctx)
	if subjectCtx == nil {
		return nil, fmt.Errorf("not authorized to call tool %q: no authenticated user", toolName)
	}

	profile, err := pdp.GetSubjectProfile(ctx, &authzcore.ProfileRequest{
		SubjectContext: authzcore.GetAuthzSubjectContext(subjectCtx),
		Scope:          callToolScope(req),
	})
	if err != nil {
		return nil, fmt.Errorf("not authorized to call tool %q: could not evaluate permissions", toolName)
	}

	if !hasActionCapability(perm.Action, profile) {
		return nil, fmt.Errorf("not authorized to call tool %q: missing permission %q", toolName, perm.Action)
	}

	return next(ctx, method, req)
}

// authzFilteringActive reports whether MCP-layer authz filtering should be
// applied for this request. It is active only when a PDP is configured and the
// per-session filterByAuthz flag has not been explicitly set to false.
func authzFilteringActive(ctx context.Context, pdp authzcore.PDP) bool {
	if pdp == nil {
		return false
	}
	if filter, set := FilterByAuthzFromContext(ctx); set && !filter {
		return false
	}
	return true
}

// toolInRequestedToolsets returns true if the named tool belongs to at least
// one of the toolsets the client requested. Tools without a toolset entry
// (an unexpected condition since Register always indexes registered tools)
// are returned by default to avoid hiding tools after a rollout.
func toolInRequestedToolsets(
	toolName string,
	toolToToolsets map[string]map[ToolsetType]bool,
	requested map[ToolsetType]bool,
) bool {
	owned, ok := toolToToolsets[toolName]
	if !ok || len(owned) == 0 {
		return true
	}
	for ts := range owned {
		if requested[ts] {
			return true
		}
	}
	return false
}

// isAllowed returns true if the user's profile grants at least one resource for the
// tool's required action. If the tool has no permission entry in perms, it is shown
// by default (safe-default: don't hide tools added after a deploy).
func isAllowed(toolName string, perms map[string]ToolPermission, profile *authzcore.UserCapabilitiesResponse) bool {
	perm, ok := perms[toolName]
	if !ok {
		return true
	}
	return hasActionCapability(perm.Action, profile)
}

// hasActionCapability returns true if the profile has at least one allowed resource
// for the given action.
func hasActionCapability(action string, profile *authzcore.UserCapabilitiesResponse) bool {
	if profile == nil {
		return false
	}
	cap, ok := profile.Capabilities[action]
	if !ok || cap == nil {
		return false
	}
	return len(cap.Allowed) > 0
}

// callToolName extracts the tool name from a tools/call Request.
// The Params field is of type *mcp.CallToolParamsRaw (server-side) which embeds Name.
func callToolName(req mcp.Request) string {
	if req == nil {
		return ""
	}
	params := req.GetParams()
	if params == nil {
		return ""
	}
	if p, ok := params.(*mcp.CallToolParamsRaw); ok && p != nil {
		return p.Name
	}
	// Fallback: try CallToolParams (client-side, shouldn't happen on server).
	if p, ok := params.(*mcp.CallToolParams); ok && p != nil {
		return p.Name
	}
	return ""
}

// callToolScope derives the resource hierarchy scope from the tools/call arguments.
// It looks for the conventional namespace_name, project_name and component_name
// fields used by MCP tools in this package. Missing fields remain empty, which
// the PDP interprets as a broader scope.
func callToolScope(req mcp.Request) authzcore.ResourceHierarchy {
	if req == nil {
		return authzcore.ResourceHierarchy{}
	}
	params := req.GetParams()
	if params == nil {
		return authzcore.ResourceHierarchy{}
	}
	p, ok := params.(*mcp.CallToolParamsRaw)
	if !ok || p == nil || len(p.Arguments) == 0 {
		return authzcore.ResourceHierarchy{}
	}
	var args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
	}
	if err := json.Unmarshal(p.Arguments, &args); err != nil {
		return authzcore.ResourceHierarchy{}
	}
	return authzcore.ResourceHierarchy{
		Namespace: args.NamespaceName,
		Project:   args.ProjectName,
		Component: args.ComponentName,
	}
}
