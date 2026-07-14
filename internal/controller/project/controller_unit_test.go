// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// ── Condition helpers ────────────────────────────────────────────────────────

func TestNewProjectCreatedCondition(t *testing.T) {
	tests := []struct {
		name       string
		generation int64
		wantType   string
		wantStatus metav1.ConditionStatus
		wantReason string
	}{
		{
			name:       "generation 1",
			generation: 1,
			wantType:   string(ConditionCreated),
			wantStatus: metav1.ConditionTrue,
			wantReason: string(ReasonProjectCreated),
		},
		{
			name:       "generation 0",
			generation: 0,
			wantType:   string(ConditionCreated),
			wantStatus: metav1.ConditionTrue,
			wantReason: string(ReasonProjectCreated),
		},
		{
			name:       "high generation",
			generation: 42,
			wantType:   string(ConditionCreated),
			wantStatus: metav1.ConditionTrue,
			wantReason: string(ReasonProjectCreated),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewProjectCreatedCondition(tt.generation)
			if cond.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", cond.Type, tt.wantType)
			}
			if cond.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", cond.Status, tt.wantStatus)
			}
			if cond.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", cond.Reason, tt.wantReason)
			}
			if cond.Message != "Project is created" {
				t.Errorf("Message = %q, want %q", cond.Message, "Project is created")
			}
			if cond.ObservedGeneration != tt.generation {
				t.Errorf("ObservedGeneration = %d, want %d", cond.ObservedGeneration, tt.generation)
			}
			if cond.LastTransitionTime.IsZero() {
				t.Error("LastTransitionTime should not be zero")
			}
		})
	}
}

func TestNewProjectFinalizingCondition(t *testing.T) {
	tests := []struct {
		name       string
		generation int64
		wantType   string
		wantStatus metav1.ConditionStatus
		wantReason string
	}{
		{
			name:       "generation 1",
			generation: 1,
			wantType:   string(ConditionFinalizing),
			wantStatus: metav1.ConditionTrue,
			wantReason: string(ReasonProjectFinalizing),
		},
		{
			name:       "generation 0",
			generation: 0,
			wantType:   string(ConditionFinalizing),
			wantStatus: metav1.ConditionTrue,
			wantReason: string(ReasonProjectFinalizing),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewProjectFinalizingCondition(tt.generation)
			if cond.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", cond.Type, tt.wantType)
			}
			if cond.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", cond.Status, tt.wantStatus)
			}
			if cond.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", cond.Reason, tt.wantReason)
			}
			if cond.Message != "Project is finalizing" {
				t.Errorf("Message = %q, want %q", cond.Message, "Project is finalizing")
			}
			if cond.ObservedGeneration != tt.generation {
				t.Errorf("ObservedGeneration = %d, want %d", cond.ObservedGeneration, tt.generation)
			}
			if cond.LastTransitionTime.IsZero() {
				t.Error("LastTransitionTime should not be zero")
			}
		})
	}
}

// ── Condition type/reason string representations ─────────────────────────────

func TestConditionConstants(t *testing.T) {
	t.Run("ConditionCreated string", func(t *testing.T) {
		if got := ConditionCreated.String(); got != "Created" {
			t.Errorf("ConditionCreated.String() = %q, want %q", got, "Created")
		}
	})
	t.Run("ConditionFinalizing string", func(t *testing.T) {
		if got := ConditionFinalizing.String(); got != "Finalizing" {
			t.Errorf("ConditionFinalizing.String() = %q, want %q", got, "Finalizing")
		}
	})
	t.Run("ReasonProjectCreated", func(t *testing.T) {
		if string(ReasonProjectCreated) != "ProjectCreated" {
			t.Errorf("ReasonProjectCreated = %q, want %q", string(ReasonProjectCreated), "ProjectCreated")
		}
	})
	t.Run("ReasonProjectFinalizing", func(t *testing.T) {
		if string(ReasonProjectFinalizing) != "ProjectFinalizing" {
			t.Errorf("ReasonProjectFinalizing = %q, want %q", string(ReasonProjectFinalizing), "ProjectFinalizing")
		}
	})
}

// ── ProjectCleanupFinalizer constant ─────────────────────────────────────────

func TestProjectCleanupFinalizerConstant(t *testing.T) {
	if ProjectCleanupFinalizer != "openchoreo.dev/project-cleanup" {
		t.Errorf("ProjectCleanupFinalizer = %q, want %q", ProjectCleanupFinalizer, "openchoreo.dev/project-cleanup")
	}
}

// ── findProjectForComponent ──────────────────────────────────────────────────

func TestFindProjectForComponent(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		namespace   string
		wantLen     int
		wantName    string
		wantNS      string
	}{
		{
			name:        "component with owner project",
			projectName: "my-project",
			namespace:   "test-ns",
			wantLen:     1,
			wantName:    "my-project",
			wantNS:      "test-ns",
		},
		{
			name:        "component with empty project name",
			projectName: "",
			namespace:   "test-ns",
			wantLen:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{}
			comp := &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-component",
					Namespace: tt.namespace,
				},
				Spec: openchoreov1alpha1.ComponentSpec{
					Owner: openchoreov1alpha1.ComponentOwner{
						ProjectName: tt.projectName,
					},
				},
			}

			result := r.findProjectForComponent(context.Background(), comp)
			if len(result) != tt.wantLen {
				t.Fatalf("expected %d requests, got %d", tt.wantLen, len(result))
			}
			if tt.wantLen > 0 {
				if result[0].Name != tt.wantName {
					t.Errorf("expected name %q, got %q", tt.wantName, result[0].Name)
				}
				if result[0].Namespace != tt.wantNS {
					t.Errorf("expected namespace %q, got %q", tt.wantNS, result[0].Namespace)
				}
			}
		})
	}
}

// ── findEnvironmentNamesFromDeploymentPipeline ───────────────────────────────

func TestFindEnvironmentNamesFromDeploymentPipeline(t *testing.T) {
	tests := []struct {
		name     string
		pipeline *openchoreov1alpha1.DeploymentPipeline
		wantEnvs map[string]bool
	}{
		{
			name: "single source, no targets",
			pipeline: &openchoreov1alpha1.DeploymentPipeline{
				Spec: openchoreov1alpha1.DeploymentPipelineSpec{
					PromotionPaths: []openchoreov1alpha1.PromotionPath{
						{
							SourceEnvironmentRef:  openchoreov1alpha1.EnvironmentRef{Name: "dev"},
							TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{},
						},
					},
				},
			},
			wantEnvs: map[string]bool{"dev": true},
		},
		{
			name: "source with targets",
			pipeline: &openchoreov1alpha1.DeploymentPipeline{
				Spec: openchoreov1alpha1.DeploymentPipelineSpec{
					PromotionPaths: []openchoreov1alpha1.PromotionPath{
						{
							SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: "dev"},
							TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
								{Name: "staging"},
								{Name: "prod"},
							},
						},
					},
				},
			},
			wantEnvs: map[string]bool{"dev": true, "staging": true, "prod": true},
		},
		{
			name: "multiple paths with deduplication",
			pipeline: &openchoreov1alpha1.DeploymentPipeline{
				Spec: openchoreov1alpha1.DeploymentPipelineSpec{
					PromotionPaths: []openchoreov1alpha1.PromotionPath{
						{
							SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: "dev"},
							TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
								{Name: "staging"},
							},
						},
						{
							SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: "staging"},
							TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
								{Name: "prod"},
							},
						},
					},
				},
			},
			wantEnvs: map[string]bool{"dev": true, "staging": true, "prod": true},
		},
		{
			name: "empty promotion paths",
			pipeline: &openchoreov1alpha1.DeploymentPipeline{
				Spec: openchoreov1alpha1.DeploymentPipelineSpec{
					PromotionPaths: []openchoreov1alpha1.PromotionPath{},
				},
			},
			wantEnvs: map[string]bool{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{}
			got := r.findEnvironmentNamesFromDeploymentPipeline(tt.pipeline)

			if len(got) != len(tt.wantEnvs) {
				t.Fatalf("expected %d envs, got %d: %v", len(tt.wantEnvs), len(got), got)
			}
			for _, env := range got {
				if !tt.wantEnvs[env] {
					t.Errorf("unexpected environment %q", env)
				}
			}
		})
	}
}

// ── makeProjectContext ───────────────────────────────────────────────────────

func TestMakeProjectContext_EmptyDeploymentPipeline(t *testing.T) {
	s := newSeedTestScheme(t)
	pipeline := &openchoreov1alpha1.DeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "empty-pipeline", Namespace: "test-ns"},
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: []openchoreov1alpha1.PromotionPath{},
		},
	}
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: "my-project", Namespace: "test-ns"},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "empty-pipeline"},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(pipeline, project).Build()
	r := &Reconciler{Client: cli, Scheme: s}

	projectCtx, err := r.makeProjectContext(context.Background(), project)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if projectCtx == nil {
		t.Fatal("expected non-nil project context")
	}
	if projectCtx.Project == nil || projectCtx.Project.Name != "my-project" {
		t.Errorf("expected project my-project, got %#v", projectCtx.Project)
	}
	if projectCtx.DeploymentPipeline == nil || projectCtx.DeploymentPipeline.Name != "empty-pipeline" {
		t.Errorf("expected pipeline empty-pipeline, got %#v", projectCtx.DeploymentPipeline)
	}
	if len(projectCtx.EnvironmentNames) != 0 {
		t.Errorf("expected no environment names, got %v", projectCtx.EnvironmentNames)
	}
	if len(projectCtx.NamespaceNames) != 0 {
		t.Errorf("expected no namespace names, got %v", projectCtx.NamespaceNames)
	}
}

func TestMakeProjectContext_MissingDeploymentPipeline(t *testing.T) {
	s := newSeedTestScheme(t)
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: "my-project", Namespace: "test-ns"},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "already-gone"},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(project).Build()
	r := &Reconciler{Client: cli, Scheme: s}

	projectCtx, err := r.makeProjectContext(context.Background(), project)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if projectCtx == nil || projectCtx.Project == nil {
		t.Fatal("expected project context with project set")
	}
	if projectCtx.DeploymentPipeline != nil {
		t.Errorf("expected nil pipeline when not found, got %#v", projectCtx.DeploymentPipeline)
	}
}

func TestDeleteExternalResourcesAndWait_EmptyPipeline(t *testing.T) {
	s := newSeedTestScheme(t)
	pipeline := &openchoreov1alpha1.DeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "empty-pipeline", Namespace: "test-ns"},
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: nil,
		},
	}
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: "my-project", Namespace: "test-ns"},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "empty-pipeline"},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(pipeline, project).Build()
	r := &Reconciler{Client: cli, Scheme: s, Recorder: record.NewFakeRecorder(10)}

	done, err := r.deleteExternalResourcesAndWait(context.Background(), project)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !done {
		t.Fatal("expected cleanup to complete for empty pipeline")
	}
}

// ── makeExternalResourceHandlers ─────────────────────────────────────────────

func TestMakeExternalResourceHandlers(t *testing.T) {
	r := &Reconciler{}
	handlers := r.makeExternalResourceHandlers()
	if len(handlers) != 1 {
		t.Fatalf("expected 1 handler, got %d", len(handlers))
	}
	if handlers[0].Name() != "KubernetesNamespace" {
		t.Errorf("expected handler name %q, got %q", "KubernetesNamespace", handlers[0].Name())
	}
}

// ── findProjectForComponent returns correct ObjectKey ────────────────────────

func TestFindProjectForComponentObjectKey(t *testing.T) {
	r := &Reconciler{}
	comp := &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "comp-1",
			Namespace: "org-ns",
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{
				ProjectName: "proj-alpha",
			},
		},
	}

	result := r.findProjectForComponent(context.Background(), comp)
	if len(result) != 1 {
		t.Fatalf("expected 1 request, got %d", len(result))
	}

	expected := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "proj-alpha",
			Namespace: "org-ns",
		},
	}
	if result[0] != expected {
		t.Errorf("expected %v, got %v", expected, result[0])
	}
}

// ── Verify ConditionType implements the controller.ConditionType contract ────

func TestConditionTypeValues(t *testing.T) {
	// Ensure the condition types match the shared controller constants
	if string(ConditionCreated) != controller.TypeCreated {
		t.Errorf("ConditionCreated = %q, want %q", string(ConditionCreated), controller.TypeCreated)
	}
}

// ── findProjectForProjectReleaseBinding ──────────────────────────────────────

func TestFindProjectForProjectReleaseBinding(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		wantLen     int
	}{
		{name: "binding with owner project", projectName: "my-project", wantLen: 1},
		{name: "binding with empty project name", projectName: "", wantLen: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{}
			binding := &openchoreov1alpha1.ProjectReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-binding",
					Namespace: "test-ns",
				},
				Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
					Owner: openchoreov1alpha1.ProjectReleaseBindingOwner{
						ProjectName: tt.projectName,
					},
					Environment: "development",
				},
			}

			result := r.findProjectForProjectReleaseBinding(context.Background(), binding)
			if len(result) != tt.wantLen {
				t.Fatalf("expected %d requests, got %d", tt.wantLen, len(result))
			}
			if tt.wantLen > 0 {
				if result[0].Name != tt.projectName {
					t.Errorf("expected name %q, got %q", tt.projectName, result[0].Name)
				}
				if result[0].Namespace != "test-ns" {
					t.Errorf("expected namespace %q, got %q", "test-ns", result[0].Namespace)
				}
			}
		})
	}
}

// ── seedBindingPins / seedBindingPin ─────────────────────────────────────────

func newSeedTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := openchoreov1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}
	return s
}

func newSeedTestBinding(name, projectName, env, pin string) *openchoreov1alpha1.ProjectReleaseBinding {
	return &openchoreov1alpha1.ProjectReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
		Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
			Owner: openchoreov1alpha1.ProjectReleaseBindingOwner{
				ProjectName: projectName,
			},
			Environment:    env,
			ProjectRelease: pin,
		},
	}
}

func newSeedTestProject(latestRelease string) *openchoreov1alpha1.Project {
	p := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-project",
			Namespace: "test-ns",
		},
	}
	if latestRelease != "" {
		p.Status.LatestRelease = &openchoreov1alpha1.LatestProjectRelease{
			Name: latestRelease,
			Hash: "abc123",
		}
	}
	return p
}

func newSeedTestClientBuilder(t *testing.T) *fake.ClientBuilder {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(newSeedTestScheme(t)).
		WithIndex(&openchoreov1alpha1.ProjectReleaseBinding{},
			controller.IndexKeyProjectReleaseBindingOwner, controller.IndexProjectReleaseBindingOwner)
}

func TestSeedBindingPinsSkipsWithoutLatestRelease(t *testing.T) {
	cli := newSeedTestClientBuilder(t).
		WithObjects(newSeedTestBinding("b-dev", "my-project", "development", "")).
		Build()
	r := &Reconciler{Client: cli, Scheme: cli.Scheme(), Recorder: record.NewFakeRecorder(10)}

	if err := r.seedBindingPins(context.Background(), newSeedTestProject("")); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	got := &openchoreov1alpha1.ProjectReleaseBinding{}
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "b-dev", Namespace: "test-ns"}, got); err != nil {
		t.Fatalf("get binding: %v", err)
	}
	if got.Spec.ProjectRelease != "" {
		t.Errorf("expected pin to stay empty without a latest release, got %q", got.Spec.ProjectRelease)
	}
}

func TestSeedBindingPins(t *testing.T) {
	cli := newSeedTestClientBuilder(t).
		WithObjects(
			newSeedTestBinding("b-dev", "my-project", "development", ""),
			newSeedTestBinding("b-manual", "my-project", "staging", "user-pinned"),
			newSeedTestBinding("b-other", "other-project", "development", ""),
		).
		Build()
	r := &Reconciler{Client: cli, Scheme: cli.Scheme(), Recorder: record.NewFakeRecorder(10)}

	if err := r.seedBindingPins(context.Background(), newSeedTestProject("my-project-abc123")); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	wantPins := map[string]string{
		"b-dev":    "my-project-abc123", // seeded
		"b-manual": "user-pinned",       // explicit pin untouched
		"b-other":  "",                  // different project, out of scope
	}
	for name, want := range wantPins {
		got := &openchoreov1alpha1.ProjectReleaseBinding{}
		if err := cli.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "test-ns"}, got); err != nil {
			t.Fatalf("get binding %s: %v", name, err)
		}
		if got.Spec.ProjectRelease != want {
			t.Errorf("binding %s: expected pin %q, got %q", name, want, got.Spec.ProjectRelease)
		}
	}
}

func TestSeedBindingPinsPropagatesUpdateError(t *testing.T) {
	// A failed Update (409, transient, etc.) must surface as a reconcile error
	// so controller-runtime re-enqueues; the next pass lists a fresh copy.
	cli := newSeedTestClientBuilder(t).
		WithObjects(newSeedTestBinding("b-dev", "my-project", "development", "")).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*openchoreov1alpha1.ProjectReleaseBinding); ok {
					return errors.New("simulated update error")
				}
				return c.Update(ctx, obj, opts...)
			},
		}).
		Build()
	r := &Reconciler{Client: cli, Scheme: cli.Scheme(), Recorder: record.NewFakeRecorder(10)}

	if err := r.seedBindingPins(context.Background(), newSeedTestProject("my-project-abc123")); err == nil {
		t.Fatal("expected update error to propagate")
	}
}

func TestSeedBindingPinsListError(t *testing.T) {
	cli := newSeedTestClientBuilder(t).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*openchoreov1alpha1.ProjectReleaseBindingList); ok {
					return errors.New("simulated list error")
				}
				return c.List(ctx, list, opts...)
			},
		}).
		Build()
	r := &Reconciler{Client: cli, Scheme: cli.Scheme(), Recorder: record.NewFakeRecorder(10)}

	err := r.seedBindingPins(context.Background(), newSeedTestProject("my-project-abc123"))
	if err == nil {
		t.Fatal("expected list error to propagate")
	}
}

// ── deleteProjectReleaseBindingsAndWait ──────────────────────────────────────

func TestDeleteProjectReleaseBindingsAndWait(t *testing.T) {
	cli := newSeedTestClientBuilder(t).
		WithObjects(
			newSeedTestBinding("b-dev", "my-project", "development", "r1"),
			newSeedTestBinding("b-prod", "my-project", "production", "r1"),
			newSeedTestBinding("b-other", "other-project", "development", "r1"),
		).
		Build()
	r := &Reconciler{Client: cli, Scheme: cli.Scheme(), Recorder: record.NewFakeRecorder(10)}
	project := newSeedTestProject("")

	// First pass issues the deletes and reports not-done.
	done, err := r.deleteProjectReleaseBindingsAndWait(context.Background(), project)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if done {
		t.Error("expected done=false while bindings were being deleted")
	}

	// Second pass sees them gone.
	done, err = r.deleteProjectReleaseBindingsAndWait(context.Background(), project)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !done {
		t.Error("expected done=true once the project's bindings are gone")
	}

	// The other project's binding must be untouched.
	got := &openchoreov1alpha1.ProjectReleaseBinding{}
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "b-other", Namespace: "test-ns"}, got); err != nil {
		t.Errorf("expected other project's binding to survive, got %v", err)
	}
}

func TestDeleteProjectReleaseBindingsAndWaitDeleteError(t *testing.T) {
	cli := newSeedTestClientBuilder(t).
		WithObjects(newSeedTestBinding("b-dev", "my-project", "development", "r1")).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if _, ok := obj.(*openchoreov1alpha1.ProjectReleaseBinding); ok {
					return errors.New("simulated delete error")
				}
				return c.Delete(ctx, obj, opts...)
			},
		}).
		Build()
	r := &Reconciler{Client: cli, Scheme: cli.Scheme(), Recorder: record.NewFakeRecorder(10)}

	_, err := r.deleteProjectReleaseBindingsAndWait(context.Background(), newSeedTestProject(""))
	if err == nil {
		t.Fatal("expected delete error to propagate")
	}
}

// ── reconcileProjectRelease error propagation ────────────────────────────────

func TestReconcileProjectReleaseSeedErrorPropagates(t *testing.T) {
	pt := &openchoreov1alpha1.ProjectType{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pt", Namespace: "test-ns"},
	}
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: "my-project", Namespace: "test-ns"},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"},
			Type:                  openchoreov1alpha1.ProjectTypeRef{Name: "test-pt"},
		},
	}
	cli := newSeedTestClientBuilder(t).
		WithObjects(pt, project).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*openchoreov1alpha1.ProjectReleaseBindingList); ok {
					return errors.New("simulated list error")
				}
				return c.List(ctx, list, opts...)
			},
		}).
		Build()
	r := &Reconciler{Client: cli, Scheme: cli.Scheme(), Recorder: record.NewFakeRecorder(10)}

	if err := r.reconcileProjectRelease(context.Background(), project); err == nil {
		t.Fatal("expected seeding failure to propagate out of reconcileProjectRelease")
	}
}
