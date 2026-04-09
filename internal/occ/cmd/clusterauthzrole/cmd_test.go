// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterauthzrole

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

// --- NewClusterAuthzRoleCmd structure ---

func TestNewClusterAuthzRoleCmd_Use(t *testing.T) {
	cmd := NewClusterAuthzRoleCmd(errFactory("unused"))
	assert.Equal(t, "clusterauthzrole", cmd.Use)
	assert.Contains(t, cmd.Aliases, "car")
	assert.Contains(t, cmd.Aliases, "clusterauthzroles")
}

func TestNewClusterAuthzRoleCmd_Subcommands(t *testing.T) {
	cmd := NewClusterAuthzRoleCmd(errFactory("unused"))
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
	mc.EXPECT().ListClusterRoles(mock.Anything, mock.Anything).Return(&gen.ClusterAuthzRoleList{
		Items:      []gen.ClusterAuthzRole{{Metadata: gen.ObjectMeta{Name: "cluster-admin"}}},
		Pagination: gen.Pagination{},
	}, nil)

	cmd := newListCmd(mockFactory(mc))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, nil))
	})
	assert.Contains(t, out, "cluster-admin")
}

// --- get ---

func TestGetCmd_MissingArg(t *testing.T) {
	cmd := newGetCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required argument")
}

func TestGetCmd_TooManyArgs(t *testing.T) {
	cmd := newGetCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{"a", "b"})
	assert.EqualError(t, err, "accepts 1 arg(s), received 2")
}

func TestGetCmd_FactoryError(t *testing.T) {
	cmd := newGetCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"cluster-admin"})
	assert.EqualError(t, err, "factory failed")
}

func TestGetCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterRole(mock.Anything, "cluster-admin").Return(
		&gen.ClusterAuthzRole{Metadata: gen.ObjectMeta{Name: "cluster-admin"}}, nil,
	)

	cmd := newGetCmd(mockFactory(mc))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"cluster-admin"}))
	})
	assert.Contains(t, out, "cluster-admin")
}

// --- delete ---

func TestDeleteCmd_MissingArg(t *testing.T) {
	cmd := newDeleteCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CLUSTER_AUTHZ_ROLE_NAME")
}

func TestDeleteCmd_FactoryError(t *testing.T) {
	cmd := newDeleteCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"cluster-admin"})
	assert.EqualError(t, err, "factory failed")
}

func TestDeleteCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteClusterRole(mock.Anything, "cluster-admin").Return(nil)

	cmd := newDeleteCmd(mockFactory(mc))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"cluster-admin"}))
	})
	assert.Contains(t, out, "deleted")
}
