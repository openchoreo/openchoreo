// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	workflowrunsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun"
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
