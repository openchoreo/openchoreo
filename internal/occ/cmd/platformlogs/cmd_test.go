// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package platformlogs

import (
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
)

func mockFactory(mc *mocks.MockInterface) client.NewClientFunc {
	return func() (client.Interface, error) {
		return mc, nil
	}
}

func errFactory(msg string) client.NewClientFunc {
	return func() (client.Interface, error) {
		return nil, fmt.Errorf("%s", msg)
	}
}

// --- structure ---

func TestNewClusterPlaneLogsCmd_Use(t *testing.T) {
	cmd := NewClusterPlaneLogsCmd(errFactory("unused"))
	assert.Equal(t, "logs [CLUSTER_OBSERVABILITY_PLANE_NAME]", cmd.Use)
	assert.Equal(t, "logs", cmd.Name())
	// The plane resource is cluster-scoped, so there is no OpenChoreo namespace to give.
	assert.Nil(t, cmd.Flags().Lookup("namespace"))
}

func TestNewNamespacedPlaneLogsCmd_Use(t *testing.T) {
	cmd := NewNamespacedPlaneLogsCmd(errFactory("unused"))
	assert.Equal(t, "logs [OBSERVABILITYPLANE_NAME]", cmd.Use)
	assert.NotNil(t, cmd.Flags().Lookup("namespace"))
}

func TestLogsCmd_Flags(t *testing.T) {
	for _, newCmd := range []func(client.NewClientFunc) *cobra.Command{
		NewClusterPlaneLogsCmd, NewNamespacedPlaneLogsCmd,
	} {
		cmd := newCmd(errFactory("unused"))
		for _, name := range []string{
			"cluster", "pod-namespace", "pod", "container", "selector",
			"level", "search", "output", "since", "tail", "follow",
		} {
			assert.NotNil(t, cmd.Flags().Lookup(name), "missing flag --%s", name)
		}

		assert.Equal(t, "l", cmd.Flags().Lookup("selector").Shorthand)
		assert.Equal(t, "o", cmd.Flags().Lookup("output").Shorthand)
		assert.Equal(t, "f", cmd.Flags().Lookup("follow").Shorthand)
		// `-c` denotes --component elsewhere in the CLI, so --container takes no shorthand.
		assert.Empty(t, cmd.Flags().Lookup("container").Shorthand)
		assert.Equal(t, outputText, cmd.Flags().Lookup("output").DefValue)
	}
}

// --- args ---

func TestLogsCmd_MissingArg(t *testing.T) {
	cmd := NewClusterPlaneLogsCmd(errFactory("unused"))
	err := cmd.Args(cmd, nil)
	assert.ErrorContains(t, err, "required argument CLUSTER_OBSERVABILITY_PLANE_NAME not provided")
}

func TestLogsCmd_TooManyArgs(t *testing.T) {
	cmd := NewNamespacedPlaneLogsCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{"a", "b"})
	assert.ErrorContains(t, err, "accepts 1 arg(s), received 2")
}

func TestLogsCmd_FactoryError(t *testing.T) {
	cmd := NewClusterPlaneLogsCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"default"})
	assert.EqualError(t, err, "factory failed")
}

func TestNamespacedLogsCmd_RequiresNamespace(t *testing.T) {
	cmd := NewNamespacedPlaneLogsCmd(mockFactory(mocks.NewMockInterface(t)))
	err := cmd.RunE(cmd, []string{"primary"})
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

// --- flag binding ---

func TestLogsParams(t *testing.T) {
	cmd := NewClusterPlaneLogsCmd(errFactory("unused"))
	for flag, value := range map[string]string{
		"cluster":       "clusterX,clusterY",
		"pod-namespace": "openchoreo-control-plane",
		"pod":           "controller-manager-abc",
		"container":     "manager",
		"selector":      "openchoreo.dev/plane=controlplane",
		"level":         "ERROR,WARN",
		"search":        "reconcile failed",
		"output":        outputJSON,
		"since":         "10m",
		"tail":          "25",
		"follow":        "true",
	} {
		require.NoError(t, cmd.Flags().Set(flag, value), flag)
	}

	params := logsParams(cmd)
	assert.Equal(t, []string{"clusterX", "clusterY"}, params.Clusters)
	assert.Equal(t, []string{"openchoreo-control-plane"}, params.PodNamespaces)
	assert.Equal(t, []string{"controller-manager-abc"}, params.Pods)
	assert.Equal(t, []string{"manager"}, params.Containers)
	assert.Equal(t, "openchoreo.dev/plane=controlplane", params.Selector)
	assert.Equal(t, []string{"ERROR", "WARN"}, params.Levels)
	assert.Equal(t, "reconcile failed", params.Search)
	assert.Equal(t, outputJSON, params.Output)
	assert.Equal(t, "10m", params.Since)
	assert.Equal(t, 25, params.Tail)
	assert.True(t, params.Follow)
}

func TestLogsParams_Defaults(t *testing.T) {
	params := logsParams(NewClusterPlaneLogsCmd(errFactory("unused")))
	assert.Empty(t, params.Clusters)
	assert.Empty(t, params.Selector)
	assert.Equal(t, outputText, params.Output)
	assert.Equal(t, 0, params.Tail)
	assert.False(t, params.Follow)
}
