// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

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
	assert.Contains(t, out, "No component types found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.ComponentType{}))
	})
	assert.Contains(t, out, "No component types found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	workloadType := gen.ComponentTypeSpecWorkloadTypeDeployment
	items := []gen.ComponentType{
		{
			Metadata: gen.ObjectMeta{
				Name:              "web-app",
				CreationTimestamp: &now,
			},
			Spec: &gen.ComponentTypeSpec{
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
	items := []gen.ComponentType{
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
	ct := New(mocks.NewMockInterface(t))
	assert.Error(t, ct.List(ListParams{Namespace: ""}))
}

func TestGet_ValidationError(t *testing.T) {
	ct := New(mocks.NewMockInterface(t))
	assert.Error(t, ct.Get(GetParams{Namespace: ""}))
}

func TestDelete_ValidationError(t *testing.T) {
	ct := New(mocks.NewMockInterface(t))
	assert.Error(t, ct.Delete(DeleteParams{Namespace: "my-org", ComponentTypeName: ""}))
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponentTypes(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("server error"))

	ct := New(mc)
	assert.EqualError(t, ct.List(ListParams{Namespace: "my-org"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponentTypes(mock.Anything, "my-org", mock.Anything).Return(&gen.ComponentTypeList{
		Items:      []gen.ComponentType{{Metadata: gen.ObjectMeta{Name: "web-app"}}},
		Pagination: gen.Pagination{},
	}, nil)

	ct := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, ct.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "web-app")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	workloadType := gen.ComponentTypeSpecWorkloadTypeDeployment
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponentTypes(mock.Anything, "my-org", mock.Anything).Return(&gen.ComponentTypeList{
		Items: []gen.ComponentType{
			{
				Metadata: gen.ObjectMeta{Name: "web-app", CreationTimestamp: &now},
				Spec:     &gen.ComponentTypeSpec{WorkloadType: workloadType},
			},
			{
				Metadata: gen.ObjectMeta{Name: "batch-job", CreationTimestamp: &now},
			},
		},
		Pagination: gen.Pagination{},
	}, nil)

	ct := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, ct.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "web-app")
	assert.Contains(t, out, "batch-job")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponentTypes(mock.Anything, "my-org", mock.Anything).Return(&gen.ComponentTypeList{
		Items:      []gen.ComponentType{},
		Pagination: gen.Pagination{},
	}, nil)

	ct := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, ct.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "No component types found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentType(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("not found: missing"))

	ct := New(mc)
	assert.EqualError(t, ct.Get(GetParams{Namespace: "my-org", ComponentTypeName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentType(mock.Anything, "my-org", "web-app").Return(&gen.ComponentType{
		Metadata: gen.ObjectMeta{Name: "web-app"},
	}, nil)

	ct := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, ct.Get(GetParams{Namespace: "my-org", ComponentTypeName: "web-app"}))
	})

	assert.Contains(t, out, "name: web-app")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteComponentType(mock.Anything, "my-org", "web-app").Return(fmt.Errorf("forbidden: web-app"))

	ct := New(mc)
	assert.EqualError(t, ct.Delete(DeleteParams{Namespace: "my-org", ComponentTypeName: "web-app"}), "forbidden: web-app")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteComponentType(mock.Anything, "my-org", "web-app").Return(nil)

	ct := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, ct.Delete(DeleteParams{Namespace: "my-org", ComponentTypeName: "web-app"}))
	})

	assert.Contains(t, out, "ComponentType 'web-app' deleted")
}
