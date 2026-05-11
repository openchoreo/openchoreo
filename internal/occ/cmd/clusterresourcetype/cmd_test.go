// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterresourcetype

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

// --- NewClusterResourceTypeCmd structure ---

func TestNewClusterResourceTypeCmd_Use(t *testing.T) {
	cmd := NewClusterResourceTypeCmd(errFactory("unused"))
	assert.Equal(t, "clusterresourcetype", cmd.Use)
	assert.Contains(t, cmd.Aliases, "crt")
	assert.Contains(t, cmd.Aliases, "clusterresourcetypes")
}

func TestNewClusterResourceTypeCmd_Subcommands(t *testing.T) {
	cmd := NewClusterResourceTypeCmd(errFactory("unused"))
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
	mc.EXPECT().ListClusterResourceTypes(mock.Anything, mock.Anything).Return(&gen.ClusterResourceTypeList{
		Items:      []gen.ClusterResourceType{{Metadata: gen.ObjectMeta{Name: "mysql"}}},
		Pagination: gen.Pagination{},
	}, nil)

	cmd := newListCmd(mockFactory(mc))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, nil))
	})
	assert.Contains(t, out, "mysql")
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
	err := cmd.RunE(cmd, []string{"mysql"})
	assert.EqualError(t, err, "factory failed")
}

func TestGetCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterResourceType(mock.Anything, "mysql").Return(
		&gen.ClusterResourceType{Metadata: gen.ObjectMeta{Name: "mysql"}}, nil,
	)

	cmd := newGetCmd(mockFactory(mc))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"mysql"}))
	})
	assert.Contains(t, out, "mysql")
}

// --- delete ---

func TestDeleteCmd_MissingArg(t *testing.T) {
	cmd := newDeleteCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CLUSTER_RESOURCE_TYPE_NAME")
}

func TestDeleteCmd_FactoryError(t *testing.T) {
	cmd := newDeleteCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"mysql"})
	assert.EqualError(t, err, "factory failed")
}

func TestDeleteCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteClusterResourceType(mock.Anything, "mysql").Return(nil)

	cmd := newDeleteCmd(mockFactory(mc))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"mysql"}))
	})
	assert.Contains(t, out, "deleted")
}
