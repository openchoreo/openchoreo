// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// Revision history and rollback for ReleaseBindings and ResourceReleaseBindings.
// The descriptions spell out the rollback procedure an agent must follow, since
// a rollback changes what runs in an environment.

const revisionFieldsDescription = "Each revision records one distinct spec of the binding. Its object name is " +
	"{binding}-r{N} and it is shown to people as displayName {release}-r{N} (e.g. frontend-v7-r1); N counts " +
	"from 1 per binding: " +
	"revision number, cause (Deploy, Promote, Override, Undeploy, Redeploy), the release it bound, health " +
	"(Progressing, Healthy, Degraded, Failed, Unknown), active (the one the binding applies now) and " +
	"knownGood (stayed Healthy for the controller's known-good window; never reverts). "

const rollbackProcedure = "Procedure — follow it every time: " +
	"(1) Pick the target: prefer the newest revision with knownGood=true that is not the active one; " +
	"use the list tool to find it. Only pick a revision that is not known-good if the user asks for it explicitly. " +
	"(2) Call this tool with dry_run=true (the default). Nothing is changed. " +
	"(3) Report to the user: the target revision, the fields in 'changes', and every entry in 'warnings' " +
	"verbatim, then ask the user to confirm. " +
	"(4) Only after the user confirms, call again with the same revision and dry_run=false. " +
	"Never skip the dry run or the confirmation, and never set dry_run=false on the first call. " +
	"The rollback restores the target's spec. No revision is created: " +
	"the target revision becomes active again under its own name and number, the binding's " +
	"status.currentRevision names it, and its health restarts at Progressing. " +
	"Errors: not found (unknown binding or revision); conflict when the release the revision binds was " +
	"deleted, the binding changed concurrently (re-run the dry run before retrying), or auto-deploy is " +
	"enabled for the binding's component or release line and the binding is in its first environment " +
	"(auto-deploy would undo the rollback: tell the user to turn auto-deploy off for the component or " +
	"release line the error names first), or the binding is managed by GitOps " +
	"(rollback is not supported: tell the user to revert the binding's configuration in Git and let GitOps " +
	"reconcile the change)."

//nolint:dupl // paginated list handlers share similar structure
func (t *Toolsets) RegisterListReleaseBindingRevisions(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_release_binding_revisions"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List the release binding revision history of a component, ordered by environment and " +
			"then newest first. " + revisionFieldsDescription +
			"Use this to choose a rollback target for rollback_release_binding: prefer the newest knownGood " +
			"revision that is not active. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": stringProperty("Component whose release binding revisions to list."),
			"environment":    stringProperty("Optional: only revisions of the binding in this environment."),
			"binding_name": stringProperty("Optional: only revisions of this binding. A component can have one " +
				"binding per release line in an environment, each numbered from r1, so pass it to keep one " +
				"binding's history."),
		}), []string{"namespace_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ComponentName string `json:"component_name"`
		Environment   string `json:"environment"`
		BindingName   string `json:"binding_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.ListReleaseBindingRevisions(ctx, args.NamespaceName,
			args.ComponentName, args.Environment, args.BindingName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetReleaseBindingRevision(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_release_binding_revision"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get one release binding revision, including the full binding spec it recorded " +
			"(release, overrides, state). " + revisionFieldsDescription,
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name": stringProperty("Revision object name ({binding}-r{N}, e.g. frontend-development-r3). " +
				"Use list_release_binding_revisions to discover names."),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.GetReleaseBindingRevision(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterRollbackReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "rollback_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionRollbackReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: "Roll a ReleaseBinding back to an earlier revision of its spec. " + rollbackProcedure,
		InputSchema: createSchema(rollbackProperties("ReleaseBinding name, e.g. {component}-{environment}. "+
			"Use list_release_bindings to discover names."), []string{"namespace_name", "name", "revision"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args rollbackArgs) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.RollbackReleaseBinding(ctx, args.NamespaceName, args.Name,
			args.Revision, args.dryRun(), args.Message)
		return handleToolResult(result, err)
	})
}

//nolint:dupl // paginated list handlers share similar structure
func (t *Toolsets) RegisterListResourceReleaseBindingRevisions(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_resource_release_binding_revisions"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewResourceReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List the resource release binding revision history of a Resource, ordered by environment " +
			"and then newest first. " + revisionFieldsDescription +
			"Use this to choose a rollback target for rollback_resource_release_binding: prefer the newest " +
			"knownGood revision that is not active. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
			"resource_name":  stringProperty("Resource whose release binding revisions to list."),
			"environment":    stringProperty("Optional: only revisions of the binding in this environment."),
			"binding_name": stringProperty("Optional: only revisions of this binding. A resource can have one " +
				"binding per release line in an environment, each numbered from r1, so pass it to keep one " +
				"binding's history."),
		}), []string{"namespace_name", "resource_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ResourceName  string `json:"resource_name"`
		Environment   string `json:"environment"`
		BindingName   string `json:"binding_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.ListResourceReleaseBindingRevisions(ctx, args.NamespaceName,
			args.ResourceName, args.Environment, args.BindingName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetResourceReleaseBindingRevision(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_resource_release_binding_revision"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewResourceReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get one resource release binding revision, including the full binding spec it " +
			"recorded (pinned ResourceRelease, environment configs, retain policy). " + revisionFieldsDescription,
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name": stringProperty("Revision object name ({binding}-r{N}, e.g. frontend-development-r3). " +
				"Use list_resource_release_binding_revisions to discover names."),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.GetResourceReleaseBindingRevision(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterRollbackResourceReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "rollback_resource_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionRollbackResourceReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Roll a ResourceReleaseBinding back to an earlier revision of its spec " +
			"(the pinned ResourceRelease and per-environment configs). " + rollbackProcedure,
		InputSchema: createSchema(rollbackProperties("ResourceReleaseBinding name. "+
			"Use list_resource_release_bindings to discover names."), []string{"namespace_name", "name", "revision"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args rollbackArgs) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.RollbackResourceReleaseBinding(ctx, args.NamespaceName, args.Name,
			args.Revision, args.dryRun(), args.Message)
		return handleToolResult(result, err)
	})
}

// rollbackArgs are the inputs of both rollback tools.
type rollbackArgs struct {
	NamespaceName string `json:"namespace_name"`
	Name          string `json:"name"`
	Revision      int64  `json:"revision"`
	DryRun        *bool  `json:"dry_run,omitempty"`
	Message       string `json:"message,omitempty"`
}

// dryRun defaults to true: a rollback is only applied when dry_run is explicitly false.
func (a rollbackArgs) dryRun() bool {
	return a.DryRun == nil || *a.DryRun
}

func rollbackProperties(bindingDescription string) map[string]any {
	return map[string]any{
		"namespace_name": defaultStringProperty(),
		"name":           stringProperty(bindingDescription),
		"revision": map[string]any{
			"type":        "integer",
			"minimum":     1,
			"description": "Number of the revision to roll back to. Prefer the newest knownGood revision that is not active.",
		},
		"dry_run": map[string]any{
			"type":    "boolean",
			"default": true,
			"description": "Defaults to true: preview the rollback (changes and warnings) without applying it. " +
				"Set false only after showing the dry-run result to the user and receiving their confirmation.",
		},
		"message": map[string]any{
			"type":      "string",
			"maxLength": 1024,
			"description": "Optional reason for the rollback, in the user's words, e.g. \"payments failing since r4\". " +
				"It goes on the binding's RevisionReactivated event and the audit log. Leave it out unless the user gave one.",
		},
	}
}
