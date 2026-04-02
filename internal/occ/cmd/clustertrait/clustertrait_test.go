// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// mockClient implements Client for testing.
type mockClient struct {
	listFn   func(ctx context.Context, params *gen.ListClusterTraitsParams) (*gen.ClusterTraitList, error)
	getFn    func(ctx context.Context, name string) (*gen.ClusterTrait, error)
	deleteFn func(ctx context.Context, name string) error
}

func (m *mockClient) ListClusterTraits(ctx context.Context, params *gen.ListClusterTraitsParams) (*gen.ClusterTraitList, error) {
	return m.listFn(ctx, params)
}

func (m *mockClient) GetClusterTrait(ctx context.Context, name string) (*gen.ClusterTrait, error) {
	return m.getFn(ctx, name)
}

func (m *mockClient) DeleteClusterTrait(ctx context.Context, name string) error {
	return m.deleteFn(ctx, name)
}

func newTestClusterTrait(mc *mockClient) *ClusterTrait {
	return New(mc)
}

// captureStdout captures stdout output from a function call.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	origStdout := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
		w.Close()
		r.Close()
	}()

	fn()

	os.Stdout = origStdout
	w.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	return buf.String()
}

func TestPrint_Nil(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList(nil))
	})
	assert.Contains(t, out, "No cluster traits found")
}

func TestPrint_Empty(t *testing.T) {
	out := captureStdout(t, func() {
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

	out := captureStdout(t, func() {
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

	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "no-timestamp")
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := &mockClient{
		listFn: func(_ context.Context, _ *gen.ListClusterTraitsParams) (*gen.ClusterTraitList, error) {
			return nil, fmt.Errorf("server error")
		},
	}
	ct := newTestClusterTrait(mc)
	assert.EqualError(t, ct.List(), "server error")
}

func TestList_Success(t *testing.T) {
	mc := &mockClient{
		listFn: func(_ context.Context, _ *gen.ListClusterTraitsParams) (*gen.ClusterTraitList, error) {
			return &gen.ClusterTraitList{
				Items: []gen.ClusterTrait{
					{Metadata: gen.ObjectMeta{Name: "ingress"}},
				},
				Pagination: gen.Pagination{},
			}, nil
		},
	}
	ct := newTestClusterTrait(mc)

	out := captureStdout(t, func() {
		require.NoError(t, ct.List())
	})

	assert.Contains(t, out, "ingress")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := &mockClient{
		listFn: func(_ context.Context, _ *gen.ListClusterTraitsParams) (*gen.ClusterTraitList, error) {
			return &gen.ClusterTraitList{
				Items: []gen.ClusterTrait{
					{Metadata: gen.ObjectMeta{Name: "ingress", CreationTimestamp: &now}},
					{Metadata: gen.ObjectMeta{Name: "observability-alert-rule", CreationTimestamp: &now}},
				},
				Pagination: gen.Pagination{},
			}, nil
		},
	}
	ct := newTestClusterTrait(mc)

	out := captureStdout(t, func() {
		require.NoError(t, ct.List())
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "ingress")
	assert.Contains(t, out, "observability-alert-rule")
}

func TestList_Empty(t *testing.T) {
	mc := &mockClient{
		listFn: func(_ context.Context, _ *gen.ListClusterTraitsParams) (*gen.ClusterTraitList, error) {
			return &gen.ClusterTraitList{
				Items:      []gen.ClusterTrait{},
				Pagination: gen.Pagination{},
			}, nil
		},
	}
	ct := newTestClusterTrait(mc)

	out := captureStdout(t, func() {
		require.NoError(t, ct.List())
	})

	assert.Contains(t, out, "No cluster traits found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := &mockClient{
		getFn: func(_ context.Context, name string) (*gen.ClusterTrait, error) {
			return nil, fmt.Errorf("not found: %s", name)
		},
	}
	ct := newTestClusterTrait(mc)
	assert.EqualError(t, ct.Get(GetParams{ClusterTraitName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := &mockClient{
		getFn: func(_ context.Context, name string) (*gen.ClusterTrait, error) {
			return &gen.ClusterTrait{
				Metadata: gen.ObjectMeta{Name: name},
			}, nil
		},
	}
	ct := newTestClusterTrait(mc)

	out := captureStdout(t, func() {
		require.NoError(t, ct.Get(GetParams{ClusterTraitName: "ingress"}))
	})

	assert.Contains(t, out, "name: ingress")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := &mockClient{
		deleteFn: func(_ context.Context, name string) error {
			return fmt.Errorf("forbidden: %s", name)
		},
	}
	ct := newTestClusterTrait(mc)
	assert.EqualError(t, ct.Delete(DeleteParams{ClusterTraitName: "ingress"}), "forbidden: ingress")
}

func TestDelete_Success(t *testing.T) {
	mc := &mockClient{
		deleteFn: func(_ context.Context, _ string) error {
			return nil
		},
	}
	ct := newTestClusterTrait(mc)

	out := captureStdout(t, func() {
		require.NoError(t, ct.Delete(DeleteParams{ClusterTraitName: "ingress"}))
	})

	assert.Contains(t, out, "ClusterTrait 'ingress' deleted")
}
