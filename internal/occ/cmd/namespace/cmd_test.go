// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package namespace

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

// mockFactory returns a NewClientFunc that provides the given mock.
func mockFactory(mc *mocks.MockInterface) client.NewClientFunc {
	return func() (client.Interface, error) {
		return mc, nil
	}
}

// errFactory returns a NewClientFunc that always fails.
func errFactory(msg string) client.NewClientFunc {
	return func() (client.Interface, error) {
		return nil, fmt.Errorf("%s", msg)
	}
}

// --- NewNamespaceCmd structure ---

func TestNewNamespaceCmd_Use(t *testing.T) {
	cmd := NewNamespaceCmd(errFactory("unused"))
	assert.Equal(t, "namespace", cmd.Use)
	assert.Contains(t, cmd.Aliases, "ns")
	assert.Contains(t, cmd.Aliases, "namespaces")
}

func TestNewNamespaceCmd_Subcommands(t *testing.T) {
	cmd := NewNamespaceCmd(errFactory("unused"))
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
	mc.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{
		Items:      []gen.Namespace{{Metadata: gen.ObjectMeta{Name: "org-a"}}},
		Pagination: gen.Pagination{},
	}, nil)

	cmd := newListCmd(mockFactory(mc))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, nil))
	})
	assert.Contains(t, out, "org-a")
}

// --- get ---

func TestGetCmd_MissingArg(t *testing.T) {
	cmd := newGetCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required argument NAMESPACE_NAME not provided")
}

func TestGetCmd_TooManyArgs(t *testing.T) {
	cmd := newGetCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{"a", "b"})
	assert.Error(t, err)
}

func TestGetCmd_FactoryError(t *testing.T) {
	cmd := newGetCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"org-a"})
	assert.EqualError(t, err, "factory failed")
}

func TestGetCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetNamespace(mock.Anything, "org-a").Return(
		&gen.Namespace{Metadata: gen.ObjectMeta{Name: "org-a"}}, nil,
	)

	cmd := newGetCmd(mockFactory(mc))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"org-a"}))
	})
	assert.Contains(t, out, "org-a")
}

// --- delete ---

func TestDeleteCmd_MissingArg(t *testing.T) {
	cmd := newDeleteCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required argument NAMESPACE_NAME not provided")
}

func TestDeleteCmd_FactoryError(t *testing.T) {
	cmd := newDeleteCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"org-a"})
	assert.EqualError(t, err, "factory failed")
}

func TestDeleteCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteNamespace(mock.Anything, "org-a").Return(nil)

	cmd := newDeleteCmd(mockFactory(mc))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"org-a"}))
	})
	assert.Contains(t, out, "Namespace 'org-a' deleted")
}
