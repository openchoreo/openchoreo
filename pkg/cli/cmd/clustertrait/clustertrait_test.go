// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

import (
	"fmt"
	"testing"

	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// mockImpl satisfies api.CommandImplementationInterface for testing command wiring.
type mockImpl struct {
	api.CommandImplementationInterface
	listClusterTraitsCalled bool
	listClusterTraitsErr    error
}

func (m *mockImpl) ListClusterTraits() error {
	m.listClusterTraitsCalled = true
	return m.listClusterTraitsErr
}

func (m *mockImpl) IsLoggedIn() bool       { return true }
func (m *mockImpl) GetLoginPrompt() string { return "" }

func TestNewClusterTraitCmd(t *testing.T) {
	impl := &mockImpl{}
	cmd := NewClusterTraitCmd(impl)

	if cmd.Use != "clustertrait" {
		t.Errorf("expected Use to be 'clustertrait', got %q", cmd.Use)
	}

	expectedAliases := map[string]bool{"clustertraits": true}
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

func TestNewClusterTraitCmd_NoNamespaceFlag(t *testing.T) {
	impl := &mockImpl{}
	cmd := NewClusterTraitCmd(impl)

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

func TestListClusterTraitCmd_Execute(t *testing.T) {
	impl := &mockImpl{}
	cmd := NewClusterTraitCmd(impl)
	cmd.SetArgs([]string{"list"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error executing list command: %v", err)
	}

	if !impl.listClusterTraitsCalled {
		t.Error("expected ListClusterTraits to be called")
	}
}

func TestListClusterTraitCmd_ExecuteError(t *testing.T) {
	impl := &mockImpl{
		listClusterTraitsErr: fmt.Errorf("api error"),
	}
	cmd := NewClusterTraitCmd(impl)
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
