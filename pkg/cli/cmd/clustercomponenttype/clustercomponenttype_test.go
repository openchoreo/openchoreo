// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

import (
	"fmt"
	"testing"

	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// mockImpl satisfies api.CommandImplementationInterface for testing command wiring.
type mockImpl struct {
	api.CommandImplementationInterface
	listClusterComponentTypesCalled bool
	listClusterComponentTypesErr    error
}

func (m *mockImpl) ListClusterComponentTypes() error {
	m.listClusterComponentTypesCalled = true
	return m.listClusterComponentTypesErr
}

func (m *mockImpl) IsLoggedIn() bool       { return true }
func (m *mockImpl) GetLoginPrompt() string { return "" }

func TestNewClusterComponentTypeCmd(t *testing.T) {
	impl := &mockImpl{}
	cmd := NewClusterComponentTypeCmd(impl)

	if cmd.Use != "clustercomponenttype" {
		t.Errorf("expected Use to be 'clustercomponenttype', got %q", cmd.Use)
	}

	expectedAliases := map[string]bool{"cct": true, "clustercomponenttypes": true}
	for _, alias := range cmd.Aliases {
		if !expectedAliases[alias] {
			t.Errorf("unexpected alias %q", alias)
		}
	}
	if len(cmd.Aliases) != len(expectedAliases) {
		t.Errorf("expected %d aliases, got %d", len(expectedAliases), len(cmd.Aliases))
	}

	// Verify list subcommand exists
	listCmd, _, err := cmd.Find([]string{"list"})
	if err != nil {
		t.Fatalf("expected 'list' subcommand to exist: %v", err)
	}
	if listCmd.Use != "list" {
		t.Errorf("expected list subcommand Use to be 'list', got %q", listCmd.Use)
	}
}

func TestNewClusterComponentTypeCmd_NoNamespaceFlag(t *testing.T) {
	impl := &mockImpl{}
	cmd := NewClusterComponentTypeCmd(impl)

	listCmd, _, err := cmd.Find([]string{"list"})
	if err != nil {
		t.Fatalf("expected 'list' subcommand to exist: %v", err)
	}

	// Cluster-scoped commands should NOT have a namespace flag
	nsFlag := listCmd.Flags().Lookup("namespace")
	if nsFlag != nil {
		t.Error("cluster-scoped command should not have a --namespace flag")
	}
}

func TestListClusterComponentTypeCmd_Execute(t *testing.T) {
	impl := &mockImpl{}
	cmd := NewClusterComponentTypeCmd(impl)
	cmd.SetArgs([]string{"list"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error executing list command: %v", err)
	}

	if !impl.listClusterComponentTypesCalled {
		t.Error("expected ListClusterComponentTypes to be called")
	}
}

func TestListClusterComponentTypeCmd_ExecuteError(t *testing.T) {
	impl := &mockImpl{
		listClusterComponentTypesErr: fmt.Errorf("api error"),
	}
	cmd := NewClusterComponentTypeCmd(impl)
	cmd.SetArgs([]string{"list"})
	// Silence usage output on error
	cmd.SilenceUsage = true

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "api error" {
		t.Errorf("expected 'api error', got %q", err.Error())
	}
}
