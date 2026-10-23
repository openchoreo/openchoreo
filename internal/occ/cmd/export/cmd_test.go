// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bufio"
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestNewExportCmd_RequiresOutputFlag(t *testing.T) {
	cmd := NewExportCmd(func() (client.Interface, error) { return nil, errors.New("unused") })
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.PreRunE = nil
	cmd.SetArgs(nil)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output")
}

func TestRun_PrintsIncompleteCoverage(t *testing.T) {
	mockClient := mocks.NewMockInterface(t)
	expectNoClusterResources(mockClient)
	mockClient.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{}, nil)
	var output bytes.Buffer

	require.NoError(t, Run(t.Context(), mockClient, &output, Params{OutputDir: t.TempDir()}))
	assert.Contains(t, output.String(), "0 resource(s)")
	assert.Contains(t, output.String(), "Coverage is incomplete")
	assert.Contains(t, output.String(), "ProjectRelease and ProjectReleaseBinding")
}

func TestRun_HidesDeniedResourceKind(t *testing.T) {
	mockClient := mocks.NewMockInterface(t)
	mockClient.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{}, nil)
	mockClient.EXPECT().ListClusterComponentTypes(mock.Anything, mock.Anything).Return(&gen.ClusterComponentTypeList{}, nil)
	mockClient.EXPECT().ListClusterTraits(mock.Anything, mock.Anything).Return(&gen.ClusterTraitList{}, nil)
	mockClient.EXPECT().ListClusterWorkflows(mock.Anything, mock.Anything).Return(&gen.ClusterWorkflowList{}, nil)
	mockClient.EXPECT().ListClusterDataPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterDataPlaneList{}, nil)
	mockClient.EXPECT().ListClusterWorkflowPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterWorkflowPlaneList{}, nil)
	mockClient.EXPECT().ListClusterObservabilityPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterObservabilityPlaneList{}, nil)
	mockClient.EXPECT().ListClusterRoles(mock.Anything, mock.Anything).Return(nil, errors.New("You do not have permission to perform this operation"))

	var output bytes.Buffer
	err := Run(t.Context(), mockClient, &output, Params{OutputDir: t.TempDir()})
	require.EqualError(t, err, "you do not have permission to perform this export operation")
	assert.NotContains(t, err.Error(), "ClusterAuthzRole")
	assert.Empty(t, output.String(), "authorization failures should not print partial export details")
}

func TestPromptKeepPlaintext(t *testing.T) {
	finding := Withheld{
		Kind: "Component", Namespace: "acme", Project: "shop", Name: "api",
		Reasons: []string{".spec.parameters.dbPassword"},
	}
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "yes keeps", input: "y\n", want: true},
		{name: "no strips", input: "n\n", want: false},
		{name: "empty defaults to strip", input: "\n", want: false},
		{name: "invalid then yes", input: "maybe\nyes\n", want: true},
		{name: "eof defaults to strip", input: "", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			got := promptKeepPlaintext(bufio.NewReader(bytes.NewBufferString(test.input)), &output, finding)
			assert.Equal(t, test.want, got)
			assert.Contains(t, output.String(), ".spec.parameters.dbPassword")
			assert.NotContains(t, output.String(), "secret-value")
		})
	}
}
