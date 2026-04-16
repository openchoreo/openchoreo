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

// NewToolFilterMiddleware returns an MCP receiving middleware that filters tools/list
// results and rejects tools/call requests based on the authenticated user's permissions.
//
// Design: this is a UX improvement layer, not the security boundary. The actual
// security boundary is enforced by the service layer (NewServiceWithAuthz wrappers).
// If pdp is nil, all tools are returned unfiltered (graceful degradation / authz disabled).
//
// The perms map is produced by ToolPermissions(). It maps tool name to ToolPermission,
// which carries the required authz action for that tool.
func NewToolFilterMiddleware(pdp authzcore.PDP, perms map[string]ToolPermission) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			// If no PDP configured, pass through unfiltered.
			if pdp == nil {
				return next(ctx, method, req)
			}

			switch method {
			case methodListTools:
				return filterListTools(ctx, next, req, pdp, perms)
			case methodCallTool:
				return filterCallTool(ctx, next, method, req, pdp, perms)
			default:
				return next(ctx, method, req)
			}
		}
	}
}

// filterListTools calls the next handler, then removes tools that the user is not
// permitted to use. Unknown tools (no permission entry) are shown by default to
// avoid accidentally hiding tools during a rollout.
func filterListTools(
	ctx context.Context,
	next mcp.MethodHandler,
	req mcp.Request,
	pdp authzcore.PDP,
	perms map[string]ToolPermission,
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

	subjectCtx, _ := auth.GetSubjectContextFromContext(ctx)
	if subjectCtx == nil {
		// No authenticated user in context — return no tools.
		listResult.Tools = []*mcp.Tool{}
		return listResult, nil
	}

	profile, err := pdp.GetSubjectProfile(ctx, &authzcore.ProfileRequest{
		SubjectContext: authzcore.GetAuthzSubjectContext(subjectCtx),
	})
	if err != nil {
		// On PDP error, be safe: return no tools. The service layer will
		// independently deny any calls the user makes.
		listResult.Tools = []*mcp.Tool{}
		return listResult, nil
	}

	filtered := listResult.Tools[:0:0]
	for _, tool := range listResult.Tools {
		if isAllowed(tool.Name, perms, profile) {
			filtered = append(filtered, tool)
		}
	}
	listResult.Tools = filtered
	return listResult, nil
}

// filterCallTool checks whether the user is permitted to call the requested tool
// before forwarding to the next handler.
func filterCallTool(
	ctx context.Context,
	next mcp.MethodHandler,
	method string,
	req mcp.Request,
	pdp authzcore.PDP,
	perms map[string]ToolPermission,
) (mcp.Result, error) {
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
