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

const testReleaseName = "rel-1"

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListReleaseBindings(mock.Anything, "ns", mock.Anything).Return(nil, fmt.Errorf("server error"))

	rb := New(mc)
	assert.EqualError(t, rb.List(ListParams{Namespace: "ns"}), "server error")
}

func TestList_Success(t *testing.T) {
	relName := testReleaseName
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
	rel1 := testReleaseName
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

// --- Validation error tests ---

func TestList_ValidationError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	rb := New(mc)
	err := rb.List(ListParams{Namespace: ""})
	assert.ErrorContains(t, err, "Missing required parameter")
}

func TestGet_ValidationError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	rb := New(mc)
	err := rb.Get(GetParams{Namespace: "", ReleaseBindingName: "binding-1"})
	assert.ErrorContains(t, err, "Missing required parameter")
}

func TestDelete_ValidationError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	rb := New(mc)
	err := rb.Delete(DeleteParams{Namespace: "", ReleaseBindingName: "binding-1"})
	assert.ErrorContains(t, err, "Missing required parameter")
}

func TestDelete_ValidationError_MissingName(t *testing.T) {
	mc := mocks.NewMockClient(t)
	rb := New(mc)
	err := rb.Delete(DeleteParams{Namespace: "ns", ReleaseBindingName: ""})
	assert.ErrorContains(t, err, "Missing required parameter")
}

// --- Constructor test ---

func TestNew(t *testing.T) {
	mc := mocks.NewMockClient(t)
	rb := New(mc)
	assert.NotNil(t, rb)
	assert.Equal(t, mc, rb.client)
}

// --- printReleaseBindings pure function tests ---

func TestPrintReleaseBindings_Nil(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printReleaseBindings(nil))
	})
	assert.Contains(t, out, "No release bindings found")
}

func TestPrintReleaseBindings_NilTimestamp(t *testing.T) {
	items := []gen.ReleaseBinding{
		{Metadata: gen.ObjectMeta{Name: "binding-no-ts"}, Spec: &gen.ReleaseBindingSpec{Environment: "dev"}},
	}
	out := captureStdout(t, func() {
		require.NoError(t, printReleaseBindings(items))
	})
	assert.Contains(t, out, "binding-no-ts")
	assert.Contains(t, out, "dev")
}

func TestPrintReleaseBindings_NilSpec(t *testing.T) {
	items := []gen.ReleaseBinding{
		{Metadata: gen.ObjectMeta{Name: "binding-nil-spec"}, Spec: nil},
	}
	out := captureStdout(t, func() {
		require.NoError(t, printReleaseBindings(items))
	})
	assert.Contains(t, out, "binding-nil-spec")
}

func TestPrintReleaseBindings_NilStatus(t *testing.T) {
	items := []gen.ReleaseBinding{
		{
			Metadata: gen.ObjectMeta{Name: "binding-nil-status"},
			Spec:     &gen.ReleaseBindingSpec{Environment: "dev"},
			Status:   nil,
		},
	}
	out := captureStdout(t, func() {
		require.NoError(t, printReleaseBindings(items))
	})
	assert.Contains(t, out, "binding-nil-status")
	assert.Contains(t, out, "dev")
}

func TestPrintReleaseBindings_StatusWithoutReadyCondition(t *testing.T) {
	conds := []gen.Condition{
		{Type: "Progressing", Reason: "InProgress", LastTransitionTime: time.Now(), Status: "True"},
	}
	items := []gen.ReleaseBinding{
		{
			Metadata: gen.ObjectMeta{Name: "binding-no-ready"},
			Spec:     &gen.ReleaseBindingSpec{Environment: "dev"},
			Status:   &gen.ReleaseBindingStatus{Conditions: &conds},
		},
	}
	out := captureStdout(t, func() {
		require.NoError(t, printReleaseBindings(items))
	})
	assert.Contains(t, out, "binding-no-ready")
	// Status column should be empty since no Ready condition exists
	assert.NotContains(t, out, "InProgress")
}

func TestPrintReleaseBindings_WithReadyCondition(t *testing.T) {
	relName := testReleaseName
	now := time.Now()
	conds := []gen.Condition{
		{Type: "Ready", Reason: "Available", LastTransitionTime: now, Status: "True"},
	}
	items := []gen.ReleaseBinding{
		{
			Metadata: gen.ObjectMeta{Name: "binding-ready", CreationTimestamp: &now},
			Spec:     &gen.ReleaseBindingSpec{Environment: "dev", ReleaseName: &relName},
			Status:   &gen.ReleaseBindingStatus{Conditions: &conds},
		},
	}
	out := captureStdout(t, func() {
		require.NoError(t, printReleaseBindings(items))
	})
	assert.Contains(t, out, "binding-ready")
	assert.Contains(t, out, "dev")
	assert.Contains(t, out, testReleaseName)
	assert.Contains(t, out, "Available")
}

func TestPrintReleaseBindings_NilReleaseName(t *testing.T) {
	items := []gen.ReleaseBinding{
		{
			Metadata: gen.ObjectMeta{Name: "binding-nil-rel"},
			Spec:     &gen.ReleaseBindingSpec{Environment: "staging", ReleaseName: nil},
		},
	}
	out := captureStdout(t, func() {
		require.NoError(t, printReleaseBindings(items))
	})
	assert.Contains(t, out, "binding-nil-rel")
	assert.Contains(t, out, "staging")
}

func TestPrintReleaseBindings_EmptyConditions(t *testing.T) {
	conds := []gen.Condition{}
	items := []gen.ReleaseBinding{
		{
			Metadata: gen.ObjectMeta{Name: "binding-empty-conds"},
			Spec:     &gen.ReleaseBindingSpec{Environment: "dev"},
			Status:   &gen.ReleaseBindingStatus{Conditions: &conds},
		},
	}
	out := captureStdout(t, func() {
		require.NoError(t, printReleaseBindings(items))
	})
	assert.Contains(t, out, "binding-empty-conds")
}

// --- List with component filter ---

func TestList_WithComponentFilter(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListReleaseBindings(mock.Anything, "ns", mock.MatchedBy(func(p *gen.ListReleaseBindingsParams) bool {
		return p.Component != nil && *p.Component == "my-comp"
	})).Return(&gen.ReleaseBindingList{
		Items:      []gen.ReleaseBinding{{Metadata: gen.ObjectMeta{Name: "binding-1"}}},
		Pagination: gen.Pagination{},
	}, nil)

	rb := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, rb.List(ListParams{Namespace: "ns", Component: "my-comp"}))
	})
	assert.Contains(t, out, "binding-1")
}

// --- Pagination ---

func TestList_Pagination(t *testing.T) {
	next := "cursor-2"
	mc := mocks.NewMockClient(t)

	// First page — no cursor
	mc.EXPECT().ListReleaseBindings(mock.Anything, "ns", mock.MatchedBy(func(p *gen.ListReleaseBindingsParams) bool {
		return p.Cursor == nil
	})).Return(&gen.ReleaseBindingList{
		Items:      []gen.ReleaseBinding{{Metadata: gen.ObjectMeta{Name: "binding-page1"}}},
		Pagination: gen.Pagination{NextCursor: &next},
	}, nil).Once()

	// Second page — with cursor
	mc.EXPECT().ListReleaseBindings(mock.Anything, "ns", mock.MatchedBy(func(p *gen.ListReleaseBindingsParams) bool {
		return p.Cursor != nil && *p.Cursor == "cursor-2"
	})).Return(&gen.ReleaseBindingList{
		Items:      []gen.ReleaseBinding{{Metadata: gen.ObjectMeta{Name: "binding-page2"}}},
		Pagination: gen.Pagination{},
	}, nil).Once()

	rb := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, rb.List(ListParams{Namespace: "ns"}))
	})
	assert.Contains(t, out, "binding-page1")
	assert.Contains(t, out, "binding-page2")
}

func TestList_NilTimestamp(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListReleaseBindings(mock.Anything, "ns", mock.Anything).Return(&gen.ReleaseBindingList{
		Items: []gen.ReleaseBinding{
			{Metadata: gen.ObjectMeta{Name: "binding-no-ts", CreationTimestamp: nil}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	rb := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, rb.List(ListParams{Namespace: "ns"}))
	})
	assert.Contains(t, out, "binding-no-ts")
}
