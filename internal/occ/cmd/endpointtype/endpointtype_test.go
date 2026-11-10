// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointtype

import (
	"encoding/json"
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
	assert.Contains(t, out, "No endpoint types found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.EndpointType{}))
	})
	assert.Contains(t, out, "No endpoint types found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.EndpointType{
		{
			Metadata: gen.ObjectMeta{
				Name:              "public-http",
				CreationTimestamp: &now,
			},
			Spec: specWithResources(t, "httproute"),
		},
		{
			Metadata: gen.ObjectMeta{
				Name: "internal-grpc",
			},
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "RESOURCES")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "public-http")
	assert.Contains(t, out, "httproute")
	assert.Contains(t, out, "internal-grpc")
}

func TestPrint_NilSpec(t *testing.T) {
	now := time.Now()
	items := []gen.EndpointType{
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
	et := New(mocks.NewMockInterface(t))
	assert.Error(t, et.List(ListParams{Namespace: ""}))
}

func TestGet_ValidationError(t *testing.T) {
	et := New(mocks.NewMockInterface(t))
	assert.Error(t, et.Get(GetParams{Namespace: ""}))
}

func TestDelete_ValidationError(t *testing.T) {
	et := New(mocks.NewMockInterface(t))
	assert.Error(t, et.Delete(DeleteParams{Namespace: "my-org", EndpointTypeName: ""}))
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListEndpointTypes(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("server error"))

	et := New(mc)
	assert.EqualError(t, et.List(ListParams{Namespace: "my-org"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListEndpointTypes(mock.Anything, "my-org", mock.Anything).Return(&gen.EndpointTypeList{
		Items:      []gen.EndpointType{{Metadata: gen.ObjectMeta{Name: "public-http"}}},
		Pagination: gen.Pagination{},
	}, nil)

	et := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, et.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "public-http")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListEndpointTypes(mock.Anything, "my-org", mock.Anything).Return(&gen.EndpointTypeList{
		Items: []gen.EndpointType{
			{
				Metadata: gen.ObjectMeta{Name: "public-http", CreationTimestamp: &now},
				Spec:     specWithResources(t, "httproute"),
			},
			{
				Metadata: gen.ObjectMeta{Name: "internal-grpc", CreationTimestamp: &now},
			},
		},
		Pagination: gen.Pagination{},
	}, nil)

	et := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, et.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "public-http")
	assert.Contains(t, out, "internal-grpc")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListEndpointTypes(mock.Anything, "my-org", mock.Anything).Return(&gen.EndpointTypeList{
		Items:      []gen.EndpointType{},
		Pagination: gen.Pagination{},
	}, nil)

	et := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, et.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "No endpoint types found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpointType(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("not found: missing"))

	et := New(mc)
	assert.EqualError(t, et.Get(GetParams{Namespace: "my-org", EndpointTypeName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpointType(mock.Anything, "my-org", "public-http").Return(&gen.EndpointType{
		Metadata: gen.ObjectMeta{Name: "public-http"},
	}, nil)

	et := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, et.Get(GetParams{Namespace: "my-org", EndpointTypeName: "public-http"}))
	})

	assert.Contains(t, out, "name: public-http")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteEndpointType(mock.Anything, "my-org", "public-http").Return(fmt.Errorf("forbidden: public-http"))

	et := New(mc)
	assert.EqualError(t, et.Delete(DeleteParams{Namespace: "my-org", EndpointTypeName: "public-http"}), "forbidden: public-http")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteEndpointType(mock.Anything, "my-org", "public-http").Return(nil)

	et := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, et.Delete(DeleteParams{Namespace: "my-org", EndpointTypeName: "public-http"}))
	})

	assert.Contains(t, out, "EndpointType 'public-http' deleted")
}

// specWithResources builds an EndpointTypeSpec whose resources carry the given
// ids; Resources is an anonymous struct slice, so decode it from JSON.
func specWithResources(t *testing.T, ids ...string) *gen.EndpointTypeSpec {
	t.Helper()
	resources := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		resources = append(resources, map[string]any{"id": id, "template": map[string]any{}})
	}
	raw, err := json.Marshal(map[string]any{"resources": resources})
	require.NoError(t, err)
	spec := &gen.EndpointTypeSpec{}
	require.NoError(t, json.Unmarshal(raw, spec))
	return spec
}
