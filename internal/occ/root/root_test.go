// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package root

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRootCmd_Metadata(t *testing.T) {
	cmd := BuildRootCmd()
	assert.Equal(t, "occ", cmd.Use)
	assert.Equal(t, "OpenChoreo CLI", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
}

func TestBuildRootCmd_Subcommands(t *testing.T) {
	cmd := BuildRootCmd()

	expected := []string{
		"apply",
		"login",
		"logout",
		"config",
		"version",
		"componentrelease",
		"resourcerelease",
		"resourcereleasebinding",
		"releasebinding",
		"namespace",
		"project",
		"component",
		"resource",
		"environment",
		"dataplane",
		"workflowplane",
		"observabilityplane",
		"componenttype",
		"resourcetype",
		"clustercomponenttype",
		"clusterresourcetype",
		"clusterdataplane",
		"clusterobservabilityplane",
		"clusterworkflowplane",
		"trait",
		"clustertrait",
		"clusterworkflow",
		"clusterauthzrole",
		"clusterauthzrolebinding",
		"authzrole",
		"authzrolebinding",
		"workflow",
		"workflowrun",
		"secretreference",
		"workload",
		"deploymentpipeline",
		"observabilityalertsnotificationchannel",
	}

	commands := cmd.Commands()
	names := make([]string, len(commands))
	for i, c := range commands {
		names[i] = c.Name()
	}

	for _, name := range expected {
		assert.Contains(t, names, name, "missing subcommand: %s", name)
	}
	assert.Len(t, commands, len(expected), "unexpected number of subcommands")
}

func TestBuildRootCmd_HelpDoesNotError(t *testing.T) {
	cmd := BuildRootCmd()
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	require.NoError(t, err)
}
