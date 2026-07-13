// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projecttype

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

func TestPrint_Nil(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(nil))
	})
	assert.Contains(t, out, "No project types found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.ProjectType{}))
	})
	assert.Contains(t, out, "No project types found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.ProjectType{
		{
			Metadata: gen.ObjectMeta{
				Name:              "web-service",
				CreationTimestamp: &now,
			},
			Spec: &gen.ProjectTypeSpec{},
		},
		{
			Metadata: gen.ObjectMeta{
				Name: "batch",
			},
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "RESOURCES")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "web-service")
	assert.Contains(t, out, "batch")
}

func TestPrint_NilSpec(t *testing.T) {
	now := time.Now()
	items := []gen.ProjectType{
		{
			Metadata: gen.ObjectMeta{
				Name:              "no-spec-type",
				CreationTimestamp: &now,
			},
			Spec: nil,
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "no-spec-type")
}

// --- Validation tests ---

func TestList_ValidationError(t *testing.T) {
	pt := New(mocks.NewMockInterface(t))
	assert.Error(t, pt.List(ListParams{Namespace: ""}))
}

func TestGet_ValidationError(t *testing.T) {
	pt := New(mocks.NewMockInterface(t))
	assert.Error(t, pt.Get(GetParams{Namespace: ""}))
}

func TestDelete_ValidationError(t *testing.T) {
	pt := New(mocks.NewMockInterface(t))
	assert.Error(t, pt.Delete(DeleteParams{Namespace: "my-org", ProjectTypeName: ""}))
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListProjectTypes(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("server error"))

	pt := New(mc)
	assert.EqualError(t, pt.List(ListParams{Namespace: "my-org"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListProjectTypes(mock.Anything, "my-org", mock.Anything).Return(&gen.ProjectTypeList{
		Items:      []gen.ProjectType{{Metadata: gen.ObjectMeta{Name: "web-service"}}},
		Pagination: gen.Pagination{},
	}, nil)

	pt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, pt.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "web-service")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListProjectTypes(mock.Anything, "my-org", mock.Anything).Return(&gen.ProjectTypeList{
		Items:      []gen.ProjectType{},
		Pagination: gen.Pagination{},
	}, nil)

	pt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, pt.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "No project types found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectType(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("not found: missing"))

	pt := New(mc)
	assert.EqualError(t, pt.Get(GetParams{Namespace: "my-org", ProjectTypeName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectType(mock.Anything, "my-org", "web-service").Return(&gen.ProjectType{
		Metadata: gen.ObjectMeta{Name: "web-service"},
	}, nil)

	pt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, pt.Get(GetParams{Namespace: "my-org", ProjectTypeName: "web-service"}))
	})

	assert.Contains(t, out, "name: web-service")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteProjectType(mock.Anything, "my-org", "web-service").Return(fmt.Errorf("forbidden: web-service"))

	pt := New(mc)
	assert.EqualError(t, pt.Delete(DeleteParams{Namespace: "my-org", ProjectTypeName: "web-service"}), "forbidden: web-service")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteProjectType(mock.Anything, "my-org", "web-service").Return(nil)

	pt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, pt.Delete(DeleteParams{Namespace: "my-org", ProjectTypeName: "web-service"}))
	})

	assert.Contains(t, out, "ProjectType 'web-service' deleted")
}
