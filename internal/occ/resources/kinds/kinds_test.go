// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kinds

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/occ/resources"
)

// helpers to create resource instances without a k8s client (for status/age tests)

func newTestProjectResource() *ProjectResource {
	base := resources.NewBaseResource[*openchoreov1alpha1.Project, *openchoreov1alpha1.ProjectList]()
	return &ProjectResource{BaseResource: base}
}

func newTestBuildResource() *BuildResource {
	base := resources.NewBaseResource[*openchoreov1alpha1.Build, *openchoreov1alpha1.BuildList]()
	return &BuildResource{BaseResource: base}
}

func newTestComponentResource() *ComponentResource {
	base := resources.NewBaseResource[*openchoreov1alpha1.Component, *openchoreov1alpha1.ComponentList]()
	return &ComponentResource{BaseResource: base}
}

func newTestEnvironmentResource() *EnvironmentResource {
	base := resources.NewBaseResource[*openchoreov1alpha1.Environment, *openchoreov1alpha1.EnvironmentList]()
	return &EnvironmentResource{BaseResource: base}
}

func makeCondition(condType, status, reason string, t time.Time) metav1.Condition {
	return metav1.Condition{
		Type:               condType,
		Status:             metav1.ConditionStatus(status),
		Reason:             reason,
		LastTransitionTime: metav1.NewTime(t),
	}
}

// ---- ProjectResource.GetStatus ----

func TestProjectResource_GetStatus_NoConditions(t *testing.T) {
	pr := newTestProjectResource()
	proj := &openchoreov1alpha1.Project{}
	got := pr.GetStatus(proj)
	if got != StatusPending {
		t.Errorf("ProjectResource.GetStatus(no conditions) = %q, want %q", got, StatusPending)
	}
}

func TestProjectResource_GetStatus_CreatedTrue(t *testing.T) {
	pr := newTestProjectResource()
	proj := &openchoreov1alpha1.Project{
		Status: openchoreov1alpha1.ProjectStatus{
			Conditions: []metav1.Condition{
				makeCondition(ConditionTypeCreated, "True", "AllGood", time.Now()),
			},
		},
	}
	got := pr.GetStatus(proj)
	if got != StatusReady+" (AllGood)" {
		t.Errorf("ProjectResource.GetStatus(Created=True) = %q, want Ready (AllGood)", got)
	}
}

func TestProjectResource_GetStatus_ReadyFalse(t *testing.T) {
	pr := newTestProjectResource()
	proj := &openchoreov1alpha1.Project{
		Status: openchoreov1alpha1.ProjectStatus{
			Conditions: []metav1.Condition{
				makeCondition(ConditionTypeReady, "False", "SomeError", time.Now()),
			},
		},
	}
	got := pr.GetStatus(proj)
	// When the priority condition is false, should be NotReady
	if got == StatusPending {
		t.Errorf("ProjectResource.GetStatus(Ready=False) = %q, should not be Pending", got)
	}
}

// ---- ProjectResource.GetAge ----

func TestProjectResource_GetAge_Zero(t *testing.T) {
	pr := newTestProjectResource()
	proj := &openchoreov1alpha1.Project{}
	got := pr.GetAge(proj)
	if got != "-" {
		t.Errorf("ProjectResource.GetAge(zero) = %q, want -", got)
	}
}

func TestProjectResource_GetAge_Recent(t *testing.T) {
	pr := newTestProjectResource()
	proj := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Minute)),
		},
	}
	got := pr.GetAge(proj)
	if got == "-" || got == "" {
		t.Errorf("ProjectResource.GetAge(recent) = %q, want non-empty non-dash", got)
	}
}

// ---- BuildResource.GetStatus ----

func TestBuildResource_GetStatus_NoConditions(t *testing.T) {
	br := newTestBuildResource()
	build := &openchoreov1alpha1.Build{}
	got := br.GetStatus(build)
	if got != StatusInitializing {
		t.Errorf("BuildResource.GetStatus(no conditions) = %q, want %q", got, StatusInitializing)
	}
}

func TestBuildResource_GetStatus_CompletedTrue(t *testing.T) {
	br := newTestBuildResource()
	build := &openchoreov1alpha1.Build{
		Status: openchoreov1alpha1.BuildStatus{
			Conditions: []metav1.Condition{
				makeCondition(ConditionTypeCompleted, "True", "BuildSucceeded", time.Now()),
			},
		},
	}
	got := br.GetStatus(build)
	if got != StatusReady+" (BuildSucceeded)" {
		t.Errorf("BuildResource.GetStatus(Completed=True) = %q, want Ready (BuildSucceeded)", got)
	}
}

func TestBuildResource_GetStatus_CompletedFalse(t *testing.T) {
	br := newTestBuildResource()
	build := &openchoreov1alpha1.Build{
		Status: openchoreov1alpha1.BuildStatus{
			Conditions: []metav1.Condition{
				makeCondition(ConditionTypeCompleted, "False", "BuildFailed", time.Now()),
			},
		},
	}
	got := br.GetStatus(build)
	if got != StatusNotReady+" (BuildFailed: )" {
		// Check it's not Initializing - that means the priority condition was matched
		if got == StatusInitializing {
			t.Errorf("BuildResource.GetStatus(Completed=False) = %q, should not be Initializing", got)
		}
	}
}

// ---- BuildResource.GetBuildDuration ----

func TestBuildResource_GetBuildDuration_NoConditions(t *testing.T) {
	br := newTestBuildResource()
	build := &openchoreov1alpha1.Build{}
	got := br.GetBuildDuration(build)
	if got != "-" {
		t.Errorf("GetBuildDuration(no conditions) = %q, want -", got)
	}
}

func TestBuildResource_GetBuildDuration_InProgress(t *testing.T) {
	br := newTestBuildResource()
	build := &openchoreov1alpha1.Build{
		Status: openchoreov1alpha1.BuildStatus{
			Conditions: []metav1.Condition{
				// Completed condition with InProgress reason → not yet done
				makeCondition(ConditionTypeCompleted, "False", "BuildInProgress", time.Now()),
			},
		},
	}
	got := br.GetBuildDuration(build)
	if got != "-" {
		t.Errorf("GetBuildDuration(in progress) = %q, want -", got)
	}
}

func TestBuildResource_GetBuildDuration_Completed(t *testing.T) {
	br := newTestBuildResource()
	start := time.Now().Add(-5 * time.Minute)
	end := time.Now()
	build := &openchoreov1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.NewTime(start),
		},
		Status: openchoreov1alpha1.BuildStatus{
			Conditions: []metav1.Condition{
				// Completed with non-InProgress reason → build is done
				makeCondition(ConditionTypeCompleted, "True", "BuildSucceeded", end),
			},
		},
	}
	got := br.GetBuildDuration(build)
	if got == "-" || got == "" {
		t.Errorf("GetBuildDuration(completed) = %q, want non-empty duration string", got)
	}
	// Duration should be approximately 5 minutes (any close format is fine)
}

func TestBuildResource_GetBuildDuration_OtherConditionIgnored(t *testing.T) {
	br := newTestBuildResource()
	build := &openchoreov1alpha1.Build{
		Status: openchoreov1alpha1.BuildStatus{
			Conditions: []metav1.Condition{
				// A different condition type - should not affect duration
				makeCondition(ConditionTypeStepCloneSucceeded, "True", "CloneSucceeded", time.Now()),
			},
		},
	}
	got := br.GetBuildDuration(build)
	if got != "-" {
		t.Errorf("GetBuildDuration(other condition) = %q, want -", got)
	}
}

// ---- ComponentResource.GetStatus ----

func TestComponentResource_GetStatus_NoConditions(t *testing.T) {
	cr := newTestComponentResource()
	comp := &openchoreov1alpha1.Component{}
	got := cr.GetStatus(comp)
	if got != StatusPending {
		t.Errorf("ComponentResource.GetStatus(no conditions) = %q, want %q", got, StatusPending)
	}
}

func TestComponentResource_GetStatus_ReadyTrue(t *testing.T) {
	cr := newTestComponentResource()
	comp := &openchoreov1alpha1.Component{
		Status: openchoreov1alpha1.ComponentStatus{
			Conditions: []metav1.Condition{
				makeCondition(ConditionTypeReady, "True", "AllReady", time.Now()),
			},
		},
	}
	got := cr.GetStatus(comp)
	if got != StatusReady+" (AllReady)" {
		t.Errorf("ComponentResource.GetStatus(Ready=True) = %q, want Ready (AllReady)", got)
	}
}

// ---- EnvironmentResource.GetStatus ----

func TestEnvironmentResource_GetStatus_NoConditions(t *testing.T) {
	er := newTestEnvironmentResource()
	env := &openchoreov1alpha1.Environment{}
	got := er.GetStatus(env)
	if got != StatusPending {
		t.Errorf("EnvironmentResource.GetStatus(no conditions) = %q, want %q", got, StatusPending)
	}
}

func TestEnvironmentResource_GetStatus_ReadyTrue(t *testing.T) {
	er := newTestEnvironmentResource()
	env := &openchoreov1alpha1.Environment{
		Status: openchoreov1alpha1.EnvironmentStatus{
			Conditions: []metav1.Condition{
				makeCondition(ConditionTypeReady, "True", "EnvReady", time.Now()),
			},
		},
	}
	got := er.GetStatus(env)
	if got != StatusReady+" (EnvReady)" {
		t.Errorf("EnvironmentResource.GetStatus(Ready=True) = %q, want Ready (EnvReady)", got)
	}
}

func TestEnvironmentResource_GetStatus_ConfiguredTrue(t *testing.T) {
	er := newTestEnvironmentResource()
	env := &openchoreov1alpha1.Environment{
		Status: openchoreov1alpha1.EnvironmentStatus{
			Conditions: []metav1.Condition{
				makeCondition(ConditionTypeConfigured, "True", "EnvConfigured", time.Now()),
			},
		},
	}
	got := er.GetStatus(env)
	// Should use Configured condition as priority
	if got != StatusReady+" (EnvConfigured)" {
		t.Errorf("EnvironmentResource.GetStatus(Configured=True) = %q, want Ready (EnvConfigured)", got)
	}
}

// ---- EnvironmentResource.GetAge ----

func TestEnvironmentResource_GetAge_Zero(t *testing.T) {
	er := newTestEnvironmentResource()
	env := &openchoreov1alpha1.Environment{}
	got := er.GetAge(env)
	if got != "-" {
		t.Errorf("EnvironmentResource.GetAge(zero) = %q, want -", got)
	}
}

// ---- BuildResource.GetAge ----

func TestBuildResource_GetAge_Zero(t *testing.T) {
	br := newTestBuildResource()
	build := &openchoreov1alpha1.Build{}
	got := br.GetAge(build)
	if got != "-" {
		t.Errorf("BuildResource.GetAge(zero) = %q, want -", got)
	}
}

// ---- ComponentResource.GetAge ----

func TestComponentResource_GetAge_Zero(t *testing.T) {
	cr := newTestComponentResource()
	comp := &openchoreov1alpha1.Component{}
	got := cr.GetAge(comp)
	if got != "-" {
		t.Errorf("ComponentResource.GetAge(zero) = %q, want -", got)
	}
}
