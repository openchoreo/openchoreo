// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReleaseBindingCmd_Structure(t *testing.T) {
	cmd := NewReleaseBindingCmd()

	assert.Equal(t, "releasebinding", cmd.Use)
	assert.Contains(t, cmd.Aliases, "rb")

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

func TestReleaseBindingCmd_GenerateFlags(t *testing.T) {
	cmd := NewReleaseBindingCmd()
	genCmd, _, err := cmd.Find([]string{"generate"})
	require.NoError(t, err)

	expectedFlags := []string{
		"all", "project", "component", "target-env",
		"use-pipeline", "component-release", "output-path",
		"dry-run", "mode", "root-dir",
	}
	for _, name := range expectedFlags {
		assert.NotNil(t, genCmd.Flags().Lookup(name), "expected flag --%s on generate", name)
	}
}

func TestReleaseBindingCmd_ListFlags(t *testing.T) {
	cmd := NewReleaseBindingCmd()
	listCmd, _, err := cmd.Find([]string{"list"})
	require.NoError(t, err)

	for _, name := range []string{"namespace", "project", "component"} {
		assert.NotNil(t, listCmd.Flags().Lookup(name), "expected flag --%s on list", name)
	}
}

func TestReleaseBindingCmd_GetAndDeleteRequireArg(t *testing.T) {
	cmd := NewReleaseBindingCmd()

	for _, name := range []string{"get", "delete"} {
		t.Run(name, func(t *testing.T) {
			subCmd, _, err := cmd.Find([]string{name})
			require.NoError(t, err)

			assert.NotNil(t, subCmd.Flags().Lookup("namespace"))
			assert.Error(t, subCmd.Args(subCmd, []string{}))
			assert.NoError(t, subCmd.Args(subCmd, []string{"my-binding"}))
		})
	}
}
