// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
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
	mockClient.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{}, nil)
	var output bytes.Buffer

	require.NoError(t, Run(t.Context(), mockClient, &output, t.TempDir()))
	assert.Contains(t, output.String(), "0 component(s)")
	assert.Contains(t, output.String(), "Coverage is incomplete")
	assert.Contains(t, output.String(), "workloads")
}
