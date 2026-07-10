// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterprojecttype

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
	assert.Contains(t, out, "No cluster project types found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.ClusterProjectType{}))
	})
	assert.Contains(t, out, "No cluster project types found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.ClusterProjectType{
		{
			Metadata: gen.ObjectMeta{
				Name:              "default",
				CreationTimestamp: &now,
			},
			Spec: &gen.ProjectTypeSpec{},
		},
		{
			Metadata: gen.ObjectMeta{
				Name: "web-service",
			},
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "RESOURCES")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "default")
	assert.Contains(t, out, "web-service")
}

func TestPrint_NilSpec(t *testing.T) {
	now := time.Now()
	items := []gen.ClusterProjectType{
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

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListClusterProjectTypes(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("server error"))

	cpt := New(mc)
	assert.EqualError(t, cpt.List(), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListClusterProjectTypes(mock.Anything, mock.Anything).Return(&gen.ClusterProjectTypeList{
		Items:      []gen.ClusterProjectType{{Metadata: gen.ObjectMeta{Name: "default"}}},
		Pagination: gen.Pagination{},
	}, nil)

	cpt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cpt.List())
	})

	assert.Contains(t, out, "default")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListClusterProjectTypes(mock.Anything, mock.Anything).Return(&gen.ClusterProjectTypeList{
		Items:      []gen.ClusterProjectType{},
		Pagination: gen.Pagination{},
	}, nil)

	cpt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cpt.List())
	})

	assert.Contains(t, out, "No cluster project types found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterProjectType(mock.Anything, "missing").Return(nil, fmt.Errorf("not found: missing"))

	cpt := New(mc)
	assert.EqualError(t, cpt.Get(GetParams{ClusterProjectTypeName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterProjectType(mock.Anything, "default").Return(&gen.ClusterProjectType{
		Metadata: gen.ObjectMeta{Name: "default"},
	}, nil)

	cpt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cpt.Get(GetParams{ClusterProjectTypeName: "default"}))
	})

	assert.Contains(t, out, "name: default")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteClusterProjectType(mock.Anything, "default").Return(fmt.Errorf("forbidden: default"))

	cpt := New(mc)
	assert.EqualError(t, cpt.Delete(DeleteParams{ClusterProjectTypeName: "default"}), "forbidden: default")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteClusterProjectType(mock.Anything, "default").Return(nil)

	cpt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cpt.Delete(DeleteParams{ClusterProjectTypeName: "default"}))
	})

	assert.Contains(t, out, "ClusterProjectType 'default' deleted")
}
