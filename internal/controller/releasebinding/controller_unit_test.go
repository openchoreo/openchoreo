// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Unit tests for pure helper functions that don't require k8s environment

func newTestReconciler() *Reconciler {
	return &Reconciler{
		Scheme: runtime.NewScheme(),
	}
}

func makeValidComponentRelease(project, component string) *openchoreov1alpha1.ComponentRelease {
	return &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
		},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   project,
				ComponentName: component,
			},
			ComponentType: openchoreov1alpha1.ComponentTypeSpec{
				WorkloadType: "deployment",
				Resources: []openchoreov1alpha1.ResourceTemplate{
					{ID: "deployment"},
				},
			},
		},
	}
}

func makeValidReleaseBinding(project, component string) *openchoreov1alpha1.ReleaseBinding {
	return &openchoreov1alpha1.ReleaseBinding{
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner: openchoreov1alpha1.ReleaseBindingOwner{
				ProjectName:   project,
				ComponentName: component,
			},
		},
	}
}

// ---- validateComponentRelease tests ----

func TestValidateComponentRelease_Valid(t *testing.T) {
	r := newTestReconciler()
	cr := makeValidComponentRelease("my-project", "my-component")
	rb := makeValidReleaseBinding("my-project", "my-component")

	if err := r.validateComponentRelease(cr, rb); err != nil {
		t.Errorf("validateComponentRelease returned unexpected error: %v", err)
	}
}

func TestValidateComponentRelease_NilResources(t *testing.T) {
	r := newTestReconciler()

	cr := &openchoreov1alpha1.ComponentRelease{
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   "proj",
				ComponentName: "comp",
			},
			ComponentType: openchoreov1alpha1.ComponentTypeSpec{
				WorkloadType: "deployment",
				Resources:    nil, // nil resources
			},
		},
	}
	rb := makeValidReleaseBinding("proj", "comp")

	err := r.validateComponentRelease(cr, rb)
	if err == nil {
		t.Error("validateComponentRelease should return error when resources is nil")
	}
}

func TestValidateComponentRelease_MissingProjectName(t *testing.T) {
	r := newTestReconciler()

	cr := &openchoreov1alpha1.ComponentRelease{
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   "", // missing
				ComponentName: "comp",
			},
			ComponentType: openchoreov1alpha1.ComponentTypeSpec{
				WorkloadType: "deployment",
				Resources:    []openchoreov1alpha1.ResourceTemplate{{ID: "deployment"}},
			},
		},
	}
	rb := makeValidReleaseBinding("", "comp")

	err := r.validateComponentRelease(cr, rb)
	if err == nil {
		t.Error("validateComponentRelease should return error when projectName is empty")
	}
}

func TestValidateComponentRelease_MissingComponentName(t *testing.T) {
	r := newTestReconciler()

	cr := &openchoreov1alpha1.ComponentRelease{
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   "proj",
				ComponentName: "", // missing
			},
			ComponentType: openchoreov1alpha1.ComponentTypeSpec{
				WorkloadType: "deployment",
				Resources:    []openchoreov1alpha1.ResourceTemplate{{ID: "deployment"}},
			},
		},
	}
	rb := makeValidReleaseBinding("proj", "")

	err := r.validateComponentRelease(cr, rb)
	if err == nil {
		t.Error("validateComponentRelease should return error when componentName is empty")
	}
}

func TestValidateComponentRelease_OwnerMismatch_Project(t *testing.T) {
	r := newTestReconciler()
	cr := makeValidComponentRelease("project-A", "comp")
	rb := makeValidReleaseBinding("project-B", "comp") // different project

	err := r.validateComponentRelease(cr, rb)
	if err == nil {
		t.Error("validateComponentRelease should return error when project names don't match")
	}
}

func TestValidateComponentRelease_OwnerMismatch_Component(t *testing.T) {
	r := newTestReconciler()
	cr := makeValidComponentRelease("proj", "comp-A")
	rb := makeValidReleaseBinding("proj", "comp-B") // different component

	err := r.validateComponentRelease(cr, rb)
	if err == nil {
		t.Error("validateComponentRelease should return error when component names don't match")
	}
}

// ---- NewReleaseBindingFinalizingCondition ----

func TestNewReleaseBindingFinalizingCondition(t *testing.T) {
	cond := NewReleaseBindingFinalizingCondition(5)
	if cond.Type != string(ConditionFinalizing) {
		t.Errorf("Type = %q, want %q", cond.Type, ConditionFinalizing)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
	if cond.ObservedGeneration != 5 {
		t.Errorf("ObservedGeneration = %d, want 5", cond.ObservedGeneration)
	}
}

// ---- buildMetadataContext tests ----

func TestBuildMetadataContext_FieldsPopulated(t *testing.T) {
	r := newTestReconciler()

	componentRelease := &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
		},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   "my-project",
				ComponentName: "my-component",
			},
		},
	}
	component := &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			UID: "comp-uid-123",
		},
	}
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			UID: "proj-uid-456",
		},
	}
	dataPlane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-dataplane",
			UID:  "dp-uid-789",
		},
	}
	environment := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			UID: "env-uid-101",
		},
	}
	environmentName := "staging"

	ctx := r.buildMetadataContext(componentRelease, component, project, dataPlane, environment, environmentName)

	if ctx.ComponentName != "my-component" {
		t.Errorf("ComponentName = %q, want %q", ctx.ComponentName, "my-component")
	}
	if ctx.ProjectName != "my-project" {
		t.Errorf("ProjectName = %q, want %q", ctx.ProjectName, "my-project")
	}
	if ctx.DataPlaneName != "my-dataplane" {
		t.Errorf("DataPlaneName = %q, want %q", ctx.DataPlaneName, "my-dataplane")
	}
	if ctx.EnvironmentName != "staging" {
		t.Errorf("EnvironmentName = %q, want %q", ctx.EnvironmentName, "staging")
	}
	if ctx.ComponentUID != "comp-uid-123" {
		t.Errorf("ComponentUID = %q, want %q", ctx.ComponentUID, "comp-uid-123")
	}
	if ctx.ProjectUID != "proj-uid-456" {
		t.Errorf("ProjectUID = %q, want %q", ctx.ProjectUID, "proj-uid-456")
	}
	if ctx.DataPlaneUID != "dp-uid-789" {
		t.Errorf("DataPlaneUID = %q, want %q", ctx.DataPlaneUID, "dp-uid-789")
	}
	if ctx.EnvironmentUID != "env-uid-101" {
		t.Errorf("EnvironmentUID = %q, want %q", ctx.EnvironmentUID, "env-uid-101")
	}
	if ctx.Name == "" {
		t.Error("Name should not be empty")
	}
	if ctx.Namespace == "" {
		t.Error("Namespace should not be empty")
	}
	if len(ctx.Labels) == 0 {
		t.Error("Labels should not be empty")
	}
	if ctx.Annotations == nil {
		t.Error("Annotations should not be nil")
	}
	if len(ctx.PodSelectors) == 0 {
		t.Error("PodSelectors should not be empty")
	}
}
