// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

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

	"github.com/openchoreo/openchoreo/internal/occ/cmd/componentrelease/mocks"
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

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListComponentReleases(mock.Anything, "ns", mock.Anything).Return(nil, fmt.Errorf("server error"))

	cr := New(mc)
	assert.EqualError(t, cr.List(ListParams{Namespace: "ns"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListComponentReleases(mock.Anything, "ns", mock.Anything).Return(&gen.ComponentReleaseList{
		Items:      []gen.ComponentRelease{{Metadata: gen.ObjectMeta{Name: "rel-1"}}},
		Pagination: gen.Pagination{},
	}, nil)

	cr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cr.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "rel-1")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListComponentReleases(mock.Anything, "ns", mock.Anything).Return(&gen.ComponentReleaseList{
		Items: []gen.ComponentRelease{
			{Metadata: gen.ObjectMeta{Name: "rel-1", CreationTimestamp: &now}, Spec: &gen.ComponentReleaseSpec{Owner: struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{ComponentName: "comp-a"}}},
			{Metadata: gen.ObjectMeta{Name: "rel-2", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	cr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cr.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "rel-1")
	assert.Contains(t, out, "rel-2")
	assert.Contains(t, out, "comp-a")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListComponentReleases(mock.Anything, "ns", mock.Anything).Return(&gen.ComponentReleaseList{
		Items:      []gen.ComponentRelease{},
		Pagination: gen.Pagination{},
	}, nil)

	cr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cr.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "No component releases found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetComponentRelease(mock.Anything, "ns", "missing").Return(nil, fmt.Errorf("not found: missing"))

	cr := New(mc)
	assert.EqualError(t, cr.Get(GetParams{Namespace: "ns", ComponentReleaseName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetComponentRelease(mock.Anything, "ns", "rel-1").Return(&gen.ComponentRelease{
		Metadata: gen.ObjectMeta{Name: "rel-1"},
	}, nil)

	cr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cr.Get(GetParams{Namespace: "ns", ComponentReleaseName: "rel-1"}))
	})

	assert.Contains(t, out, "name: rel-1")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteComponentRelease(mock.Anything, "ns", "rel-1").Return(fmt.Errorf("forbidden: rel-1"))

	cr := New(mc)
	assert.EqualError(t, cr.Delete(DeleteParams{Namespace: "ns", ComponentReleaseName: "rel-1"}), "forbidden: rel-1")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteComponentRelease(mock.Anything, "ns", "rel-1").Return(nil)

	cr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cr.Delete(DeleteParams{Namespace: "ns", ComponentReleaseName: "rel-1"}))
	})

	assert.Contains(t, out, "ComponentRelease 'rel-1' deleted")
}
