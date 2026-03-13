// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// findTrait reports whether traits contains an entry matching the given kind and name.
func findTrait(traits []openchoreov1alpha1.ComponentReleaseTrait, kind openchoreov1alpha1.TraitRefKind, name string) bool {
	for _, t := range traits {
		if t.Kind == kind && t.Name == name {
			return true
		}
	}
	return false
}

func makeComponent(project, name string, spec openchoreov1alpha1.ComponentSpec) *openchoreov1alpha1.Component {
	spec.Owner.ProjectName = project
	return &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       spec,
	}
}

func makeCT() openchoreov1alpha1.ComponentReleaseComponentType {
	return openchoreov1alpha1.ComponentReleaseComponentType{
		Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType,
		Name: "deployment/web-app",
		Spec: openchoreov1alpha1.ComponentTypeSpec{WorkloadType: "deployment"},
	}
}

func TestBuildSpec_NilInputs(t *testing.T) {
	ct := makeCT()
	workload := &openchoreov1alpha1.WorkloadTemplateSpec{
		Container: openchoreov1alpha1.Container{Image: "nginx:1.21"},
	}

	tests := []struct {
		name    string
		input   BuildInput
		wantErr string
	}{
		{
			name: "nil component returns error",
			input: BuildInput{
				Component:     nil,
				ComponentType: ct,
				Workload:      workload,
			},
			wantErr: "component cannot be nil",
		},
		{
			name: "empty componentType name returns error",
			input: BuildInput{
				Component:     makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
				ComponentType: openchoreov1alpha1.ComponentReleaseComponentType{},
				Workload:      workload,
			},
			wantErr: "componentType name cannot be empty",
		},
		{
			name: "nil workload returns error",
			input: BuildInput{
				Component:     makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
				ComponentType: ct,
				Workload:      nil,
			},
			wantErr: "workload cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildSpec(tt.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestBuildSpec_BasicFields(t *testing.T) {
	ct := makeCT()
	workload := &openchoreov1alpha1.WorkloadTemplateSpec{
		Container: openchoreov1alpha1.Container{Image: "nginx:1.21"},
	}

	t.Run("minimal input with no traits or parameters", func(t *testing.T) {
		spec, err := BuildSpec(BuildInput{
			Component:     makeComponent("my-project", "my-component", openchoreov1alpha1.ComponentSpec{}),
			ComponentType: ct,
			Workload:      workload,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if spec.Owner.ProjectName != "my-project" {
			t.Errorf("expected projectName 'my-project', got %q", spec.Owner.ProjectName)
		}
		if spec.Owner.ComponentName != "my-component" {
			t.Errorf("expected componentName 'my-component', got %q", spec.Owner.ComponentName)
		}
		if spec.ComponentType.Kind != openchoreov1alpha1.ComponentTypeRefKindComponentType {
			t.Errorf("expected kind 'ComponentType', got %q", spec.ComponentType.Kind)
		}
		if spec.ComponentType.Name != "deployment/web-app" {
			t.Errorf("expected name 'deployment/web-app', got %q", spec.ComponentType.Name)
		}
		if spec.ComponentType.Spec.WorkloadType != "deployment" {
			t.Errorf("expected workloadType 'deployment', got %q", spec.ComponentType.Spec.WorkloadType)
		}
		if spec.Traits != nil {
			t.Errorf("expected nil traits, got %v", spec.Traits)
		}
		if spec.ComponentProfile != nil {
			t.Errorf("expected nil componentProfile, got %v", spec.ComponentProfile)
		}
		if spec.Workload.Container.Image != "nginx:1.21" {
			t.Errorf("expected image 'nginx:1.21', got %q", spec.Workload.Container.Image)
		}
	})

	t.Run("default kind applied when kind is empty", func(t *testing.T) {
		spec, err := BuildSpec(BuildInput{
			Component: makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
			ComponentType: openchoreov1alpha1.ComponentReleaseComponentType{
				Name: "deployment/default-type",
				Spec: openchoreov1alpha1.ComponentTypeSpec{WorkloadType: "deployment"},
			},
			Workload: workload,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if spec.ComponentType.Kind != openchoreov1alpha1.ComponentTypeRefKindComponentType {
			t.Errorf("expected default kind 'ComponentType', got %q", spec.ComponentType.Kind)
		}
	})

	t.Run("cluster component type kind preserved", func(t *testing.T) {
		spec, err := BuildSpec(BuildInput{
			Component: makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
			ComponentType: openchoreov1alpha1.ComponentReleaseComponentType{
				Kind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType,
				Name: "deployment/cluster-type",
				Spec: openchoreov1alpha1.ComponentTypeSpec{WorkloadType: "deployment"},
			},
			Workload: workload,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if spec.ComponentType.Kind != openchoreov1alpha1.ComponentTypeRefKindClusterComponentType {
			t.Errorf("expected kind 'ClusterComponentType', got %q", spec.ComponentType.Kind)
		}
	})

	t.Run("nil traits map produces nil", func(t *testing.T) {
		spec, err := BuildSpec(BuildInput{
			Component:     makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
			ComponentType: ct,
			Workload:      workload,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if spec.Traits != nil {
			t.Errorf("expected nil traits, got %v", spec.Traits)
		}
	})
}

func TestBuildSpec_WithTraits(t *testing.T) {
	ct := makeCT()
	workload := &openchoreov1alpha1.WorkloadTemplateSpec{
		Container: openchoreov1alpha1.Container{Image: "nginx:1.21"},
	}

	t.Run("with traits", func(t *testing.T) {
		spec, err := BuildSpec(BuildInput{
			Component:     makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
			ComponentType: ct,
			Traits: map[string]openchoreov1alpha1.TraitSpec{
				"trait-a": {Creates: []openchoreov1alpha1.TraitCreate{{TargetPlane: "dataplane"}}},
				"trait-b": {},
			},
			Workload: workload,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(spec.Traits) != 2 {
			t.Fatalf("expected 2 traits, got %d", len(spec.Traits))
		}
		if !findTrait(spec.Traits, openchoreov1alpha1.TraitRefKindTrait, "trait-a") {
			t.Error("expected Trait:trait-a in traits slice")
		}
		if !findTrait(spec.Traits, openchoreov1alpha1.TraitRefKindTrait, "trait-b") {
			t.Error("expected Trait:trait-b in traits slice")
		}
	})

	t.Run("traits and cluster traits merged", func(t *testing.T) {
		spec, err := BuildSpec(BuildInput{
			Component:     makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
			ComponentType: ct,
			Traits: map[string]openchoreov1alpha1.TraitSpec{
				"trait-a": {},
			},
			ClusterTraits: map[string]openchoreov1alpha1.ClusterTraitSpec{
				"cluster-trait-b": {},
			},
			Workload: workload,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(spec.Traits) != 2 {
			t.Fatalf("expected 2 traits, got %d", len(spec.Traits))
		}
		if !findTrait(spec.Traits, openchoreov1alpha1.TraitRefKindTrait, "trait-a") {
			t.Error("expected Trait:trait-a in merged traits slice")
		}
		if !findTrait(spec.Traits, openchoreov1alpha1.TraitRefKindClusterTrait, "cluster-trait-b") {
			t.Error("expected ClusterTrait:cluster-trait-b in merged traits slice")
		}
	})

	t.Run("same-name Trait and ClusterTrait coexist as separate entries", func(t *testing.T) {
		spec, err := BuildSpec(BuildInput{
			Component:     makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
			ComponentType: ct,
			Traits: map[string]openchoreov1alpha1.TraitSpec{
				"foo": {},
			},
			ClusterTraits: map[string]openchoreov1alpha1.ClusterTraitSpec{
				"foo": {},
			},
			Workload: workload,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(spec.Traits) != 2 {
			t.Fatalf("expected 2 traits (one per kind), got %d", len(spec.Traits))
		}
		if !findTrait(spec.Traits, openchoreov1alpha1.TraitRefKindTrait, "foo") {
			t.Error("expected Trait:foo in traits slice")
		}
		if !findTrait(spec.Traits, openchoreov1alpha1.TraitRefKindClusterTrait, "foo") {
			t.Error("expected ClusterTrait:foo in traits slice")
		}
	})
}

func TestBuildSpec_WithComponentTraits(t *testing.T) {
	ct := makeCT()
	workload := &openchoreov1alpha1.WorkloadTemplateSpec{
		Container: openchoreov1alpha1.Container{Image: "nginx:1.21"},
	}

	t.Run("with parameters and component traits", func(t *testing.T) {
		spec, err := BuildSpec(BuildInput{
			Component: makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{
				Parameters: &runtime.RawExtension{Raw: []byte(`{"replicas": 3}`)},
				Traits: []openchoreov1alpha1.ComponentTrait{
					{Name: "trait-a", InstanceName: "trait-a-1"},
				},
			}),
			ComponentType: ct,
			Traits: map[string]openchoreov1alpha1.TraitSpec{
				"trait-a": {},
			},
			Workload: workload,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if spec.ComponentProfile == nil {
			t.Fatal("expected non-nil componentProfile")
		}
		if spec.ComponentProfile.Parameters == nil {
			t.Error("expected non-nil parameters")
		}
		if len(spec.ComponentProfile.Traits) != 1 {
			t.Errorf("expected 1 component trait, got %d", len(spec.ComponentProfile.Traits))
		}
		pt := spec.ComponentProfile.Traits[0]
		if pt.Kind != openchoreov1alpha1.TraitRefKindTrait {
			t.Errorf("expected Kind=Trait, got %q", pt.Kind)
		}
		if pt.Name != "trait-a" {
			t.Errorf("expected Name=trait-a, got %q", pt.Name)
		}
		if pt.InstanceName != "trait-a-1" {
			t.Errorf("expected InstanceName=trait-a-1, got %q", pt.InstanceName)
		}
	})

	t.Run("missing component trait returns error", func(t *testing.T) {
		_, err := BuildSpec(BuildInput{
			Component: makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{
				Traits: []openchoreov1alpha1.ComponentTrait{
					{Name: "missing-trait"},
				},
			}),
			ComponentType: ct,
			Workload:      workload,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), `component trait "missing-trait" is missing`) {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("cluster trait in wrong map returns error", func(t *testing.T) {
		_, err := BuildSpec(BuildInput{
			Component: makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{
				Traits: []openchoreov1alpha1.ComponentTrait{
					{Kind: openchoreov1alpha1.TraitRefKindClusterTrait, Name: "my-trait"},
				},
			}),
			ComponentType: ct,
			Traits: map[string]openchoreov1alpha1.TraitSpec{
				"my-trait": {},
			},
			Workload: workload,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), `component trait "my-trait" is missing`) {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("component trait in cluster traits passes validation", func(t *testing.T) {
		spec, err := BuildSpec(BuildInput{
			Component: makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{
				Traits: []openchoreov1alpha1.ComponentTrait{
					{Kind: openchoreov1alpha1.TraitRefKindClusterTrait, Name: "my-cluster-trait"},
				},
			}),
			ComponentType: ct,
			ClusterTraits: map[string]openchoreov1alpha1.ClusterTraitSpec{
				"my-cluster-trait": {},
			},
			Workload: workload,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(spec.Traits) != 1 {
			t.Fatalf("expected 1 trait, got %d", len(spec.Traits))
		}
		if !findTrait(spec.Traits, openchoreov1alpha1.TraitRefKindClusterTrait, "my-cluster-trait") {
			t.Error("expected ClusterTrait:my-cluster-trait in traits slice")
		}
	})
}

func TestBuildSpec_EmbeddedTraits(t *testing.T) {
	workload := &openchoreov1alpha1.WorkloadTemplateSpec{
		Container: openchoreov1alpha1.Container{Image: "nginx:1.21"},
	}

	t.Run("missing embedded trait returns error", func(t *testing.T) {
		_, err := BuildSpec(BuildInput{
			Component: makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
			ComponentType: openchoreov1alpha1.ComponentReleaseComponentType{
				Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType,
				Name: "deployment/web-app",
				Spec: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
					Traits: []openchoreov1alpha1.ComponentTypeTrait{
						{Name: "required-trait"},
					},
				},
			},
			Traits:   map[string]openchoreov1alpha1.TraitSpec{},
			Workload: workload,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), `embedded trait "required-trait" required by ComponentType is missing`) {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("all embedded traits present passes validation", func(t *testing.T) {
		spec, err := BuildSpec(BuildInput{
			Component: makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
			ComponentType: openchoreov1alpha1.ComponentReleaseComponentType{
				Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType,
				Name: "deployment/web-app",
				Spec: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
					Traits: []openchoreov1alpha1.ComponentTypeTrait{
						{Name: "required-trait"},
					},
				},
			},
			Traits: map[string]openchoreov1alpha1.TraitSpec{
				"required-trait": {},
			},
			Workload: workload,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(spec.Traits) != 1 {
			t.Fatalf("expected 1 trait, got %d", len(spec.Traits))
		}
		if !findTrait(spec.Traits, openchoreov1alpha1.TraitRefKindTrait, "required-trait") {
			t.Error("expected Trait:required-trait in traits slice")
		}
	})

	t.Run("embedded trait in cluster traits passes validation", func(t *testing.T) {
		spec, err := BuildSpec(BuildInput{
			Component: makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
			ComponentType: openchoreov1alpha1.ComponentReleaseComponentType{
				Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType,
				Name: "deployment/web-app",
				Spec: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
					Traits: []openchoreov1alpha1.ComponentTypeTrait{
						{Kind: openchoreov1alpha1.TraitRefKindClusterTrait, Name: "required-cluster-trait"},
					},
				},
			},
			ClusterTraits: map[string]openchoreov1alpha1.ClusterTraitSpec{
				"required-cluster-trait": {},
			},
			Workload: workload,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(spec.Traits) != 1 {
			t.Fatalf("expected 1 trait, got %d", len(spec.Traits))
		}
		if !findTrait(spec.Traits, openchoreov1alpha1.TraitRefKindClusterTrait, "required-cluster-trait") {
			t.Error("expected ClusterTrait:required-cluster-trait in traits slice")
		}
	})
}
