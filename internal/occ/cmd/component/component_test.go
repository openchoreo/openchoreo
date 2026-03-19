// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestFindLowestEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		pipeline *gen.DeploymentPipeline
		want     string
		wantErr  string
	}{
		{
			name: "linear dev -> staging -> prod",
			pipeline: makePipeline([]gen.PromotionPath{
				promotionPath("dev", "staging"),
				promotionPath("staging", "prod"),
			}),
			want: "dev",
		},
		{
			name:     "nil spec",
			pipeline: &gen.DeploymentPipeline{},
			wantErr:  "no promotion paths",
		},
		{
			name: "empty paths",
			pipeline: &gen.DeploymentPipeline{
				Spec: &gen.DeploymentPipelineSpec{
					PromotionPaths: &[]gen.PromotionPath{},
				},
			},
			wantErr: "no promotion paths",
		},
		{
			name: "single environment",
			pipeline: makePipeline([]gen.PromotionPath{
				promotionPath("dev", "prod"),
			}),
			want: "dev",
		},
		{
			name: "diamond shape",
			pipeline: makePipeline([]gen.PromotionPath{
				promotionPath("dev", "staging-a", "staging-b"),
				promotionPath("staging-a", "prod"),
				promotionPath("staging-b", "prod"),
			}),
			want: "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findLowestEnvironment(tt.pipeline)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestFindSourceEnvironment(t *testing.T) {
	pipeline := makePipeline([]gen.PromotionPath{
		promotionPath("dev", "staging"),
		promotionPath("staging", "prod"),
	})

	tests := []struct {
		name      string
		pipeline  *gen.DeploymentPipeline
		targetEnv string
		want      string
		wantErr   string
	}{
		{
			name:      "valid target",
			pipeline:  pipeline,
			targetEnv: "staging",
			want:      "dev",
		},
		{
			name:      "target not found",
			pipeline:  pipeline,
			targetEnv: "unknown",
			wantErr:   "no promotion path found",
		},
		{
			name:      "nil spec",
			pipeline:  &gen.DeploymentPipeline{},
			targetEnv: "staging",
			wantErr:   "no promotion paths",
		},
		{
			name: "empty paths",
			pipeline: &gen.DeploymentPipeline{
				Spec: &gen.DeploymentPipelineSpec{
					PromotionPaths: &[]gen.PromotionPath{},
				},
			},
			targetEnv: "staging",
			wantErr:   "no promotion paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findSourceEnvironment(tt.pipeline, tt.targetEnv)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseComponentType(t *testing.T) {
	tests := []struct {
		name          string
		typeStr       string
		wantWorkload  string
		wantComponent string
		wantErr       string
	}{
		{
			name:          "valid deployment/web-app",
			typeStr:       "deployment/web-app",
			wantWorkload:  "deployment",
			wantComponent: "web-app",
		},
		{
			name:    "empty string",
			typeStr: "",
			wantErr: "component type is required",
		},
		{
			name:    "no slash",
			typeStr: "deployment",
			wantErr: "invalid component type format",
		},
		{
			name:    "empty workload",
			typeStr: "/web-app",
			wantErr: "invalid component type format",
		},
		{
			name:    "empty name",
			typeStr: "deployment/",
			wantErr: "invalid component type format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workload, component, err := parseComponentType(tt.typeStr)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantWorkload, workload)
				assert.Equal(t, tt.wantComponent, component)
			}
		})
	}
}

func TestValidateScaffoldParams(t *testing.T) {
	validParams := ScaffoldParams{
		ComponentName: "my-comp",
		Namespace:     "default",
		ProjectName:   "my-project",
		ComponentType: "deployment/web-app",
	}

	tests := []struct {
		name    string
		params  ScaffoldParams
		wantErr string
	}{
		{
			name:   "valid params",
			params: validParams,
		},
		{
			name:    "empty component name",
			params:  ScaffoldParams{Namespace: "ns", ProjectName: "proj", ComponentType: "deployment/web-app"},
			wantErr: "component name is required",
		},
		{
			name:    "empty namespace",
			params:  ScaffoldParams{ComponentName: "c", ProjectName: "proj", ComponentType: "deployment/web-app"},
			wantErr: "namespace is required",
		},
		{
			name:    "empty project",
			params:  ScaffoldParams{ComponentName: "c", Namespace: "ns", ComponentType: "deployment/web-app"},
			wantErr: "project is required",
		},
		{
			name: "both component type flags",
			params: ScaffoldParams{
				ComponentName:        "c",
				Namespace:            "ns",
				ProjectName:          "proj",
				ComponentType:        "deployment/web-app",
				ClusterComponentType: "deployment/web-app",
			},
			wantErr: "mutually exclusive",
		},
		{
			name:    "neither component type flag",
			params:  ScaffoldParams{ComponentName: "c", Namespace: "ns", ProjectName: "proj"},
			wantErr: "one of --componenttype or --clustercomponenttype is required",
		},
		{
			name: "both workflow flags",
			params: ScaffoldParams{
				ComponentName:       "c",
				Namespace:           "ns",
				ProjectName:         "proj",
				ComponentType:       "deployment/web-app",
				WorkflowName:        "wf",
				ClusterWorkflowName: "cwf",
			},
			wantErr: "mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateScaffoldParams(tt.params)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestResolveScaffoldScope(t *testing.T) {
	t.Run("namespace-scoped component type", func(t *testing.T) {
		params := ScaffoldParams{
			ComponentName: "c",
			Namespace:     "ns",
			ProjectName:   "proj",
			ComponentType: "deployment/web-app",
			WorkflowName:  "build",
			Traits:        []string{"ingress"},
		}
		res, err := resolveScaffoldScope(params)
		require.NoError(t, err)
		assert.Equal(t, "deployment", res.workloadType)
		assert.Equal(t, "web-app", res.componentTypeName)
		assert.Equal(t, "ComponentType", res.componentTypeKind)
		assert.Equal(t, "Workflow", res.workflowKind)
		assert.Equal(t, "build", res.workflowName)
		assert.False(t, res.useClusterCT)
		assert.False(t, res.useClusterWorkflow)
		assert.Equal(t, []string{"ingress"}, res.traitNames)
		assert.Equal(t, "Trait", res.traitKinds["ingress"])
	})

	t.Run("cluster-scoped component type", func(t *testing.T) {
		params := ScaffoldParams{
			ComponentName:        "c",
			Namespace:            "ns",
			ProjectName:          "proj",
			ClusterComponentType: "deployment/web-app",
			ClusterWorkflowName:  "cluster-build",
		}
		res, err := resolveScaffoldScope(params)
		require.NoError(t, err)
		assert.Equal(t, "deployment", res.workloadType)
		assert.Equal(t, "web-app", res.componentTypeName)
		assert.Equal(t, "ClusterComponentType", res.componentTypeKind)
		assert.Equal(t, "ClusterWorkflow", res.workflowKind)
		assert.Equal(t, "cluster-build", res.workflowName)
		assert.True(t, res.useClusterCT)
		assert.True(t, res.useClusterWorkflow)
	})

	t.Run("with cluster workflow and cluster traits", func(t *testing.T) {
		params := ScaffoldParams{
			ComponentName:       "c",
			Namespace:           "ns",
			ProjectName:         "proj",
			ComponentType:       "deployment/web-app",
			ClusterWorkflowName: "cwf",
			Traits:              []string{"ns-trait"},
			ClusterTraits:       []string{"cluster-trait"},
		}
		res, err := resolveScaffoldScope(params)
		require.NoError(t, err)
		assert.Equal(t, "ClusterWorkflow", res.workflowKind)
		assert.Equal(t, "cwf", res.workflowName)
		assert.Contains(t, res.traitNames, "ns-trait")
		assert.Contains(t, res.traitNames, "cluster-trait")
		assert.Equal(t, "Trait", res.traitKinds["ns-trait"])
		assert.Equal(t, "ClusterTrait", res.traitKinds["cluster-trait"])
	})
}

func TestMergeOverridesWithBinding(t *testing.T) {
	baseBinding := func() *gen.ReleaseBinding {
		env := "dev"
		return &gen.ReleaseBinding{
			Spec: &gen.ReleaseBindingSpec{
				Environment: env,
				Owner: struct {
					ComponentName string `json:"componentName"`
					ProjectName   string `json:"projectName"`
				}{
					ComponentName: "my-comp",
					ProjectName:   "my-proj",
				},
			},
		}
	}

	t.Run("simple override", func(t *testing.T) {
		rb, err := mergeOverridesWithBinding(baseBinding(), []string{"spec.environment=staging"})
		require.NoError(t, err)
		assert.Equal(t, "staging", rb.Spec.Environment)
		assert.Equal(t, "my-comp", rb.Spec.Owner.ComponentName)
	})

	t.Run("multiple overrides", func(t *testing.T) {
		rb, err := mergeOverridesWithBinding(baseBinding(), []string{
			"spec.environment=staging",
			"spec.owner.projectName=new-proj",
		})
		require.NoError(t, err)
		assert.Equal(t, "staging", rb.Spec.Environment)
		assert.Equal(t, "new-proj", rb.Spec.Owner.ProjectName)
	})

	t.Run("invalid set value", func(t *testing.T) {
		_, err := mergeOverridesWithBinding(baseBinding(), []string{"invalid-no-equals"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to merge overrides")
	})

	t.Run("empty set values", func(t *testing.T) {
		rb, err := mergeOverridesWithBinding(baseBinding(), []string{})
		require.NoError(t, err)
		assert.Equal(t, "dev", rb.Spec.Environment)
		assert.Equal(t, "my-comp", rb.Spec.Owner.ComponentName)
	})
}
