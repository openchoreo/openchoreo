// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfigCmd_Structure(t *testing.T) {
	cmd := NewConfigCmd()

	assert.Equal(t, "config", cmd.Use)

	// Verify top-level subcommands
	subCmds := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subCmds[sub.Use] = true
	}
	assert.True(t, subCmds["context"], "expected 'context' subcommand")
	assert.True(t, subCmds["controlplane"], "expected 'controlplane' subcommand")
	assert.True(t, subCmds["credentials"], "expected 'credentials' subcommand")
	assert.Len(t, cmd.Commands(), 3, "unexpected subcommands")
}

func TestContextCmd_Subcommands(t *testing.T) {
	cmd := NewConfigCmd()
	ctxCmd, _, err := cmd.Find([]string{"context"})
	require.NoError(t, err)

	expected := []string{"add", "list", "delete", "update", "use"}
	subCmds := map[string]bool{}
	for _, sub := range ctxCmd.Commands() {
		subCmds[sub.Name()] = true
	}
	for _, name := range expected {
		assert.True(t, subCmds[name], "expected '%s' subcommand under context", name)
	}
	assert.Len(t, ctxCmd.Commands(), len(expected), "unexpected subcommands")
}

func TestControlPlaneCmd_Subcommands(t *testing.T) {
	cmd := NewConfigCmd()
	cpCmd, _, err := cmd.Find([]string{"controlplane"})
	require.NoError(t, err)

	expected := []string{"add", "list", "update", "delete"}
	subCmds := map[string]bool{}
	for _, sub := range cpCmd.Commands() {
		subCmds[sub.Name()] = true
	}
	for _, name := range expected {
		assert.True(t, subCmds[name], "expected '%s' subcommand under controlplane", name)
	}
	assert.Len(t, cpCmd.Commands(), len(expected), "unexpected subcommands")
}

func TestCredentialsCmd_Subcommands(t *testing.T) {
	cmd := NewConfigCmd()
	credCmd, _, err := cmd.Find([]string{"credentials"})
	require.NoError(t, err)

	expected := []string{"add", "list", "delete"}
	subCmds := map[string]bool{}
	for _, sub := range credCmd.Commands() {
		subCmds[sub.Name()] = true
	}
	for _, name := range expected {
		assert.True(t, subCmds[name], "expected '%s' subcommand under credentials", name)
	}
	assert.Len(t, credCmd.Commands(), len(expected), "unexpected subcommands")
}

func TestContextAddCmd_RequiredFlags(t *testing.T) {
	cmd := NewConfigCmd()
	addCmd, _, err := cmd.Find([]string{"context", "add"})
	require.NoError(t, err)

	cpFlag := addCmd.Flags().Lookup("controlplane")
	require.NotNil(t, cpFlag)

	credFlag := addCmd.Flags().Lookup("credentials")
	require.NotNil(t, credFlag)

	// These should be marked as required
	annotations := addCmd.Flags().Lookup("controlplane").Annotations
	assert.Contains(t, annotations, "cobra_annotation_bash_completion_one_required_flag")

	annotations = addCmd.Flags().Lookup("credentials").Annotations
	assert.Contains(t, annotations, "cobra_annotation_bash_completion_one_required_flag")
}

func TestContextAddCmd_Flags(t *testing.T) {
	cmd := NewConfigCmd()
	addCmd, _, err := cmd.Find([]string{"context", "add"})
	require.NoError(t, err)

	expectedFlags := []string{"controlplane", "credentials", "namespace", "project", "component"}
	for _, name := range expectedFlags {
		assert.NotNil(t, addCmd.Flags().Lookup(name), "expected flag --%s", name)
	}
}

func TestContextUpdateCmd_Flags(t *testing.T) {
	cmd := NewConfigCmd()
	updateCmd, _, err := cmd.Find([]string{"context", "update"})
	require.NoError(t, err)

	expectedFlags := []string{"namespace", "project", "component", "controlplane", "credentials"}
	for _, name := range expectedFlags {
		assert.NotNil(t, updateCmd.Flags().Lookup(name), "expected flag --%s", name)
	}
}

func TestControlPlaneAddCmd_RequiredFlags(t *testing.T) {
	cmd := NewConfigCmd()
	addCmd, _, err := cmd.Find([]string{"controlplane", "add"})
	require.NoError(t, err)

	urlFlag := addCmd.Flags().Lookup("url")
	require.NotNil(t, urlFlag)

	annotations := urlFlag.Annotations
	assert.Contains(t, annotations, "cobra_annotation_bash_completion_one_required_flag")
}

func TestControlPlaneUpdateCmd_Flags(t *testing.T) {
	cmd := NewConfigCmd()
	updateCmd, _, err := cmd.Find([]string{"controlplane", "update"})
	require.NoError(t, err)

	assert.NotNil(t, updateCmd.Flags().Lookup("url"), "expected flag --url")
}

func TestListCommands_NoArgs(t *testing.T) {
	cmd := NewConfigCmd()

	listPaths := [][]string{
		{"context", "list"},
		{"controlplane", "list"},
		{"credentials", "list"},
	}

	for _, path := range listPaths {
		t.Run(path[0]+"/"+path[1], func(t *testing.T) {
			listCmd, _, err := cmd.Find(path)
			require.NoError(t, err)

			// list commands should reject arguments
			listCmd.SilenceErrors = true
			listCmd.SilenceUsage = true
			err = listCmd.Args(listCmd, []string{"unexpected-arg"})
			assert.Error(t, err)
		})
	}
}

func TestArgRequiringCommands_RejectNoArgs(t *testing.T) {
	cmd := NewConfigCmd()

	paths := [][]string{
		{"context", "add"},
		{"context", "delete"},
		{"context", "update"},
		{"context", "use"},
		{"controlplane", "add"},
		{"controlplane", "update"},
		{"controlplane", "delete"},
		{"credentials", "add"},
		{"credentials", "delete"},
	}

	for _, path := range paths {
		t.Run(path[0]+"/"+path[1], func(t *testing.T) {
			subCmd, _, err := cmd.Find(path)
			require.NoError(t, err)

			// These commands require exactly one argument
			subCmd.SilenceErrors = true
			subCmd.SilenceUsage = true
			err = subCmd.Args(subCmd, []string{})
			assert.Error(t, err, "command %s/%s should reject zero args", path[0], path[1])
		})
	}
}
