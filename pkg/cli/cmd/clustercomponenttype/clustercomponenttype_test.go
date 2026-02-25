// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

import (
	"testing"
)

func TestNewClusterComponentTypeCmd(t *testing.T) {
	cmd := NewClusterComponentTypeCmd()

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
	cmd := NewClusterComponentTypeCmd()

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
