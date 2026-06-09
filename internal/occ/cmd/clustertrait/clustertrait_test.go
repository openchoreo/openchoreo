// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

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
	assert.Contains(t, out, "No cluster traits found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.ClusterTrait{}))
	})
	assert.Contains(t, out, "No cluster traits found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.ClusterTrait{
		{
			Metadata: gen.ObjectMeta{
				Name:              "ingress",
				CreationTimestamp: &now,
			},
		},
		{
			Metadata: gen.ObjectMeta{
				Name: "storage",
			},
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "ingress")
	assert.Contains(t, out, "storage")
}

func TestPrint_NilTimestamp(t *testing.T) {
	items := []gen.ClusterTrait{
		{
			Metadata: gen.ObjectMeta{
				Name:              "no-timestamp",
				CreationTimestamp: nil,
			},
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "no-timestamp")
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListClusterTraits(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("server error"))

	ct := New(mc)
	assert.EqualError(t, ct.List(), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListClusterTraits(mock.Anything, mock.Anything).Return(&gen.ClusterTraitList{
		Items:      []gen.ClusterTrait{{Metadata: gen.ObjectMeta{Name: "ingress"}}},
		Pagination: gen.Pagination{},
	}, nil)

	ct := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, ct.List())
	})

	assert.Contains(t, out, "ingress")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListClusterTraits(mock.Anything, mock.Anything).Return(&gen.ClusterTraitList{
		Items: []gen.ClusterTrait{
			{Metadata: gen.ObjectMeta{Name: "ingress", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "observability-alert-rule", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	ct := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, ct.List())
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "ingress")
	assert.Contains(t, out, "observability-alert-rule")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListClusterTraits(mock.Anything, mock.Anything).Return(&gen.ClusterTraitList{
		Items:      []gen.ClusterTrait{},
		Pagination: gen.Pagination{},
	}, nil)

	ct := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, ct.List())
	})

	assert.Contains(t, out, "No cluster traits found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterTrait(mock.Anything, "missing").Return(nil, fmt.Errorf("not found: missing"))

	ct := New(mc)
	assert.EqualError(t, ct.Get(GetParams{ClusterTraitName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterTrait(mock.Anything, "ingress").Return(&gen.ClusterTrait{
		Metadata: gen.ObjectMeta{Name: "ingress"},
	}, nil)

	ct := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, ct.Get(GetParams{ClusterTraitName: "ingress"}))
	})

	assert.Contains(t, out, "name: ingress")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteClusterTrait(mock.Anything, "ingress").Return(fmt.Errorf("forbidden: ingress"))

	ct := New(mc)
	assert.EqualError(t, ct.Delete(DeleteParams{ClusterTraitName: "ingress"}), "forbidden: ingress")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteClusterTrait(mock.Anything, "ingress").Return(nil)

	ct := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, ct.Delete(DeleteParams{ClusterTraitName: "ingress"}))
	})

	assert.Contains(t, out, "ClusterTrait 'ingress' deleted")
}
