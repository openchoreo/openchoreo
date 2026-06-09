// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

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
	assert.Contains(t, out, "No traits found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.Trait{}))
	})
	assert.Contains(t, out, "No traits found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.Trait{
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
	items := []gen.Trait{
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

// --- Validation tests ---

func TestList_ValidationError(t *testing.T) {
	tr := New(mocks.NewMockInterface(t))
	assert.Error(t, tr.List(ListParams{Namespace: ""}))
}

func TestGet_ValidationError(t *testing.T) {
	tr := New(mocks.NewMockInterface(t))
	assert.Error(t, tr.Get(GetParams{Namespace: ""}))
}

func TestDelete_ValidationError(t *testing.T) {
	tr := New(mocks.NewMockInterface(t))
	assert.Error(t, tr.Delete(DeleteParams{Namespace: "my-org", TraitName: ""}))
}

// --- Params tests ---

func TestListParams_GetNamespace(t *testing.T) {
	assert.Equal(t, "my-ns", ListParams{Namespace: "my-ns"}.GetNamespace())
}

func TestGetParams_GetNamespace(t *testing.T) {
	assert.Equal(t, "my-ns", GetParams{Namespace: "my-ns"}.GetNamespace())
}

func TestDeleteParams_Getters(t *testing.T) {
	p := DeleteParams{Namespace: "my-ns", TraitName: "trait-a"}
	assert.Equal(t, "my-ns", p.GetNamespace())
	assert.Equal(t, "trait-a", p.GetTraitName())
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListTraits(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("server error"))

	tr := New(mc)
	assert.EqualError(t, tr.List(ListParams{Namespace: "my-org"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListTraits(mock.Anything, "my-org", mock.Anything).Return(&gen.TraitList{
		Items:      []gen.Trait{{Metadata: gen.ObjectMeta{Name: "ingress"}}},
		Pagination: gen.Pagination{},
	}, nil)

	tr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, tr.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "ingress")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListTraits(mock.Anything, "my-org", mock.Anything).Return(&gen.TraitList{
		Items: []gen.Trait{
			{Metadata: gen.ObjectMeta{Name: "ingress", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "storage", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	tr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, tr.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "ingress")
	assert.Contains(t, out, "storage")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListTraits(mock.Anything, "my-org", mock.Anything).Return(&gen.TraitList{
		Items:      []gen.Trait{},
		Pagination: gen.Pagination{},
	}, nil)

	tr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, tr.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "No traits found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetTrait(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("not found: missing"))

	tr := New(mc)
	assert.EqualError(t, tr.Get(GetParams{Namespace: "my-org", TraitName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetTrait(mock.Anything, "my-org", "ingress").Return(&gen.Trait{
		Metadata: gen.ObjectMeta{Name: "ingress"},
	}, nil)

	tr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, tr.Get(GetParams{Namespace: "my-org", TraitName: "ingress"}))
	})

	assert.Contains(t, out, "name: ingress")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteTrait(mock.Anything, "my-org", "ingress").Return(fmt.Errorf("forbidden: ingress"))

	tr := New(mc)
	assert.EqualError(t, tr.Delete(DeleteParams{Namespace: "my-org", TraitName: "ingress"}), "forbidden: ingress")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteTrait(mock.Anything, "my-org", "ingress").Return(nil)

	tr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, tr.Delete(DeleteParams{Namespace: "my-org", TraitName: "ingress"}))
	})

	assert.Contains(t, out, "Trait 'ingress' deleted")
}
