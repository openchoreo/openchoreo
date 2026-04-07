// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	workflowsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflow"
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

func testWorkflowRunObj() *openchoreov1alpha1.WorkflowRun {
	return &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "run-1",
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
		svc := newWorkflowRunService(t, []client.Object{testWorkflowRunObj()}, &allowAllPDP{})
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
		svc := newWorkflowRunService(t, []client.Object{testWorkflowRunObj()}, &denyAllPDP{})
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

// --- helpers for the additional test coverage below ---

func newWorkflowService(t *testing.T, objects []client.Object, pdp authzcore.PDP) workflowsvc.Service {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	return workflowsvc.NewServiceWithAuthz(fakeClient, pdp, slog.Default())
}

func newHandlerWithWorkflowService(svc workflowsvc.Service) *Handler {
	return &Handler{
		services: &handlerservices.Services{WorkflowService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func testWorkflowObj(name string) *openchoreov1alpha1.Workflow {
	return &openchoreov1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
		Spec: openchoreov1alpha1.WorkflowSpec{
			Parameters: &openchoreov1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{"replicas":"integer"}`)},
			},
		},
	}
}

func newDiscardLoggerHandler() *Handler {
	return &Handler{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// --- ListWorkflows Handler ---

func TestListWorkflowsHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success returns items", func(t *testing.T) {
		svc := newWorkflowService(t, []client.Object{testWorkflowObj("wf-1")}, &allowAllPDP{})
		h := newHandlerWithWorkflowService(svc)

		resp, err := h.ListWorkflows(ctx, gen.ListWorkflowsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListWorkflows200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "wf-1", typed.Items[0].Metadata.Name)
	})

	t.Run("empty list returns 200", func(t *testing.T) {
		svc := newWorkflowService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkflowService(svc)

		resp, err := h.ListWorkflows(ctx, gen.ListWorkflowsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListWorkflows200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Empty(t, typed.Items)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := newWorkflowService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkflowService(svc)

		resp, err := h.ListWorkflows(ctx, gen.ListWorkflowsRequestObject{
			NamespaceName: ns,
			Params:        gen.ListWorkflowsParams{LabelSelector: ptr.To("===invalid")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.ListWorkflows400JSONResponse{}, resp)
	})

	t.Run("internal error from service returns 500", func(t *testing.T) {
		svc := workflowmocks.NewMockService(t)
		svc.EXPECT().ListWorkflows(mock.Anything, ns, mock.Anything).Return(nil, errors.New("internal server error"))
		h := &Handler{
			services: &handlerservices.Services{WorkflowService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.ListWorkflows(ctx, gen.ListWorkflowsRequestObject{NamespaceName: ns})
		require.NoError(t, err)
		assert.IsType(t, gen.ListWorkflows500JSONResponse{}, resp)
	})
}

// --- GetWorkflow Handler ---

func TestGetWorkflowHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newWorkflowService(t, []client.Object{testWorkflowObj("wf-1")}, &allowAllPDP{})
		h := newHandlerWithWorkflowService(svc)
		resp, err := h.GetWorkflow(ctx, gen.GetWorkflowRequestObject{NamespaceName: ns, WorkflowName: "wf-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetWorkflow200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "wf-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newWorkflowService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkflowService(svc)
		resp, err := h.GetWorkflow(ctx, gen.GetWorkflowRequestObject{NamespaceName: ns, WorkflowName: "nope"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetWorkflow404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newWorkflowService(t, []client.Object{testWorkflowObj("wf-1")}, &denyAllPDP{})
		h := newHandlerWithWorkflowService(svc)
		resp, err := h.GetWorkflow(ctx, gen.GetWorkflowRequestObject{NamespaceName: ns, WorkflowName: "wf-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetWorkflow403JSONResponse{}, resp)
	})

	t.Run("internal error returns 500", func(t *testing.T) {
		svc := workflowmocks.NewMockService(t)
		svc.EXPECT().GetWorkflow(mock.Anything, ns, "wf-1").Return(nil, errors.New("internal server error"))
		h := &Handler{
			services: &handlerservices.Services{WorkflowService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.GetWorkflow(ctx, gen.GetWorkflowRequestObject{NamespaceName: ns, WorkflowName: "wf-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetWorkflow500JSONResponse{}, resp)
	})
}

// --- CreateWorkflow Handler ---

func TestCreateWorkflowHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	validBody := &gen.Workflow{Metadata: gen.ObjectMeta{Name: "new-wf"}}

	t.Run("success", func(t *testing.T) {
		svc := newWorkflowService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkflowService(svc)
		resp, err := h.CreateWorkflow(ctx, gen.CreateWorkflowRequestObject{NamespaceName: ns, Body: validBody})
		require.NoError(t, err)
		typed, ok := resp.(gen.CreateWorkflow201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "new-wf", typed.Metadata.Name)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := newWorkflowService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkflowService(svc)
		resp, err := h.CreateWorkflow(ctx, gen.CreateWorkflowRequestObject{NamespaceName: ns, Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateWorkflow400JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		svc := newWorkflowService(t, []client.Object{testWorkflowObj("new-wf")}, &allowAllPDP{})
		h := newHandlerWithWorkflowService(svc)
		resp, err := h.CreateWorkflow(ctx, gen.CreateWorkflowRequestObject{NamespaceName: ns, Body: validBody})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateWorkflow409JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newWorkflowService(t, nil, &denyAllPDP{})
		h := newHandlerWithWorkflowService(svc)
		resp, err := h.CreateWorkflow(ctx, gen.CreateWorkflowRequestObject{NamespaceName: ns, Body: validBody})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateWorkflow403JSONResponse{}, resp)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := workflowmocks.NewMockService(t)
		svc.EXPECT().CreateWorkflow(mock.Anything, ns, mock.Anything).Return(nil, &svcpkg.ValidationError{Msg: "invalid spec"})
		h := &Handler{
			services: &handlerservices.Services{WorkflowService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.CreateWorkflow(ctx, gen.CreateWorkflowRequestObject{NamespaceName: ns, Body: validBody})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateWorkflow400JSONResponse{}, resp)
	})

	t.Run("internal error returns 500", func(t *testing.T) {
		svc := workflowmocks.NewMockService(t)
		svc.EXPECT().CreateWorkflow(mock.Anything, ns, mock.Anything).Return(nil, errors.New("internal server error"))
		h := &Handler{
			services: &handlerservices.Services{WorkflowService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.CreateWorkflow(ctx, gen.CreateWorkflowRequestObject{NamespaceName: ns, Body: validBody})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateWorkflow500JSONResponse{}, resp)
	})
}

// --- UpdateWorkflow Handler ---

func TestUpdateWorkflowHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"
	body := &gen.Workflow{Metadata: gen.ObjectMeta{Name: "wf-1"}}

	t.Run("nil body returns 400", func(t *testing.T) {
		h := newDiscardLoggerHandler()
		h.services = &handlerservices.Services{WorkflowService: workflowmocks.NewMockService(t)}
		resp, err := h.UpdateWorkflow(ctx, gen.UpdateWorkflowRequestObject{NamespaceName: ns, WorkflowName: "wf-1", Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateWorkflow400JSONResponse{}, resp)
	})

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.UpdateWorkflow403JSONResponse{}},
		{"not found -> 404", workflowsvc.ErrWorkflowNotFound, gen.UpdateWorkflow404JSONResponse{}},
		{"validation -> 400", &svcpkg.ValidationError{Msg: "invalid request"}, gen.UpdateWorkflow400JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.UpdateWorkflow500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := workflowmocks.NewMockService(t)
			svc.EXPECT().UpdateWorkflow(mock.Anything, ns, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{WorkflowService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			resp, err := h.UpdateWorkflow(ctx, gen.UpdateWorkflowRequestObject{NamespaceName: ns, WorkflowName: "wf-1", Body: body})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

// --- DeleteWorkflow Handler ---

func TestDeleteWorkflowHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newWorkflowService(t, []client.Object{testWorkflowObj("wf-1")}, &allowAllPDP{})
		h := newHandlerWithWorkflowService(svc)
		resp, err := h.DeleteWorkflow(ctx, gen.DeleteWorkflowRequestObject{NamespaceName: ns, WorkflowName: "wf-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteWorkflow204Response{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newWorkflowService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkflowService(svc)
		resp, err := h.DeleteWorkflow(ctx, gen.DeleteWorkflowRequestObject{NamespaceName: ns, WorkflowName: "nope"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteWorkflow404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newWorkflowService(t, []client.Object{testWorkflowObj("wf-1")}, &denyAllPDP{})
		h := newHandlerWithWorkflowService(svc)
		resp, err := h.DeleteWorkflow(ctx, gen.DeleteWorkflowRequestObject{NamespaceName: ns, WorkflowName: "wf-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteWorkflow403JSONResponse{}, resp)
	})

	t.Run("internal error returns 500", func(t *testing.T) {
		svc := workflowmocks.NewMockService(t)
		svc.EXPECT().DeleteWorkflow(mock.Anything, ns, "wf-1").Return(errors.New("internal server error"))
		h := &Handler{
			services: &handlerservices.Services{WorkflowService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.DeleteWorkflow(ctx, gen.DeleteWorkflowRequestObject{NamespaceName: ns, WorkflowName: "wf-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteWorkflow500JSONResponse{}, resp)
	})
}

// --- GetWorkflowSchema Handler ---

func TestGetWorkflowSchemaHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newWorkflowService(t, []client.Object{testWorkflowObj("wf-1")}, &allowAllPDP{})
		h := newHandlerWithWorkflowService(svc)
		resp, err := h.GetWorkflowSchema(ctx, gen.GetWorkflowSchemaRequestObject{NamespaceName: ns, WorkflowName: "wf-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetWorkflowSchema200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.NotNil(t, typed)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newWorkflowService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkflowService(svc)
		resp, err := h.GetWorkflowSchema(ctx, gen.GetWorkflowSchemaRequestObject{NamespaceName: ns, WorkflowName: "nope"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetWorkflowSchema404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newWorkflowService(t, []client.Object{testWorkflowObj("wf-1")}, &denyAllPDP{})
		h := newHandlerWithWorkflowService(svc)
		resp, err := h.GetWorkflowSchema(ctx, gen.GetWorkflowSchemaRequestObject{NamespaceName: ns, WorkflowName: "wf-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetWorkflowSchema403JSONResponse{}, resp)
	})

	t.Run("internal error returns 500", func(t *testing.T) {
		svc := workflowmocks.NewMockService(t)
		svc.EXPECT().GetWorkflowSchema(mock.Anything, ns, "wf-1").Return(nil, errors.New("internal server error"))
		h := &Handler{
			services: &handlerservices.Services{WorkflowService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.GetWorkflowSchema(ctx, gen.GetWorkflowSchemaRequestObject{NamespaceName: ns, WorkflowName: "wf-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetWorkflowSchema500JSONResponse{}, resp)
	})
}

// --- ListWorkflowRuns Handler error mapping ---

func TestListWorkflowRunsHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.ListWorkflowRuns403JSONResponse{}},
		{"validation -> 400", &svcpkg.ValidationError{Msg: "bad request"}, gen.ListWorkflowRuns400JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.ListWorkflowRuns500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := workflowrunmocks.NewMockService(t)
			svc.EXPECT().ListWorkflowRuns(mock.Anything, ns, "", "", "", mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{WorkflowRunService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			resp, err := h.ListWorkflowRuns(ctx, gen.ListWorkflowRunsRequestObject{NamespaceName: ns})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

// --- CreateWorkflowRun Handler ---

func TestCreateWorkflowRunHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"
	body := &gen.WorkflowRun{Metadata: gen.ObjectMeta{Name: "run-1"}}

	t.Run("success", func(t *testing.T) {
		svc := workflowrunmocks.NewMockService(t)
		svc.EXPECT().CreateWorkflowRun(mock.Anything, ns, mock.Anything).RunAndReturn(func(_ context.Context, _ string, wfRun *openchoreov1alpha1.WorkflowRun) (*openchoreov1alpha1.WorkflowRun, error) {
			return wfRun, nil
		})
		h := &Handler{
			services: &handlerservices.Services{WorkflowRunService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.CreateWorkflowRun(ctx, gen.CreateWorkflowRunRequestObject{NamespaceName: ns, Body: body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateWorkflowRun201JSONResponse{}, resp)
	})

	t.Run("nil body returns 400", func(t *testing.T) {
		h := &Handler{
			services: &handlerservices.Services{WorkflowRunService: workflowrunmocks.NewMockService(t)},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.CreateWorkflowRun(ctx, gen.CreateWorkflowRunRequestObject{NamespaceName: ns, Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateWorkflowRun400JSONResponse{}, resp)
	})

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"workflow not found -> 404", workflowrunsvc.ErrWorkflowNotFound, gen.CreateWorkflowRun404JSONResponse{}},
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.CreateWorkflowRun403JSONResponse{}},
		{"validation -> 400", &svcpkg.ValidationError{Msg: "bad request"}, gen.CreateWorkflowRun400JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.CreateWorkflowRun500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := workflowrunmocks.NewMockService(t)
			svc.EXPECT().CreateWorkflowRun(mock.Anything, ns, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{WorkflowRunService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			resp, err := h.CreateWorkflowRun(ctx, gen.CreateWorkflowRunRequestObject{NamespaceName: ns, Body: body})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

// --- GetWorkflowRun Handler ---

func TestGetWorkflowRunHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		svc := newWorkflowRunService(t, []client.Object{testWorkflowRunObj()}, &allowAllPDP{})
		h := newHandlerWithWorkflowRunService(svc)
		resp, err := h.GetWorkflowRun(ctx, gen.GetWorkflowRunRequestObject{NamespaceName: ns, RunName: "run-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetWorkflowRun200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "run-1", typed.Metadata.Name)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := newWorkflowRunService(t, nil, &allowAllPDP{})
		h := newHandlerWithWorkflowRunService(svc)
		resp, err := h.GetWorkflowRun(ctx, gen.GetWorkflowRunRequestObject{NamespaceName: ns, RunName: "nope"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetWorkflowRun404JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := newWorkflowRunService(t, []client.Object{testWorkflowRunObj()}, &denyAllPDP{})
		h := newHandlerWithWorkflowRunService(svc)
		resp, err := h.GetWorkflowRun(ctx, gen.GetWorkflowRunRequestObject{NamespaceName: ns, RunName: "run-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetWorkflowRun403JSONResponse{}, resp)
	})

	t.Run("internal error returns 500", func(t *testing.T) {
		svc := workflowrunmocks.NewMockService(t)
		svc.EXPECT().GetWorkflowRun(mock.Anything, ns, "run-1").Return(nil, errors.New("internal server error"))
		h := &Handler{
			services: &handlerservices.Services{WorkflowRunService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.GetWorkflowRun(ctx, gen.GetWorkflowRunRequestObject{NamespaceName: ns, RunName: "run-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetWorkflowRun500JSONResponse{}, resp)
	})
}

// --- UpdateWorkflowRun Handler error mapping ---

func TestUpdateWorkflowRunHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"
	body := &gen.WorkflowRun{Metadata: gen.ObjectMeta{Name: "run-1"}}

	t.Run("nil body returns 400", func(t *testing.T) {
		h := &Handler{
			services: &handlerservices.Services{WorkflowRunService: workflowrunmocks.NewMockService(t)},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		resp, err := h.UpdateWorkflowRun(ctx, gen.UpdateWorkflowRunRequestObject{NamespaceName: ns, RunName: "run-1", Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateWorkflowRun400JSONResponse{}, resp)
	})

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.UpdateWorkflowRun403JSONResponse{}},
		{"not found -> 404", workflowrunsvc.ErrWorkflowRunNotFound, gen.UpdateWorkflowRun404JSONResponse{}},
		{"validation -> 400", &svcpkg.ValidationError{Msg: "bad request"}, gen.UpdateWorkflowRun400JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.UpdateWorkflowRun500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := workflowrunmocks.NewMockService(t)
			svc.EXPECT().UpdateWorkflowRun(mock.Anything, ns, mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{WorkflowRunService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			resp, err := h.UpdateWorkflowRun(ctx, gen.UpdateWorkflowRunRequestObject{NamespaceName: ns, RunName: "run-1", Body: body})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

// --- DeleteWorkflowRun additional error path ---

func TestDeleteWorkflowRunHandler_InternalError(t *testing.T) {
	ctx := testContext()
	svc := workflowrunmocks.NewMockService(t)
	svc.EXPECT().DeleteWorkflowRun(mock.Anything, "test-ns", "run-1").Return(errors.New("internal server error"))
	h := &Handler{
		services: &handlerservices.Services{WorkflowRunService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	resp, err := h.DeleteWorkflowRun(ctx, gen.DeleteWorkflowRunRequestObject{NamespaceName: "test-ns", RunName: "run-1"})
	require.NoError(t, err)
	assert.IsType(t, gen.DeleteWorkflowRun500JSONResponse{}, resp)
}

// --- GetWorkflowRunStatus Handler ---

func TestGetWorkflowRunStatusHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success", func(t *testing.T) {
		started := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		svc := workflowrunmocks.NewMockService(t)
		svc.EXPECT().GetWorkflowRunStatus(mock.Anything, ns, "run-1", "https://gw").Return(&models.WorkflowRunStatusResponse{
			Status:               "Succeeded",
			HasLiveObservability: true,
			Steps: []models.WorkflowStepStatus{
				{Name: "build", Phase: "Succeeded", StartedAt: &started},
				{Name: "test", Phase: "Omitted"},
				{Name: "deploy", Phase: "Bogus"},
				{Name: "lint", Phase: "Pending"},
			},
		}, nil)
		h := &Handler{
			services: &handlerservices.Services{WorkflowRunService: svc},
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			Config:   &config.Config{ClusterGateway: config.ClusterGatewayConfig{URL: "https://gw"}},
		}
		resp, err := h.GetWorkflowRunStatus(ctx, gen.GetWorkflowRunStatusRequestObject{NamespaceName: ns, RunName: "run-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetWorkflowRunStatus200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.True(t, typed.HasLiveObservability)
		require.Len(t, typed.Steps, 4)
		assert.Equal(t, gen.WorkflowStepStatusPhaseSucceeded, typed.Steps[0].Phase)
		assert.Equal(t, gen.WorkflowStepStatusPhaseSkipped, typed.Steps[1].Phase, "Omitted should map to Skipped")
		assert.Equal(t, gen.WorkflowStepStatusPhaseError, typed.Steps[2].Phase, "Unknown phases should map to Error")
		assert.Equal(t, gen.WorkflowStepStatusPhasePending, typed.Steps[3].Phase)
	})

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"not found -> 404", workflowrunsvc.ErrWorkflowRunNotFound, gen.GetWorkflowRunStatus404JSONResponse{}},
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.GetWorkflowRunStatus403JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.GetWorkflowRunStatus500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := workflowrunmocks.NewMockService(t)
			svc.EXPECT().GetWorkflowRunStatus(mock.Anything, ns, "run-1", "https://gw").Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{WorkflowRunService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
				Config:   &config.Config{ClusterGateway: config.ClusterGatewayConfig{URL: "https://gw"}},
			}
			resp, err := h.GetWorkflowRunStatus(ctx, gen.GetWorkflowRunStatusRequestObject{NamespaceName: ns, RunName: "run-1"})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

// --- GetWorkflowRunLogs error mapping ---

func TestGetWorkflowRunLogsHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"not found -> 404", workflowrunsvc.ErrWorkflowRunNotFound, gen.GetWorkflowRunLogs404JSONResponse{}},
		{"reference not found -> 404", workflowrunsvc.ErrWorkflowRunReferenceNotFound, gen.GetWorkflowRunLogs404JSONResponse{}},
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.GetWorkflowRunLogs403JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.GetWorkflowRunLogs500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := workflowrunmocks.NewMockService(t)
			svc.EXPECT().GetWorkflowRunLogs(mock.Anything, ns, "run-1", "", "https://gw", mock.Anything).Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{WorkflowRunService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
				Config:   &config.Config{ClusterGateway: config.ClusterGatewayConfig{URL: "https://gw"}},
			}
			resp, err := h.GetWorkflowRunLogs(ctx, gen.GetWorkflowRunLogsRequestObject{NamespaceName: ns, RunName: "run-1"})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

// --- GetWorkflowRunEvents error mapping ---

func TestGetWorkflowRunEventsHandler_MapsErrors(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"not found -> 404", workflowrunsvc.ErrWorkflowRunNotFound, gen.GetWorkflowRunEvents404JSONResponse{}},
		{"reference not found -> 404", workflowrunsvc.ErrWorkflowRunReferenceNotFound, gen.GetWorkflowRunEvents404JSONResponse{}},
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.GetWorkflowRunEvents403JSONResponse{}},
		{"internal -> 500", errors.New("internal server error"), gen.GetWorkflowRunEvents500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := workflowrunmocks.NewMockService(t)
			svc.EXPECT().GetWorkflowRunEvents(mock.Anything, ns, "run-1", "", "https://gw").Return(nil, tt.svcErr)
			h := &Handler{
				services: &handlerservices.Services{WorkflowRunService: svc},
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
				Config:   &config.Config{ClusterGateway: config.ClusterGatewayConfig{URL: "https://gw"}},
			}
			resp, err := h.GetWorkflowRunEvents(ctx, gen.GetWorkflowRunEventsRequestObject{NamespaceName: ns, RunName: "run-1"})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

// --- normalizeStepPhase ---

func TestNormalizeStepPhase(t *testing.T) {
	tests := []struct {
		input string
		want  gen.WorkflowStepStatusPhase
	}{
		{"Pending", gen.WorkflowStepStatusPhasePending},
		{"Running", gen.WorkflowStepStatusPhaseRunning},
		{"Succeeded", gen.WorkflowStepStatusPhaseSucceeded},
		{"Failed", gen.WorkflowStepStatusPhaseFailed},
		{"Skipped", gen.WorkflowStepStatusPhaseSkipped},
		{"Error", gen.WorkflowStepStatusPhaseError},
		{"Omitted", gen.WorkflowStepStatusPhaseSkipped},
		{"", gen.WorkflowStepStatusPhaseError},
		{"random", gen.WorkflowStepStatusPhaseError},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeStepPhase(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
