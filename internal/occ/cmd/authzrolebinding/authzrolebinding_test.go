// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzrolebinding

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
	assert.Contains(t, out, "No authz role bindings found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.AuthzRoleBinding{}))
	})
	assert.Contains(t, out, "No authz role bindings found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.AuthzRoleBinding{
		{
			Metadata: gen.ObjectMeta{
				Name:              "admin-binding",
				CreationTimestamp: &now,
			},
		},
		{
			Metadata: gen.ObjectMeta{
				Name: "viewer-binding",
			},
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "admin-binding")
	assert.Contains(t, out, "viewer-binding")
}

func TestPrint_NilTimestamp(t *testing.T) {
	items := []gen.AuthzRoleBinding{
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
	mc.EXPECT().ListNamespaceRoleBindings(mock.Anything, "default", mock.Anything).Return(nil, fmt.Errorf("server error"))

	r := New(mc)
	err := r.List(ListParams{Namespace: "default"})
	assert.ErrorContains(t, err, "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListNamespaceRoleBindings(mock.Anything, "default", mock.Anything).Return(&gen.AuthzRoleBindingList{
		Items:      []gen.AuthzRoleBinding{{Metadata: gen.ObjectMeta{Name: "admin-binding"}}},
		Pagination: gen.Pagination{},
	}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.List(ListParams{Namespace: "default"}))
	})

	assert.Contains(t, out, "admin-binding")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListNamespaceRoleBindings(mock.Anything, "default", mock.Anything).Return(&gen.AuthzRoleBindingList{
		Items: []gen.AuthzRoleBinding{
			{Metadata: gen.ObjectMeta{Name: "admin-binding", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "viewer-binding", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.List(ListParams{Namespace: "default"}))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "admin-binding")
	assert.Contains(t, out, "viewer-binding")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListNamespaceRoleBindings(mock.Anything, "default", mock.Anything).Return(&gen.AuthzRoleBindingList{
		Items:      []gen.AuthzRoleBinding{},
		Pagination: gen.Pagination{},
	}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.List(ListParams{Namespace: "default"}))
	})

	assert.Contains(t, out, "No authz role bindings found")
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
	mc.EXPECT().GetNamespaceRoleBinding(mock.Anything, "default", "missing").Return(nil, fmt.Errorf("not found: missing"))

	r := New(mc)
	err := r.Get(GetParams{Namespace: "default", Name: "missing"})
	assert.ErrorContains(t, err, "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetNamespaceRoleBinding(mock.Anything, "default", "admin-binding").Return(&gen.AuthzRoleBinding{
		Metadata: gen.ObjectMeta{Name: "admin-binding"},
	}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.Get(GetParams{Namespace: "default", Name: "admin-binding"}))
	})

	assert.Contains(t, out, "name: admin-binding")
}

func TestGet_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)

	r := New(mc)
	err := r.Get(GetParams{Namespace: "", Name: "admin-binding"})
	assert.Error(t, err)
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteNamespaceRoleBinding(mock.Anything, "default", "admin-binding").Return(fmt.Errorf("forbidden: admin-binding"))

	r := New(mc)
	err := r.Delete(DeleteParams{Namespace: "default", Name: "admin-binding"})
	assert.ErrorContains(t, err, "forbidden: admin-binding")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteNamespaceRoleBinding(mock.Anything, "default", "admin-binding").Return(nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.Delete(DeleteParams{Namespace: "default", Name: "admin-binding"}))
	})

	assert.Contains(t, out, "Authz role binding 'admin-binding' deleted")
}

func TestDelete_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)

	r := New(mc)
	err := r.Delete(DeleteParams{Namespace: "", Name: "admin-binding"})
	assert.Error(t, err)
}
