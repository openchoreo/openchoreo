// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcerelease

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

func releaseFor(name string) gen.ResourceRelease {
	r := gen.ResourceRelease{
		Metadata: gen.ObjectMeta{Name: name},
		Spec:     &gen.ResourceReleaseSpec{},
	}
	r.Spec.Owner.ResourceName = "analytics-db"
	return r
}

// --- printList tests ---

func TestPrint_Nil(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(nil))
	})
	assert.Contains(t, out, "No resource releases found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.ResourceRelease{}))
	})
	assert.Contains(t, out, "No resource releases found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.ResourceRelease{
		releaseFor("analytics-db-abc123"),
		releaseFor("analytics-db-def456"),
	}
	items[0].Metadata.CreationTimestamp = &now

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "RESOURCE")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "analytics-db-abc123")
	assert.Contains(t, out, "analytics-db")
}

func TestPrint_NilSpec(t *testing.T) {
	now := time.Now()
	items := []gen.ResourceRelease{
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
	rr := New(mocks.NewMockInterface(t))
	assert.Error(t, rr.List(ListParams{Namespace: ""}))
}

func TestGet_ValidationError(t *testing.T) {
	rr := New(mocks.NewMockInterface(t))
	assert.Error(t, rr.Get(GetParams{Namespace: ""}))
}

func TestDelete_ValidationError(t *testing.T) {
	rr := New(mocks.NewMockInterface(t))
	assert.Error(t, rr.Delete(DeleteParams{Namespace: "my-org", ResourceReleaseName: ""}))
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResourceReleases(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("server error"))

	rr := New(mc)
	assert.EqualError(t, rr.List(ListParams{Namespace: "my-org"}), "server error")
}

func TestList_Success_NoResourceFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResourceReleases(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListResourceReleasesParams) bool {
		return p.Resource == nil
	})).Return(&gen.ResourceReleaseList{
		Items:      []gen.ResourceRelease{releaseFor("analytics-db-abc123")},
		Pagination: gen.Pagination{},
	}, nil)

	rr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, rr.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "analytics-db-abc123")
}

func TestList_Success_WithResourceFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResourceReleases(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListResourceReleasesParams) bool {
		return p.Resource != nil && *p.Resource == "analytics-db"
	})).Return(&gen.ResourceReleaseList{
		Items:      []gen.ResourceRelease{releaseFor("analytics-db-abc123")},
		Pagination: gen.Pagination{},
	}, nil)

	rr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, rr.List(ListParams{Namespace: "my-org", Resource: "analytics-db"}))
	})

	assert.Contains(t, out, "analytics-db-abc123")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResourceReleases(mock.Anything, "my-org", mock.Anything).Return(&gen.ResourceReleaseList{
		Items:      []gen.ResourceRelease{},
		Pagination: gen.Pagination{},
	}, nil)

	rr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, rr.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "No resource releases found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResourceRelease(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("not found"))

	rr := New(mc)
	assert.EqualError(t, rr.Get(GetParams{Namespace: "my-org", ResourceReleaseName: "missing"}), "not found")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResourceRelease(mock.Anything, "my-org", "analytics-db-abc123").Return(&gen.ResourceRelease{
		Metadata: gen.ObjectMeta{Name: "analytics-db-abc123"},
	}, nil)

	rr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, rr.Get(GetParams{Namespace: "my-org", ResourceReleaseName: "analytics-db-abc123"}))
	})

	assert.Contains(t, out, "name: analytics-db-abc123")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteResourceRelease(mock.Anything, "my-org", "analytics-db-abc123").Return(fmt.Errorf("forbidden"))

	rr := New(mc)
	assert.EqualError(t, rr.Delete(DeleteParams{Namespace: "my-org", ResourceReleaseName: "analytics-db-abc123"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteResourceRelease(mock.Anything, "my-org", "analytics-db-abc123").Return(nil)

	rr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, rr.Delete(DeleteParams{Namespace: "my-org", ResourceReleaseName: "analytics-db-abc123"}))
	})

	assert.Contains(t, out, "ResourceRelease 'analytics-db-abc123' deleted")
}
