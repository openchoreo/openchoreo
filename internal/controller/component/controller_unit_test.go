// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
					SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: "dev"},
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
			wantEnv: "dev",
		},
		{
			name: "circular all sources are targets",
			paths: []openchoreov1alpha1.PromotionPath{
				{
					SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: "staging"},
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
					SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: ""},
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
