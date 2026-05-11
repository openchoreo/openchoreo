// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcetype

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
	assert.Contains(t, out, "No resource types found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.ResourceType{}))
	})
	assert.Contains(t, out, "No resource types found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	retain := gen.ResourceTypeSpecRetainPolicy("Retain")
	items := []gen.ResourceType{
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
	items := []gen.ResourceType{
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
	rt := New(mocks.NewMockInterface(t))
	assert.Error(t, rt.List(ListParams{Namespace: ""}))
}

func TestGet_ValidationError(t *testing.T) {
	rt := New(mocks.NewMockInterface(t))
	assert.Error(t, rt.Get(GetParams{Namespace: ""}))
}

func TestDelete_ValidationError(t *testing.T) {
	rt := New(mocks.NewMockInterface(t))
	assert.Error(t, rt.Delete(DeleteParams{Namespace: "my-org", ResourceTypeName: ""}))
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResourceTypes(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("server error"))

	rt := New(mc)
	assert.EqualError(t, rt.List(ListParams{Namespace: "my-org"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResourceTypes(mock.Anything, "my-org", mock.Anything).Return(&gen.ResourceTypeList{
		Items:      []gen.ResourceType{{Metadata: gen.ObjectMeta{Name: "mysql"}}},
		Pagination: gen.Pagination{},
	}, nil)

	rt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, rt.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "mysql")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	retain := gen.ResourceTypeSpecRetainPolicy("Retain")
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResourceTypes(mock.Anything, "my-org", mock.Anything).Return(&gen.ResourceTypeList{
		Items: []gen.ResourceType{
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

	rt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, rt.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "mysql")
	assert.Contains(t, out, "redis")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResourceTypes(mock.Anything, "my-org", mock.Anything).Return(&gen.ResourceTypeList{
		Items:      []gen.ResourceType{},
		Pagination: gen.Pagination{},
	}, nil)

	rt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, rt.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "No resource types found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResourceType(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("not found: missing"))

	rt := New(mc)
	assert.EqualError(t, rt.Get(GetParams{Namespace: "my-org", ResourceTypeName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResourceType(mock.Anything, "my-org", "mysql").Return(&gen.ResourceType{
		Metadata: gen.ObjectMeta{Name: "mysql"},
	}, nil)

	rt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, rt.Get(GetParams{Namespace: "my-org", ResourceTypeName: "mysql"}))
	})

	assert.Contains(t, out, "name: mysql")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteResourceType(mock.Anything, "my-org", "mysql").Return(fmt.Errorf("forbidden: mysql"))

	rt := New(mc)
	assert.EqualError(t, rt.Delete(DeleteParams{Namespace: "my-org", ResourceTypeName: "mysql"}), "forbidden: mysql")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteResourceType(mock.Anything, "my-org", "mysql").Return(nil)

	rt := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, rt.Delete(DeleteParams{Namespace: "my-org", ResourceTypeName: "mysql"}))
	})

	assert.Contains(t, out, "ResourceType 'mysql' deleted")
}
