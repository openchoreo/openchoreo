// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestParseComponentType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType string
		wantName string
		wantErr  bool
	}{
		{
			name:     "valid deployment",
			input:    "deployment/my-ct",
			wantType: "deployment",
			wantName: "my-ct",
		},
		{
			// SplitN(2) means everything after the first slash is the name
			name:     "statefulset with slash in name",
			input:    "statefulset/my/ct",
			wantType: "statefulset",
			wantName: "my/ct",
		},
		{
			name:    "no slash",
			input:   "badformat",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workloadType, ctName, err := parseComponentType(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got workloadType=%q ctName=%q", workloadType, ctName)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if workloadType != tt.wantType {
				t.Errorf("workloadType = %q, want %q", workloadType, tt.wantType)
			}
			if ctName != tt.wantName {
				t.Errorf("ctName = %q, want %q", ctName, tt.wantName)
			}
		})
	}
}

func TestFindRootEnvironment(t *testing.T) {
	makePipeline := func(paths []openchoreov1alpha1.PromotionPath) *openchoreov1alpha1.DeploymentPipeline {
		return &openchoreov1alpha1.DeploymentPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline"},
			Spec:       openchoreov1alpha1.DeploymentPipelineSpec{PromotionPaths: paths},
		}
	}

	tests := []struct {
		name    string
		paths   []openchoreov1alpha1.PromotionPath
		wantEnv string
		wantErr bool
	}{
		{
			name:    "no promotion paths",
			paths:   nil,
			wantErr: true,
		},
		{
			name: "single path dev to staging",
			paths: []openchoreov1alpha1.PromotionPath{
				{
					SourceEnvironmentRef: "dev",
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
						{Name: "staging"},
					},
				},
			},
			wantEnv: "dev",
		},
		{
			name: "linear chain dev to staging to prod",
			paths: []openchoreov1alpha1.PromotionPath{
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
			wantEnv: "dev",
		},
		{
			name: "circular all sources are targets",
			paths: []openchoreov1alpha1.PromotionPath{
				{
					SourceEnvironmentRef: "staging",
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
						{Name: "staging"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "empty source ref is skipped",
			paths: []openchoreov1alpha1.PromotionPath{
				{
					SourceEnvironmentRef: "",
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
						{Name: "staging"},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := findRootEnvironment(makePipeline(tt.paths))
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got env=%q", env)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if env != tt.wantEnv {
				t.Errorf("env = %q, want %q", env, tt.wantEnv)
			}
		})
	}
}

func TestBuildTraitsMap(t *testing.T) {
	makeTrait := func(name string) openchoreov1alpha1.Trait {
		return openchoreov1alpha1.Trait{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       openchoreov1alpha1.TraitSpec{},
		}
	}

	t.Run("nil slice returns nil", func(t *testing.T) {
		result := buildTraitsMap(nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("empty slice returns nil", func(t *testing.T) {
		result := buildTraitsMap([]openchoreov1alpha1.Trait{})
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("single trait", func(t *testing.T) {
		result := buildTraitsMap([]openchoreov1alpha1.Trait{makeTrait("my-trait")})
		if len(result) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(result))
		}
		if _, ok := result["my-trait"]; !ok {
			t.Errorf("expected key 'my-trait' in map")
		}
	})

	t.Run("multiple traits each keyed by name", func(t *testing.T) {
		names := []string{"alpha", "beta", "gamma"}
		result := buildTraitsMap([]openchoreov1alpha1.Trait{
			makeTrait("alpha"),
			makeTrait("beta"),
			makeTrait("gamma"),
		})
		if len(result) != 3 {
			t.Errorf("expected 3 entries, got %d", len(result))
		}
		for _, name := range names {
			if _, ok := result[name]; !ok {
				t.Errorf("missing key %q", name)
			}
		}
	})
}

func TestBuildComponentProfile(t *testing.T) {
	t.Run("no params no traits returns nil", func(t *testing.T) {
		comp := &openchoreov1alpha1.Component{
			Spec: openchoreov1alpha1.ComponentSpec{
				Parameters: nil,
				Traits:     nil,
			},
		}
		if got := buildComponentProfile(comp); got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("has parameters returns non-nil profile", func(t *testing.T) {
		comp := &openchoreov1alpha1.Component{
			Spec: openchoreov1alpha1.ComponentSpec{
				Parameters: &runtime.RawExtension{Raw: []byte(`{"key":"val"}`)},
			},
		}
		profile := buildComponentProfile(comp)
		if profile == nil {
			t.Fatal("expected non-nil profile")
		}
		if profile.Parameters == nil {
			t.Error("expected profile.Parameters to be non-nil")
		}
		if len(profile.Traits) != 0 {
			t.Errorf("expected no traits, got %d", len(profile.Traits))
		}
	})

	t.Run("has traits returns non-nil profile", func(t *testing.T) {
		comp := &openchoreov1alpha1.Component{
			Spec: openchoreov1alpha1.ComponentSpec{
				Traits: []openchoreov1alpha1.ComponentTrait{
					{Name: "t1", InstanceName: "inst1"},
				},
			},
		}
		profile := buildComponentProfile(comp)
		if profile == nil {
			t.Fatal("expected non-nil profile")
		}
		if profile.Parameters != nil {
			t.Error("expected nil Parameters")
		}
		if len(profile.Traits) != 1 {
			t.Errorf("expected 1 trait, got %d", len(profile.Traits))
		}
	})

	t.Run("has both params and traits", func(t *testing.T) {
		comp := &openchoreov1alpha1.Component{
			Spec: openchoreov1alpha1.ComponentSpec{
				Parameters: &runtime.RawExtension{Raw: []byte(`{}`)},
				Traits: []openchoreov1alpha1.ComponentTrait{
					{Name: "t1", InstanceName: "inst1"},
					{Name: "t2", InstanceName: "inst2"},
				},
			},
		}
		profile := buildComponentProfile(comp)
		if profile == nil {
			t.Fatal("expected non-nil profile")
		}
		if profile.Parameters == nil {
			t.Error("expected non-nil Parameters")
		}
		if len(profile.Traits) != 2 {
			t.Errorf("expected 2 traits, got %d", len(profile.Traits))
		}
	})
}
