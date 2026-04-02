// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterobservabilityplane

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

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterobservabilityplane/mocks"
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

func TestPrint_Nil(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList(nil))
	})
	assert.Contains(t, out, "No cluster observability planes found")
}

func TestPrint_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList([]gen.ClusterObservabilityPlane{}))
	})
	assert.Contains(t, out, "No cluster observability planes found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.ClusterObservabilityPlane{
		{Metadata: gen.ObjectMeta{Name: "obs-prod", CreationTimestamp: &now}},
		{Metadata: gen.ObjectMeta{Name: "obs-dev"}},
	}

	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "obs-prod")
	assert.Contains(t, out, "obs-dev")
}

func TestPrint_NilTimestamp(t *testing.T) {
	items := []gen.ClusterObservabilityPlane{
		{Metadata: gen.ObjectMeta{Name: "no-timestamp", CreationTimestamp: nil}},
	}

	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "no-timestamp")
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterObservabilityPlanes(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("server error"))

	cop := New(mc)
	assert.EqualError(t, cop.List(), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterObservabilityPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterObservabilityPlaneList{
		Items:      []gen.ClusterObservabilityPlane{{Metadata: gen.ObjectMeta{Name: "obs-prod"}}},
		Pagination: gen.Pagination{},
	}, nil)

	cop := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cop.List())
	})
	assert.Contains(t, out, "obs-prod")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterObservabilityPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterObservabilityPlaneList{
		Items: []gen.ClusterObservabilityPlane{
			{Metadata: gen.ObjectMeta{Name: "obs-prod", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "obs-dev", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	cop := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cop.List())
	})
	assert.Contains(t, out, "obs-prod")
	assert.Contains(t, out, "obs-dev")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterObservabilityPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterObservabilityPlaneList{
		Items:      []gen.ClusterObservabilityPlane{},
		Pagination: gen.Pagination{},
	}, nil)

	cop := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cop.List())
	})
	assert.Contains(t, out, "No cluster observability planes found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetClusterObservabilityPlane(mock.Anything, "missing").Return(nil, fmt.Errorf("not found: missing"))

	cop := New(mc)
	assert.EqualError(t, cop.Get(GetParams{ClusterObservabilityPlaneName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetClusterObservabilityPlane(mock.Anything, "obs-prod").Return(&gen.ClusterObservabilityPlane{
		Metadata: gen.ObjectMeta{Name: "obs-prod"},
	}, nil)

	cop := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cop.Get(GetParams{ClusterObservabilityPlaneName: "obs-prod"}))
	})
	assert.Contains(t, out, "name: obs-prod")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteClusterObservabilityPlane(mock.Anything, "obs-prod").Return(fmt.Errorf("forbidden"))

	cop := New(mc)
	assert.EqualError(t, cop.Delete(DeleteParams{ClusterObservabilityPlaneName: "obs-prod"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteClusterObservabilityPlane(mock.Anything, "obs-prod").Return(nil)

	cop := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cop.Delete(DeleteParams{ClusterObservabilityPlaneName: "obs-prod"}))
	})
	assert.Contains(t, out, "ClusterObservabilityPlane 'obs-prod' deleted")
}
