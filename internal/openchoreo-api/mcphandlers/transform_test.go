// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

func TestExtractCommonMeta(t *testing.T) {
	ts := metav1.NewTime(time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC))
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-ns",
			CreationTimestamp: ts,
			Annotations: map[string]string{
				controller.AnnotationKeyDisplayName: "Test Namespace",
				controller.AnnotationKeyDescription: "A test namespace",
			},
		},
	}

	m := extractCommonMeta(ns)

	assert.Equal(t, "test-ns", m["name"])
	assert.Equal(t, "Test Namespace", m["displayName"])
	assert.Equal(t, "A test namespace", m["description"])
	assert.Equal(t, "2025-06-15T10:30:00Z", m["createdAt"])
	assert.NotContains(t, m, "namespace")
}

func TestExtractCommonMeta_NamespacedResource(t *testing.T) {
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-project",
			Namespace: "org-ns",
		},
	}

	m := extractCommonMeta(project)

	assert.Equal(t, "my-project", m["name"])
	assert.Equal(t, "org-ns", m["namespace"])
	assert.NotContains(t, m, "displayName")
	assert.NotContains(t, m, "createdAt")
}

func TestReadyStatus(t *testing.T) {
	tests := []struct {
		name       string
		conditions []metav1.Condition
		want       string
	}{
		{
			name:       "no conditions",
			conditions: nil,
			want:       "",
		},
		{
			name: "ready true",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
			},
			want: "Ready",
		},
		{
			name: "ready false with reason",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Pending"},
			},
			want: "Pending",
		},
		{
			name: "no ready condition",
			conditions: []metav1.Condition{
				{Type: "Reconciled", Status: metav1.ConditionTrue},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, readyStatus(tt.conditions))
		})
	}
}

func TestConditionsSummary(t *testing.T) {
	assert.Nil(t, conditionsSummary(nil))

	conditions := []metav1.Condition{
		{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "AllGood",
			Message: "Everything is fine",
		},
		{
			Type:   "Reconciled",
			Status: metav1.ConditionTrue,
		},
	}

	result := conditionsSummary(conditions)
	require.Len(t, result, 2)

	assert.Equal(t, "Ready", result[0]["type"])
	assert.Equal(t, "True", result[0]["status"])
	assert.Equal(t, "AllGood", result[0]["reason"])
	assert.Equal(t, "Everything is fine", result[0]["message"])

	assert.NotContains(t, result[1], "reason")
	assert.NotContains(t, result[1], "message")
}

func TestTransformList(t *testing.T) {
	items := []openchoreov1alpha1.Project{
		{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "ns"}},
	}

	result := transformList(items, projectSummary)
	require.Len(t, result, 2)
	assert.Equal(t, "p1", result[0]["name"])
	assert.Equal(t, "p2", result[1]["name"])
}

func TestWrapTransformedList(t *testing.T) {
	items := []openchoreov1alpha1.Project{
		{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns"}},
	}

	result := wrapTransformedList("projects", items, "", projectSummary)
	projects, ok := result["projects"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, projects, 1)
	assert.NotContains(t, result, "next_cursor")

	result = wrapTransformedList("projects", items, "abc123", projectSummary)
	assert.Equal(t, "abc123", result["next_cursor"])
}

func TestMutationResult(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ns"},
	}

	m := mutationResult(ns, "created")
	assert.Equal(t, "test-ns", m["name"])
	assert.Equal(t, "created", m["action"])
	assert.NotContains(t, m, "namespace")

	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns"},
	}
	m = mutationResult(project, "patched", map[string]any{"extra": "value"})
	assert.Equal(t, "ns", m["namespace"])
	assert.Equal(t, "value", m["extra"])
}

func TestNamespaceSummary(t *testing.T) {
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ns"},
	}
	m := namespaceSummary(ns)
	assert.Equal(t, "test-ns", m["name"])
}

func TestProjectSummary(t *testing.T) {
	p := openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-project",
			Namespace: "org-ns",
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
				Name: "default-pipeline",
			},
		},
		Status: openchoreov1alpha1.ProjectStatus{
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
			},
		},
	}

	m := projectSummary(p)
	assert.Equal(t, "my-project", m["name"])
	assert.Equal(t, "default-pipeline", m["deploymentPipelineRef"])
	assert.Equal(t, "Ready", m["status"])
}

func TestComponentSummary(t *testing.T) {
	c := openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-comp",
			Namespace: "org-ns",
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{
				ProjectName: "my-project",
			},
			ComponentType: openchoreov1alpha1.ComponentTypeRef{
				Name: "deployment/my-type",
			},
			AutoDeploy: true,
		},
		Status: openchoreov1alpha1.ComponentStatus{
			LatestRelease: &openchoreov1alpha1.LatestRelease{
				Name: "v1",
			},
		},
	}

	m := componentSummary(c)
	assert.Equal(t, "my-project", m["projectName"])
	assert.Equal(t, "deployment/my-type", m["componentType"])
	assert.Equal(t, true, m["autoDeploy"])
	assert.Equal(t, "v1", m["latestRelease"])
}

func TestComponentDetail(t *testing.T) {
	t.Run("workflow with kind and parameters", func(t *testing.T) {
		c := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "org-ns"},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner:      openchoreov1alpha1.ComponentOwner{ProjectName: "my-project"},
				AutoDeploy: true,
				Workflow: &openchoreov1alpha1.ComponentWorkflowConfig{
					Kind:       openchoreov1alpha1.WorkflowRefKindClusterWorkflow,
					Name:       "go-build",
					Parameters: &runtime.RawExtension{Raw: []byte(`{"branch":"main"}`)},
				},
			},
		}

		m := componentDetail(c)

		wf, ok := m["workflow"].(map[string]any)
		require.True(t, ok, "expected workflow to be a map")
		assert.Equal(t, "go-build", wf["name"])
		assert.Equal(t, "ClusterWorkflow", wf["kind"])
		assert.NotNil(t, wf["parameters"])
	})

	t.Run("workflow without kind", func(t *testing.T) {
		c := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "org-ns"},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner: openchoreov1alpha1.ComponentOwner{ProjectName: "my-project"},
				Workflow: &openchoreov1alpha1.ComponentWorkflowConfig{
					Name: "ns-build",
				},
			},
		}

		m := componentDetail(c)

		wf, ok := m["workflow"].(map[string]any)
		require.True(t, ok, "expected workflow to be a map")
		assert.Equal(t, "ns-build", wf["name"])
		assert.NotContains(t, wf, "kind")
	})

	t.Run("workflow kind Workflow", func(t *testing.T) {
		c := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "org-ns"},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner: openchoreov1alpha1.ComponentOwner{ProjectName: "my-project"},
				Workflow: &openchoreov1alpha1.ComponentWorkflowConfig{
					Kind: openchoreov1alpha1.WorkflowRefKindWorkflow,
					Name: "ns-build",
				},
			},
		}

		m := componentDetail(c)

		wf, ok := m["workflow"].(map[string]any)
		require.True(t, ok, "expected workflow to be a map")
		assert.Equal(t, "Workflow", wf["kind"])
	})

	t.Run("no workflow", func(t *testing.T) {
		c := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "org-ns"},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner: openchoreov1alpha1.ComponentOwner{ProjectName: "my-project"},
			},
		}

		m := componentDetail(c)
		assert.NotContains(t, m, "workflow")
	})

	t.Run("autoBuild, parameters, traits, latestRelease, conditions", func(t *testing.T) {
		autoBuild := true
		c := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "full-comp", Namespace: "org-ns"},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner:     openchoreov1alpha1.ComponentOwner{ProjectName: "proj"},
				AutoBuild: &autoBuild,
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{"replicas":3}`),
				},
				Traits: []openchoreov1alpha1.ComponentTrait{
					{
						Kind:         openchoreov1alpha1.TraitRefKindClusterTrait,
						Name:         "ingress",
						InstanceName: "ingress-1",
						Parameters:   &runtime.RawExtension{Raw: []byte(`{"host":"example.com"}`)},
					},
					{
						Name:         "logging",
						InstanceName: "logging-1",
					},
				},
			},
			Status: openchoreov1alpha1.ComponentStatus{
				LatestRelease: &openchoreov1alpha1.LatestRelease{Name: "v2"},
				Conditions: []metav1.Condition{
					{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood", Message: "ok"},
				},
			},
		}

		m := componentDetail(c)

		assert.Equal(t, true, m["autoBuild"])

		params, ok := m["parameters"].(map[string]any)
		require.True(t, ok, "expected parameters to be map[string]any")
		assert.Equal(t, float64(3), params["replicas"])

		traits, ok := m["traits"].([]map[string]any)
		require.True(t, ok, "expected traits to be []map[string]any")
		require.Len(t, traits, 2)
		assert.Equal(t, "ClusterTrait", traits[0]["kind"])
		assert.Equal(t, "ingress", traits[0]["name"])
		assert.Equal(t, "ingress-1", traits[0]["instanceName"])

		traitParams, ok := traits[0]["parameters"].(map[string]any)
		require.True(t, ok, "expected trait[0].parameters to be map[string]any")
		assert.Equal(t, "example.com", traitParams["host"])

		assert.NotContains(t, traits[1], "kind")
		assert.Nil(t, traits[1]["parameters"])

		assert.Equal(t, "v2", m["latestRelease"])

		conds, ok := m["conditions"].([]map[string]any)
		require.True(t, ok, "expected conditions to be []map[string]any")
		assert.Equal(t, "Ready", conds[0]["type"])
		assert.Equal(t, "AllGood", conds[0]["reason"])
	})

	t.Run("minimal - no optional fields", func(t *testing.T) {
		c := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "bare-comp", Namespace: "org-ns"},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner: openchoreov1alpha1.ComponentOwner{ProjectName: "proj"},
			},
		}
		m := componentDetail(c)
		assert.NotContains(t, m, "autoBuild")
		assert.NotContains(t, m, "parameters")
		assert.NotContains(t, m, "traits")
		assert.NotContains(t, m, "latestRelease")
		assert.NotContains(t, m, "conditions")
	})
}

func TestWorkloadSummary(t *testing.T) {
	w := openchoreov1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl-1", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{
				ProjectName:   "proj",
				ComponentName: "comp",
			},
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
				Container: openchoreov1alpha1.Container{Image: "nginx:latest"},
				Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
					"http": {Type: "HTTP", Port: 8080},
				},
			},
		},
	}

	m := workloadSummary(w)
	assert.Equal(t, "proj", m["projectName"])
	assert.Equal(t, "comp", m["componentName"])
	assert.Equal(t, "nginx:latest", m["image"])

	eps, ok := m["endpoints"].([]string)
	require.True(t, ok, "expected endpoints to be []string")
	assert.Len(t, eps, 1)
}

func TestWorkloadSummary_NoEndpoints(t *testing.T) {
	w := openchoreov1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl-bare", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{
				ProjectName:   "proj",
				ComponentName: "comp",
			},
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
				Container: openchoreov1alpha1.Container{Image: "app:v1"},
			},
		},
	}
	m := workloadSummary(w)
	assert.NotContains(t, m, "endpoints")
}

func TestWorkloadDetail(t *testing.T) {
	w := &openchoreov1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl-1", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{
				ProjectName:   "proj",
				ComponentName: "comp",
			},
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
				Container: openchoreov1alpha1.Container{
					Image:   "app:v1",
					Command: []string{"/app"},
					Args:    []string{"--port=8080"},
					Env: []openchoreov1alpha1.EnvVar{
						{Key: "DB_HOST", Value: "localhost"},
						{Key: "SECRET_KEY"},
					},
				},
				Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
					"api": {
						Type:        "HTTP",
						Port:        8080,
						TargetPort:  9090,
						DisplayName: "API",
						BasePath:    "/v1",
						Visibility:  []openchoreov1alpha1.EndpointVisibility{"namespace"},
					},
				},
				Dependencies: &openchoreov1alpha1.WorkloadDependencies{
					Endpoints: []openchoreov1alpha1.WorkloadConnection{
						{
							Project:    "other-proj",
							Component:  "db-service",
							Name:       "grpc",
							Visibility: "project",
							EnvBindings: openchoreov1alpha1.ConnectionEnvBindings{
								Address:  "DB_ADDR",
								Host:     "DB_HOST",
								Port:     "DB_PORT",
								BasePath: "DB_PATH",
							},
						},
						{
							Component:  "cache",
							Name:       "redis",
							Visibility: "project",
							EnvBindings: openchoreov1alpha1.ConnectionEnvBindings{
								Address: "CACHE_ADDR",
							},
						},
					},
				},
			},
		},
	}

	m := workloadDetail(w)
	assert.Equal(t, "proj", m["projectName"])
	assert.Equal(t, "comp", m["componentName"])

	container, ok := m["container"].(map[string]any)
	require.True(t, ok, "expected container to be a map")
	assert.Equal(t, "app:v1", container["image"])
	assert.NotNil(t, container["command"])
	assert.NotNil(t, container["args"])

	envs, ok := container["env"].([]map[string]any)
	require.True(t, ok, "expected container.env to be []map[string]any")
	require.Len(t, envs, 2)
	assert.Equal(t, "DB_HOST", envs[0]["key"])
	assert.Equal(t, "localhost", envs[0]["value"])
	assert.NotContains(t, envs[1], "value")

	eps, ok := m["endpoints"].(map[string]any)
	require.True(t, ok, "expected endpoints to be map[string]any")
	apiEp, ok := eps["api"].(map[string]any)
	require.True(t, ok, "expected api endpoint to be a map")
	assert.Equal(t, "HTTP", apiEp["type"])
	assert.Equal(t, int32(8080), apiEp["port"])
	assert.Equal(t, int32(9090), apiEp["targetPort"])
	assert.Equal(t, "API", apiEp["displayName"])
	assert.Equal(t, "/v1", apiEp["basePath"])
	vis, ok := apiEp["visibility"].([]string)
	require.True(t, ok, "expected visibility to be []string")
	assert.Len(t, vis, 1)

	deps, ok := m["dependencies"].([]map[string]any)
	require.True(t, ok, "expected dependencies to be []map[string]any")
	require.Len(t, deps, 2)
	assert.Equal(t, "other-proj", deps[0]["project"])
	assert.Equal(t, "db-service", deps[0]["component"])

	bindings, ok := deps[0]["envBindings"].(map[string]any)
	require.True(t, ok, "expected envBindings to be a map")
	assert.Equal(t, "DB_ADDR", bindings["address"])
	assert.Equal(t, "DB_HOST", bindings["host"])
	assert.Equal(t, "DB_PORT", bindings["port"])
	assert.Equal(t, "DB_PATH", bindings["basePath"])

	assert.NotContains(t, deps[1], "project")
	bindings2, ok := deps[1]["envBindings"].(map[string]any)
	require.True(t, ok, "expected dep[1].envBindings to be a map")
	assert.Equal(t, "CACHE_ADDR", bindings2["address"])
	assert.NotContains(t, bindings2, "host")
}

func TestContainerToMap(t *testing.T) {
	c := &openchoreov1alpha1.Container{Image: "minimal:v1"}
	m := containerToMap(c)
	assert.Equal(t, "minimal:v1", m["image"])
	assert.NotContains(t, m, "command")
	assert.NotContains(t, m, "args")
	assert.NotContains(t, m, "env")
}

func TestEnvironmentSummary(t *testing.T) {
	e := openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dev",
			Namespace: "org-ns",
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			IsProduction: false,
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
				Kind: "DataPlane",
				Name: "dp-1",
			},
		},
	}

	m := environmentSummary(e)
	assert.Equal(t, "dev", m["name"])
	assert.Equal(t, false, m["isProduction"])

	ref, ok := m["dataPlaneRef"].(map[string]any)
	require.True(t, ok, "expected dataPlaneRef to be a map")
	assert.Equal(t, "DataPlane", ref["kind"])
	assert.Equal(t, "dp-1", ref["name"])
}

func TestEnvironmentSummary_NilDataPlaneRef(t *testing.T) {
	e := openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{Name: "staging", Namespace: "org-ns"},
		Spec:       openchoreov1alpha1.EnvironmentSpec{IsProduction: true},
	}
	m := environmentSummary(e)
	assert.Equal(t, true, m["isProduction"])
	assert.NotContains(t, m, "dataPlaneRef")
}

func TestDataplaneSummary(t *testing.T) {
	dp := openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dp-1",
			Namespace: "org-ns",
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "plane-123",
		},
		Status: openchoreov1alpha1.DataPlaneStatus{
			AgentConnection: &openchoreov1alpha1.AgentConnectionStatus{
				Connected: true,
			},
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
			},
		},
	}

	m := dataplaneSummary(dp)
	assert.Equal(t, "plane-123", m["planeID"])
	assert.Equal(t, true, m["agentConnected"])
	assert.Equal(t, "Ready", m["status"])
}

func TestDataplaneDetail(t *testing.T) {
	connTime := metav1.NewTime(time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC))
	disconnTime := metav1.NewTime(time.Date(2025, 6, 15, 9, 0, 0, 0, time.UTC))
	heartbeatTime := metav1.NewTime(time.Date(2025, 6, 15, 10, 1, 0, 0, time.UTC))

	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp-1", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "plane-1",
			ObservabilityPlaneRef: &openchoreov1alpha1.ObservabilityPlaneRef{
				Kind: "ObservabilityPlane",
				Name: "obs-1",
			},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{Name: "my-store"},
		},
		Status: openchoreov1alpha1.DataPlaneStatus{
			AgentConnection: &openchoreov1alpha1.AgentConnectionStatus{
				Connected:            true,
				ConnectedAgents:      2,
				LastConnectedTime:    &connTime,
				LastDisconnectedTime: &disconnTime,
				LastHeartbeatTime:    &heartbeatTime,
				Message:              "all agents healthy",
			},
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK"},
			},
		},
	}

	m := dataplaneDetail(dp)
	assert.Equal(t, "plane-1", m["planeID"])

	obsRef, ok := m["observabilityPlaneRef"].(map[string]any)
	require.True(t, ok, "expected observabilityPlaneRef to be a map")
	assert.Equal(t, "obs-1", obsRef["name"])

	assert.Equal(t, "my-store", m["secretStoreRef"])

	ac, ok := m["agentConnection"].(map[string]any)
	require.True(t, ok, "expected agentConnection to be a map")
	assert.Equal(t, true, ac["connected"])
	assert.Equal(t, 2, ac["connectedAgents"])
	assert.Equal(t, "2025-06-15T10:00:00Z", ac["lastConnectedTime"])
	assert.Equal(t, "2025-06-15T09:00:00Z", ac["lastDisconnectedTime"])
	assert.Equal(t, "2025-06-15T10:01:00Z", ac["lastHeartbeatTime"])
	assert.Equal(t, "all agents healthy", ac["message"])
	assert.Len(t, m["conditions"], 1)
}

func TestDataplaneDetail_Minimal(t *testing.T) {
	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp-bare", Namespace: "org-ns"},
	}
	m := dataplaneDetail(dp)
	assert.Equal(t, "dp-bare", m["name"])
	assert.NotContains(t, m, "planeID")
	assert.NotContains(t, m, "observabilityPlaneRef")
	assert.NotContains(t, m, "secretStoreRef")
	assert.NotContains(t, m, "agentConnection")
	assert.NotContains(t, m, "conditions")
}

func TestAgentConnectionToMap_Minimal(t *testing.T) {
	ac := &openchoreov1alpha1.AgentConnectionStatus{
		Connected:       false,
		ConnectedAgents: 0,
	}
	m := agentConnectionToMap(ac)
	assert.Equal(t, false, m["connected"])
	assert.NotContains(t, m, "lastConnectedTime")
	assert.NotContains(t, m, "lastDisconnectedTime")
	assert.NotContains(t, m, "lastHeartbeatTime")
}

func TestDeploymentPipelineSummary(t *testing.T) {
	dp := openchoreov1alpha1.DeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "pipeline-1", Namespace: "org-ns"},
		Status: openchoreov1alpha1.DeploymentPipelineStatus{
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
			},
		},
	}
	m := deploymentPipelineSummary(dp)
	assert.Equal(t, "pipeline-1", m["name"])
	assert.Equal(t, "Ready", m["status"])
}

func TestDeploymentPipelineDetail(t *testing.T) {
	dp := &openchoreov1alpha1.DeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "pipeline-1", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: []openchoreov1alpha1.PromotionPath{
				{
					SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{
						Kind: "Environment",
						Name: "dev",
					},
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
						{Kind: "Environment", Name: "staging"},
						{Kind: "Environment", Name: "prod"},
					},
				},
			},
		},
		Status: openchoreov1alpha1.DeploymentPipelineStatus{
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK"},
			},
		},
	}

	m := deploymentPipelineDetail(dp)
	paths, ok := m["promotionPaths"].([]map[string]any)
	require.True(t, ok, "expected promotionPaths to be []map[string]any")
	require.Len(t, paths, 1)

	src, ok := paths[0]["sourceEnvironmentRef"].(map[string]any)
	require.True(t, ok, "expected sourceEnvironmentRef to be a map")
	assert.Equal(t, "dev", src["name"])

	targets, ok := paths[0]["targetEnvironmentRefs"].([]map[string]any)
	require.True(t, ok, "expected targetEnvironmentRefs to be []map[string]any")
	require.Len(t, targets, 2)
	assert.Equal(t, "staging", targets[0]["name"])
	assert.Len(t, m["conditions"], 1)
}

func TestComponentReleaseSummary(t *testing.T) {
	cr := openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{Name: "cr-1", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   "proj",
				ComponentName: "comp",
			},
		},
	}
	m := componentReleaseSummary(cr)
	assert.Equal(t, "cr-1", m["name"])
	assert.Equal(t, "proj", m["projectName"])
	assert.Equal(t, "comp", m["componentName"])
}

func TestComponentReleaseDetail(t *testing.T) {
	cr := &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{Name: "cr-1", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   "proj",
				ComponentName: "comp",
			},
			ComponentType: openchoreov1alpha1.ComponentReleaseComponentType{
				Kind: "ComponentType",
				Name: "deployment/web",
				Spec: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
					Resources:    []openchoreov1alpha1.ResourceTemplate{{ID: "deployment"}},
				},
			},
			Workload: openchoreov1alpha1.WorkloadTemplateSpec{
				Container: openchoreov1alpha1.Container{Image: "myapp:v1"},
				Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
					"http": {Type: "HTTP", Port: 80},
				},
				Dependencies: &openchoreov1alpha1.WorkloadDependencies{
					Endpoints: []openchoreov1alpha1.WorkloadConnection{
						{Component: "db", Name: "pg", Visibility: "project"},
					},
				},
			},
			ComponentProfile: &openchoreov1alpha1.ComponentProfile{
				Parameters: &runtime.RawExtension{Raw: []byte(`{"replicas":2}`)},
			},
		},
	}

	m := componentReleaseDetail(cr)
	assert.Equal(t, "proj", m["projectName"])
	assert.Equal(t, "comp", m["componentName"])

	ctRef, ok := m["componentType"].(map[string]any)
	require.True(t, ok, "expected componentType to be a map")
	assert.Equal(t, "ComponentType", ctRef["kind"])
	assert.Equal(t, "deployment/web", ctRef["name"])
	assert.Equal(t, "deployment", m["workloadType"])
	assert.Equal(t, "myapp:v1", m["image"])
	assert.Len(t, m["endpoints"], 1)
	deps, ok := m["dependencies"].(*openchoreov1alpha1.WorkloadDependencies)
	require.True(t, ok, "expected dependencies to be *WorkloadDependencies")
	assert.Len(t, deps.Endpoints, 1)

	params, ok := m["parameters"].(map[string]any)
	require.True(t, ok, "expected parameters to be map[string]any")
	assert.Equal(t, float64(2), params["replicas"])
}

func TestComponentReleaseDetail_Minimal(t *testing.T) {
	cr := &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{Name: "cr-min", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   "proj",
				ComponentName: "comp",
			},
			ComponentType: openchoreov1alpha1.ComponentReleaseComponentType{
				Kind: "ComponentType",
				Name: "deployment/web",
				Spec: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
					Resources:    []openchoreov1alpha1.ResourceTemplate{{ID: "deployment"}},
				},
			},
			Workload: openchoreov1alpha1.WorkloadTemplateSpec{
				Container: openchoreov1alpha1.Container{Image: "myapp:v1"},
			},
		},
	}

	m := componentReleaseDetail(cr)
	assert.NotContains(t, m, "endpoints")
	assert.NotContains(t, m, "dependencies")
	assert.NotContains(t, m, "parameters")
}

func TestReleaseBindingSummary(t *testing.T) {
	rb := openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "rb-1", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner: openchoreov1alpha1.ReleaseBindingOwner{
				ProjectName:   "proj",
				ComponentName: "comp",
			},
			Environment: "dev",
			ReleaseName: "v1",
			State:       openchoreov1alpha1.ReleaseStateActive,
		},
		Status: openchoreov1alpha1.ReleaseBindingStatus{
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Progressing"},
			},
		},
	}

	m := releaseBindingSummary(rb)
	assert.Equal(t, "proj", m["projectName"])
	assert.Equal(t, "comp", m["componentName"])
	assert.Equal(t, "dev", m["environment"])
	assert.Equal(t, "v1", m["releaseName"])
	assert.Equal(t, "Active", m["state"])
	assert.Equal(t, "Progressing", m["status"])
}

func TestReleaseBindingDetail(t *testing.T) {
	rb := &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "rb-1", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner: openchoreov1alpha1.ReleaseBindingOwner{
				ProjectName:   "proj",
				ComponentName: "comp",
			},
			Environment: "dev",
			ReleaseName: "v1",
			State:       openchoreov1alpha1.ReleaseStateActive,
			ComponentTypeEnvironmentConfigs: &runtime.RawExtension{
				Raw: []byte(`{"scaling":"auto"}`),
			},
			TraitEnvironmentConfigs: map[string]runtime.RawExtension{
				"ingress-1": {Raw: []byte(`{"tls":true}`)},
			},
			WorkloadOverrides: &openchoreov1alpha1.WorkloadOverrideTemplateSpec{
				Container: &openchoreov1alpha1.ContainerOverride{
					Env: []openchoreov1alpha1.EnvVar{{Key: "EXTRA"}},
				},
			},
		},
		Status: openchoreov1alpha1.ReleaseBindingStatus{
			Endpoints: []openchoreov1alpha1.EndpointURLStatus{
				{Name: "http"},
			},
			ConnectionTargets:   []openchoreov1alpha1.ConnectionTarget{{Namespace: "ns"}},
			ResolvedConnections: []openchoreov1alpha1.ResolvedConnection{{Namespace: "ns"}},
			PendingConnections:  []openchoreov1alpha1.PendingConnection{{Namespace: "ns"}},
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
			},
		},
	}

	m := releaseBindingDetail(rb)
	assert.Equal(t, "proj", m["projectName"])
	assert.Equal(t, "comp", m["componentName"])
	assert.Equal(t, "dev", m["environment"])
	assert.Equal(t, "v1", m["releaseName"])
	assert.Equal(t, "Active", m["state"])

	ctec, ok := m["componentTypeEnvironmentConfigs"].(map[string]any)
	require.True(t, ok, "expected componentTypeEnvironmentConfigs to be map[string]any")
	assert.Equal(t, "auto", ctec["scaling"])

	tec, ok := m["traitEnvironmentConfigs"].(map[string]any)
	require.True(t, ok, "expected traitEnvironmentConfigs to be map[string]any")
	ingressConfig, ok := tec["ingress-1"].(map[string]any)
	require.True(t, ok, "expected ingress-1 config to be map[string]any")
	assert.Equal(t, true, ingressConfig["tls"])

	wo, ok := m["workloadOverrides"].(*openchoreov1alpha1.WorkloadOverrideTemplateSpec)
	require.True(t, ok, "expected workloadOverrides to be *WorkloadOverrideTemplateSpec")
	assert.NotNil(t, wo.Container)
	assert.Len(t, m["endpoints"], 1)
	assert.Len(t, m["connectionTargets"], 1)
	assert.Len(t, m["resolvedConnections"], 1)
	assert.Len(t, m["pendingConnections"], 1)
	assert.Equal(t, "Ready", m["status"])
}

func TestReleaseBindingDetail_Minimal(t *testing.T) {
	rb := &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "rb-min", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner: openchoreov1alpha1.ReleaseBindingOwner{
				ProjectName:   "proj",
				ComponentName: "comp",
			},
			Environment: "dev",
		},
	}
	m := releaseBindingDetail(rb)
	assert.NotContains(t, m, "releaseName")
	assert.NotContains(t, m, "state")
	assert.NotContains(t, m, "componentTypeEnvironmentConfigs")
	assert.NotContains(t, m, "traitEnvironmentConfigs")
	assert.NotContains(t, m, "workloadOverrides")
	assert.NotContains(t, m, "connectionTargets")
	assert.NotContains(t, m, "resolvedConnections")
	assert.NotContains(t, m, "pendingConnections")
}

func TestRawExtensionToAny(t *testing.T) {
	assert.Nil(t, rawExtensionToAny(nil))
	assert.Nil(t, rawExtensionToAny(&runtime.RawExtension{}))

	raw := &runtime.RawExtension{Raw: []byte(`{"key":"value"}`)}
	result := rawExtensionToAny(raw)
	m, ok := result.(map[string]any)
	require.True(t, ok, "expected map[string]any")
	assert.Equal(t, "value", m["key"])

	invalid := &runtime.RawExtension{Raw: []byte(`not-json`)}
	result = rawExtensionToAny(invalid)
	assert.Equal(t, "not-json", result)
}

func TestWorkflowRunSummary(t *testing.T) {
	started := metav1.NewTime(time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC))
	completed := metav1.NewTime(time.Date(2025, 6, 15, 10, 5, 0, 0, time.UTC))

	wr := openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{Name: "build-run-1", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.WorkflowRunSpec{
			Workflow: openchoreov1alpha1.WorkflowRunConfig{Name: "build"},
		},
		Status: openchoreov1alpha1.WorkflowRunStatus{
			Conditions:  []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}},
			StartedAt:   &started,
			CompletedAt: &completed,
		},
	}

	m := workflowRunSummary(wr)
	assert.Equal(t, "build", m["workflowName"])
	assert.Equal(t, "Ready", m["status"])
	assert.Equal(t, "2025-06-15T10:00:00Z", m["startedAt"])
	assert.Equal(t, "2025-06-15T10:05:00Z", m["completedAt"])
}

func TestWorkflowRunDetail(t *testing.T) {
	started := metav1.NewTime(time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC))
	completed := metav1.NewTime(time.Date(2025, 6, 15, 10, 5, 0, 0, time.UTC))
	taskStarted := metav1.NewTime(time.Date(2025, 6, 15, 10, 1, 0, 0, time.UTC))
	taskCompleted := metav1.NewTime(time.Date(2025, 6, 15, 10, 3, 0, 0, time.UTC))

	wr := &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{Name: "run-1", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.WorkflowRunSpec{
			Workflow: openchoreov1alpha1.WorkflowRunConfig{
				Name:       "build",
				Parameters: &runtime.RawExtension{Raw: []byte(`{"branch":"main"}`)},
			},
		},
		Status: openchoreov1alpha1.WorkflowRunStatus{
			Conditions:  []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Succeeded"}},
			StartedAt:   &started,
			CompletedAt: &completed,
			Tasks: []openchoreov1alpha1.WorkflowTask{
				{
					Name:        "build-step",
					Phase:       "Succeeded",
					Message:     "done",
					StartedAt:   &taskStarted,
					CompletedAt: &taskCompleted,
				},
				{Name: "pending-step"},
			},
		},
	}

	m := workflowRunDetail(wr)
	assert.Equal(t, "build", m["workflowName"])
	params, ok := m["parameters"].(map[string]any)
	require.True(t, ok, "expected parameters to be map[string]any")
	assert.Equal(t, "main", params["branch"])
	assert.Equal(t, "2025-06-15T10:00:00Z", m["startedAt"])
	assert.Equal(t, "2025-06-15T10:05:00Z", m["completedAt"])

	tasks, ok := m["tasks"].([]map[string]any)
	require.True(t, ok, "expected tasks to be []map[string]any")
	require.Len(t, tasks, 2)
	assert.Equal(t, "build-step", tasks[0]["name"])
	assert.Equal(t, "Succeeded", tasks[0]["phase"])
	assert.Equal(t, "done", tasks[0]["message"])
	assert.Equal(t, "2025-06-15T10:01:00Z", tasks[0]["startedAt"])
	assert.Equal(t, "2025-06-15T10:03:00Z", tasks[0]["completedAt"])
	assert.NotContains(t, tasks[1], "phase")
	assert.Len(t, m["conditions"], 1)
}

func TestWorkflowRunDetail_Minimal(t *testing.T) {
	wr := &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{Name: "run-min", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.WorkflowRunSpec{
			Workflow: openchoreov1alpha1.WorkflowRunConfig{Name: "build"},
		},
	}
	m := workflowRunDetail(wr)
	assert.Equal(t, "build", m["workflowName"])
	assert.NotContains(t, m, "parameters")
	assert.NotContains(t, m, "startedAt")
	assert.NotContains(t, m, "completedAt")
	assert.NotContains(t, m, "tasks")
	assert.NotContains(t, m, "conditions")
}

func TestWorkflowSummary(t *testing.T) {
	wf := openchoreov1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "build-wf", Namespace: "org-ns"},
		Spec:       openchoreov1alpha1.WorkflowSpec{TTLAfterCompletion: "90d"},
		Status: openchoreov1alpha1.WorkflowStatus{
			Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}},
		},
	}
	m := workflowSummary(wf)
	assert.Equal(t, "build-wf", m["name"])
	assert.Equal(t, "90d", m["ttlAfterCompletion"])
	assert.Equal(t, "Ready", m["status"])
}

func TestWorkflowDetail(t *testing.T) {
	wf := &openchoreov1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "build-wf", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.WorkflowSpec{
			TTLAfterCompletion: "90d",
			RunTemplate:        &runtime.RawExtension{Raw: []byte(`{"apiVersion":"v1"}`)},
		},
		Status: openchoreov1alpha1.WorkflowStatus{
			Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}},
		},
	}
	m := workflowDetail(wf)
	spec, ok := m["spec"].(map[string]any)
	require.True(t, ok, "expected spec to be map[string]any")
	assert.Equal(t, "90d", spec["ttlAfterCompletion"])
	assert.Equal(t, "Ready", m["status"])
}

func TestComponentTypeSummary(t *testing.T) {
	ct := openchoreov1alpha1.ComponentType{
		ObjectMeta: metav1.ObjectMeta{Name: "web-app", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.ComponentTypeSpec{
			WorkloadType:     "deployment",
			AllowedWorkflows: []openchoreov1alpha1.WorkflowRef{{Kind: "ClusterWorkflow", Name: "go-build"}},
			Resources:        []openchoreov1alpha1.ResourceTemplate{{ID: "deployment"}},
		},
	}
	m := componentTypeSummary(ct)
	assert.Equal(t, "deployment", m["workloadType"])
	wfs, ok := m["allowedWorkflows"].([]map[string]string)
	require.True(t, ok, "expected allowedWorkflows to be []map[string]string")
	require.Len(t, wfs, 1)
	assert.Equal(t, "go-build", wfs[0]["name"])
}

func TestComponentTypeDetail(t *testing.T) {
	ct := &openchoreov1alpha1.ComponentType{
		ObjectMeta: metav1.ObjectMeta{Name: "web-app", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.ComponentTypeSpec{
			WorkloadType: "deployment",
			Resources:    []openchoreov1alpha1.ResourceTemplate{{ID: "deployment"}},
		},
	}
	m := componentTypeDetail(ct)
	spec, ok := m["spec"].(map[string]any)
	require.True(t, ok, "expected spec to be map[string]any")
	assert.Equal(t, "deployment", spec["workloadType"])
}

func TestTraitSummary(t *testing.T) {
	tr := openchoreov1alpha1.Trait{
		ObjectMeta: metav1.ObjectMeta{Name: "ingress", Namespace: "org-ns"},
	}
	assert.Equal(t, "ingress", traitSummary(tr)["name"])
}

func TestTraitDetail(t *testing.T) {
	tr := &openchoreov1alpha1.Trait{
		ObjectMeta: metav1.ObjectMeta{Name: "ingress", Namespace: "org-ns"},
		Spec:       openchoreov1alpha1.TraitSpec{Creates: []openchoreov1alpha1.TraitCreate{{}}},
	}
	spec, ok := traitDetail(tr)["spec"].(map[string]any)
	require.True(t, ok, "expected spec to be map[string]any")
	assert.Contains(t, spec, "creates")
}

func TestWorkflowPlaneSummary(t *testing.T) {
	wp := openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp-1", Namespace: "org-ns"},
		Spec:       openchoreov1alpha1.WorkflowPlaneSpec{PlaneID: "wf-plane-1"},
		Status: openchoreov1alpha1.WorkflowPlaneStatus{
			AgentConnection: &openchoreov1alpha1.AgentConnectionStatus{Connected: true},
			Conditions:      []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}},
		},
	}
	m := workflowPlaneSummary(wp)
	assert.Equal(t, "wf-plane-1", m["planeID"])
	assert.Equal(t, true, m["agentConnected"])
	assert.Equal(t, "Ready", m["status"])
}

func TestWorkflowPlaneDetail(t *testing.T) {
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp-1", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			PlaneID: "wf-plane-1",
			ObservabilityPlaneRef: &openchoreov1alpha1.ObservabilityPlaneRef{
				Kind: "ObservabilityPlane",
				Name: "obs-1",
			},
		},
		Status: openchoreov1alpha1.WorkflowPlaneStatus{
			AgentConnection: &openchoreov1alpha1.AgentConnectionStatus{Connected: true},
			Conditions:      []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK"}},
		},
	}
	m := workflowPlaneDetail(wp)
	assert.Equal(t, "wf-plane-1", m["planeID"])

	obsRef, ok := m["observabilityPlaneRef"].(map[string]any)
	require.True(t, ok, "expected observabilityPlaneRef to be a map")
	assert.Equal(t, "obs-1", obsRef["name"])
	ac, ok := m["agentConnection"].(map[string]any)
	require.True(t, ok, "expected agentConnection to be map[string]any")
	assert.Equal(t, true, ac["connected"])
	assert.Len(t, m["conditions"], 1)
}

func TestWorkflowPlaneDetail_Minimal(t *testing.T) {
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp-bare", Namespace: "org-ns"},
	}
	m := workflowPlaneDetail(wp)
	assert.Equal(t, "wp-bare", m["name"])
	assert.NotContains(t, m, "planeID")
	assert.NotContains(t, m, "observabilityPlaneRef")
	assert.NotContains(t, m, "agentConnection")
	assert.NotContains(t, m, "conditions")
}

func TestObservabilityPlaneSummary(t *testing.T) {
	op := openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "obs-1", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.ObservabilityPlaneSpec{
			PlaneID:     "obs-plane-1",
			ObserverURL: "https://observer.example.com",
		},
		Status: openchoreov1alpha1.ObservabilityPlaneStatus{
			AgentConnection: &openchoreov1alpha1.AgentConnectionStatus{Connected: true},
			Conditions:      []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}},
		},
	}
	m := observabilityPlaneSummary(op)
	assert.Equal(t, "obs-plane-1", m["planeID"])
	assert.Equal(t, "https://observer.example.com", m["observerURL"])
	assert.Equal(t, true, m["agentConnected"])
	assert.Equal(t, "Ready", m["status"])
}

func TestObservabilityPlaneDetail(t *testing.T) {
	op := &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "obs-1", Namespace: "org-ns"},
		Spec: openchoreov1alpha1.ObservabilityPlaneSpec{
			PlaneID:     "obs-plane-1",
			ObserverURL: "https://observer.example.com",
		},
		Status: openchoreov1alpha1.ObservabilityPlaneStatus{
			AgentConnection: &openchoreov1alpha1.AgentConnectionStatus{Connected: true},
			Conditions:      []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK"}},
		},
	}
	m := observabilityPlaneDetail(op)
	assert.Equal(t, "obs-plane-1", m["planeID"])
	assert.Equal(t, "https://observer.example.com", m["observerURL"])
	ac, ok := m["agentConnection"].(map[string]any)
	require.True(t, ok, "expected agentConnection to be map[string]any")
	assert.Equal(t, true, ac["connected"])
	assert.Len(t, m["conditions"], 1)
}

func TestSecretReferenceSummary(t *testing.T) {
	sr := openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{Name: "sr-1", Namespace: "org-ns"},
		Status: openchoreov1alpha1.SecretReferenceStatus{
			Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}},
		},
	}
	m := secretReferenceSummary(sr)
	assert.Equal(t, "sr-1", m["name"])
	assert.Equal(t, "Ready", m["status"])
}

func TestClusterDataPlaneSummary(t *testing.T) {
	cdp := openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cdp-1"},
		Spec:       openchoreov1alpha1.ClusterDataPlaneSpec{PlaneID: "cdp-plane"},
		Status: openchoreov1alpha1.ClusterDataPlaneStatus{
			AgentConnection: &openchoreov1alpha1.AgentConnectionStatus{Connected: true},
			Conditions:      []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}},
		},
	}
	m := clusterDataPlaneSummary(cdp)
	assert.Equal(t, "cdp-plane", m["planeID"])
	assert.Equal(t, true, m["agentConnected"])
	assert.Equal(t, "Ready", m["status"])
}

func TestClusterDataPlaneDetail(t *testing.T) {
	cdp := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cdp-1"},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			PlaneID: "cdp-plane",
			ObservabilityPlaneRef: &openchoreov1alpha1.ClusterObservabilityPlaneRef{
				Kind: "ClusterObservabilityPlane",
				Name: "cobs-1",
			},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{Name: "store"},
		},
		Status: openchoreov1alpha1.ClusterDataPlaneStatus{
			AgentConnection: &openchoreov1alpha1.AgentConnectionStatus{Connected: true},
			Conditions:      []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK"}},
		},
	}
	m := clusterDataPlaneDetail(cdp)
	assert.Equal(t, "cdp-plane", m["planeID"])

	obsRef, ok := m["observabilityPlaneRef"].(map[string]any)
	require.True(t, ok, "expected observabilityPlaneRef to be a map")
	assert.Equal(t, "cobs-1", obsRef["name"])
	assert.Equal(t, "store", m["secretStoreRef"])
	ac, ok := m["agentConnection"].(map[string]any)
	require.True(t, ok, "expected agentConnection to be map[string]any")
	assert.Equal(t, true, ac["connected"])
	assert.Len(t, m["conditions"], 1)
}

func TestClusterWorkflowPlaneSummary(t *testing.T) {
	cwp := openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cwp-1"},
		Spec:       openchoreov1alpha1.ClusterWorkflowPlaneSpec{PlaneID: "cwp-plane"},
		Status: openchoreov1alpha1.ClusterWorkflowPlaneStatus{
			AgentConnection: &openchoreov1alpha1.AgentConnectionStatus{Connected: true},
		},
	}
	m := clusterWorkflowPlaneSummary(cwp)
	assert.Equal(t, "cwp-plane", m["planeID"])
	assert.Equal(t, true, m["agentConnected"])
}

func TestClusterWorkflowPlaneDetail(t *testing.T) {
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cwp-1"},
		Spec:       openchoreov1alpha1.ClusterWorkflowPlaneSpec{PlaneID: "cwp-plane"},
		Status: openchoreov1alpha1.ClusterWorkflowPlaneStatus{
			AgentConnection: &openchoreov1alpha1.AgentConnectionStatus{Connected: true},
			Conditions:      []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK"}},
		},
	}
	m := clusterWorkflowPlaneDetail(cwp)
	assert.Equal(t, "cwp-plane", m["planeID"])
	ac, ok := m["agentConnection"].(map[string]any)
	require.True(t, ok, "expected agentConnection to be map[string]any")
	assert.Equal(t, true, ac["connected"])
	assert.Len(t, m["conditions"], 1)
}

func TestClusterObservabilityPlaneSummary(t *testing.T) {
	cop := openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cop-1"},
		Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
			PlaneID:     "cop-plane",
			ObserverURL: "https://obs.example.com",
		},
		Status: openchoreov1alpha1.ClusterObservabilityPlaneStatus{
			AgentConnection: &openchoreov1alpha1.AgentConnectionStatus{Connected: false},
		},
	}
	m := clusterObservabilityPlaneSummary(cop)
	assert.Equal(t, "cop-plane", m["planeID"])
	assert.Equal(t, "https://obs.example.com", m["observerURL"])
	assert.Equal(t, false, m["agentConnected"])
}

func TestClusterObservabilityPlaneDetail(t *testing.T) {
	cop := &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cop-1"},
		Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
			PlaneID:     "cop-plane",
			ObserverURL: "https://obs.example.com",
		},
		Status: openchoreov1alpha1.ClusterObservabilityPlaneStatus{
			AgentConnection: &openchoreov1alpha1.AgentConnectionStatus{Connected: true},
			Conditions:      []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK"}},
		},
	}
	m := clusterObservabilityPlaneDetail(cop)
	assert.Equal(t, "cop-plane", m["planeID"])
	assert.Equal(t, "https://obs.example.com", m["observerURL"])
	ac, ok := m["agentConnection"].(map[string]any)
	require.True(t, ok, "expected agentConnection to be map[string]any")
	assert.Equal(t, true, ac["connected"])
	assert.Len(t, m["conditions"], 1)
}

func TestClusterComponentTypeSummary(t *testing.T) {
	cct := openchoreov1alpha1.ClusterComponentType{
		ObjectMeta: metav1.ObjectMeta{Name: "cct-1"},
		Spec: openchoreov1alpha1.ClusterComponentTypeSpec{
			WorkloadType:     "deployment",
			AllowedWorkflows: []openchoreov1alpha1.ClusterWorkflowRef{{Kind: "ClusterWorkflow", Name: "go-build"}},
			Resources:        []openchoreov1alpha1.ResourceTemplate{{ID: "deployment"}},
		},
	}
	m := clusterComponentTypeSummary(cct)
	assert.Equal(t, "deployment", m["workloadType"])
	wfs, ok := m["allowedWorkflows"].([]map[string]string)
	require.True(t, ok, "expected allowedWorkflows to be []map[string]string")
	require.Len(t, wfs, 1)
	assert.Equal(t, "go-build", wfs[0]["name"])
}

func TestClusterComponentTypeDetail(t *testing.T) {
	cct := &openchoreov1alpha1.ClusterComponentType{
		ObjectMeta: metav1.ObjectMeta{Name: "cct-1"},
		Spec: openchoreov1alpha1.ClusterComponentTypeSpec{
			WorkloadType: "deployment",
			Resources:    []openchoreov1alpha1.ResourceTemplate{{ID: "deployment"}},
		},
	}
	assert.NotNil(t, clusterComponentTypeDetail(cct)["spec"])
}

func TestClusterTraitSummary(t *testing.T) {
	ct := openchoreov1alpha1.ClusterTrait{ObjectMeta: metav1.ObjectMeta{Name: "ct-1"}}
	assert.Equal(t, "ct-1", clusterTraitSummary(ct)["name"])
}

func TestClusterTraitDetail(t *testing.T) {
	ct := &openchoreov1alpha1.ClusterTrait{
		ObjectMeta: metav1.ObjectMeta{Name: "ct-1"},
		Spec:       openchoreov1alpha1.ClusterTraitSpec{Creates: []openchoreov1alpha1.TraitCreate{{}}},
	}
	assert.NotNil(t, clusterTraitDetail(ct)["spec"])
}

func TestClusterWorkflowSummary(t *testing.T) {
	cwf := openchoreov1alpha1.ClusterWorkflow{ObjectMeta: metav1.ObjectMeta{Name: "cwf-1"}}
	assert.Equal(t, "cwf-1", clusterWorkflowSummary(cwf)["name"])
}

func TestClusterWorkflowDetail(t *testing.T) {
	cwf := &openchoreov1alpha1.ClusterWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "cwf-1"},
		Spec: openchoreov1alpha1.ClusterWorkflowSpec{
			TTLAfterCompletion: "30d",
			RunTemplate:        &runtime.RawExtension{Raw: []byte(`{"apiVersion":"v1"}`)},
		},
	}
	assert.NotNil(t, clusterWorkflowDetail(cwf)["spec"])
}

func TestSpecToMap(t *testing.T) {
	spec := struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}{Name: "test", Count: 5}

	m := specToMap(spec)
	require.NotNil(t, m)
	assert.Equal(t, "test", m["name"])

	assert.Nil(t, specToMap(make(chan int)))
	assert.Nil(t, specToMap("not-an-object"))
}
