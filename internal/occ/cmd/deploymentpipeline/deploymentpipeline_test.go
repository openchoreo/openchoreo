// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

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
	assert.Contains(t, out, "No deployment pipelines found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.DeploymentPipeline{}))
	})
	assert.Contains(t, out, "No deployment pipelines found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.DeploymentPipeline{
		{Metadata: gen.ObjectMeta{Name: "main-pipeline", CreationTimestamp: &now}},
		{Metadata: gen.ObjectMeta{Name: "hotfix-pipeline"}},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "main-pipeline")
	assert.Contains(t, out, "hotfix-pipeline")
}

func TestPrint_NilTimestamp(t *testing.T) {
	items := []gen.DeploymentPipeline{
		{Metadata: gen.ObjectMeta{Name: "no-timestamp", CreationTimestamp: nil}},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "no-timestamp")
}

// --- Params tests ---

func TestListParams_GetNamespace(t *testing.T) {
	assert.Equal(t, "my-ns", ListParams{Namespace: "my-ns"}.GetNamespace())
}

func TestGetParams_GetNamespace(t *testing.T) {
	assert.Equal(t, "my-ns", GetParams{Namespace: "my-ns"}.GetNamespace())
}

func TestDeleteParams_Getters(t *testing.T) {
	p := DeleteParams{Namespace: "my-ns", DeploymentPipelineName: "pipeline-a"}
	assert.Equal(t, "my-ns", p.GetNamespace())
	assert.Equal(t, "pipeline-a", p.GetDeploymentPipelineName())
}

// --- List tests ---

func TestList_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	dp := New(mc)
	err := dp.List(ListParams{Namespace: ""})
	assert.Error(t, err)
}

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListDeploymentPipelines(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("server error"))

	dp := New(mc)
	assert.EqualError(t, dp.List(ListParams{Namespace: "my-org"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListDeploymentPipelines(mock.Anything, "my-org", mock.Anything).Return(&gen.DeploymentPipelineList{
		Items:      []gen.DeploymentPipeline{{Metadata: gen.ObjectMeta{Name: "main-pipeline"}}},
		Pagination: gen.Pagination{},
	}, nil)

	dp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, dp.List(ListParams{Namespace: "my-org"}))
	})
	assert.Contains(t, out, "main-pipeline")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListDeploymentPipelines(mock.Anything, "my-org", mock.Anything).Return(&gen.DeploymentPipelineList{
		Items: []gen.DeploymentPipeline{
			{Metadata: gen.ObjectMeta{Name: "main-pipeline", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "hotfix-pipeline", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	dp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, dp.List(ListParams{Namespace: "my-org"}))
	})
	assert.Contains(t, out, "main-pipeline")
	assert.Contains(t, out, "hotfix-pipeline")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListDeploymentPipelines(mock.Anything, "my-org", mock.Anything).Return(&gen.DeploymentPipelineList{
		Items:      []gen.DeploymentPipeline{},
		Pagination: gen.Pagination{},
	}, nil)

	dp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, dp.List(ListParams{Namespace: "my-org"}))
	})
	assert.Contains(t, out, "No deployment pipelines found")
}

// --- Get tests ---

func TestGet_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	dp := New(mc)
	err := dp.Get(GetParams{Namespace: "", DeploymentPipelineName: "main-pipeline"})
	assert.Error(t, err)
}

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetDeploymentPipeline(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("not found: missing"))

	dp := New(mc)
	assert.EqualError(t, dp.Get(GetParams{Namespace: "my-org", DeploymentPipelineName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetDeploymentPipeline(mock.Anything, "my-org", "main-pipeline").Return(&gen.DeploymentPipeline{
		Metadata: gen.ObjectMeta{Name: "main-pipeline"},
	}, nil)

	dp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, dp.Get(GetParams{Namespace: "my-org", DeploymentPipelineName: "main-pipeline"}))
	})
	assert.Contains(t, out, "name: main-pipeline")
}

// --- Delete tests ---

func TestDelete_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	dp := New(mc)
	err := dp.Delete(DeleteParams{Namespace: "", DeploymentPipelineName: "main-pipeline"})
	assert.Error(t, err)
}

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteDeploymentPipeline(mock.Anything, "my-org", "main-pipeline").Return(fmt.Errorf("forbidden"))

	dp := New(mc)
	assert.EqualError(t, dp.Delete(DeleteParams{Namespace: "my-org", DeploymentPipelineName: "main-pipeline"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteDeploymentPipeline(mock.Anything, "my-org", "main-pipeline").Return(nil)

	dp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, dp.Delete(DeleteParams{Namespace: "my-org", DeploymentPipelineName: "main-pipeline"}))
	})
	assert.Contains(t, out, "DeploymentPipeline 'main-pipeline' deleted")
}
