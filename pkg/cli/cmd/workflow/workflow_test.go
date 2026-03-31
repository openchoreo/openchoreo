// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWorkflowCmd_Structure(t *testing.T) {
	cmd := NewWorkflowCmd()

	assert.Equal(t, "workflow", cmd.Use)
	assert.Contains(t, cmd.Aliases, "workflows")

	expected := []string{"list", "get", "delete", "run", "logs"}
	subCmds := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subCmds[sub.Name()] = true
	}
	for _, name := range expected {
		assert.True(t, subCmds[name], "expected '%s' subcommand", name)
	}
	assert.Len(t, cmd.Commands(), len(expected), "unexpected subcommands")
}

func TestWorkflowCmd_ListFlags(t *testing.T) {
	cmd := NewWorkflowCmd()
	listCmd, _, err := cmd.Find([]string{"list"})
	require.NoError(t, err)

	assert.NotNil(t, listCmd.Flags().Lookup("namespace"))
}

func TestWorkflowCmd_GetAndDeleteRequireArg(t *testing.T) {
	cmd := NewWorkflowCmd()

	for _, name := range []string{"get", "delete"} {
		t.Run(name, func(t *testing.T) {
			subCmd, _, err := cmd.Find([]string{name})
			require.NoError(t, err)

			assert.NotNil(t, subCmd.Flags().Lookup("namespace"))
			assert.Error(t, subCmd.Args(subCmd, []string{}))
			assert.NoError(t, subCmd.Args(subCmd, []string{"my-workflow"}))
		})
	}
}

func TestWorkflowCmd_StartFlags(t *testing.T) {
	cmd := NewWorkflowCmd()
	startCmd, _, err := cmd.Find([]string{"run"})
	require.NoError(t, err)

	assert.NotNil(t, startCmd.Flags().Lookup("namespace"))
	assert.NotNil(t, startCmd.Flags().Lookup("set"))
	assert.Error(t, startCmd.Args(startCmd, []string{}))
}

func TestWorkflowCmd_LogsFlags(t *testing.T) {
	cmd := NewWorkflowCmd()
	logsCmd, _, err := cmd.Find([]string{"logs"})
	require.NoError(t, err)

	expectedFlags := []string{"namespace", "follow", "since", "workflowrun"}
	for _, name := range expectedFlags {
		assert.NotNil(t, logsCmd.Flags().Lookup(name), "expected flag --%s on logs", name)
	}
	assert.Error(t, logsCmd.Args(logsCmd, []string{}))
}
