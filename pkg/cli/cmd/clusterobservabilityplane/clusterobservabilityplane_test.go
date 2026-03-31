// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterobservabilityplane

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClusterObservabilityPlaneCmd_Structure(t *testing.T) {
	cmd := NewClusterObservabilityPlaneCmd()

	assert.Equal(t, "clusterobservabilityplane", cmd.Use)
	assert.Contains(t, cmd.Aliases, "clusterobservabilityplanes")
	assert.Contains(t, cmd.Aliases, "cop")

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

func TestClusterObservabilityPlaneCmd_GetAndDeleteRequireArg(t *testing.T) {
	cmd := NewClusterObservabilityPlaneCmd()

	for _, name := range []string{"get", "delete"} {
		t.Run(name, func(t *testing.T) {
			subCmd, _, err := cmd.Find([]string{name})
			require.NoError(t, err)

			assert.Error(t, subCmd.Args(subCmd, []string{}))
			assert.NoError(t, subCmd.Args(subCmd, []string{"my-plane"}))
		})
	}
}

func TestClusterObservabilityPlaneCmd_NoNamespaceFlag(t *testing.T) {
	cmd := NewClusterObservabilityPlaneCmd()

	for _, name := range []string{"list", "get", "delete"} {
		t.Run(name, func(t *testing.T) {
			subCmd, _, err := cmd.Find([]string{name})
			require.NoError(t, err)

			nsFlag := subCmd.Flags().Lookup("namespace")
			assert.Nil(t, nsFlag, "cluster-scoped command should not have --namespace flag")
		})
	}
}
