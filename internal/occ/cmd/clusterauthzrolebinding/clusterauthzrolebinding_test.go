// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterauthzrolebinding

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterauthzrolebinding/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// captureStdout captures stdout output from a function call.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	origStdout := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
		w.Close()
		r.Close()
	}()

	fn()

	os.Stdout = origStdout
	w.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	return buf.String()
}

// --- printList tests ---

func TestPrint_Nil(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList(nil))
	})
	assert.Contains(t, out, "No authz cluster role bindings found")
}

func TestPrint_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList([]gen.ClusterAuthzRoleBinding{}))
	})
	assert.Contains(t, out, "No authz cluster role bindings found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.ClusterAuthzRoleBinding{
		{
			Metadata: gen.ObjectMeta{
				Name:              "admin-cluster-binding",
				CreationTimestamp: &now,
			},
		},
		{
			Metadata: gen.ObjectMeta{
				Name: "viewer-cluster-binding",
			},
		},
	}

	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "admin-cluster-binding")
	assert.Contains(t, out, "viewer-cluster-binding")
}

func TestPrint_NilTimestamp(t *testing.T) {
	items := []gen.ClusterAuthzRoleBinding{
		{
			Metadata: gen.ObjectMeta{
				Name:              "no-timestamp",
				CreationTimestamp: nil,
			},
		},
	}

	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "no-timestamp")
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterRoleBindings(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("server error"))

	crb := New(mc)
	err := crb.List()
	assert.ErrorContains(t, err, "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterRoleBindings(mock.Anything, mock.Anything).Return(&gen.ClusterAuthzRoleBindingList{
		Items:      []gen.ClusterAuthzRoleBinding{{Metadata: gen.ObjectMeta{Name: "admin-cluster-binding"}}},
		Pagination: gen.Pagination{},
	}, nil)

	crb := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, crb.List())
	})

	assert.Contains(t, out, "admin-cluster-binding")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterRoleBindings(mock.Anything, mock.Anything).Return(&gen.ClusterAuthzRoleBindingList{
		Items: []gen.ClusterAuthzRoleBinding{
			{Metadata: gen.ObjectMeta{Name: "admin-cluster-binding", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "viewer-cluster-binding", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	crb := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, crb.List())
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "admin-cluster-binding")
	assert.Contains(t, out, "viewer-cluster-binding")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterRoleBindings(mock.Anything, mock.Anything).Return(&gen.ClusterAuthzRoleBindingList{
		Items:      []gen.ClusterAuthzRoleBinding{},
		Pagination: gen.Pagination{},
	}, nil)

	crb := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, crb.List())
	})

	assert.Contains(t, out, "No authz cluster role bindings found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetClusterRoleBinding(mock.Anything, "missing").Return(nil, fmt.Errorf("not found: missing"))

	crb := New(mc)
	err := crb.Get(GetParams{Name: "missing"})
	assert.ErrorContains(t, err, "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetClusterRoleBinding(mock.Anything, "admin-cluster-binding").Return(&gen.ClusterAuthzRoleBinding{
		Metadata: gen.ObjectMeta{Name: "admin-cluster-binding"},
	}, nil)

	crb := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, crb.Get(GetParams{Name: "admin-cluster-binding"}))
	})

	assert.Contains(t, out, "name: admin-cluster-binding")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteClusterRoleBinding(mock.Anything, "admin-cluster-binding").Return(fmt.Errorf("forbidden: admin-cluster-binding"))

	crb := New(mc)
	err := crb.Delete(DeleteParams{Name: "admin-cluster-binding"})
	assert.ErrorContains(t, err, "forbidden: admin-cluster-binding")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteClusterRoleBinding(mock.Anything, "admin-cluster-binding").Return(nil)

	crb := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, crb.Delete(DeleteParams{Name: "admin-cluster-binding"}))
	})

	assert.Contains(t, out, "Authz cluster role binding 'admin-cluster-binding' deleted")
}
