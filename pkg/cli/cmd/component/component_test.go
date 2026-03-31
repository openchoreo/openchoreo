// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCSV(t *testing.T) {
	tests := []struct {
		name     string
		flagVal  string
		wantNil  bool
		wantVals []string
	}{
		{
			name:     "comma separated values",
			flagVal:  "a,b,c",
			wantVals: []string{"a", "b", "c"},
		},
		{
			name:    "empty string",
			flagVal: "",
			wantNil: true,
		},
		{
			name:     "single value",
			flagVal:  "only",
			wantVals: []string{"only"},
		},
		{
			name:     "values with whitespace",
			flagVal:  " a , b , c ",
			wantVals: []string{"a", "b", "c"},
		},
		{
			name:     "trailing comma produces no empty entry",
			flagVal:  "a,b,",
			wantVals: []string{"a", "b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("test-flag", "", "")
			if tt.flagVal != "" {
				_ = cmd.Flags().Set("test-flag", tt.flagVal)
			}
			result := parseCSV(cmd, "test-flag")
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			assert.Equal(t, tt.wantVals, result)
		})
	}
}

func TestNewComponentCmd_Structure(t *testing.T) {
	cmd := NewComponentCmd()

	assert.Equal(t, "component", cmd.Use)
	assert.Contains(t, cmd.Aliases, "comp")
	assert.Contains(t, cmd.Aliases, "components")

	expected := []string{"list", "get", "delete", "scaffold", "deploy", "logs", "workflow", "workflowrun"}
	subCmds := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subCmds[sub.Name()] = true
	}
	for _, name := range expected {
		assert.True(t, subCmds[name], "expected '%s' subcommand", name)
	}
	assert.Len(t, cmd.Commands(), len(expected), "unexpected subcommands")
}

func TestComponentCmd_ListFlags(t *testing.T) {
	cmd := NewComponentCmd()
	listCmd, _, err := cmd.Find([]string{"list"})
	require.NoError(t, err)

	assert.NotNil(t, listCmd.Flags().Lookup("namespace"))
	assert.NotNil(t, listCmd.Flags().Lookup("project"))
}

func TestComponentCmd_GetAndDeleteRequireArg(t *testing.T) {
	cmd := NewComponentCmd()

	for _, name := range []string{"get", "delete"} {
		t.Run(name, func(t *testing.T) {
			subCmd, _, err := cmd.Find([]string{name})
			require.NoError(t, err)

			assert.NotNil(t, subCmd.Flags().Lookup("namespace"))

			// Should reject zero args
			err = subCmd.Args(subCmd, []string{})
			assert.Error(t, err)

			// Should accept one arg
			err = subCmd.Args(subCmd, []string{"my-component"})
			assert.NoError(t, err)

			// Should reject two args
			err = subCmd.Args(subCmd, []string{"a", "b"})
			assert.Error(t, err)
		})
	}
}

func TestComponentCmd_ScaffoldFlags(t *testing.T) {
	cmd := NewComponentCmd()
	scaffoldCmd, _, err := cmd.Find([]string{"scaffold"})
	require.NoError(t, err)

	expectedFlags := []string{
		"componenttype", "clustercomponenttype",
		"traits", "clustertraits",
		"workflow", "clusterworkflow",
		"project", "namespace",
		"output-file", "skip-comments", "skip-optional",
	}
	for _, name := range expectedFlags {
		assert.NotNil(t, scaffoldCmd.Flags().Lookup(name), "expected flag --%s on scaffold", name)
	}
}

func TestComponentCmd_DeployFlags(t *testing.T) {
	cmd := NewComponentCmd()
	deployCmd, _, err := cmd.Find([]string{"deploy"})
	require.NoError(t, err)

	expectedFlags := []string{"namespace", "project", "release", "to", "set", "output"}
	for _, name := range expectedFlags {
		assert.NotNil(t, deployCmd.Flags().Lookup(name), "expected flag --%s on deploy", name)
	}
}

func TestComponentCmd_LogsFlags(t *testing.T) {
	cmd := NewComponentCmd()
	logsCmd, _, err := cmd.Find([]string{"logs"})
	require.NoError(t, err)

	expectedFlags := []string{"namespace", "project", "env", "follow", "since", "tail"}
	for _, name := range expectedFlags {
		assert.NotNil(t, logsCmd.Flags().Lookup(name), "expected flag --%s on logs", name)
	}
}

func TestComponentCmd_WorkflowSubcommands(t *testing.T) {
	cmd := NewComponentCmd()
	wfCmd, _, err := cmd.Find([]string{"workflow"})
	require.NoError(t, err)

	assert.Contains(t, wfCmd.Aliases, "wf")

	subCmds := map[string]bool{}
	for _, sub := range wfCmd.Commands() {
		subCmds[sub.Name()] = true
	}
	assert.True(t, subCmds["run"], "expected 'run' subcommand under workflow")
	assert.True(t, subCmds["logs"], "expected 'logs' subcommand under workflow")
}

func TestComponentCmd_WorkflowRunSubcommands(t *testing.T) {
	cmd := NewComponentCmd()
	wrCmd, _, err := cmd.Find([]string{"workflowrun"})
	require.NoError(t, err)

	assert.Contains(t, wrCmd.Aliases, "wfrun")
	assert.Contains(t, wrCmd.Aliases, "wr")

	subCmds := map[string]bool{}
	for _, sub := range wrCmd.Commands() {
		subCmds[sub.Name()] = true
	}
	assert.True(t, subCmds["list"], "expected 'list' subcommand under workflowrun")
	assert.True(t, subCmds["logs"], "expected 'logs' subcommand under workflowrun")
}
