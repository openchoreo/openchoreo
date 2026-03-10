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

func makeComponent(project, name string, spec openchoreov1alpha1.ComponentSpec) *openchoreov1alpha1.Component {
	spec.Owner.ProjectName = project
	return &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       spec,
	}
}

func TestBuildSpec(t *testing.T) {
	ctSpec := &openchoreov1alpha1.ComponentTypeSpec{
		WorkloadType: "deployment",
	}
	workload := &openchoreov1alpha1.WorkloadTemplateSpec{
		Container: openchoreov1alpha1.Container{
			Image: "nginx:1.21",
		},
	}

	tests := []struct {
		name     string
		input    BuildInput
		wantErr  string
		validate func(t *testing.T, spec *openchoreov1alpha1.ComponentReleaseSpec)
	}{
		{
			name: "nil component returns error",
			input: BuildInput{
				Component:     nil,
				ComponentType: ctSpec,
				Workload:      workload,
			},
			wantErr: "component cannot be nil",
		},
		{
			name: "nil componentType returns error",
			input: BuildInput{
				Component:     makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
				ComponentType: nil,
				Workload:      workload,
			},
			wantErr: "componentType cannot be nil",
		},
		{
			name: "nil workload returns error",
			input: BuildInput{
				Component:     makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
				ComponentType: ctSpec,
				Workload:      nil,
			},
			wantErr: "workload cannot be nil",
		},
		{
			name: "minimal input with no traits or parameters",
			input: BuildInput{
				Component:     makeComponent("my-project", "my-component", openchoreov1alpha1.ComponentSpec{}),
				ComponentType: ctSpec,
				Workload:      workload,
			},
			validate: func(t *testing.T, spec *openchoreov1alpha1.ComponentReleaseSpec) {
				if spec.Owner.ProjectName != "my-project" {
					t.Errorf("expected projectName 'my-project', got %q", spec.Owner.ProjectName)
				}
				if spec.Owner.ComponentName != "my-component" {
					t.Errorf("expected componentName 'my-component', got %q", spec.Owner.ComponentName)
				}
				if spec.Traits != nil {
					t.Errorf("expected nil traits, got %v", spec.Traits)
				}
				if spec.ComponentProfile != nil {
					t.Errorf("expected nil componentProfile, got %v", spec.ComponentProfile)
				}
				if spec.ComponentType.WorkloadType != "deployment" {
					t.Errorf("expected workloadType 'deployment', got %q", spec.ComponentType.WorkloadType)
				}
				if spec.Workload.Container.Image != "nginx:1.21" {
					t.Errorf("expected image 'nginx:1.21', got %q", spec.Workload.Container.Image)
				}
			},
		},
		{
			name: "with traits",
			input: BuildInput{
				Component:     makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
				ComponentType: ctSpec,
				Traits: map[string]openchoreov1alpha1.TraitSpec{
					"trait-a": {Creates: []openchoreov1alpha1.TraitCreate{{TargetPlane: "dataplane"}}},
					"trait-b": {},
				},
				Workload: workload,
			},
			validate: func(t *testing.T, spec *openchoreov1alpha1.ComponentReleaseSpec) {
				if len(spec.Traits) != 2 {
					t.Fatalf("expected 2 traits, got %d", len(spec.Traits))
				}
				if _, ok := spec.Traits["trait-a"]; !ok {
					t.Error("expected trait-a in traits map")
				}
				if _, ok := spec.Traits["trait-b"]; !ok {
					t.Error("expected trait-b in traits map")
				}
			},
		},
		{
			name: "with parameters and component traits",
			input: BuildInput{
				Component: makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{
					Parameters: &runtime.RawExtension{Raw: []byte(`{"replicas": 3}`)},
					Traits: []openchoreov1alpha1.ComponentTrait{
						{Name: "trait-a"},
					},
				}),
				ComponentType: ctSpec,
				Traits: map[string]openchoreov1alpha1.TraitSpec{
					"trait-a": {},
				},
				Workload: workload,
			},
			validate: func(t *testing.T, spec *openchoreov1alpha1.ComponentReleaseSpec) {
				if spec.ComponentProfile == nil {
					t.Fatal("expected non-nil componentProfile")
				}
				if spec.ComponentProfile.Parameters == nil {
					t.Error("expected non-nil parameters")
				}
				if len(spec.ComponentProfile.Traits) != 1 {
					t.Errorf("expected 1 component trait, got %d", len(spec.ComponentProfile.Traits))
				}
			},
		},
		{
			name: "missing component trait returns error",
			input: BuildInput{
				Component: makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{
					Traits: []openchoreov1alpha1.ComponentTrait{
						{Name: "missing-trait"},
					},
				}),
				ComponentType: ctSpec,
				Workload:      workload,
			},
			wantErr: `component trait "missing-trait" is missing`,
		},
		{
			name: "cluster trait in wrong map returns error",
			input: BuildInput{
				Component: makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{
					Traits: []openchoreov1alpha1.ComponentTrait{
						{Kind: openchoreov1alpha1.TraitRefKindClusterTrait, Name: "my-trait"},
					},
				}),
				ComponentType: ctSpec,
				Traits: map[string]openchoreov1alpha1.TraitSpec{
					"my-trait": {},
				},
				Workload: workload,
			},
			wantErr: `component trait "my-trait" is missing`,
		},
		{
			name: "missing embedded trait returns error",
			input: BuildInput{
				Component: makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
				ComponentType: &openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
					Traits: []openchoreov1alpha1.ComponentTypeTrait{
						{Name: "required-trait"},
					},
				},
				Traits:   map[string]openchoreov1alpha1.TraitSpec{},
				Workload: workload,
			},
			wantErr: `embedded trait "required-trait" required by ComponentType is missing`,
		},
		{
			name: "all embedded traits present passes validation",
			input: BuildInput{
				Component: makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
				ComponentType: &openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
					Traits: []openchoreov1alpha1.ComponentTypeTrait{
						{Name: "required-trait"},
					},
				},
				Traits: map[string]openchoreov1alpha1.TraitSpec{
					"required-trait": {},
				},
				Workload: workload,
			},
			validate: func(t *testing.T, spec *openchoreov1alpha1.ComponentReleaseSpec) {
				if len(spec.Traits) != 1 {
					t.Fatalf("expected 1 trait, got %d", len(spec.Traits))
				}
			},
		},
		{
			name: "nil traits map produces nil",
			input: BuildInput{
				Component:     makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
				ComponentType: ctSpec,
				Workload:      workload,
			},
			validate: func(t *testing.T, spec *openchoreov1alpha1.ComponentReleaseSpec) {
				if spec.Traits != nil {
					t.Errorf("expected nil traits, got %v", spec.Traits)
				}
			},
		},
		{
			name: "traits and cluster traits merged",
			input: BuildInput{
				Component:     makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
				ComponentType: ctSpec,
				Traits: map[string]openchoreov1alpha1.TraitSpec{
					"trait-a": {},
				},
				ClusterTraits: map[string]openchoreov1alpha1.ClusterTraitSpec{
					"cluster-trait-b": {},
				},
				Workload: workload,
			},
			validate: func(t *testing.T, spec *openchoreov1alpha1.ComponentReleaseSpec) {
				if len(spec.Traits) != 2 {
					t.Fatalf("expected 2 traits, got %d", len(spec.Traits))
				}
				if _, ok := spec.Traits["trait-a"]; !ok {
					t.Error("expected trait-a in merged traits map")
				}
				if _, ok := spec.Traits["cluster-trait-b"]; !ok {
					t.Error("expected cluster-trait-b in merged traits map")
				}
			},
		},
		{
			name: "embedded trait in cluster traits passes validation",
			input: BuildInput{
				Component: makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
				ComponentType: &openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
					Traits: []openchoreov1alpha1.ComponentTypeTrait{
						{Kind: openchoreov1alpha1.TraitRefKindClusterTrait, Name: "required-cluster-trait"},
					},
				},
				ClusterTraits: map[string]openchoreov1alpha1.ClusterTraitSpec{
					"required-cluster-trait": {},
				},
				Workload: workload,
			},
			validate: func(t *testing.T, spec *openchoreov1alpha1.ComponentReleaseSpec) {
				if len(spec.Traits) != 1 {
					t.Fatalf("expected 1 trait, got %d", len(spec.Traits))
				}
			},
		},
		{
			name: "trait name collision across kinds returns error",
			input: BuildInput{
				Component:     makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{}),
				ComponentType: ctSpec,
				Traits: map[string]openchoreov1alpha1.TraitSpec{
					"foo": {},
				},
				ClusterTraits: map[string]openchoreov1alpha1.ClusterTraitSpec{
					"foo": {},
				},
				Workload: workload,
			},
			wantErr: `trait name "foo" exists as both Trait and ClusterTrait`,
		},
		{
			name: "component trait in cluster traits passes validation",
			input: BuildInput{
				Component: makeComponent("proj", "comp", openchoreov1alpha1.ComponentSpec{
					Traits: []openchoreov1alpha1.ComponentTrait{
						{Kind: openchoreov1alpha1.TraitRefKindClusterTrait, Name: "my-cluster-trait"},
					},
				}),
				ComponentType: ctSpec,
				ClusterTraits: map[string]openchoreov1alpha1.ClusterTraitSpec{
					"my-cluster-trait": {},
				},
				Workload: workload,
			},
			validate: func(t *testing.T, spec *openchoreov1alpha1.ComponentReleaseSpec) {
				if len(spec.Traits) != 1 {
					t.Fatalf("expected 1 trait, got %d", len(spec.Traits))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := BuildSpec(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, spec)
			}
		})
	}
}
