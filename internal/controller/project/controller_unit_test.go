// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

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
							SourceEnvironmentRef:  "dev",
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
							SourceEnvironmentRef: "dev",
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
							SourceEnvironmentRef: "dev",
							TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
								{Name: "staging"},
							},
						},
						{
							SourceEnvironmentRef: "staging",
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
