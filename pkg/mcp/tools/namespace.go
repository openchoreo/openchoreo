// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// decodeSpecStrict marshals an untyped MCP arg map and decodes it into a typed gen spec
// with DisallowUnknownFields, so typos like `refresh_Interval` (mis-cased) or `dat` (typo)
// fail with an error instead of being silently dropped.
func decodeSpecStrict[T any](raw map[string]interface{}, dst *T) error {
	specBytes, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal spec: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(specBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("decode spec: %w", err)
	}
	return nil
}

func (t *Toolsets) RegisterListNamespaces(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_namespaces"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewNamespace}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all namespaces. Namespaces are top-level containers for organizing " +
			"projects, components, and other resources. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{}), []string{}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Limit  int    `json:"limit,omitempty"`
		Cursor string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.NamespaceToolset.ListNamespaces(ctx, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateNamespace(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_namespace"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateNamespace}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a new namespace. Namespaces are top-level containers for organizing " +
			"projects, components, and other resources.",
		InputSchema: createSchema(map[string]any{
			"name":         stringProperty("The name of the namespace to create"),
			"display_name": stringProperty("Optional display name for the namespace"),
			"description":  stringProperty("Optional description of the namespace"),
		}, []string{"name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name,omitempty"`
		Description string `json:"description,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := map[string]string{}
		if args.DisplayName != "" {
			annotations["openchoreo.dev/display-name"] = args.DisplayName
		}
		if args.Description != "" {
			annotations["openchoreo.dev/description"] = args.Description
		}

		createReq := &gen.CreateNamespaceJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
		}
		result, err := t.NamespaceToolset.CreateNamespace(ctx, createReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListSecretReferences(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_secret_references"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewSecretReference}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all secret references for an namespace. Secret references are " +
			"credentials and sensitive configuration that can be used by components. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.NamespaceToolset.ListSecretReferences(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetSecretReference(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_secret_reference"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewSecretReference}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get a single secret reference by name. Returns the full spec " +
			"(template, data sources, refresh interval, target plane). " +
			"For actual sync status, query get_resource_events against the rendered ExternalSecret " +
			"(group: external-secrets.io, version: v1, kind: ExternalSecret) on the release binding " +
			"that consumes this SecretReference.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"secret_reference_name": stringProperty(
				"Name of the secret reference. Use list_secret_references to discover valid names."),
		}, []string{"namespace_name", "secret_reference_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName       string `json:"namespace_name"`
		SecretReferenceName string `json:"secret_reference_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.NamespaceToolset.GetSecretReference(ctx, args.NamespaceName, args.SecretReferenceName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEListSecretReferences(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_secret_references"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewSecretReference}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all secret references for an namespace. Secret references are " +
			"credentials and sensitive configuration that can be used by components. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.ListSecretReferences(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEGetSecretReference(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_secret_reference"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewSecretReference}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get a single secret reference by name. Returns the full spec " +
			"(template, data sources, refresh interval, target plane). " +
			"For actual sync status, query get_resource_events against the rendered ExternalSecret " +
			"(group: external-secrets.io, version: v1, kind: ExternalSecret) on the release binding " +
			"that consumes this SecretReference.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"secret_reference_name": stringProperty(
				"Name of the secret reference. Use list_secret_references to discover valid names."),
		}, []string{"namespace_name", "secret_reference_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName       string `json:"namespace_name"`
		SecretReferenceName string `json:"secret_reference_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetSecretReference(ctx, args.NamespaceName, args.SecretReferenceName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPECreateSecretReference(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_secret_reference"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateSecretReference}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a new secret reference in a namespace. The spec must include 'template' " +
			"(Kubernetes Secret type) and 'data' (mapping of secret keys to external references). " +
			"'refreshInterval' defaults to 1h.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name":   stringProperty("Optional human-readable display name"),
			"description":    stringProperty("Optional human-readable description"),
			"spec": map[string]any{
				"type": "object",
				"description": "SecretReferenceSpec — must include 'template' (e.g. {type: Opaque}) and " +
					"'data' (array of {secretKey, remoteRef: {key, property?, version?}}). " +
					"Optional: 'refreshInterval' (default 1h), 'targetPlane' ({kind, name}).",
			},
		}, []string{"namespace_name", "name", "spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		Name          string                 `json:"name"`
		DisplayName   string                 `json:"display_name,omitempty"`
		Description   string                 `json:"description,omitempty"`
		Spec          map[string]interface{} `json:"spec"`
	}) (*mcp.CallToolResult, any, error) {
		var spec gen.SecretReferenceSpec
		if err := decodeSpecStrict(args.Spec, &spec); err != nil {
			return nil, nil, err
		}

		createReq := &gen.CreateSecretReferenceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: args.Name},
			Spec:     &spec,
		}
		if annotations := buildAnnotations(args.DisplayName, args.Description); len(annotations) > 0 {
			createReq.Metadata.Annotations = &annotations
		}
		result, err := t.PEToolset.CreateSecretReference(ctx, args.NamespaceName, createReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEUpdateSecretReference(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "update_secret_reference"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateSecretReference}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Update an existing secret reference. spec is replaced wholesale when " +
			"provided; omitting it leaves the existing spec unchanged. display_name and description, " +
			"when provided, replace the corresponding annotations (empty string is treated as no-change).",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"secret_reference_name": stringProperty(
				"Name of the secret reference to update. Use list_secret_references to discover valid names."),
			"display_name": stringProperty("Optional updated display name"),
			"description":  stringProperty("Optional updated description"),
			"spec": map[string]any{
				"type":        "object",
				"description": "Optional: full SecretReferenceSpec to replace the existing spec.",
			},
		}, []string{"namespace_name", "secret_reference_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName       string                 `json:"namespace_name"`
		SecretReferenceName string                 `json:"secret_reference_name"`
		DisplayName         string                 `json:"display_name,omitempty"`
		Description         string                 `json:"description,omitempty"`
		Spec                map[string]interface{} `json:"spec,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		updateReq := &gen.UpdateSecretReferenceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: args.SecretReferenceName},
		}
		if annotations := buildAnnotations(args.DisplayName, args.Description); len(annotations) > 0 {
			updateReq.Metadata.Annotations = &annotations
		}
		if args.Spec != nil {
			var spec gen.SecretReferenceSpec
			if err := decodeSpecStrict(args.Spec, &spec); err != nil {
				return nil, nil, err
			}
			updateReq.Spec = &spec
		}
		result, err := t.PEToolset.UpdateSecretReference(ctx, args.NamespaceName, updateReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEDeleteSecretReference(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_secret_reference"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteSecretReference}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Delete a secret reference. Destructive: the underlying Kubernetes Secret " +
			"will be removed by the controller.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"secret_reference_name": stringProperty(
				"Name of the secret reference to delete. Use list_secret_references to discover valid names."),
		}, []string{"namespace_name", "secret_reference_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName       string `json:"namespace_name"`
		SecretReferenceName string `json:"secret_reference_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.DeleteSecretReference(ctx, args.NamespaceName, args.SecretReferenceName)
		return handleToolResult(result, err)
	})
}
