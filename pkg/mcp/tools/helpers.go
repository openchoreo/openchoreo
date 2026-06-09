// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Scope values used by scope-collapsed MCP tools. A scope-collapsed tool replaces
// a namespace-scoped tool and its cluster-scoped counterpart with a single tool
// that selects behavior via the `scope` argument.
const (
	// ScopeNamespace selects the namespace-scoped resource. It is the default
	// and requires the namespace_name argument.
	ScopeNamespace = "namespace"
	// ScopeCluster selects the cluster-scoped resource. namespace_name is ignored.
	ScopeCluster = "cluster"
)

// Helper functions to create JSON Schema definitions
func stringProperty(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func defaultStringProperty() map[string]any {
	return map[string]any{
		"type": "string",
	}
}

// scopeProperty returns the JSON schema fragment for the `scope` argument shared
// by scope-collapsed tools. resourceNoun is a short human-readable name for the
// resource (e.g. "component type"); it is woven into the description.
func scopeProperty(resourceNoun string) map[string]any {
	return map[string]any{
		"type":    "string",
		"enum":    []string{ScopeNamespace, ScopeCluster},
		"default": ScopeNamespace,
		"description": fmt.Sprintf(
			"Resource scope. %q (default) operates on a namespace-scoped %s and requires namespace_name; "+
				"%q operates on a cluster-scoped %s and ignores namespace_name.",
			ScopeNamespace, resourceNoun, ScopeCluster, resourceNoun),
	}
}

// scopedNamespaceProperty returns the JSON schema fragment for the namespace_name
// argument on a scope-collapsed tool, where it is conditionally required.
func scopedNamespaceProperty() map[string]any {
	return stringProperty(`Required when scope is "namespace"; ignored when scope is "cluster".`)
}

// resolveScope normalizes a raw `scope` argument value. An empty value defaults
// to ScopeNamespace. Any value other than ScopeNamespace or ScopeCluster is an
// error.
func resolveScope(raw string) (string, error) {
	switch raw {
	case "", ScopeNamespace:
		return ScopeNamespace, nil
	case ScopeCluster:
		return ScopeCluster, nil
	default:
		return "", fmt.Errorf("invalid scope %q: must be %q or %q", raw, ScopeNamespace, ScopeCluster)
	}
}

// requireNamespaceForScope returns an error when scope is ScopeNamespace but no
// namespace_name was supplied.
func requireNamespaceForScope(scope, namespaceName string) error {
	if scope == ScopeNamespace && namespaceName == "" {
		return fmt.Errorf("namespace_name is required when scope is %q", ScopeNamespace)
	}
	return nil
}

func handleToolResult(result any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	jsonData, err := json.Marshal(result)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
	}, result, nil
}

// deprecationWarning builds the standard deprecation message for a deprecated
// cluster-prefixed alias tool, pointing the caller at the canonical scope-collapsed
// tool.
func deprecationWarning(aliasName, canonicalName string) string {
	return fmt.Sprintf("%s is deprecated; use %s with scope=%q", aliasName, canonicalName, ScopeCluster)
}

// handleDeprecatedToolResult is handleToolResult for a deprecated alias tool: on
// success it prepends a text content block carrying the deprecation warning so
// callers that have pinned the alias name get a migration signal. The structured
// result is left unchanged.
func handleDeprecatedToolResult(
	aliasName, canonicalName string, result any, err error,
) (*mcp.CallToolResult, any, error) {
	res, structured, rerr := handleToolResult(result, err)
	if rerr != nil || res == nil {
		return res, structured, rerr
	}
	res.Content = append(
		[]mcp.Content{&mcp.TextContent{Text: "deprecation_warning: " + deprecationWarning(aliasName, canonicalName)}},
		res.Content...,
	)
	return res, structured, nil
}

func intProperty(description string) map[string]any {
	return map[string]any{
		"type":        "integer",
		"description": description,
	}
}

// addPaginationProperties adds optional "limit" and "cursor" properties to a
// property map used for list tool input schemas.
func addPaginationProperties(properties map[string]any) map[string]any {
	properties["limit"] = intProperty(
		fmt.Sprintf("Maximum number of items to return per page (default %d)", DefaultPageSize))
	properties["cursor"] = stringProperty(
		"Opaque pagination cursor from a previous response's next_cursor field")
	return properties
}

func createSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
