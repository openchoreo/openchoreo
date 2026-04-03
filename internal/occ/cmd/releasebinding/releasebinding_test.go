// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/releasebinding/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

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

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListReleaseBindings(mock.Anything, "ns", mock.Anything).Return(nil, fmt.Errorf("server error"))

	rb := New(mc)
	assert.EqualError(t, rb.List(ListParams{Namespace: "ns"}), "server error")
}

func TestList_Success(t *testing.T) {
	relName := "rel-1"
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListReleaseBindings(mock.Anything, "ns", mock.Anything).Return(&gen.ReleaseBindingList{
		Items: []gen.ReleaseBinding{{
			Metadata: gen.ObjectMeta{Name: "binding-1"},
			Spec:     &gen.ReleaseBindingSpec{Environment: "dev", ReleaseName: &relName},
		}},
		Pagination: gen.Pagination{},
	}, nil)

	rb := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, rb.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "binding-1")
	assert.Contains(t, out, "dev")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	rel1 := "rel-1"
	rel2 := "rel-2"
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListReleaseBindings(mock.Anything, "ns", mock.Anything).Return(&gen.ReleaseBindingList{
		Items: []gen.ReleaseBinding{
			{Metadata: gen.ObjectMeta{Name: "binding-1", CreationTimestamp: &now}, Spec: &gen.ReleaseBindingSpec{Environment: "dev", ReleaseName: &rel1}},
			{Metadata: gen.ObjectMeta{Name: "binding-2", CreationTimestamp: &now}, Spec: &gen.ReleaseBindingSpec{Environment: "staging", ReleaseName: &rel2}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	rb := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, rb.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "binding-1")
	assert.Contains(t, out, "binding-2")
	assert.Contains(t, out, "dev")
	assert.Contains(t, out, "staging")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListReleaseBindings(mock.Anything, "ns", mock.Anything).Return(&gen.ReleaseBindingList{
		Items:      []gen.ReleaseBinding{},
		Pagination: gen.Pagination{},
	}, nil)

	rb := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, rb.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "No release bindings found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetReleaseBinding(mock.Anything, "ns", "missing").Return(nil, fmt.Errorf("not found: missing"))

	rb := New(mc)
	assert.EqualError(t, rb.Get(GetParams{Namespace: "ns", ReleaseBindingName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetReleaseBinding(mock.Anything, "ns", "binding-1").Return(&gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "binding-1"},
	}, nil)

	rb := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, rb.Get(GetParams{Namespace: "ns", ReleaseBindingName: "binding-1"}))
	})

	assert.Contains(t, out, "name: binding-1")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteReleaseBinding(mock.Anything, "ns", "binding-1").Return(fmt.Errorf("forbidden: binding-1"))

	rb := New(mc)
	assert.EqualError(t, rb.Delete(DeleteParams{Namespace: "ns", ReleaseBindingName: "binding-1"}), "forbidden: binding-1")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteReleaseBinding(mock.Anything, "ns", "binding-1").Return(nil)

	rb := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, rb.Delete(DeleteParams{Namespace: "ns", ReleaseBindingName: "binding-1"}))
	})

	assert.Contains(t, out, "ReleaseBinding 'binding-1' deleted")
}
