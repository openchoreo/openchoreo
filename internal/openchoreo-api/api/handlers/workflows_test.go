// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	workflowmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflow/mocks"
	workflowrunsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun"
	workflowrunmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun/mocks"
)

func newWorkflowRunService(t *testing.T, objects []client.Object, pdp authzcore.PDP) workflowrunsvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return workflowrunsvc.NewServiceWithAuthz(fakeClient, nil, nil, pdp, slog.Default())
}

func newHandlerWithWorkflowRunService(svc workflowrunsvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{WorkflowRunService: svc},
		logger:   slog.Default(),
	}
}

func testWorkflowRunObj(name string) *openchoreov1alpha1.WorkflowRun {
	return &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
		Spec: openchoreov1alpha1.WorkflowRunSpec{
			Workflow: openchoreov1alpha1.WorkflowRunConfig{
				Kind: openchoreov1alpha1.WorkflowRefKindWorkflow,
				Name: "test-workflow",
			},
		},
	}
}

// --- DeleteWorkflowRun Handler ---

func TestDeleteWorkflowRunHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newWorkflowRunService(t, []client.Object{testWorkflowRunObj("run-1")}, &allowAllPDP{})
		h := newHandlerWithWorkflowRunService(svc)

		resp, err := h.DeleteWorkflowRun(ctx, gen.DeleteWorkflowRunRequestObject{NamespaceName: ns, RunName: "run-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteWorkflowRun204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newWorkflowRunService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkflowRunService(svc)

		resp, err := h.DeleteWorkflowRun(ctx, gen.DeleteWorkflowRunRequestObject{NamespaceName: ns, RunName: "nonexistent"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteWorkflowRun404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newWorkflowRunService(t, []client.Object{testWorkflowRunObj("run-1")}, &denyAllPDP{})
		h := newHandlerWithWorkflowRunService(svc)

		resp, err := h.DeleteWorkflowRun(ctx, gen.DeleteWorkflowRunRequestObject{NamespaceName: ns, RunName: "run-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteWorkflowRun403JSONResponse{}, resp)
	})
}

func TestUpdateWorkflowHandler_UsesPathName(t *testing.T) {
	ctx := testContext()
	svc := workflowmocks.NewMockService(t)
	svc.EXPECT().UpdateWorkflow(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, namespace string, wf *openchoreov1alpha1.Workflow) (*openchoreov1alpha1.Workflow, error) {
		assert.Equal(t, "test-ns", namespace)
		assert.Equal(t, "wf-from-path", wf.Name, "path must override body name")
		return wf, nil
	})
	h := &Handler{
		services: &handlerservices.Services{WorkflowService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	body := gen.Workflow{Metadata: gen.ObjectMeta{Name: "wf-from-body"}}
	resp, err := h.UpdateWorkflow(ctx, gen.UpdateWorkflowRequestObject{
		NamespaceName: "test-ns",
		WorkflowName:  "wf-from-path",
		Body:          &body,
	})
	require.NoError(t, err)
	typed, ok := resp.(gen.UpdateWorkflow200JSONResponse)
	require.True(t, ok, "expected 200 response, got %T", resp)
	assert.Equal(t, "wf-from-path", typed.Metadata.Name)
}

func TestUpdateWorkflowRunHandler_ClearsStatusAndUsesPathName(t *testing.T) {
	ctx := testContext()
	svc := workflowrunmocks.NewMockService(t)
	svc.EXPECT().UpdateWorkflowRun(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, namespace string, wfRun *openchoreov1alpha1.WorkflowRun) (*openchoreov1alpha1.WorkflowRun, error) {
		assert.Equal(t, "test-ns", namespace)
		assert.Equal(t, "run-from-path", wfRun.Name, "path must override body name")
		assert.Empty(t, wfRun.Status, "handler must clear status to avoid user-set status updates")
		return wfRun, nil
	})
	h := &Handler{
		services: &handlerservices.Services{WorkflowRunService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Config:   &config.Config{ClusterGateway: config.ClusterGatewayConfig{URL: "https://gateway.example"}},
	}

	body := gen.WorkflowRun{
		Metadata: gen.ObjectMeta{Name: "run-from-body"},
		Status: &gen.WorkflowRunStatus{
			RunReference: &gen.ResourceReference{ApiVersion: "v1", Kind: "Pod", Name: "pod-1"},
		},
	}
	resp, err := h.UpdateWorkflowRun(ctx, gen.UpdateWorkflowRunRequestObject{
		NamespaceName: "test-ns",
		RunName:       "run-from-path",
		Body:          &body,
	})
	require.NoError(t, err)
	typed, ok := resp.(gen.UpdateWorkflowRun200JSONResponse)
	require.True(t, ok, "expected 200 response, got %T", resp)
	assert.Equal(t, "run-from-path", typed.Metadata.Name)
}

func TestGetWorkflowRunLogsHandler_ParsesRFC3339AndRFC3339Nano(t *testing.T) {
	ctx := testContext()
	svc := workflowrunmocks.NewMockService(t)
	svc.EXPECT().GetWorkflowRunLogs(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, namespace, runName, taskName, gatewayURL string, sinceSeconds *int64) ([]models.WorkflowRunLogEntry, error) {
		require.Equal(t, "test-ns", namespace)
		require.Equal(t, "run-1", runName)
		require.Equal(t, "task-a", taskName)
		require.Equal(t, "https://gw", gatewayURL)
		require.NotNil(t, sinceSeconds)
		assert.Equal(t, int64(12), *sinceSeconds)
		return []models.WorkflowRunLogEntry{
			{Log: "a", Timestamp: "2026-01-02T03:04:05Z"},
			{Log: "b", Timestamp: "2026-01-02T03:04:05.123456789Z"},
			{Log: "c", Timestamp: "not-a-time"},
			{Log: "d", Timestamp: ""},
		}, nil
	})
	h := &Handler{
		services: &handlerservices.Services{WorkflowRunService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Config:   &config.Config{ClusterGateway: config.ClusterGatewayConfig{URL: "https://gw"}},
	}

	task := "task-a"
	since := int64(12)
	resp, err := h.GetWorkflowRunLogs(ctx, gen.GetWorkflowRunLogsRequestObject{
		NamespaceName: "test-ns",
		RunName:       "run-1",
		Params: gen.GetWorkflowRunLogsParams{
			Task:         &task,
			SinceSeconds: &since,
		},
	})
	require.NoError(t, err)

	typed, ok := resp.(gen.GetWorkflowRunLogs200JSONResponse)
	require.True(t, ok, "expected 200 response, got %T", resp)
	require.Len(t, typed, 4)

	require.NotNil(t, typed[0].Timestamp)
	assert.Equal(t, "2026-01-02T03:04:05Z", typed[0].Timestamp.Format(time.RFC3339))
	require.NotNil(t, typed[1].Timestamp)
	assert.Equal(t, "2026-01-02T03:04:05.123456789Z", typed[1].Timestamp.Format(time.RFC3339Nano))
	assert.Nil(t, typed[2].Timestamp)
	assert.Nil(t, typed[3].Timestamp)
}

func TestGetWorkflowRunEventsHandler_InvalidTimestampDoesNotPanic(t *testing.T) {
	ctx := testContext()
	svc := workflowrunmocks.NewMockService(t)
	svc.EXPECT().GetWorkflowRunEvents(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, namespace, runName, taskName, gatewayURL string) ([]models.WorkflowRunEventEntry, error) {
		return []models.WorkflowRunEventEntry{
			{Timestamp: "not-a-time", Type: "Warning", Reason: "X", Message: "bad ts"},
			{Timestamp: "2026-01-02T03:04:05Z", Type: "Normal", Reason: "Y", Message: "good ts"},
		}, nil
	})
	h := &Handler{
		services: &handlerservices.Services{WorkflowRunService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Config:   &config.Config{ClusterGateway: config.ClusterGatewayConfig{URL: "https://gw"}},
	}

	resp, err := h.GetWorkflowRunEvents(ctx, gen.GetWorkflowRunEventsRequestObject{
		NamespaceName: "test-ns",
		RunName:       "run-1",
	})
	require.NoError(t, err)

	typed, ok := resp.(gen.GetWorkflowRunEvents200JSONResponse)
	require.True(t, ok, "expected 200 response, got %T", resp)
	require.Len(t, typed, 2)
	// First timestamp should be zero value, but entry still returned (avoids dropping events).
	assert.True(t, typed[0].Timestamp.IsZero())
	assert.False(t, typed[1].Timestamp.IsZero())
}

func TestListWorkflowRunsHandler_ForwardsWorkflowFilter(t *testing.T) {
	ctx := testContext()
	svc := workflowrunmocks.NewMockService(t)
	svc.EXPECT().ListWorkflowRuns(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, namespace, project, component, workflow string, _ svcpkg.ListOptions) (*svcpkg.ListResult[openchoreov1alpha1.WorkflowRun], error) {
		assert.Equal(t, "test-ns", namespace)
		assert.Equal(t, "", project)
		assert.Equal(t, "", component)
		assert.Equal(t, "wf-a", workflow)
		return &svcpkg.ListResult[openchoreov1alpha1.WorkflowRun]{Items: nil}, nil
	})
	h := &Handler{
		services: &handlerservices.Services{WorkflowRunService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Config:   &config.Config{ClusterGateway: config.ClusterGatewayConfig{URL: "https://gw"}},
	}

	wf := "wf-a"
	resp, err := h.ListWorkflowRuns(ctx, gen.ListWorkflowRunsRequestObject{
		NamespaceName: "test-ns",
		Params:        gen.ListWorkflowRunsParams{Workflow: &wf},
	})
	require.NoError(t, err)
	assert.IsType(t, gen.ListWorkflowRuns200JSONResponse{}, resp)
}
