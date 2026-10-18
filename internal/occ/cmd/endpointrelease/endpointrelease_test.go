// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointrelease

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

func releaseFor(name string) gen.EndpointRelease {
	r := gen.EndpointRelease{
		Metadata: gen.ObjectMeta{Name: name},
		Spec:     &gen.EndpointReleaseSpec{},
	}
	r.Spec.Owner.EndpointName = "orders-api"
	return r
}

// --- printList tests ---

func TestPrint_Nil(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(nil))
	})
	assert.Contains(t, out, "No endpoint releases found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.EndpointRelease{}))
	})
	assert.Contains(t, out, "No endpoint releases found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.EndpointRelease{
		releaseFor("orders-api-abc123"),
		releaseFor("orders-api-def456"),
	}
	items[0].Metadata.CreationTimestamp = &now

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "ENDPOINT")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "orders-api-abc123")
	assert.Contains(t, out, "orders-api")
}

func TestPrint_NilSpec(t *testing.T) {
	now := time.Now()
	items := []gen.EndpointRelease{
		{
			Metadata: gen.ObjectMeta{Name: "no-spec", CreationTimestamp: &now},
			Spec:     nil,
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "no-spec")
}

// --- Validation tests ---

func TestList_ValidationError(t *testing.T) {
	er := New(mocks.NewMockInterface(t))
	assert.Error(t, er.List(ListParams{Namespace: ""}))
}

func TestGet_ValidationError(t *testing.T) {
	er := New(mocks.NewMockInterface(t))
	assert.Error(t, er.Get(GetParams{Namespace: ""}))
}

func TestDelete_ValidationError(t *testing.T) {
	er := New(mocks.NewMockInterface(t))
	assert.Error(t, er.Delete(DeleteParams{Namespace: "my-org", EndpointReleaseName: ""}))
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListEndpointReleases(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("server error"))

	er := New(mc)
	assert.EqualError(t, er.List(ListParams{Namespace: "my-org"}), "server error")
}

func TestList_Success_NoEndpointFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListEndpointReleases(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListEndpointReleasesParams) bool {
		return p.Endpoint == nil
	})).Return(&gen.EndpointReleaseList{
		Items:      []gen.EndpointRelease{releaseFor("orders-api-abc123")},
		Pagination: gen.Pagination{},
	}, nil)

	er := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, er.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "orders-api-abc123")
}

func TestList_Success_WithEndpointFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListEndpointReleases(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListEndpointReleasesParams) bool {
		return p.Endpoint != nil && *p.Endpoint == "orders-api"
	})).Return(&gen.EndpointReleaseList{
		Items:      []gen.EndpointRelease{releaseFor("orders-api-abc123")},
		Pagination: gen.Pagination{},
	}, nil)

	er := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, er.List(ListParams{Namespace: "my-org", Endpoint: "orders-api"}))
	})

	assert.Contains(t, out, "orders-api-abc123")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListEndpointReleases(mock.Anything, "my-org", mock.Anything).Return(&gen.EndpointReleaseList{
		Items:      []gen.EndpointRelease{},
		Pagination: gen.Pagination{},
	}, nil)

	er := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, er.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "No endpoint releases found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpointRelease(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("not found"))

	er := New(mc)
	assert.EqualError(t, er.Get(GetParams{Namespace: "my-org", EndpointReleaseName: "missing"}), "not found")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpointRelease(mock.Anything, "my-org", "orders-api-abc123").Return(&gen.EndpointRelease{
		Metadata: gen.ObjectMeta{Name: "orders-api-abc123"},
	}, nil)

	er := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, er.Get(GetParams{Namespace: "my-org", EndpointReleaseName: "orders-api-abc123"}))
	})

	assert.Contains(t, out, "name: orders-api-abc123")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteEndpointRelease(mock.Anything, "my-org", "orders-api-abc123").Return(fmt.Errorf("forbidden"))

	er := New(mc)
	assert.EqualError(t, er.Delete(DeleteParams{Namespace: "my-org", EndpointReleaseName: "orders-api-abc123"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteEndpointRelease(mock.Anything, "my-org", "orders-api-abc123").Return(nil)

	er := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, er.Delete(DeleteParams{Namespace: "my-org", EndpointReleaseName: "orders-api-abc123"}))
	})

	assert.Contains(t, out, "EndpointRelease 'orders-api-abc123' deleted")
}
