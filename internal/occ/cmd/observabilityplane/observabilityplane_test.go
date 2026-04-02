// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

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

	"github.com/openchoreo/openchoreo/internal/occ/cmd/observabilityplane/mocks"
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
	assert.Contains(t, out, "No observability planes found")
}

func TestPrintList_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList([]gen.ObservabilityPlane{}))
	})
	assert.Contains(t, out, "No observability planes found")
}

func TestPrintList_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.ObservabilityPlane{
		{Metadata: gen.ObjectMeta{Name: "obs-plane-1", CreationTimestamp: &now}},
		{Metadata: gen.ObjectMeta{Name: "obs-plane-2"}},
	}
	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "obs-plane-1")
	assert.Contains(t, out, "obs-plane-2")
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListObservabilityPlanes(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("server error"))

	op := New(mc)
	assert.EqualError(t, op.List(ListParams{Namespace: "org-a"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListObservabilityPlanes(mock.Anything, "org-a", mock.Anything).Return(&gen.ObservabilityPlaneList{
		Items:      []gen.ObservabilityPlane{{Metadata: gen.ObjectMeta{Name: "obs-plane-1"}}},
		Pagination: gen.Pagination{},
	}, nil)

	op := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, op.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "obs-plane-1")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListObservabilityPlanes(mock.Anything, "org-a", mock.Anything).Return(&gen.ObservabilityPlaneList{
		Items: []gen.ObservabilityPlane{
			{Metadata: gen.ObjectMeta{Name: "obs-plane-1", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "obs-plane-2", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	op := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, op.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "obs-plane-1")
	assert.Contains(t, out, "obs-plane-2")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListObservabilityPlanes(mock.Anything, "org-a", mock.Anything).Return(&gen.ObservabilityPlaneList{
		Items:      []gen.ObservabilityPlane{},
		Pagination: gen.Pagination{},
	}, nil)

	op := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, op.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "No observability planes found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetObservabilityPlane(mock.Anything, "org-a", "missing").Return(nil, fmt.Errorf("not found: missing"))

	op := New(mc)
	assert.EqualError(t, op.Get(GetParams{Namespace: "org-a", ObservabilityPlaneName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetObservabilityPlane(mock.Anything, "org-a", "obs-plane-1").Return(&gen.ObservabilityPlane{
		Metadata: gen.ObjectMeta{Name: "obs-plane-1"},
	}, nil)

	op := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, op.Get(GetParams{Namespace: "org-a", ObservabilityPlaneName: "obs-plane-1"}))
	})
	assert.Contains(t, out, "name: obs-plane-1")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteObservabilityPlane(mock.Anything, "org-a", "obs-plane-1").Return(fmt.Errorf("forbidden"))

	op := New(mc)
	assert.EqualError(t, op.Delete(DeleteParams{Namespace: "org-a", ObservabilityPlaneName: "obs-plane-1"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteObservabilityPlane(mock.Anything, "org-a", "obs-plane-1").Return(nil)

	op := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, op.Delete(DeleteParams{Namespace: "org-a", ObservabilityPlaneName: "obs-plane-1"}))
	})
	assert.Contains(t, out, "ObservabilityPlane 'obs-plane-1' deleted")
}
