// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// denyAllPDP is a PDP stub that always denies authorization.
type denyAllPDP struct{}

func (d *denyAllPDP) Evaluate(_ context.Context, _ *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	return &authzcore.Decision{Decision: false, Context: &authzcore.DecisionContext{}}, nil
}

func (d *denyAllPDP) BatchEvaluate(_ context.Context, _ *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	return nil, nil
}

func (d *denyAllPDP) GetSubjectProfile(_ context.Context, _ *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, nil
}

// allowAllPDP is a PDP stub that always allows authorization.
type allowAllPDP struct{}

func (a *allowAllPDP) Evaluate(_ context.Context, _ *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	return &authzcore.Decision{Decision: true, Context: &authzcore.DecisionContext{}}, nil
}

func (a *allowAllPDP) BatchEvaluate(_ context.Context, _ *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	return nil, nil
}

func (a *allowAllPDP) GetSubjectProfile(_ context.Context, _ *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, nil
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := openchoreov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}
	return scheme
}

func TestGetWorkflowRunEvents(t *testing.T) {
	const (
		namespace = "test-ns"
		runName   = "my-run"
	)

	t.Run("authz denial returns ErrForbidden", func(t *testing.T) {
		svc := &WorkflowRunService{
			k8sClient: fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build(),
			logger:    slog.Default(),
			authzPDP:  &denyAllPDP{},
		}

		_, err := svc.GetWorkflowRunEvents(context.Background(), namespace, runName, "", "")
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("workflow run not found returns ErrWorkflowRunNotFound", func(t *testing.T) {
		svc := &WorkflowRunService{
			k8sClient: fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build(),
			logger:    slog.Default(),
			authzPDP:  &allowAllPDP{},
		}

		_, err := svc.GetWorkflowRunEvents(context.Background(), namespace, runName, "", "")
		if !errors.Is(err, ErrWorkflowRunNotFound) {
			t.Errorf("expected ErrWorkflowRunNotFound, got %v", err)
		}
	})

	t.Run("nil RunReference returns ErrWorkflowRunReferenceNotFound", func(t *testing.T) {
		wfRun := &openchoreov1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: runName, Namespace: namespace},
			Spec:       openchoreov1alpha1.WorkflowRunSpec{Workflow: openchoreov1alpha1.WorkflowRunConfig{Name: "my-workflow"}},
			// Status.RunReference intentionally left nil
		}

		svc := &WorkflowRunService{
			k8sClient: fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(wfRun).Build(),
			logger:    slog.Default(),
			authzPDP:  &allowAllPDP{},
		}

		_, err := svc.GetWorkflowRunEvents(context.Background(), namespace, runName, "", "")
		if !errors.Is(err, ErrWorkflowRunReferenceNotFound) {
			t.Errorf("expected ErrWorkflowRunReferenceNotFound, got %v", err)
		}
	})

	t.Run("empty RunReference.Name returns ErrWorkflowRunReferenceNotFound", func(t *testing.T) {
		wfRun := &openchoreov1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: runName, Namespace: namespace},
			Spec:       openchoreov1alpha1.WorkflowRunSpec{Workflow: openchoreov1alpha1.WorkflowRunConfig{Name: "my-workflow"}},
			Status: openchoreov1alpha1.WorkflowRunStatus{
				RunReference: &openchoreov1alpha1.ResourceReference{Name: "", Namespace: "argo-ns"},
			},
		}

		svc := &WorkflowRunService{
			k8sClient: fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(wfRun).Build(),
			logger:    slog.Default(),
			authzPDP:  &allowAllPDP{},
		}

		_, err := svc.GetWorkflowRunEvents(context.Background(), namespace, runName, "", "")
		if !errors.Is(err, ErrWorkflowRunReferenceNotFound) {
			t.Errorf("expected ErrWorkflowRunReferenceNotFound, got %v", err)
		}
	})

	t.Run("empty RunReference.Namespace returns ErrWorkflowRunReferenceNotFound", func(t *testing.T) {
		wfRun := &openchoreov1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: runName, Namespace: namespace},
			Spec:       openchoreov1alpha1.WorkflowRunSpec{Workflow: openchoreov1alpha1.WorkflowRunConfig{Name: "my-workflow"}},
			Status: openchoreov1alpha1.WorkflowRunStatus{
				RunReference: &openchoreov1alpha1.ResourceReference{Name: "argo-run", Namespace: ""},
			},
		}

		svc := &WorkflowRunService{
			k8sClient: fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(wfRun).Build(),
			logger:    slog.Default(),
			authzPDP:  &allowAllPDP{},
		}

		_, err := svc.GetWorkflowRunEvents(context.Background(), namespace, runName, "", "")
		if !errors.Is(err, ErrWorkflowRunReferenceNotFound) {
			t.Errorf("expected ErrWorkflowRunReferenceNotFound, got %v", err)
		}
	})

	t.Run("valid RunReference delegates to getArgoWorkflowRunEvents", func(t *testing.T) {
		wfRun := &openchoreov1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: runName, Namespace: namespace},
			Spec:       openchoreov1alpha1.WorkflowRunSpec{Workflow: openchoreov1alpha1.WorkflowRunConfig{Name: "my-workflow"}},
			Status: openchoreov1alpha1.WorkflowRunStatus{
				RunReference: &openchoreov1alpha1.ResourceReference{Name: "argo-run", Namespace: "argo-ns"},
			},
		}

		// A BuildPlaneService backed by an empty fake client returns ErrBuildPlaneNotFound,
		// which is enough to confirm that GetWorkflowRunEvents delegated past its own checks.
		emptyBPSvc := &BuildPlaneService{
			k8sClient: fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build(),
			logger:    slog.Default(),
		}
		svc := &WorkflowRunService{
			k8sClient:         fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(wfRun).Build(),
			logger:            slog.Default(),
			authzPDP:          &allowAllPDP{},
			buildPlaneService: emptyBPSvc,
		}

		_, err := svc.GetWorkflowRunEvents(context.Background(), namespace, runName, "", "")
		// We expect an error from the build plane (not an authz or reference error),
		// which proves GetWorkflowRunEvents successfully delegated past its own checks.
		if errors.Is(err, ErrForbidden) {
			t.Errorf("unexpected ErrForbidden: authz check should have passed")
		}
		if errors.Is(err, ErrWorkflowRunNotFound) {
			t.Errorf("unexpected ErrWorkflowRunNotFound: WorkflowRun exists")
		}
		if errors.Is(err, ErrWorkflowRunReferenceNotFound) {
			t.Errorf("unexpected ErrWorkflowRunReferenceNotFound: RunReference is set")
		}
	})
}
