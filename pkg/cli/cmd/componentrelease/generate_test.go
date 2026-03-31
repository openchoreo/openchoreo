// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewComponentReleaseCmd_Structure(t *testing.T) {
	cmd := NewComponentReleaseCmd()

	assert.Equal(t, "componentrelease", cmd.Use)
	assert.Contains(t, cmd.Aliases, "cr")

	expected := []string{"generate", "list", "get", "delete"}
	subCmds := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subCmds[sub.Name()] = true
	}
	for _, name := range expected {
		assert.True(t, subCmds[name], "expected '%s' subcommand", name)
	}
	assert.Len(t, cmd.Commands(), len(expected), "unexpected subcommands")
}

func TestComponentReleaseCmd_GenerateFlags(t *testing.T) {
	cmd := NewComponentReleaseCmd()
	genCmd, _, err := cmd.Find([]string{"generate"})
	require.NoError(t, err)

	expectedFlags := []string{"all", "project", "component", "name", "output-path", "dry-run", "mode", "root-dir"}
	for _, name := range expectedFlags {
		assert.NotNil(t, genCmd.Flags().Lookup(name), "expected flag --%s on generate", name)
	}
}

func TestComponentReleaseCmd_ListFlags(t *testing.T) {
	cmd := NewComponentReleaseCmd()
	listCmd, _, err := cmd.Find([]string{"list"})
	require.NoError(t, err)

	for _, name := range []string{"namespace", "project", "component"} {
		assert.NotNil(t, listCmd.Flags().Lookup(name), "expected flag --%s on list", name)
	}
}

func TestComponentReleaseCmd_GetAndDeleteRequireArg(t *testing.T) {
	cmd := NewComponentReleaseCmd()

	for _, name := range []string{"get", "delete"} {
		t.Run(name, func(t *testing.T) {
			subCmd, _, err := cmd.Find([]string{name})
			require.NoError(t, err)

			assert.NotNil(t, subCmd.Flags().Lookup("namespace"))
			assert.Error(t, subCmd.Args(subCmd, []string{}))
			assert.NoError(t, subCmd.Args(subCmd, []string{"my-release"}))
		})
	}
}

func TestIsFlagInArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		flagName string
		want     bool
	}{
		{
			name:     "flag present",
			args:     []string{"cmd", "--all"},
			flagName: "--all",
			want:     true,
		},
		{
			name:     "flag with value",
			args:     []string{"cmd", "--project=foo"},
			flagName: "--project",
			want:     true,
		},
		{
			name:     "flag absent",
			args:     []string{"cmd", "--other"},
			flagName: "--all",
			want:     false,
		},
		{
			name:     "empty args",
			args:     []string{},
			flagName: "--all",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origArgs := os.Args
			defer func() { os.Args = origArgs }()
			os.Args = tt.args

			assert.Equal(t, tt.want, isFlagInArgs(tt.flagName))
		})
	}
}
