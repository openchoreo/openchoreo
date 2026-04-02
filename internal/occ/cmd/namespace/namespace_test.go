// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package namespace

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

	"github.com/openchoreo/openchoreo/internal/occ/cmd/namespace/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

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

func TestPrintList_Nil(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList(nil))
	})
	assert.Contains(t, out, "No namespaces found")
}

func TestPrintList_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList([]gen.Namespace{}))
	})
	assert.Contains(t, out, "No namespaces found")
}

func TestPrintList_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.Namespace{
		{Metadata: gen.ObjectMeta{Name: "org-a", CreationTimestamp: &now}},
		{Metadata: gen.ObjectMeta{Name: "org-b"}},
	}
	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "org-a")
	assert.Contains(t, out, "org-b")
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("server error"))

	n := New(mc)
	assert.EqualError(t, n.List(), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{
		Items:      []gen.Namespace{{Metadata: gen.ObjectMeta{Name: "org-a"}}},
		Pagination: gen.Pagination{},
	}, nil)

	n := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, n.List())
	})
	assert.Contains(t, out, "org-a")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{
		Items: []gen.Namespace{
			{Metadata: gen.ObjectMeta{Name: "org-a", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "org-b", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	n := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, n.List())
	})
	assert.Contains(t, out, "org-a")
	assert.Contains(t, out, "org-b")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{
		Items:      []gen.Namespace{},
		Pagination: gen.Pagination{},
	}, nil)

	n := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, n.List())
	})
	assert.Contains(t, out, "No namespaces found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetNamespace(mock.Anything, "missing").Return(nil, fmt.Errorf("not found: missing"))

	n := New(mc)
	assert.EqualError(t, n.Get("missing"), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetNamespace(mock.Anything, "org-a").Return(&gen.Namespace{
		Metadata: gen.ObjectMeta{Name: "org-a"},
	}, nil)

	n := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, n.Get("org-a"))
	})
	assert.Contains(t, out, "name: org-a")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteNamespace(mock.Anything, "org-a").Return(fmt.Errorf("forbidden"))

	n := New(mc)
	assert.EqualError(t, n.Delete("org-a"), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteNamespace(mock.Anything, "org-a").Return(nil)

	n := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, n.Delete("org-a"))
	})
	assert.Contains(t, out, "Namespace 'org-a' deleted")
}
