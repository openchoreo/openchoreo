// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzrole

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// --- printList tests ---

func TestPrint_Nil(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(nil))
	})
	assert.Contains(t, out, "No authz roles found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.AuthzRole{}))
	})
	assert.Contains(t, out, "No authz roles found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	desc := "test description"
	items := []gen.AuthzRole{
		{
			Metadata: gen.ObjectMeta{
				Name:              "admin-role",
				CreationTimestamp: &now,
			},
			Spec: &gen.AuthzRoleSpec{
				Description: &desc,
			},
		},
		{
			Metadata: gen.ObjectMeta{
				Name: "viewer-role",
			},
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "DESCRIPTION")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "admin-role")
	assert.Contains(t, out, "test description")
	assert.Contains(t, out, "viewer-role")
}

func TestPrint_NilTimestamp(t *testing.T) {
	items := []gen.AuthzRole{
		{
			Metadata: gen.ObjectMeta{
				Name:              "no-timestamp",
				CreationTimestamp: nil,
			},
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "no-timestamp")
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListNamespaceRoles(mock.Anything, "default", mock.Anything).Return(nil, fmt.Errorf("server error"))

	r := New(mc)
	err := r.List(ListParams{Namespace: "default"})
	assert.ErrorContains(t, err, "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListNamespaceRoles(mock.Anything, "default", mock.Anything).Return(&gen.AuthzRoleList{
		Items:      []gen.AuthzRole{{Metadata: gen.ObjectMeta{Name: "admin-role"}}},
		Pagination: gen.Pagination{},
	}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.List(ListParams{Namespace: "default"}))
	})

	assert.Contains(t, out, "admin-role")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListNamespaceRoles(mock.Anything, "default", mock.Anything).Return(&gen.AuthzRoleList{
		Items: []gen.AuthzRole{
			{Metadata: gen.ObjectMeta{Name: "admin-role", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "viewer-role", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.List(ListParams{Namespace: "default"}))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "admin-role")
	assert.Contains(t, out, "viewer-role")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListNamespaceRoles(mock.Anything, "default", mock.Anything).Return(&gen.AuthzRoleList{
		Items:      []gen.AuthzRole{},
		Pagination: gen.Pagination{},
	}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.List(ListParams{Namespace: "default"}))
	})

	assert.Contains(t, out, "No authz roles found")
}

func TestList_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)

	r := New(mc)
	err := r.List(ListParams{Namespace: ""})
	assert.Error(t, err)
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetNamespaceRole(mock.Anything, "default", "missing").Return(nil, fmt.Errorf("not found: missing"))

	r := New(mc)
	err := r.Get(GetParams{Namespace: "default", Name: "missing"})
	assert.ErrorContains(t, err, "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetNamespaceRole(mock.Anything, "default", "admin-role").Return(&gen.AuthzRole{
		Metadata: gen.ObjectMeta{Name: "admin-role"},
	}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.Get(GetParams{Namespace: "default", Name: "admin-role"}))
	})

	assert.Contains(t, out, "name: admin-role")
}

func TestGet_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)

	r := New(mc)
	err := r.Get(GetParams{Namespace: "", Name: "admin-role"})
	assert.Error(t, err)
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteNamespaceRole(mock.Anything, "default", "admin-role").Return(fmt.Errorf("forbidden: admin-role"))

	r := New(mc)
	err := r.Delete(DeleteParams{Namespace: "default", Name: "admin-role"})
	assert.ErrorContains(t, err, "forbidden: admin-role")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteNamespaceRole(mock.Anything, "default", "admin-role").Return(nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.Delete(DeleteParams{Namespace: "default", Name: "admin-role"}))
	})

	assert.Contains(t, out, "Authz role 'admin-role' deleted")
}

func TestDelete_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)

	r := New(mc)
	err := r.Delete(DeleteParams{Namespace: "", Name: "admin-role"})
	assert.Error(t, err)
}
