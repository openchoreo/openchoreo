// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

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

// newTestRESTMapper creates a RESTMapper with standard Kubernetes resource mappings
// for use with the fake client in tests that need resource plural resolution.
func newTestRESTMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "", Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "batch", Version: "v1"},
	})
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}, meta.RESTScopeNamespace)
	return mapper
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

func TestGetWorkflowRunStatus(t *testing.T) {
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

		_, err := svc.GetWorkflowRunStatus(context.Background(), namespace, runName, "")
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

		_, err := svc.GetWorkflowRunStatus(context.Background(), namespace, runName, "")
		if !errors.Is(err, ErrWorkflowRunNotFound) {
			t.Errorf("expected ErrWorkflowRunNotFound, got %v", err)
		}
	})

	t.Run("k8s error returns wrapped error", func(t *testing.T) {
		injectedErr := fmt.Errorf("connection refused")
		svc := &WorkflowRunService{
			k8sClient: fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithInterceptorFuncs(interceptor.Funcs{
				Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
					return injectedErr
				},
			}).Build(),
			logger:   slog.Default(),
			authzPDP: &allowAllPDP{},
		}

		_, err := svc.GetWorkflowRunStatus(context.Background(), namespace, runName, "")
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if errors.Is(err, ErrWorkflowRunNotFound) {
			t.Errorf("expected wrapped k8s error, got ErrWorkflowRunNotFound")
		}
	})

	t.Run("success path maps tasks and overall status", func(t *testing.T) {
		startedAt := metav1.NewTime(time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC))
		completedAt := metav1.NewTime(time.Date(2025, 1, 6, 10, 1, 0, 0, time.UTC))

		wfRun := &openchoreov1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: runName, Namespace: namespace},
			Spec:       openchoreov1alpha1.WorkflowRunSpec{Workflow: openchoreov1alpha1.WorkflowRunConfig{Name: "my-workflow"}},
			Status: openchoreov1alpha1.WorkflowRunStatus{
				Conditions: []metav1.Condition{
					{
						Type:               "WorkflowSucceeded",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.Now(),
					},
				},
				Tasks: []openchoreov1alpha1.WorkflowTask{
					{Name: "clone-step", Phase: "Succeeded", StartedAt: &startedAt, CompletedAt: &completedAt},
					{Name: "build-step", Phase: "Running", StartedAt: &startedAt},
				},
			},
		}

		svc := &WorkflowRunService{
			k8sClient:         fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithStatusSubresource(wfRun).WithObjects(wfRun).Build(),
			logger:            slog.Default(),
			authzPDP:          &allowAllPDP{},
			buildPlaneService: &BuildPlaneService{k8sClient: fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build(), logger: slog.Default()},
		}

		resp, err := svc.GetWorkflowRunStatus(context.Background(), namespace, runName, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Status != "Succeeded" {
			t.Errorf("expected status Succeeded, got %q", resp.Status)
		}
		if len(resp.Steps) != 2 {
			t.Fatalf("expected 2 steps, got %d", len(resp.Steps))
		}

		cloneStep := resp.Steps[0]
		if cloneStep.Name != "clone-step" {
			t.Errorf("expected step name clone-step, got %q", cloneStep.Name)
		}
		if cloneStep.Phase != "Succeeded" {
			t.Errorf("expected phase Succeeded, got %q", cloneStep.Phase)
		}
		if cloneStep.StartedAt == nil || !cloneStep.StartedAt.Equal(startedAt.Time) {
			t.Errorf("expected StartedAt %v, got %v", startedAt.Time, cloneStep.StartedAt)
		}
		if cloneStep.FinishedAt == nil || !cloneStep.FinishedAt.Equal(completedAt.Time) {
			t.Errorf("expected FinishedAt %v, got %v", completedAt.Time, cloneStep.FinishedAt)
		}

		buildStep := resp.Steps[1]
		if buildStep.Name != "build-step" {
			t.Errorf("expected step name build-step, got %q", buildStep.Name)
		}
		if buildStep.FinishedAt != nil {
			t.Errorf("expected nil FinishedAt for running step, got %v", buildStep.FinishedAt)
		}

		// buildPlaneService has no BuildPlane object -> HasLiveObservability must be false
		if resp.HasLiveObservability {
			t.Errorf("expected HasLiveObservability false when build plane is unreachable")
		}
	})
}
