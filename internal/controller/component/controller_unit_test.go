// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ---- NewComponentFinalizingCondition ----

func TestNewComponentFinalizingCondition(t *testing.T) {
	cond := NewComponentFinalizingCondition(5)
	if cond.Type != string(ConditionFinalizing) {
		t.Errorf("Type = %q, want %q", cond.Type, ConditionFinalizing)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
	if cond.Reason != string(ReasonFinalizing) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonFinalizing)
	}
	if cond.ObservedGeneration != 5 {
		t.Errorf("ObservedGeneration = %d, want 5", cond.ObservedGeneration)
	}
}

// ---- BuildReleaseSpec ----

func TestBuildReleaseSpec_NilComponentType(t *testing.T) {
	_, err := BuildReleaseSpec(nil, nil, &openchoreov1alpha1.Component{}, &openchoreov1alpha1.Workload{})
	if err == nil {
		t.Error("BuildReleaseSpec(nil ct) expected error, got nil")
	}
}

func TestBuildReleaseSpec_NilWorkload(t *testing.T) {
	ct := &openchoreov1alpha1.ComponentType{}
	_, err := BuildReleaseSpec(ct, nil, &openchoreov1alpha1.Component{}, nil)
	if err == nil {
		t.Error("BuildReleaseSpec(nil workload) expected error, got nil")
	}
}

func TestBuildReleaseSpec_NilComponent(t *testing.T) {
	ct := &openchoreov1alpha1.ComponentType{}
	wl := &openchoreov1alpha1.Workload{}
	_, err := BuildReleaseSpec(ct, nil, nil, wl)
	if err == nil {
		t.Error("BuildReleaseSpec(nil component) expected error, got nil")
	}
}

func TestBuildReleaseSpec_ValidMinimal(t *testing.T) {
	ct := &openchoreov1alpha1.ComponentType{
		Spec: openchoreov1alpha1.ComponentTypeSpec{
			WorkloadType: "deployment",
		},
	}
	comp := &openchoreov1alpha1.Component{}
	wl := &openchoreov1alpha1.Workload{}

	spec, err := BuildReleaseSpec(ct, nil, comp, wl)
	if err != nil {
		t.Fatalf("BuildReleaseSpec unexpected error: %v", err)
	}
	if spec == nil {
		t.Fatal("BuildReleaseSpec returned nil spec")
	}
	if spec.ComponentType.WorkloadType != "deployment" {
		t.Errorf("ComponentType.WorkloadType = %q, want deployment", spec.ComponentType.WorkloadType)
	}
	if spec.Traits != nil {
		t.Errorf("Traits should be nil for empty traits slice, got %v", spec.Traits)
	}
	if spec.ComponentProfile != nil {
		t.Errorf("ComponentProfile should be nil for component with no params/traits, got %v", spec.ComponentProfile)
	}
}

func TestBuildReleaseSpec_WithTraits(t *testing.T) {
	ct := &openchoreov1alpha1.ComponentType{
		Spec: openchoreov1alpha1.ComponentTypeSpec{WorkloadType: "deployment"},
	}
	traits := []openchoreov1alpha1.Trait{
		{
			Spec: openchoreov1alpha1.TraitSpec{},
		},
	}
	traits[0].Name = "my-trait"
	comp := &openchoreov1alpha1.Component{}
	wl := &openchoreov1alpha1.Workload{}

	spec, err := BuildReleaseSpec(ct, traits, comp, wl)
	if err != nil {
		t.Fatalf("BuildReleaseSpec unexpected error: %v", err)
	}
	if spec.Traits == nil {
		t.Error("Traits should not be nil when traits provided")
	}
	if _, ok := spec.Traits["my-trait"]; !ok {
		t.Error("Expected 'my-trait' to be in traits map")
	}
}

func TestBuildReleaseSpec_WithComponentProfile(t *testing.T) {
	ct := &openchoreov1alpha1.ComponentType{
		Spec: openchoreov1alpha1.ComponentTypeSpec{WorkloadType: "service"},
	}
	comp := &openchoreov1alpha1.Component{
		Spec: openchoreov1alpha1.ComponentSpec{
			Traits: []openchoreov1alpha1.ComponentTrait{
				{Name: "my-trait"},
			},
		},
	}
	wl := &openchoreov1alpha1.Workload{}

	spec, err := BuildReleaseSpec(ct, nil, comp, wl)
	if err != nil {
		t.Fatalf("BuildReleaseSpec unexpected error: %v", err)
	}
	if spec.ComponentProfile == nil {
		t.Error("ComponentProfile should not be nil when component has traits")
	}
}

func TestBuildReleaseSpec_DifferentSpecsProduceDifferentHashes(t *testing.T) {
	ct := &openchoreov1alpha1.ComponentType{
		Spec: openchoreov1alpha1.ComponentTypeSpec{WorkloadType: "deployment"},
	}
	wl1 := &openchoreov1alpha1.Workload{
		Spec: openchoreov1alpha1.WorkloadSpec{
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
				Containers: map[string]openchoreov1alpha1.Container{
					"app": {Image: "nginx:1.21"},
				},
			},
		},
	}
	wl2 := &openchoreov1alpha1.Workload{
		Spec: openchoreov1alpha1.WorkloadSpec{
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
				Containers: map[string]openchoreov1alpha1.Container{
					"app": {Image: "nginx:1.22"},
				},
			},
		},
	}
	comp := &openchoreov1alpha1.Component{}

	spec1, err1 := BuildReleaseSpec(ct, nil, comp, wl1)
	spec2, err2 := BuildReleaseSpec(ct, nil, comp, wl2)
	if err1 != nil || err2 != nil {
		t.Fatalf("BuildReleaseSpec errors: %v, %v", err1, err2)
	}

	if EqualReleaseTemplate(spec1, spec2) {
		t.Error("Different workload images should produce different hashes")
	}
}
