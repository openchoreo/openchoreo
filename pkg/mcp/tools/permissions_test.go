// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// fullToolsets returns a Toolsets with all toolsets enabled using the shared mock handler.
func fullToolsets() *Toolsets {
	mockHandler := NewMockCoreToolsetHandler()
	return &Toolsets{
		NamespaceToolset:  mockHandler,
		ProjectToolset:    mockHandler,
		ComponentToolset:  mockHandler,
		DeploymentToolset: mockHandler,
		BuildToolset:      mockHandler,
		PEToolset:         mockHandler,
	}
}

// TestRegisteredToolsHavePermissions verifies that every tool registered by
// Register() has a corresponding non-empty permission entry in the returned
// perms map. This enforces that developers declare a permission when adding a tool.
func TestRegisteredToolsHavePermissions(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	perms, _ := fullToolsets().Register(server)

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("Failed to connect server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}
	defer clientSession.Close()

	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}
	if len(toolsResult.Tools) == 0 {
		t.Fatal("Register() registered no tools on the server")
	}

	for _, tool := range toolsResult.Tools {
		name := tool.Name
		t.Run(name, func(t *testing.T) {
			perm, ok := perms[name]
			if !ok {
				t.Errorf("tool %q has no entry in the perms map returned by Register()", name)
				return
			}
			if perm.Action == "" {
				t.Errorf("tool %q has an empty Action in its ToolPermission", name)
			}
			if perm.ToolName != name {
				t.Errorf("tool %q: ToolPermission.ToolName=%q mismatch", name, perm.ToolName)
			}
		})
	}
}

// TestRegisteredPermissionsHaveValidActions verifies that every action string in
// the perms map returned by Register() is a real action defined in actions.go.
// This prevents typos in action names from silently creating incorrect mappings.
func TestRegisteredPermissionsHaveValidActions(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	perms, _ := fullToolsets().Register(server)

	validActions := make(map[string]bool)
	for _, action := range authzcore.AllActions() {
		validActions[action.Name] = true
	}

	for toolName, perm := range perms {
		if !validActions[perm.Action] {
			t.Errorf("tool %q uses action %q which is not defined in systemActions", toolName, perm.Action)
		}
	}
}
