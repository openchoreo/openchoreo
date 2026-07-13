// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectrelease

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

// --- NewProjectReleaseCmd structure ---

func TestNewProjectReleaseCmd_Use(t *testing.T) {
	cmd := NewProjectReleaseCmd(errFactory("unused"))
	assert.Equal(t, "projectrelease", cmd.Use)
	assert.Contains(t, cmd.Aliases, "projectreleases")
}

func TestNewProjectReleaseCmd_Subcommands(t *testing.T) {
	cmd := NewProjectReleaseCmd(errFactory("unused"))
	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.ElementsMatch(t, []string{"list", "get", "delete"}, names)
}

// --- list ---

func TestListCmd_FactoryError(t *testing.T) {
	cmd := newListCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, nil)
	assert.EqualError(t, err, "factory failed")
}

func TestListCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListProjectReleases(mock.Anything, "my-org", mock.Anything).Return(&gen.ProjectReleaseList{
		Items:      []gen.ProjectRelease{releaseFor("online-store-abc123")},
		Pagination: gen.Pagination{},
	}, nil)

	cmd := newListCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "my-org"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, nil))
	})
	assert.Contains(t, out, "online-store-abc123")
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
	err := cmd.RunE(cmd, []string{"online-store-abc123"})
	assert.EqualError(t, err, "factory failed")
}

func TestGetCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectRelease(mock.Anything, "my-org", "online-store-abc123").Return(
		&gen.ProjectRelease{Metadata: gen.ObjectMeta{Name: "online-store-abc123"}}, nil,
	)

	cmd := newGetCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "my-org"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"online-store-abc123"}))
	})
	assert.Contains(t, out, "online-store-abc123")
}

// --- delete ---

func TestDeleteCmd_MissingArg(t *testing.T) {
	cmd := newDeleteCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PROJECT_RELEASE_NAME")
}

func TestDeleteCmd_FactoryError(t *testing.T) {
	cmd := newDeleteCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"online-store-abc123"})
	assert.EqualError(t, err, "factory failed")
}

func TestDeleteCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteProjectRelease(mock.Anything, "my-org", "online-store-abc123").Return(nil)

	cmd := newDeleteCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "my-org"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"online-store-abc123"}))
	})
	assert.Contains(t, out, "deleted")
}
