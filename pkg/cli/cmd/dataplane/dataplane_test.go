// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDataPlaneCmd_Structure(t *testing.T) {
	cmd := NewDataPlaneCmd()

	assert.Equal(t, "dataplane", cmd.Use)
	assert.Contains(t, cmd.Aliases, "dp")
	assert.Contains(t, cmd.Aliases, "dataplanes")

	expected := []string{"list", "get", "delete"}
	subCmds := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subCmds[sub.Name()] = true
	}
	for _, name := range expected {
		assert.True(t, subCmds[name], "expected '%s' subcommand", name)
	}
	assert.Len(t, cmd.Commands(), len(expected), "unexpected subcommands")
}

func TestDataPlaneCmd_GetAndDeleteRequireArg(t *testing.T) {
	cmd := NewDataPlaneCmd()

	for _, name := range []string{"get", "delete"} {
		t.Run(name, func(t *testing.T) {
			subCmd, _, err := cmd.Find([]string{name})
			require.NoError(t, err)

			assert.Error(t, subCmd.Args(subCmd, []string{}))
			assert.NoError(t, subCmd.Args(subCmd, []string{"my-dataplane"}))
		})
	}
}

func TestDataPlaneCmd_NamespaceFlag(t *testing.T) {
	cmd := NewDataPlaneCmd()

	for _, name := range []string{"list", "get", "delete"} {
		t.Run(name, func(t *testing.T) {
			subCmd, _, err := cmd.Find([]string{name})
			require.NoError(t, err)

			nsFlag := subCmd.Flags().Lookup("namespace")
			require.NotNil(t, nsFlag, "expected --namespace flag on %s", name)
			assert.Equal(t, "n", nsFlag.Shorthand)
		})
	}
}
