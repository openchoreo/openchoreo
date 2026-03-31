// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package namespace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNamespaceCmd_Structure(t *testing.T) {
	cmd := NewNamespaceCmd()

	assert.Equal(t, "namespace", cmd.Use)
	assert.Contains(t, cmd.Aliases, "ns")
	assert.Contains(t, cmd.Aliases, "namespaces")

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

func TestNamespaceCmd_GetAndDeleteRequireArg(t *testing.T) {
	cmd := NewNamespaceCmd()

	for _, name := range []string{"get", "delete"} {
		t.Run(name, func(t *testing.T) {
			subCmd, _, err := cmd.Find([]string{name})
			require.NoError(t, err)

			assert.Error(t, subCmd.Args(subCmd, []string{}))
			assert.NoError(t, subCmd.Args(subCmd, []string{"my-ns"}))
		})
	}
}
