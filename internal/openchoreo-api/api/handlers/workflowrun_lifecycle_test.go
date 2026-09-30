// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	workflowrunsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun"
	workflowrunmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun/mocks"
)

func TestResumeWorkflowRunHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("success returns the run", func(t *testing.T) {
		svc := workflowrunmocks.NewMockService(t)
		svc.EXPECT().ResumeWorkflowRun(mock.Anything, ns, "run-1").Return(testWorkflowRunObj(), nil)
		h := &Handler{services: &handlerservices.Services{WorkflowRunService: svc}, logger: slog.Default()}
		resp, err := h.ResumeWorkflowRun(ctx, gen.ResumeWorkflowRunRequestObject{NamespaceName: ns, RunName: "run-1"})
		require.NoError(t, err)
		typed, ok := resp.(gen.ResumeWorkflowRun200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "run-1", typed.Metadata.Name)
	})

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"not found -> 404", workflowrunsvc.ErrWorkflowRunNotFound, gen.ResumeWorkflowRun404JSONResponse{}},
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.ResumeWorkflowRun403JSONResponse{}},
		{"not suspended -> 409", workflowrunsvc.ErrWorkflowRunNotSuspended, gen.ResumeWorkflowRun409JSONResponse{}},
		{"not started -> 409", workflowrunsvc.ErrWorkflowRunNotStarted, gen.ResumeWorkflowRun409JSONResponse{}},
		{"internal -> 500", assert.AnError, gen.ResumeWorkflowRun500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := workflowrunmocks.NewMockService(t)
			svc.EXPECT().ResumeWorkflowRun(mock.Anything, ns, "run-1").Return(nil, tt.svcErr)
			h := &Handler{services: &handlerservices.Services{WorkflowRunService: svc}, logger: slog.Default()}
			resp, err := h.ResumeWorkflowRun(ctx, gen.ResumeWorkflowRunRequestObject{NamespaceName: ns, RunName: "run-1"})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestStopWorkflowRunHandler(t *testing.T) {
	ctx := testContext()
	const ns = "test-ns"

	t.Run("passes the reason through and returns the run", func(t *testing.T) {
		svc := workflowrunmocks.NewMockService(t)
		svc.EXPECT().StopWorkflowRun(mock.Anything, ns, "run-1", "rollback").Return(testWorkflowRunObj(), nil)
		h := &Handler{services: &handlerservices.Services{WorkflowRunService: svc}, logger: slog.Default()}
		resp, err := h.StopWorkflowRun(ctx, gen.StopWorkflowRunRequestObject{
			NamespaceName: ns, RunName: "run-1", Body: &gen.WorkflowRunStopRequest{Reason: ptr.To("rollback")},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.StopWorkflowRun200JSONResponse{}, resp)
	})

	t.Run("body is optional", func(t *testing.T) {
		svc := workflowrunmocks.NewMockService(t)
		svc.EXPECT().StopWorkflowRun(mock.Anything, ns, "run-1", "").Return(testWorkflowRunObj(), nil)
		h := &Handler{services: &handlerservices.Services{WorkflowRunService: svc}, logger: slog.Default()}
		resp, err := h.StopWorkflowRun(ctx, gen.StopWorkflowRunRequestObject{NamespaceName: ns, RunName: "run-1"})
		require.NoError(t, err)
		assert.IsType(t, gen.StopWorkflowRun200JSONResponse{}, resp)
	})

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"not found -> 404", workflowrunsvc.ErrWorkflowRunNotFound, gen.StopWorkflowRun404JSONResponse{}},
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.StopWorkflowRun403JSONResponse{}},
		{"completed -> 409", workflowrunsvc.ErrWorkflowRunCompleted, gen.StopWorkflowRun409JSONResponse{}},
		{"not started -> 409", workflowrunsvc.ErrWorkflowRunNotStarted, gen.StopWorkflowRun409JSONResponse{}},
		{"internal -> 500", assert.AnError, gen.StopWorkflowRun500JSONResponse{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := workflowrunmocks.NewMockService(t)
			svc.EXPECT().StopWorkflowRun(mock.Anything, ns, "run-1", "").Return(nil, tt.svcErr)
			h := &Handler{services: &handlerservices.Services{WorkflowRunService: svc}, logger: slog.Default()}
			resp, err := h.StopWorkflowRun(ctx, gen.StopWorkflowRunRequestObject{NamespaceName: ns, RunName: "run-1"})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}
