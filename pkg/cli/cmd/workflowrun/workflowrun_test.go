// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWorkflowRunCmd_Structure(t *testing.T) {
	cmd := NewWorkflowRunCmd()

	assert.Equal(t, "workflowrun", cmd.Use)
	assert.Contains(t, cmd.Aliases, "wr")
	assert.Contains(t, cmd.Aliases, "workflowruns")

	expected := []string{"list", "get", "logs"}
	subCmds := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subCmds[sub.Name()] = true
	}
	for _, name := range expected {
		assert.True(t, subCmds[name], "expected '%s' subcommand", name)
	}
	assert.Len(t, cmd.Commands(), len(expected), "unexpected subcommands")
}

func TestWorkflowRunCmd_ListFlags(t *testing.T) {
	cmd := NewWorkflowRunCmd()
	listCmd, _, err := cmd.Find([]string{"list"})
	require.NoError(t, err)

	assert.NotNil(t, listCmd.Flags().Lookup("namespace"))
	assert.NotNil(t, listCmd.Flags().Lookup("workflow"))
}

func TestWorkflowRunCmd_GetRequiresArg(t *testing.T) {
	cmd := NewWorkflowRunCmd()
	getCmd, _, err := cmd.Find([]string{"get"})
	require.NoError(t, err)

	assert.NotNil(t, getCmd.Flags().Lookup("namespace"))
	assert.Error(t, getCmd.Args(getCmd, []string{}))
	assert.NoError(t, getCmd.Args(getCmd, []string{"my-run"}))
}

func TestWorkflowRunCmd_LogsFlags(t *testing.T) {
	cmd := NewWorkflowRunCmd()
	logsCmd, _, err := cmd.Find([]string{"logs"})
	require.NoError(t, err)

	expectedFlags := []string{"namespace", "follow", "since"}
	for _, name := range expectedFlags {
		assert.NotNil(t, logsCmd.Flags().Lookup(name), "expected flag --%s on logs", name)
	}
	assert.Error(t, logsCmd.Args(logsCmd, []string{}))
}
