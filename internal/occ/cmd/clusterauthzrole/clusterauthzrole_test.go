// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterauthzrole

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

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterauthzrole/mocks"
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
	assert.Contains(t, out, "No authz cluster roles found")
}

func TestPrint_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList([]gen.ClusterAuthzRole{}))
	})
	assert.Contains(t, out, "No authz cluster roles found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	desc := "cluster admin role"
	items := []gen.ClusterAuthzRole{
		{
			Metadata: gen.ObjectMeta{
				Name:              "cluster-admin",
				CreationTimestamp: &now,
			},
			Spec: &gen.ClusterAuthzRoleSpec{
				Description: &desc,
			},
		},
		{
			Metadata: gen.ObjectMeta{
				Name: "cluster-viewer",
			},
		},
	}

	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "DESCRIPTION")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "cluster-admin")
	assert.Contains(t, out, "cluster admin role")
	assert.Contains(t, out, "cluster-viewer")
}

func TestPrint_NilTimestamp(t *testing.T) {
	items := []gen.ClusterAuthzRole{
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
	mc.EXPECT().ListClusterRoles(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("server error"))

	cr := New(mc)
	err := cr.List()
	assert.ErrorContains(t, err, "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterRoles(mock.Anything, mock.Anything).Return(&gen.ClusterAuthzRoleList{
		Items:      []gen.ClusterAuthzRole{{Metadata: gen.ObjectMeta{Name: "cluster-admin"}}},
		Pagination: gen.Pagination{},
	}, nil)

	cr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cr.List())
	})

	assert.Contains(t, out, "cluster-admin")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterRoles(mock.Anything, mock.Anything).Return(&gen.ClusterAuthzRoleList{
		Items: []gen.ClusterAuthzRole{
			{Metadata: gen.ObjectMeta{Name: "cluster-admin", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "cluster-viewer", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	cr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cr.List())
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "cluster-admin")
	assert.Contains(t, out, "cluster-viewer")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterRoles(mock.Anything, mock.Anything).Return(&gen.ClusterAuthzRoleList{
		Items:      []gen.ClusterAuthzRole{},
		Pagination: gen.Pagination{},
	}, nil)

	cr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cr.List())
	})

	assert.Contains(t, out, "No authz cluster roles found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetClusterRole(mock.Anything, "missing").Return(nil, fmt.Errorf("not found: missing"))

	cr := New(mc)
	err := cr.Get(GetParams{Name: "missing"})
	assert.ErrorContains(t, err, "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetClusterRole(mock.Anything, "cluster-admin").Return(&gen.ClusterAuthzRole{
		Metadata: gen.ObjectMeta{Name: "cluster-admin"},
	}, nil)

	cr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cr.Get(GetParams{Name: "cluster-admin"}))
	})

	assert.Contains(t, out, "name: cluster-admin")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteClusterRole(mock.Anything, "cluster-admin").Return(fmt.Errorf("forbidden: cluster-admin"))

	cr := New(mc)
	err := cr.Delete(DeleteParams{Name: "cluster-admin"})
	assert.ErrorContains(t, err, "forbidden: cluster-admin")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteClusterRole(mock.Anything, "cluster-admin").Return(nil)

	cr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cr.Delete(DeleteParams{Name: "cluster-admin"}))
	})

	assert.Contains(t, out, "Authz cluster role 'cluster-admin' deleted")
}
