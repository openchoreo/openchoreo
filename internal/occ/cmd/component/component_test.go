// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
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

func TestUnmarshalSchema(t *testing.T) {
	tests := []struct {
		name    string
		raw     *json.RawMessage
		wantErr bool
		wantKey string
	}{
		{
			name: "valid JSON schema",
			raw: func() *json.RawMessage {
				r := json.RawMessage(`{"type":"object","properties":{"port":{"type":"integer"}}}`)
				return &r
			}(),
			wantKey: "port",
		},
		{
			name: "invalid JSON",
			raw: func() *json.RawMessage {
				r := json.RawMessage(`not-json`)
				return &r
			}(),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := unmarshalSchema(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, schema)
			if tt.wantKey != "" {
				assert.Contains(t, schema.Properties, tt.wantKey)
			}
		})
	}
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponents(mock.Anything, "ns", "", mock.Anything).Return(nil, fmt.Errorf("server error"))

	cp := New(mc)
	assert.EqualError(t, cp.List(ListParams{Namespace: "ns"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponents(mock.Anything, "ns", "", mock.Anything).Return(&gen.ComponentList{
		Items:      []gen.Component{{Metadata: gen.ObjectMeta{Name: "my-comp"}}},
		Pagination: gen.Pagination{},
	}, nil)

	cp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cp.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "my-comp")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponents(mock.Anything, "ns", "", mock.Anything).Return(&gen.ComponentList{
		Items: []gen.Component{
			{Metadata: gen.ObjectMeta{Name: "comp-a", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "comp-b", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	cp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cp.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "comp-a")
	assert.Contains(t, out, "comp-b")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponents(mock.Anything, "ns", "", mock.Anything).Return(&gen.ComponentList{
		Items:      []gen.Component{},
		Pagination: gen.Pagination{},
	}, nil)

	cp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cp.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "No components found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponent(mock.Anything, "ns", "missing").Return(nil, fmt.Errorf("not found: missing"))

	cp := New(mc)
	assert.EqualError(t, cp.Get(GetParams{Namespace: "ns", ComponentName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponent(mock.Anything, "ns", "my-comp").Return(&gen.Component{
		Metadata: gen.ObjectMeta{Name: "my-comp"},
	}, nil)

	cp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cp.Get(GetParams{Namespace: "ns", ComponentName: "my-comp"}))
	})

	assert.Contains(t, out, "name: my-comp")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteComponent(mock.Anything, "ns", "my-comp").Return(fmt.Errorf("forbidden: my-comp"))

	cp := New(mc)
	assert.EqualError(t, cp.Delete(DeleteParams{Namespace: "ns", ComponentName: "my-comp"}), "forbidden: my-comp")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteComponent(mock.Anything, "ns", "my-comp").Return(nil)

	cp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cp.Delete(DeleteParams{Namespace: "ns", ComponentName: "my-comp"}))
	})

	assert.Contains(t, out, "Component 'my-comp' deleted")
}

// --- StartWorkflow tests ---

func TestStartWorkflow_MissingNamespace(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	cp := New(mc)
	err := cp.StartWorkflow(StartWorkflowParams{ComponentName: "my-comp"})
	assert.EqualError(t, err, "namespace is required")
}

func TestStartWorkflow_MissingComponentName(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	cp := New(mc)
	err := cp.StartWorkflow(StartWorkflowParams{Namespace: "ns"})
	assert.EqualError(t, err, "component name is required")
}

func TestStartWorkflow_GetComponentError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponent(mock.Anything, "ns", "my-comp").Return(nil, fmt.Errorf("not found"))

	cp := New(mc)
	err := cp.StartWorkflow(StartWorkflowParams{Namespace: "ns", ComponentName: "my-comp"})
	assert.EqualError(t, err, "not found")
}

func TestStartWorkflow_NoWorkflowConfigured(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponent(mock.Anything, "ns", "my-comp").Return(&gen.Component{
		Metadata: gen.ObjectMeta{Name: "my-comp"},
		Spec:     &gen.ComponentSpec{},
	}, nil)

	cp := New(mc)
	err := cp.StartWorkflow(StartWorkflowParams{Namespace: "ns", ComponentName: "my-comp"})
	assert.EqualError(t, err, `component "my-comp" has no workflow configured`)
}

func TestStartWorkflow_Success(t *testing.T) {
	wfName := "my-workflow"
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponent(mock.Anything, "ns", "my-comp").Return(&gen.Component{
		Metadata: gen.ObjectMeta{Name: "my-comp"},
		Spec: &gen.ComponentSpec{
			Workflow: &gen.ComponentWorkflowConfig{Name: wfName},
		},
	}, nil)
	mc.EXPECT().CreateWorkflowRun(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowRun{
		Metadata: gen.ObjectMeta{Name: "run-1"},
	}, nil)

	cp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cp.StartWorkflow(StartWorkflowParams{
			Namespace:     "ns",
			ComponentName: "my-comp",
			Project:       "my-project",
		}))
	})
	assert.Contains(t, out, "run-1")
}

// --- ListWorkflowRuns tests ---

func TestListWorkflowRuns_MissingNamespace(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	cp := New(mc)
	err := cp.ListWorkflowRuns(ListWorkflowRunsParams{ComponentName: "my-comp"})
	assert.EqualError(t, err, "namespace is required")
}

func TestListWorkflowRuns_MissingComponentName(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	cp := New(mc)
	err := cp.ListWorkflowRuns(ListWorkflowRunsParams{Namespace: "ns"})
	assert.EqualError(t, err, "component name is required")
}

func TestListWorkflowRuns_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.Anything).Return(nil, fmt.Errorf("server error"))

	cp := New(mc)
	err := cp.ListWorkflowRuns(ListWorkflowRunsParams{Namespace: "ns", ComponentName: "my-comp"})
	assert.EqualError(t, err, "server error")
}

func TestListWorkflowRuns_FiltersByComponent(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowRunList{
		Items: []gen.WorkflowRun{
			{
				Metadata: gen.ObjectMeta{
					Name:   "run-match",
					Labels: &map[string]string{"openchoreo.dev/component": "my-comp"},
				},
			},
			{
				Metadata: gen.ObjectMeta{
					Name:   "run-other",
					Labels: &map[string]string{"openchoreo.dev/component": "other-comp"},
				},
			},
		},
		Pagination: gen.Pagination{},
	}, nil)

	cp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cp.ListWorkflowRuns(ListWorkflowRunsParams{Namespace: "ns", ComponentName: "my-comp"}))
	})
	assert.Contains(t, out, "run-match")
	assert.NotContains(t, out, "run-other")
}

func TestListWorkflowRuns_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowRunList{
		Items:      []gen.WorkflowRun{},
		Pagination: gen.Pagination{},
	}, nil)

	cp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cp.ListWorkflowRuns(ListWorkflowRunsParams{Namespace: "ns", ComponentName: "my-comp"}))
	})
	assert.Contains(t, out, "No workflow runs found")
}

// --- Deploy tests ---

const testReleaseName = "rel-1"

func makeLinearPipeline() *gen.DeploymentPipeline {
	paths := []gen.PromotionPath{
		{
			SourceEnvironmentRef: struct {
				Kind *gen.PromotionPathSourceEnvironmentRefKind `json:"kind,omitempty"`
				Name string                                     `json:"name"`
			}{Name: "dev"},
			TargetEnvironmentRefs: []gen.TargetEnvironmentRef{{Name: "staging"}},
		},
		{
			SourceEnvironmentRef: struct {
				Kind *gen.PromotionPathSourceEnvironmentRefKind `json:"kind,omitempty"`
				Name string                                     `json:"name"`
			}{Name: "staging"},
			TargetEnvironmentRefs: []gen.TargetEnvironmentRef{{Name: "prod"}},
		},
	}
	return &gen.DeploymentPipeline{
		Metadata: gen.ObjectMeta{Name: "my-pipeline"},
		Spec:     &gen.DeploymentPipelineSpec{PromotionPaths: &paths},
	}
}

func makeReleaseName(name string) *string { return &name }

func TestDeploy_GenerateReleaseError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GenerateRelease(mock.Anything, "ns", "my-comp", mock.Anything).Return(nil, fmt.Errorf("generate failed"))

	cp := New(mc)
	err := cp.Deploy(DeployParams{
		Namespace:     "ns",
		Project:       "my-project",
		ComponentName: "my-comp",
	})
	assert.EqualError(t, err, "generate failed")
}

func TestDeploy_DeployToLowestEnv_CreateBinding(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GenerateRelease(mock.Anything, "ns", "my-comp", mock.Anything).Return(&gen.ComponentRelease{
		Metadata: gen.ObjectMeta{Name: testReleaseName},
	}, nil)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "ns", "my-project").Return(makeLinearPipeline(), nil)
	mc.EXPECT().GetReleaseBinding(mock.Anything, "ns", "my-comp-dev").Return(nil, nil)
	mc.EXPECT().CreateReleaseBinding(mock.Anything, "ns", mock.Anything).Return(&gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "my-comp-dev"},
		Spec: &gen.ReleaseBindingSpec{
			Environment: "dev",
			ReleaseName: makeReleaseName(testReleaseName),
		},
	}, nil)

	cp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cp.Deploy(DeployParams{
			Namespace:     "ns",
			Project:       "my-project",
			ComponentName: "my-comp",
		}))
	})
	assert.Contains(t, out, "dev")
	assert.Contains(t, out, "my-comp-dev")
}

func TestDeploy_DeployToLowestEnv_UpdateExistingBinding(t *testing.T) {
	relName := "rel-2"
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GenerateRelease(mock.Anything, "ns", "my-comp", mock.Anything).Return(&gen.ComponentRelease{
		Metadata: gen.ObjectMeta{Name: relName},
	}, nil)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "ns", "my-project").Return(makeLinearPipeline(), nil)
	mc.EXPECT().GetReleaseBinding(mock.Anything, "ns", "my-comp-dev").Return(&gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "my-comp-dev"},
		Spec: &gen.ReleaseBindingSpec{
			Environment: "dev",
			ReleaseName: makeReleaseName(testReleaseName),
		},
	}, nil)
	mc.EXPECT().UpdateReleaseBinding(mock.Anything, "ns", "my-comp-dev", mock.Anything).Return(&gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "my-comp-dev"},
		Spec: &gen.ReleaseBindingSpec{
			Environment: "dev",
			ReleaseName: makeReleaseName(relName),
		},
	}, nil)

	cp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cp.Deploy(DeployParams{
			Namespace:     "ns",
			Project:       "my-project",
			ComponentName: "my-comp",
		}))
	})
	assert.Contains(t, out, "my-comp-dev")
}

func TestDeploy_Promote_Success(t *testing.T) {
	relName := testReleaseName
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "ns", "my-project").Return(makeLinearPipeline(), nil)
	mc.EXPECT().ListReleaseBindings(mock.Anything, "ns", mock.Anything).Return(&gen.ReleaseBindingList{
		Items: []gen.ReleaseBinding{
			{
				Metadata: gen.ObjectMeta{Name: "my-comp-dev"},
				Spec: &gen.ReleaseBindingSpec{
					Environment: "dev",
					Owner: struct {
						ComponentName string `json:"componentName"`
						ProjectName   string `json:"projectName"`
					}{ComponentName: "my-comp"},
					ReleaseName: makeReleaseName(relName),
				},
			},
		},
	}, nil)
	mc.EXPECT().GetReleaseBinding(mock.Anything, "ns", "my-comp-staging").Return(nil, nil)
	mc.EXPECT().CreateReleaseBinding(mock.Anything, "ns", mock.Anything).Return(&gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "my-comp-staging"},
		Spec: &gen.ReleaseBindingSpec{
			Environment: "staging",
			ReleaseName: makeReleaseName(relName),
		},
	}, nil)

	cp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cp.Deploy(DeployParams{
			Namespace:     "ns",
			Project:       "my-project",
			ComponentName: "my-comp",
			To:            "staging",
		}))
	})
	assert.Contains(t, out, "staging")
	assert.Contains(t, out, "my-comp-staging")
}

func TestDeploy_DeployWithSet_UpdateBinding(t *testing.T) {
	relName := "rel-2"
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GenerateRelease(mock.Anything, "ns", "my-comp", mock.Anything).Return(&gen.ComponentRelease{
		Metadata: gen.ObjectMeta{Name: relName},
	}, nil)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "ns", "my-project").Return(makeLinearPipeline(), nil)
	mc.EXPECT().GetReleaseBinding(mock.Anything, "ns", "my-comp-dev").Return(&gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "my-comp-dev"},
		Spec:     &gen.ReleaseBindingSpec{Environment: "dev", ReleaseName: makeReleaseName(testReleaseName)},
	}, nil)
	mc.EXPECT().UpdateReleaseBinding(mock.Anything, "ns", "my-comp-dev", mock.Anything).Return(&gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "my-comp-dev"},
		Spec:     &gen.ReleaseBindingSpec{Environment: "dev", ReleaseName: makeReleaseName(relName)},
	}, nil)

	cp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cp.Deploy(DeployParams{
			Namespace:     "ns",
			Project:       "my-project",
			ComponentName: "my-comp",
			Set:           []string{"spec.environment=dev"},
		}))
	})
	assert.Contains(t, out, "my-comp-dev")
}

func TestDeploy_Promote_UpdateExisting(t *testing.T) {
	relName := "rel-1"
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "ns", "my-project").Return(makeLinearPipeline(), nil)
	mc.EXPECT().ListReleaseBindings(mock.Anything, "ns", mock.Anything).Return(&gen.ReleaseBindingList{
		Items: []gen.ReleaseBinding{
			{
				Metadata: gen.ObjectMeta{Name: "my-comp-dev"},
				Spec: &gen.ReleaseBindingSpec{
					Environment: "dev",
					Owner: struct {
						ComponentName string `json:"componentName"`
						ProjectName   string `json:"projectName"`
					}{ComponentName: "my-comp"},
					ReleaseName: makeReleaseName(relName),
				},
			},
		},
	}, nil)
	mc.EXPECT().GetReleaseBinding(mock.Anything, "ns", "my-comp-staging").Return(&gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "my-comp-staging"},
		Spec:     &gen.ReleaseBindingSpec{Environment: "staging", ReleaseName: makeReleaseName("old-rel")},
	}, nil)
	mc.EXPECT().UpdateReleaseBinding(mock.Anything, "ns", "my-comp-staging", mock.Anything).Return(&gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "my-comp-staging"},
		Spec:     &gen.ReleaseBindingSpec{Environment: "staging", ReleaseName: makeReleaseName(relName)},
	}, nil)

	cp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cp.Deploy(DeployParams{
			Namespace:     "ns",
			Project:       "my-project",
			ComponentName: "my-comp",
			To:            "staging",
		}))
	})
	assert.Contains(t, out, "staging")
}

func TestDeploy_Promote_NoSourceBinding(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "ns", "my-project").Return(makeLinearPipeline(), nil)
	mc.EXPECT().ListReleaseBindings(mock.Anything, "ns", mock.Anything).Return(&gen.ReleaseBindingList{
		Items: []gen.ReleaseBinding{},
	}, nil)

	cp := New(mc)
	err := cp.Deploy(DeployParams{
		Namespace:     "ns",
		Project:       "my-project",
		ComponentName: "my-comp",
		To:            "staging",
	})
	assert.ErrorContains(t, err, "no release binding found for source environment")
}

// --- fetchScaffoldSchemas ---

func TestFetchScaffoldSchemas_ClusterCT(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterComponentTypeSchema(mock.Anything, "web-app").Return(&schema, nil)

	res := &scaffoldResolution{useClusterCT: true, componentTypeName: "web-app"}
	ctSchema, traitSchemas, wfSchema, err := fetchScaffoldSchemas(context.Background(), mc, "ns", res)
	require.NoError(t, err)
	assert.NotNil(t, ctSchema)
	assert.Empty(t, traitSchemas)
	assert.Nil(t, wfSchema)
}

func TestFetchScaffoldSchemas_WithWorkflow(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	wfSchema := json.RawMessage(`{"type":"object"}`)
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentTypeSchema(mock.Anything, "ns", "web-app").Return(&schema, nil)
	mc.EXPECT().GetWorkflowSchema(mock.Anything, "ns", "build-wf").Return(&wfSchema, nil)

	res := &scaffoldResolution{componentTypeName: "web-app", workflowName: "build-wf"}
	ctSchema, _, wf, err := fetchScaffoldSchemas(context.Background(), mc, "ns", res)
	require.NoError(t, err)
	assert.NotNil(t, ctSchema)
	assert.NotNil(t, wf)
}

func TestFetchScaffoldSchemas_WithClusterWorkflow(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	wfSchema := json.RawMessage(`{"type":"object"}`)
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentTypeSchema(mock.Anything, "ns", "web-app").Return(&schema, nil)
	mc.EXPECT().GetClusterWorkflowSchema(mock.Anything, "build-wf").Return(&wfSchema, nil)

	res := &scaffoldResolution{componentTypeName: "web-app", workflowName: "build-wf", useClusterWorkflow: true}
	_, _, wf, err := fetchScaffoldSchemas(context.Background(), mc, "ns", res)
	require.NoError(t, err)
	assert.NotNil(t, wf)
}

func TestFetchScaffoldSchemas_WithClusterTrait(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	trSchema := json.RawMessage(`{"type":"object"}`)
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentTypeSchema(mock.Anything, "ns", "web-app").Return(&schema, nil)
	mc.EXPECT().GetClusterTraitSchema(mock.Anything, "my-trait").Return(&trSchema, nil)

	res := &scaffoldResolution{
		componentTypeName: "web-app",
		traitNames:        []string{"my-trait"},
		traitKinds:        map[string]string{"my-trait": "ClusterTrait"},
	}
	_, traits, _, err := fetchScaffoldSchemas(context.Background(), mc, "ns", res)
	require.NoError(t, err)
	assert.NotNil(t, traits["my-trait"])
}

func TestFetchScaffoldSchemas_CTSchemaError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentTypeSchema(mock.Anything, "ns", "web-app").Return(nil, fmt.Errorf("schema fetch failed"))

	res := &scaffoldResolution{componentTypeName: "web-app"}
	_, _, _, err := fetchScaffoldSchemas(context.Background(), mc, "ns", res)
	assert.ErrorContains(t, err, "schema fetch failed")
}

// --- WorkflowRunLogs: runName resolved from component ---

func TestWorkflowRunLogs_ResolvedFromComponent(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponent(mock.Anything, "ns", "my-comp").Return(&gen.Component{
		Spec: &gen.ComponentSpec{
			Workflow: &gen.ComponentWorkflowConfig{Name: "build-wf"},
		},
	}, nil)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowRunList{
		Items: []gen.WorkflowRun{
			{Metadata: gen.ObjectMeta{Name: "run-1", CreationTimestamp: &now,
				Labels: &map[string]string{"openchoreo.dev/component": "my-comp"}}},
		},
		Pagination: gen.Pagination{},
	}, nil)
	mc.EXPECT().GetWorkflowRunStatus(mock.Anything, "ns", "run-1").Return(
		&gen.WorkflowRunStatusResponse{HasLiveObservability: true}, nil)
	mc.EXPECT().GetWorkflowRunLogs(mock.Anything, "ns", "run-1", mock.Anything).Return(
		[]gen.WorkflowRunLogEntry{{Timestamp: &now, Log: "build log"}}, nil)

	cp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cp.WorkflowRunLogs(WorkflowRunLogsParams{
			Namespace:     "ns",
			ComponentName: "my-comp",
		}))
	})
	assert.Contains(t, out, "build log")
}

// --- Scaffold tests ---

func TestScaffold_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentTypeSchema(mock.Anything, "ns", "web-app").Return(nil, fmt.Errorf("schema not found"))

	cp := New(mc)
	err := cp.Scaffold(ScaffoldParams{
		ComponentName: "my-comp",
		Namespace:     "ns",
		ProjectName:   "my-project",
		ComponentType: "deployment/web-app",
	})
	assert.EqualError(t, err, "schema not found")
}

func TestScaffold_MissingComponentName(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	cp := New(mc)
	err := cp.Scaffold(ScaffoldParams{
		Namespace:     "ns",
		ProjectName:   "my-project",
		ComponentType: "deployment/web-app",
	})
	assert.EqualError(t, err, "component name is required")
}

func TestScaffold_MutuallyExclusiveFlags(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	cp := New(mc)
	err := cp.Scaffold(ScaffoldParams{
		ComponentName:        "my-comp",
		Namespace:            "ns",
		ProjectName:          "my-project",
		ComponentType:        "deployment/web-app",
		ClusterComponentType: "deployment/cluster-web-app",
	})
	assert.EqualError(t, err, "--componenttype and --clustercomponenttype are mutually exclusive")
}

// --- resolveComponentWorkflowName ---

func TestResolveComponentWorkflowName_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponent(mock.Anything, "ns", "my-comp").Return(&gen.Component{
		Spec: &gen.ComponentSpec{
			Workflow: &gen.ComponentWorkflowConfig{Name: "build-wf"},
		},
	}, nil)

	cp := New(mc)
	got, err := cp.resolveComponentWorkflowName("ns", "my-comp")
	require.NoError(t, err)
	assert.Equal(t, "build-wf", got)
}

func TestResolveComponentWorkflowName_NoWorkflow(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponent(mock.Anything, "ns", "my-comp").Return(&gen.Component{
		Spec: &gen.ComponentSpec{},
	}, nil)

	cp := New(mc)
	_, err := cp.resolveComponentWorkflowName("ns", "my-comp")
	require.Error(t, err)
	assert.ErrorContains(t, err, "has no workflow configured")
}

func TestResolveComponentWorkflowName_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponent(mock.Anything, "ns", "my-comp").Return(nil, fmt.Errorf("not found"))

	cp := New(mc)
	_, err := cp.resolveComponentWorkflowName("ns", "my-comp")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not found")
}

// --- WorkflowRunLogs with RunName provided ---

func TestWorkflowRunLogs_WithRunName(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkflowRunStatus(mock.Anything, "ns", "run-1").Return(
		&gen.WorkflowRunStatusResponse{HasLiveObservability: true}, nil)
	mc.EXPECT().GetWorkflowRunLogs(mock.Anything, "ns", "run-1", mock.Anything).Return(
		[]gen.WorkflowRunLogEntry{{Timestamp: &now, Log: "build output"}}, nil)

	cp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cp.WorkflowRunLogs(WorkflowRunLogsParams{
			Namespace:     "ns",
			ComponentName: "my-comp",
			RunName:       "run-1",
		}))
	})
	assert.Contains(t, out, "build output")
}

// --- WorkflowRunLogs validation tests ---

func TestWorkflowRunLogs_ValidationError(t *testing.T) {
	cp := New(nil) // client is never called when validation fails

	t.Run("missing namespace", func(t *testing.T) {
		err := cp.WorkflowRunLogs(WorkflowRunLogsParams{ComponentName: "my-comp"})
		require.Error(t, err)
		assert.EqualError(t, err, "namespace is required")
	})

	t.Run("missing component name", func(t *testing.T) {
		err := cp.WorkflowRunLogs(WorkflowRunLogsParams{Namespace: "ns"})
		require.Error(t, err)
		assert.EqualError(t, err, "component name is required")
	})
}

// --- component/logs.go: observer URL resolution ---

func makeEnvWithDataPlaneRef(kind gen.EnvironmentSpecDataPlaneRefKind, name string) *gen.Environment {
	return &gen.Environment{
		Spec: &gen.EnvironmentSpec{
			DataPlaneRef: &struct {
				Kind gen.EnvironmentSpecDataPlaneRefKind `json:"kind"`
				Name string                              `json:"name"`
			}{Kind: kind, Name: name},
		},
	}
}

func TestResolveObserverURL_ClusterDataPlane(t *testing.T) {
	url := "http://cluster-observer.example.com"
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEnvironment(mock.Anything, "ns", "dev").Return(
		makeEnvWithDataPlaneRef(gen.EnvironmentSpecDataPlaneRefKindClusterDataPlane, "my-cdp"), nil)
	mc.EXPECT().GetClusterDataPlane(mock.Anything, "my-cdp").Return(&gen.ClusterDataPlane{
		Spec: &gen.ClusterDataPlaneSpec{
			ObservabilityPlaneRef: &gen.ClusterObservabilityPlaneRef{Name: "default"},
		},
	}, nil)
	mc.EXPECT().GetClusterObservabilityPlane(mock.Anything, "default").Return(
		&gen.ClusterObservabilityPlane{Spec: &gen.ClusterObservabilityPlaneSpec{ObserverURL: &url}}, nil)

	got, err := resolveObserverURL(context.Background(), mc, "ns", "dev")
	require.NoError(t, err)
	assert.Equal(t, url, got)
}

func TestResolveObserverURL_DataPlane_NamespacedObsPlane(t *testing.T) {
	url := "http://obs.example.com"
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEnvironment(mock.Anything, "ns", "dev").Return(
		makeEnvWithDataPlaneRef(gen.EnvironmentSpecDataPlaneRefKindDataPlane, "my-dp"), nil)
	mc.EXPECT().GetDataPlane(mock.Anything, "ns", "my-dp").Return(&gen.DataPlane{
		Spec: &gen.DataPlaneSpec{
			ObservabilityPlaneRef: &gen.ObservabilityPlaneRef{
				Kind: gen.ObservabilityPlaneRefKindObservabilityPlane,
				Name: "my-obs",
			},
		},
	}, nil)
	mc.EXPECT().GetObservabilityPlane(mock.Anything, "ns", "my-obs").Return(
		&gen.ObservabilityPlane{Spec: &gen.ObservabilityPlaneSpec{ObserverURL: &url}}, nil)

	got, err := resolveObserverURL(context.Background(), mc, "ns", "dev")
	require.NoError(t, err)
	assert.Equal(t, url, got)
}

func TestResolveObserverURL_EnvironmentNotFound(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEnvironment(mock.Anything, "ns", "dev").Return(nil, fmt.Errorf("not found"))

	_, err := resolveObserverURL(context.Background(), mc, "ns", "dev")
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to get environment")
}

func TestResolveObserverURL_NoDataPlaneRef(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEnvironment(mock.Anything, "ns", "dev").Return(&gen.Environment{
		Spec: &gen.EnvironmentSpec{},
	}, nil)

	_, err := resolveObserverURL(context.Background(), mc, "ns", "dev")
	require.Error(t, err)
	assert.ErrorContains(t, err, "has no data plane reference")
}

func TestResolveObserverURL_DataPlane_ClusterObsPlane(t *testing.T) {
	url := "http://cluster-obs.example.com"
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEnvironment(mock.Anything, "ns", "dev").Return(
		makeEnvWithDataPlaneRef(gen.EnvironmentSpecDataPlaneRefKindDataPlane, "my-dp"), nil)
	mc.EXPECT().GetDataPlane(mock.Anything, "ns", "my-dp").Return(&gen.DataPlane{
		Spec: &gen.DataPlaneSpec{
			ObservabilityPlaneRef: &gen.ObservabilityPlaneRef{
				Kind: gen.ObservabilityPlaneRefKindClusterObservabilityPlane,
				Name: "my-cluster-obs",
			},
		},
	}, nil)
	mc.EXPECT().GetClusterObservabilityPlane(mock.Anything, "my-cluster-obs").Return(
		&gen.ClusterObservabilityPlane{Spec: &gen.ClusterObservabilityPlaneSpec{ObserverURL: &url}}, nil)

	got, err := resolveObserverURL(context.Background(), mc, "ns", "dev")
	require.NoError(t, err)
	assert.Equal(t, url, got)
}

func TestResolveObserverURL_DataPlane_UnsupportedKind(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEnvironment(mock.Anything, "ns", "dev").Return(
		makeEnvWithDataPlaneRef(gen.EnvironmentSpecDataPlaneRefKindDataPlane, "my-dp"), nil)
	mc.EXPECT().GetDataPlane(mock.Anything, "ns", "my-dp").Return(&gen.DataPlane{
		Spec: &gen.DataPlaneSpec{
			ObservabilityPlaneRef: &gen.ObservabilityPlaneRef{
				Kind: "Unknown",
				Name: "x",
			},
		},
	}, nil)

	_, err := resolveObserverURL(context.Background(), mc, "ns", "dev")
	require.Error(t, err)
	assert.ErrorContains(t, err, "unsupported observabilityPlaneRef kind")
}

func TestResolveObserverURL_DataPlane_GetError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEnvironment(mock.Anything, "ns", "dev").Return(
		makeEnvWithDataPlaneRef(gen.EnvironmentSpecDataPlaneRefKindDataPlane, "my-dp"), nil)
	mc.EXPECT().GetDataPlane(mock.Anything, "ns", "my-dp").Return(nil, fmt.Errorf("dp not found"))

	_, err := resolveObserverURL(context.Background(), mc, "ns", "dev")
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to get data plane")
}

func TestResolveObserverURL_ClusterDataPlane_NoObsRef(t *testing.T) {
	url := "http://default-cluster-obs.example.com"
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEnvironment(mock.Anything, "ns", "dev").Return(
		makeEnvWithDataPlaneRef(gen.EnvironmentSpecDataPlaneRefKindClusterDataPlane, "my-cdp"), nil)
	// ClusterDataPlane has no ObservabilityPlaneRef — falls back to "default"
	mc.EXPECT().GetClusterDataPlane(mock.Anything, "my-cdp").Return(&gen.ClusterDataPlane{
		Spec: &gen.ClusterDataPlaneSpec{},
	}, nil)
	mc.EXPECT().GetClusterObservabilityPlane(mock.Anything, "default").Return(
		&gen.ClusterObservabilityPlane{Spec: &gen.ClusterObservabilityPlaneSpec{ObserverURL: &url}}, nil)

	got, err := resolveObserverURL(context.Background(), mc, "ns", "dev")
	require.NoError(t, err)
	assert.Equal(t, url, got)
}

func TestGetObserverURLFromObservabilityPlane_Error(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetObservabilityPlane(mock.Anything, "ns", "my-obs").Return(nil, fmt.Errorf("not found"))

	_, err := getObserverURLFromObservabilityPlane(context.Background(), mc, "ns", "my-obs")
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to get observability plane")
}

func TestGetObserverURLFromObservabilityPlane_NoURL(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetObservabilityPlane(mock.Anything, "ns", "my-obs").Return(
		&gen.ObservabilityPlane{Spec: &gen.ObservabilityPlaneSpec{}}, nil)

	_, err := getObserverURLFromObservabilityPlane(context.Background(), mc, "ns", "my-obs")
	require.Error(t, err)
	assert.ErrorContains(t, err, "observer URL not configured")
}

func TestGetObserverURLFromClusterObservabilityPlane_Error(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterObservabilityPlane(mock.Anything, "my-obs").Return(nil, fmt.Errorf("not found"))

	_, err := getObserverURLFromClusterObservabilityPlane(context.Background(), mc, "my-obs")
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to get cluster observability plane")
}

func TestGetObserverURLFromClusterObservabilityPlane_NoURL(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterObservabilityPlane(mock.Anything, "my-obs").Return(
		&gen.ClusterObservabilityPlane{Spec: &gen.ClusterObservabilityPlaneSpec{}}, nil)

	_, err := getObserverURLFromClusterObservabilityPlane(context.Background(), mc, "my-obs")
	require.Error(t, err)
	assert.ErrorContains(t, err, "observer URL not configured")
}

func TestResolveObserverURL_DataPlane_DefaultObsPlane(t *testing.T) {
	url := "http://default-obs.example.com"
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEnvironment(mock.Anything, "ns", "dev").Return(
		makeEnvWithDataPlaneRef(gen.EnvironmentSpecDataPlaneRefKindDataPlane, "my-dp"), nil)
	// DataPlane has no ObservabilityPlaneRef — falls back to default
	mc.EXPECT().GetDataPlane(mock.Anything, "ns", "my-dp").Return(&gen.DataPlane{
		Spec: &gen.DataPlaneSpec{},
	}, nil)
	mc.EXPECT().GetObservabilityPlane(mock.Anything, "ns", "default").Return(
		&gen.ObservabilityPlane{Spec: &gen.ObservabilityPlaneSpec{ObserverURL: &url}}, nil)

	got, err := resolveObserverURL(context.Background(), mc, "ns", "dev")
	require.NoError(t, err)
	assert.Equal(t, url, got)
}

// --- List: pagination cursor branch ---

func TestList_Pagination(t *testing.T) {
	cursor := "page2"
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponents(mock.Anything, "ns", "", mock.MatchedBy(func(p *gen.ListComponentsParams) bool {
		return p.Cursor == nil
	})).Return(&gen.ComponentList{
		Items:      []gen.Component{{Metadata: gen.ObjectMeta{Name: "comp-1"}}},
		Pagination: gen.Pagination{NextCursor: &cursor},
	}, nil).Once()
	mc.EXPECT().ListComponents(mock.Anything, "ns", "", mock.MatchedBy(func(p *gen.ListComponentsParams) bool {
		return p.Cursor != nil && *p.Cursor == cursor
	})).Return(&gen.ComponentList{
		Items:      []gen.Component{{Metadata: gen.ObjectMeta{Name: "comp-2"}}},
		Pagination: gen.Pagination{},
	}, nil).Once()

	cp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cp.List(ListParams{Namespace: "ns"}))
	})
	assert.Contains(t, out, "comp-1")
	assert.Contains(t, out, "comp-2")
}

func TestList_WithProjectFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponents(mock.Anything, "ns", "", mock.MatchedBy(func(p *gen.ListComponentsParams) bool {
		return p.Project != nil && *p.Project == "my-project"
	})).Return(&gen.ComponentList{
		Items:      []gen.Component{{Metadata: gen.ObjectMeta{Name: "comp-1"}}},
		Pagination: gen.Pagination{},
	}, nil)

	cp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cp.List(ListParams{Namespace: "ns", Project: "my-project"}))
	})
	assert.Contains(t, out, "comp-1")
}

// --- deployComponent: error paths ---

func TestDeploy_PipelineError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GenerateRelease(mock.Anything, "ns", "my-comp", mock.Anything).Return(&gen.ComponentRelease{
		Metadata: gen.ObjectMeta{Name: "rel-1"},
	}, nil)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "ns", "my-project").Return(nil, fmt.Errorf("pipeline not found"))

	cp := New(mc)
	err := cp.Deploy(DeployParams{Namespace: "ns", Project: "my-project", ComponentName: "my-comp"})
	assert.ErrorContains(t, err, "pipeline not found")
}

func TestDeploy_GetReleaseBindingError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GenerateRelease(mock.Anything, "ns", "my-comp", mock.Anything).Return(&gen.ComponentRelease{
		Metadata: gen.ObjectMeta{Name: "rel-1"},
	}, nil)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "ns", "my-project").Return(makeLinearPipeline(), nil)
	mc.EXPECT().GetReleaseBinding(mock.Anything, "ns", "my-comp-dev").Return(nil, fmt.Errorf("binding fetch error"))

	cp := New(mc)
	err := cp.Deploy(DeployParams{Namespace: "ns", Project: "my-project", ComponentName: "my-comp"})
	assert.ErrorContains(t, err, "binding fetch error")
}

// --- promoteComponent: error paths ---

func TestPromote_PipelineError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "ns", "my-project").Return(nil, fmt.Errorf("pipeline not found"))

	cp := New(mc)
	err := cp.Deploy(DeployParams{Namespace: "ns", Project: "my-project", ComponentName: "my-comp", To: "staging"})
	assert.ErrorContains(t, err, "pipeline not found")
}

func TestPromote_ListBindingsError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "ns", "my-project").Return(makeLinearPipeline(), nil)
	mc.EXPECT().ListReleaseBindings(mock.Anything, "ns", mock.Anything).Return(nil, fmt.Errorf("list error"))

	cp := New(mc)
	err := cp.Deploy(DeployParams{Namespace: "ns", Project: "my-project", ComponentName: "my-comp", To: "staging"})
	assert.ErrorContains(t, err, "list error")
}

func TestPromote_GetTargetBindingError(t *testing.T) {
	relName := "rel-1"
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "ns", "my-project").Return(makeLinearPipeline(), nil)
	mc.EXPECT().ListReleaseBindings(mock.Anything, "ns", mock.Anything).Return(&gen.ReleaseBindingList{
		Items: []gen.ReleaseBinding{
			{
				Metadata: gen.ObjectMeta{Name: "my-comp-dev"},
				Spec: &gen.ReleaseBindingSpec{
					Environment: "dev",
					Owner: struct {
						ComponentName string `json:"componentName"`
						ProjectName   string `json:"projectName"`
					}{ComponentName: "my-comp"},
					ReleaseName: makeReleaseName(relName),
				},
			},
		},
	}, nil)
	mc.EXPECT().GetReleaseBinding(mock.Anything, "ns", "my-comp-staging").Return(nil, fmt.Errorf("get binding error"))

	cp := New(mc)
	err := cp.Deploy(DeployParams{Namespace: "ns", Project: "my-project", ComponentName: "my-comp", To: "staging"})
	assert.ErrorContains(t, err, "get binding error")
}

// --- StartWorkflow: with Parameters and WorkflowKind ---
