// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterresourcetype

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
	assert.Contains(t, out, "No cluster resource types found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.ClusterResourceType{}))
	})
	assert.Contains(t, out, "No cluster resource types found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	retain := gen.ResourceTypeSpecRetainPolicy("Retain")
	items := []gen.ClusterResourceType{
		{
			Metadata: gen.ObjectMeta{
				Name:              "mysql",
				CreationTimestamp: &now,
			},
			Spec: &gen.ResourceTypeSpec{
				RetainPolicy: &retain,
			},
		},
		{
			Metadata: gen.ObjectMeta{
				Name: "redis",
			},
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "RETAIN POLICY")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "mysql")
	assert.Contains(t, out, "Retain")
	assert.Contains(t, out, "redis")
}

func TestPrint_NilSpec(t *testing.T) {
	now := time.Now()
	items := []gen.ClusterResourceType{
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
	mc.EXPECT().ListClusterResourceTypes(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("server error"))

	crt := New(mc)
	assert.EqualError(t, crt.List(), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListClusterResourceTypes(mock.Anything, mock.Anything).Return(&gen.ClusterResourceTypeList{
		Items:      []gen.ClusterResourceType{{Metadata: gen.ObjectMeta{Name: "mysql"}}},
		Pagination: gen.Pagination{},
	}, nil)

	crt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, crt.List())
	})

	assert.Contains(t, out, "mysql")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	retain := gen.ResourceTypeSpecRetainPolicy("Retain")
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListClusterResourceTypes(mock.Anything, mock.Anything).Return(&gen.ClusterResourceTypeList{
		Items: []gen.ClusterResourceType{
			{
				Metadata: gen.ObjectMeta{Name: "mysql", CreationTimestamp: &now},
				Spec:     &gen.ResourceTypeSpec{RetainPolicy: &retain},
			},
			{
				Metadata: gen.ObjectMeta{Name: "redis", CreationTimestamp: &now},
			},
		},
		Pagination: gen.Pagination{},
	}, nil)

	crt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, crt.List())
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "RETAIN POLICY")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "mysql")
	assert.Contains(t, out, "redis")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListClusterResourceTypes(mock.Anything, mock.Anything).Return(&gen.ClusterResourceTypeList{
		Items:      []gen.ClusterResourceType{},
		Pagination: gen.Pagination{},
	}, nil)

	crt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, crt.List())
	})

	assert.Contains(t, out, "No cluster resource types found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterResourceType(mock.Anything, "missing").Return(nil, fmt.Errorf("not found: missing"))

	crt := New(mc)
	assert.EqualError(t, crt.Get(GetParams{ClusterResourceTypeName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterResourceType(mock.Anything, "mysql").Return(&gen.ClusterResourceType{
		Metadata: gen.ObjectMeta{Name: "mysql"},
	}, nil)

	crt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, crt.Get(GetParams{ClusterResourceTypeName: "mysql"}))
	})

	assert.Contains(t, out, "name: mysql")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteClusterResourceType(mock.Anything, "mysql").Return(fmt.Errorf("forbidden: mysql"))

	crt := New(mc)
	assert.EqualError(t, crt.Delete(DeleteParams{ClusterResourceTypeName: "mysql"}), "forbidden: mysql")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteClusterResourceType(mock.Anything, "mysql").Return(nil)

	crt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, crt.Delete(DeleteParams{ClusterResourceTypeName: "mysql"}))
	})

	assert.Contains(t, out, "ClusterResourceType 'mysql' deleted")
}
