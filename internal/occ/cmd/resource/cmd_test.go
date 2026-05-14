// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

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

// --- NewResourceCmd structure ---

func TestNewResourceCmd_Use(t *testing.T) {
	cmd := NewResourceCmd(errFactory("unused"))
	assert.Equal(t, "resource", cmd.Use)
	assert.Contains(t, cmd.Aliases, "resources")
}

func TestNewResourceCmd_Subcommands(t *testing.T) {
	cmd := NewResourceCmd(errFactory("unused"))
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
	mc.EXPECT().ListResources(mock.Anything, "my-org", mock.Anything).Return(&gen.ResourceInstanceList{
		Items:      []gen.ResourceInstance{resourceInstance("analytics-db", "ResourceType", "mysql")},
		Pagination: gen.Pagination{},
	}, nil)

	cmd := newListCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "my-org"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, nil))
	})
	assert.Contains(t, out, "analytics-db")
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
	err := cmd.RunE(cmd, []string{"analytics-db"})
	assert.EqualError(t, err, "factory failed")
}

func TestGetCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResource(mock.Anything, "my-org", "analytics-db").Return(
		&gen.ResourceInstance{Metadata: gen.ObjectMeta{Name: "analytics-db"}}, nil,
	)

	cmd := newGetCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "my-org"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"analytics-db"}))
	})
	assert.Contains(t, out, "analytics-db")
}

// --- delete ---

func TestDeleteCmd_MissingArg(t *testing.T) {
	cmd := newDeleteCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RESOURCE_NAME")
}

func TestDeleteCmd_FactoryError(t *testing.T) {
	cmd := newDeleteCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"analytics-db"})
	assert.EqualError(t, err, "factory failed")
}

func TestDeleteCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteResource(mock.Anything, "my-org", "analytics-db").Return(nil)

	cmd := newDeleteCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "my-org"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"analytics-db"}))
	})
	assert.Contains(t, out, "deleted")
}

// --- promote ---

func TestPromoteCmd_MissingArg(t *testing.T) {
	cmd := newPromoteCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RESOURCE_NAME")
}

func TestPromoteCmd_FactoryError(t *testing.T) {
	cmd := newPromoteCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"analytics-db"})
	assert.EqualError(t, err, "factory failed")
}

func TestPromoteCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResource(mock.Anything, "my-org", "analytics-db").Return(
		resourceWithLatestRelease(), nil)
	mc.EXPECT().ListResourceReleaseBindings(mock.Anything, "my-org", mock.Anything).Return(&gen.ResourceReleaseBindingList{
		Items:      []gen.ResourceReleaseBinding{bindingForEnv("analytics-db-dev", "dev")},
		Pagination: gen.Pagination{},
	}, nil)
	mc.EXPECT().UpdateResourceReleaseBinding(mock.Anything, "my-org", "analytics-db-dev", mock.Anything).
		Return(&gen.ResourceReleaseBinding{Metadata: gen.ObjectMeta{Name: "analytics-db-dev"}}, nil)

	cmd := newPromoteCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "my-org"))
	require.NoError(t, cmd.Flags().Set("env", "dev"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"analytics-db"}))
	})
	assert.Contains(t, out, "promoted")
	assert.Contains(t, out, "analytics-db-abc123")
}
