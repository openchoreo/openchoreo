// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

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

	"github.com/openchoreo/openchoreo/internal/occ/cmd/project/mocks"
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

// --- printList tests ---

func TestPrintList_Nil(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList(nil))
	})
	assert.Contains(t, out, "No projects found")
}

func TestPrintList_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList([]gen.Project{}))
	})
	assert.Contains(t, out, "No projects found")
}

func TestPrintList_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.Project{
		{Metadata: gen.ObjectMeta{Name: "proj-a", CreationTimestamp: &now}},
		{Metadata: gen.ObjectMeta{Name: "proj-b"}},
	}
	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "proj-a")
	assert.Contains(t, out, "proj-b")
}

// --- List tests ---

func TestList_ValidationError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	p := New(mc)
	err := p.List(ListParams{Namespace: ""})
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListProjects(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("server error"))

	p := New(mc)
	assert.EqualError(t, p.List(ListParams{Namespace: "org-a"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListProjects(mock.Anything, "org-a", mock.Anything).Return(&gen.ProjectList{
		Items:      []gen.Project{{Metadata: gen.ObjectMeta{Name: "proj-a"}}},
		Pagination: gen.Pagination{},
	}, nil)

	p := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, p.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "proj-a")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListProjects(mock.Anything, "org-a", mock.Anything).Return(&gen.ProjectList{
		Items: []gen.Project{
			{Metadata: gen.ObjectMeta{Name: "proj-a", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "proj-b", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	p := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, p.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "proj-a")
	assert.Contains(t, out, "proj-b")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListProjects(mock.Anything, "org-a", mock.Anything).Return(&gen.ProjectList{
		Items:      []gen.Project{},
		Pagination: gen.Pagination{},
	}, nil)

	p := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, p.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "No projects found")
}

// --- Get tests ---

func TestGet_ValidationError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	p := New(mc)
	err := p.Get(GetParams{Namespace: "", ProjectName: "proj-a"})
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetProject(mock.Anything, "org-a", "missing").Return(nil, fmt.Errorf("not found: missing"))

	p := New(mc)
	assert.EqualError(t, p.Get(GetParams{Namespace: "org-a", ProjectName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetProject(mock.Anything, "org-a", "proj-a").Return(&gen.Project{
		Metadata: gen.ObjectMeta{Name: "proj-a"},
	}, nil)

	p := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, p.Get(GetParams{Namespace: "org-a", ProjectName: "proj-a"}))
	})
	assert.Contains(t, out, "name: proj-a")
}

// --- Delete tests ---

func TestDelete_ValidationError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	p := New(mc)
	err := p.Delete(DeleteParams{Namespace: "", ProjectName: "proj-a"})
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteProject(mock.Anything, "org-a", "proj-a").Return(fmt.Errorf("forbidden"))

	p := New(mc)
	assert.EqualError(t, p.Delete(DeleteParams{Namespace: "org-a", ProjectName: "proj-a"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteProject(mock.Anything, "org-a", "proj-a").Return(nil)

	p := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, p.Delete(DeleteParams{Namespace: "org-a", ProjectName: "proj-a"}))
	})
	assert.Contains(t, out, "Project 'proj-a' deleted")
}
