// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
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

// --- NewEndpointCmd structure ---

func TestNewEndpointCmd_Use(t *testing.T) {
	cmd := NewEndpointCmd(errFactory("unused"))
	assert.Equal(t, "endpoint", cmd.Use)
	assert.Contains(t, cmd.Aliases, "endpoints")
}

func TestNewEndpointCmd_Subcommands(t *testing.T) {
	cmd := NewEndpointCmd(errFactory("unused"))
	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.ElementsMatch(t, []string{"list", "get", "delete", "promote"}, names)
}

// --- list ---

func TestListCmd_FactoryError(t *testing.T) {
	cmd := newListCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, nil)
	assert.EqualError(t, err, "factory failed")
}

func TestListCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListEndpoints(mock.Anything, "my-org", mock.Anything).Return(&gen.EndpointList{
		Items:      []gen.Endpoint{endpointInstance("orders-api", "EndpointType", "public-http")},
		Pagination: gen.Pagination{},
	}, nil)

	cmd := newListCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "my-org"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, nil))
	})
	assert.Contains(t, out, "orders-api")
}

// --- get ---

func TestGetCmd_MissingArg(t *testing.T) {
	cmd := newGetCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required argument")
}

func TestGetCmd_FactoryError(t *testing.T) {
	cmd := newGetCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"orders-api"})
	assert.EqualError(t, err, "factory failed")
}

func TestGetCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "my-org", "orders-api").Return(
		&gen.Endpoint{Metadata: gen.ObjectMeta{Name: "orders-api"}}, nil,
	)

	cmd := newGetCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "my-org"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"orders-api"}))
	})
	assert.Contains(t, out, "orders-api")
}

// --- delete ---

func TestDeleteCmd_MissingArg(t *testing.T) {
	cmd := newDeleteCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ENDPOINT_NAME")
}

func TestDeleteCmd_FactoryError(t *testing.T) {
	cmd := newDeleteCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"orders-api"})
	assert.EqualError(t, err, "factory failed")
}

func TestDeleteCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteEndpoint(mock.Anything, "my-org", "orders-api").Return(nil)

	cmd := newDeleteCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "my-org"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"orders-api"}))
	})
	assert.Contains(t, out, "deleted")
}

// --- promote ---

func TestPromoteCmd_MissingArg(t *testing.T) {
	cmd := newPromoteCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ENDPOINT_NAME")
}

func TestPromoteCmd_FactoryError(t *testing.T) {
	cmd := newPromoteCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"orders-api"})
	assert.EqualError(t, err, "factory failed")
}

func TestPromoteCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "my-org", "orders-api").Return(
		endpointWithLatestRelease(), nil)
	mc.EXPECT().ListEndpointReleaseBindings(mock.Anything, "my-org", mock.Anything).Return(&gen.EndpointReleaseBindingList{
		Items:      []gen.EndpointReleaseBinding{bindingForEnv("orders-api-dev", "dev")},
		Pagination: gen.Pagination{},
	}, nil)
	mc.EXPECT().UpdateEndpointReleaseBinding(mock.Anything, "my-org", "orders-api-dev", mock.Anything).
		Return(&gen.EndpointReleaseBinding{Metadata: gen.ObjectMeta{Name: "orders-api-dev"}}, nil)

	cmd := newPromoteCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "my-org"))
	require.NoError(t, cmd.Flags().Set("env", "dev"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"orders-api"}))
	})
	assert.Contains(t, out, "promoted")
	assert.Contains(t, out, "orders-api-abc123")
}
