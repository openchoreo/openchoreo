// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secretreference

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

	"github.com/openchoreo/openchoreo/internal/occ/cmd/secretreference/mocks"
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
	assert.Contains(t, out, "No secret references found")
}

func TestPrintList_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList([]gen.SecretReference{}))
	})
	assert.Contains(t, out, "No secret references found")
}

func TestPrintList_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.SecretReference{
		{Metadata: gen.ObjectMeta{Name: "secret-1", CreationTimestamp: &now}},
		{Metadata: gen.ObjectMeta{Name: "secret-2"}},
	}
	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "secret-1")
	assert.Contains(t, out, "secret-2")
}

// --- List tests ---

func TestList_ValidationError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	sr := New(mc)
	err := sr.List(ListParams{Namespace: ""})
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListSecretReferences(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("server error"))

	sr := New(mc)
	assert.EqualError(t, sr.List(ListParams{Namespace: "org-a"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListSecretReferences(mock.Anything, "org-a", mock.Anything).Return(&gen.SecretReferenceList{
		Items:      []gen.SecretReference{{Metadata: gen.ObjectMeta{Name: "secret-1"}}},
		Pagination: gen.Pagination{},
	}, nil)

	sr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, sr.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "secret-1")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListSecretReferences(mock.Anything, "org-a", mock.Anything).Return(&gen.SecretReferenceList{
		Items: []gen.SecretReference{
			{Metadata: gen.ObjectMeta{Name: "secret-1", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "secret-2", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	sr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, sr.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "secret-1")
	assert.Contains(t, out, "secret-2")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListSecretReferences(mock.Anything, "org-a", mock.Anything).Return(&gen.SecretReferenceList{
		Items:      []gen.SecretReference{},
		Pagination: gen.Pagination{},
	}, nil)

	sr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, sr.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "No secret references found")
}

// --- Get tests ---

func TestGet_ValidationError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	sr := New(mc)
	err := sr.Get(GetParams{Namespace: "", SecretReferenceName: "secret-1"})
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetSecretReference(mock.Anything, "org-a", "missing").Return(nil, fmt.Errorf("not found: missing"))

	sr := New(mc)
	assert.EqualError(t, sr.Get(GetParams{Namespace: "org-a", SecretReferenceName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetSecretReference(mock.Anything, "org-a", "secret-1").Return(&gen.SecretReference{
		Metadata: gen.ObjectMeta{Name: "secret-1"},
	}, nil)

	sr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, sr.Get(GetParams{Namespace: "org-a", SecretReferenceName: "secret-1"}))
	})
	assert.Contains(t, out, "name: secret-1")
}

// --- Delete tests ---

func TestDelete_ValidationError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	sr := New(mc)
	err := sr.Delete(DeleteParams{Namespace: "", SecretReferenceName: "secret-1"})
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteSecretReference(mock.Anything, "org-a", "secret-1").Return(fmt.Errorf("forbidden"))

	sr := New(mc)
	assert.EqualError(t, sr.Delete(DeleteParams{Namespace: "org-a", SecretReferenceName: "secret-1"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteSecretReference(mock.Anything, "org-a", "secret-1").Return(nil)

	sr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, sr.Delete(DeleteParams{Namespace: "org-a", SecretReferenceName: "secret-1"}))
	})
	assert.Contains(t, out, "SecretReference 'secret-1' deleted")
}
