// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

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
	assert.Contains(t, out, "No cluster component types found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.ClusterComponentType{}))
	})
	assert.Contains(t, out, "No cluster component types found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	workloadType := gen.ClusterComponentTypeSpecWorkloadTypeDeployment
	items := []gen.ClusterComponentType{
		{
			Metadata: gen.ObjectMeta{
				Name:              "web-app",
				CreationTimestamp: &now,
			},
			Spec: &gen.ClusterComponentTypeSpec{
				WorkloadType: workloadType,
			},
		},
		{
			Metadata: gen.ObjectMeta{
				Name: "batch-job",
			},
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "WORKLOAD TYPE")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "web-app")
	assert.Contains(t, out, "deployment")
	assert.Contains(t, out, "batch-job")
}

func TestPrint_NilSpec(t *testing.T) {
	now := time.Now()
	items := []gen.ClusterComponentType{
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
	mc.EXPECT().ListClusterComponentTypes(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("server error"))

	cct := New(mc)
	assert.EqualError(t, cct.List(), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListClusterComponentTypes(mock.Anything, mock.Anything).Return(&gen.ClusterComponentTypeList{
		Items:      []gen.ClusterComponentType{{Metadata: gen.ObjectMeta{Name: "web-app"}}},
		Pagination: gen.Pagination{},
	}, nil)

	cct := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cct.List())
	})

	assert.Contains(t, out, "web-app")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	workloadType := gen.ClusterComponentTypeSpecWorkloadTypeDeployment
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListClusterComponentTypes(mock.Anything, mock.Anything).Return(&gen.ClusterComponentTypeList{
		Items: []gen.ClusterComponentType{
			{
				Metadata: gen.ObjectMeta{Name: "web-app", CreationTimestamp: &now},
				Spec:     &gen.ClusterComponentTypeSpec{WorkloadType: workloadType},
			},
			{
				Metadata: gen.ObjectMeta{Name: "batch-job", CreationTimestamp: &now},
			},
		},
		Pagination: gen.Pagination{},
	}, nil)

	cct := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cct.List())
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "WORKLOAD TYPE")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "web-app")
	assert.Contains(t, out, "batch-job")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListClusterComponentTypes(mock.Anything, mock.Anything).Return(&gen.ClusterComponentTypeList{
		Items:      []gen.ClusterComponentType{},
		Pagination: gen.Pagination{},
	}, nil)

	cct := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cct.List())
	})

	assert.Contains(t, out, "No cluster component types found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterComponentType(mock.Anything, "missing").Return(nil, fmt.Errorf("not found: missing"))

	cct := New(mc)
	assert.EqualError(t, cct.Get(GetParams{ClusterComponentTypeName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterComponentType(mock.Anything, "web-app").Return(&gen.ClusterComponentType{
		Metadata: gen.ObjectMeta{Name: "web-app"},
	}, nil)

	cct := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cct.Get(GetParams{ClusterComponentTypeName: "web-app"}))
	})

	assert.Contains(t, out, "name: web-app")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteClusterComponentType(mock.Anything, "web-app").Return(fmt.Errorf("forbidden: web-app"))

	cct := New(mc)
	assert.EqualError(t, cct.Delete(DeleteParams{ClusterComponentTypeName: "web-app"}), "forbidden: web-app")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteClusterComponentType(mock.Anything, "web-app").Return(nil)

	cct := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cct.Delete(DeleteParams{ClusterComponentTypeName: "web-app"}))
	})

	assert.Contains(t, out, "ClusterComponentType 'web-app' deleted")
}
