// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWorkloadCmd_Structure(t *testing.T) {
	cmd := NewWorkloadCmd()

	assert.Equal(t, "workload", cmd.Use)
	assert.Contains(t, cmd.Aliases, "workloads")

	expected := []string{"create", "list", "get", "delete"}
	subCmds := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subCmds[sub.Name()] = true
	}
	for _, name := range expected {
		assert.True(t, subCmds[name], "expected '%s' subcommand", name)
	}
	assert.Len(t, cmd.Commands(), len(expected), "unexpected subcommands")
}

func TestWorkloadCmd_CreateFlags(t *testing.T) {
	cmd := NewWorkloadCmd()
	createCmd, _, err := cmd.Find([]string{"create"})
	require.NoError(t, err)

	expectedFlags := []string{
		"name", "namespace", "project", "component",
		"image", "output", "descriptor", "dry-run",
		"mode", "root-dir",
	}
	for _, name := range expectedFlags {
		assert.NotNil(t, createCmd.Flags().Lookup(name), "expected flag --%s on create", name)
	}
}

func TestWorkloadCmd_ListFlags(t *testing.T) {
	cmd := NewWorkloadCmd()
	listCmd, _, err := cmd.Find([]string{"list"})
	require.NoError(t, err)

	assert.NotNil(t, listCmd.Flags().Lookup("namespace"))
}

func TestWorkloadCmd_GetAndDeleteRequireArg(t *testing.T) {
	cmd := NewWorkloadCmd()

	for _, name := range []string{"get", "delete"} {
		t.Run(name, func(t *testing.T) {
			subCmd, _, err := cmd.Find([]string{name})
			require.NoError(t, err)

			assert.NotNil(t, subCmd.Flags().Lookup("namespace"))
			assert.Error(t, subCmd.Args(subCmd, []string{}))
			assert.NoError(t, subCmd.Args(subCmd, []string{"my-workload"}))
		})
	}
}
