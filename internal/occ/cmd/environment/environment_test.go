// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

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

	"github.com/openchoreo/openchoreo/internal/occ/cmd/environment/mocks"
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
	assert.Contains(t, out, "No environments found")
}

func TestPrint_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList([]gen.Environment{}))
	})
	assert.Contains(t, out, "No environments found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	isProduction := true
	items := []gen.Environment{
		{
			Metadata: gen.ObjectMeta{Name: "prod", CreationTimestamp: &now},
			Spec: &gen.EnvironmentSpec{
				DataPlaneRef: &struct {
					Kind gen.EnvironmentSpecDataPlaneRefKind `json:"kind"`
					Name string                              `json:"name"`
				}{Kind: gen.EnvironmentSpecDataPlaneRefKindClusterDataPlane, Name: "dp-prod"},
				IsProduction: &isProduction,
			},
		},
		{
			Metadata: gen.ObjectMeta{Name: "dev"},
		},
	}

	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "DATA PLANE")
	assert.Contains(t, out, "PRODUCTION")
	assert.Contains(t, out, "prod")
	assert.Contains(t, out, "true")
	assert.Contains(t, out, "dev")
	assert.Contains(t, out, "false")
}

func TestPrint_NilTimestamp(t *testing.T) {
	items := []gen.Environment{
		{Metadata: gen.ObjectMeta{Name: "no-timestamp", CreationTimestamp: nil}},
	}

	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "no-timestamp")
}

func TestPrint_NilSpec(t *testing.T) {
	items := []gen.Environment{
		{Metadata: gen.ObjectMeta{Name: "no-spec"}, Spec: nil},
	}

	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "no-spec")
	assert.Contains(t, out, "false")
}

// --- List tests ---

func TestList_ValidationError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	e := New(mc)
	err := e.List(ListParams{Namespace: ""})
	assert.Error(t, err)
}

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListEnvironments(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("server error"))

	e := New(mc)
	assert.EqualError(t, e.List(ListParams{Namespace: "my-org"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListEnvironments(mock.Anything, "my-org", mock.Anything).Return(&gen.EnvironmentList{
		Items:      []gen.Environment{{Metadata: gen.ObjectMeta{Name: "prod"}}},
		Pagination: gen.Pagination{},
	}, nil)

	e := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, e.List(ListParams{Namespace: "my-org"}))
	})
	assert.Contains(t, out, "prod")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListEnvironments(mock.Anything, "my-org", mock.Anything).Return(&gen.EnvironmentList{
		Items: []gen.Environment{
			{Metadata: gen.ObjectMeta{Name: "prod", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "dev", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	e := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, e.List(ListParams{Namespace: "my-org"}))
	})
	assert.Contains(t, out, "prod")
	assert.Contains(t, out, "dev")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListEnvironments(mock.Anything, "my-org", mock.Anything).Return(&gen.EnvironmentList{
		Items:      []gen.Environment{},
		Pagination: gen.Pagination{},
	}, nil)

	e := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, e.List(ListParams{Namespace: "my-org"}))
	})
	assert.Contains(t, out, "No environments found")
}

// --- Get tests ---

func TestGet_ValidationError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	e := New(mc)
	err := e.Get(GetParams{Namespace: "", EnvironmentName: "prod"})
	assert.Error(t, err)
}

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetEnvironment(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("not found: missing"))

	e := New(mc)
	assert.EqualError(t, e.Get(GetParams{Namespace: "my-org", EnvironmentName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetEnvironment(mock.Anything, "my-org", "prod").Return(&gen.Environment{
		Metadata: gen.ObjectMeta{Name: "prod"},
	}, nil)

	e := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, e.Get(GetParams{Namespace: "my-org", EnvironmentName: "prod"}))
	})
	assert.Contains(t, out, "name: prod")
}

// --- Delete tests ---

func TestDelete_ValidationError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	e := New(mc)
	err := e.Delete(DeleteParams{Namespace: "", EnvironmentName: "prod"})
	assert.Error(t, err)
}

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteEnvironment(mock.Anything, "my-org", "prod").Return(fmt.Errorf("forbidden"))

	e := New(mc)
	assert.EqualError(t, e.Delete(DeleteParams{Namespace: "my-org", EnvironmentName: "prod"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteEnvironment(mock.Anything, "my-org", "prod").Return(nil)

	e := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, e.Delete(DeleteParams{Namespace: "my-org", EnvironmentName: "prod"}))
	})
	assert.Contains(t, out, "Environment 'prod' deleted")
}
