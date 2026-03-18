// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// --- Mock types for unexported interfaces in params.go ---
// These can't be imported from cmd/* packages due to cyclic dependencies
// (cmd/* already imports validation).

// mockNamespaceParams satisfies the namespaceParams interface used by many validators.
type mockNamespaceParams struct{ Namespace string }

func (m mockNamespaceParams) GetNamespace() string { return m.Namespace }

// mockDeleteProjectParams satisfies deleteProjectParams.
type mockDeleteProjectParams struct{ Namespace, ProjectName string }

func (m mockDeleteProjectParams) GetNamespace() string   { return m.Namespace }
func (m mockDeleteProjectParams) GetProjectName() string { return m.ProjectName }

// mockDeleteComponentParams satisfies deleteComponentParams.
type mockDeleteComponentParams struct{ Namespace, ComponentName string }

func (m mockDeleteComponentParams) GetNamespace() string     { return m.Namespace }
func (m mockDeleteComponentParams) GetComponentName() string { return m.ComponentName }

// mockDeployComponentParams satisfies deployComponentParams.
type mockDeployComponentParams struct{ Namespace, Project, ComponentName string }

func (m mockDeployComponentParams) GetNamespace() string     { return m.Namespace }
func (m mockDeployComponentParams) GetProject() string       { return m.Project }
func (m mockDeployComponentParams) GetComponentName() string { return m.ComponentName }

// mockDeleteEnvironmentParams satisfies deleteEnvironmentParams.
type mockDeleteEnvironmentParams struct{ Namespace, EnvironmentName string }

func (m mockDeleteEnvironmentParams) GetNamespace() string       { return m.Namespace }
func (m mockDeleteEnvironmentParams) GetEnvironmentName() string { return m.EnvironmentName }

// mockDeleteDataPlaneParams satisfies deleteDataPlaneParams.
type mockDeleteDataPlaneParams struct{ Namespace, DataPlaneName string }

func (m mockDeleteDataPlaneParams) GetNamespace() string     { return m.Namespace }
func (m mockDeleteDataPlaneParams) GetDataPlaneName() string { return m.DataPlaneName }

// mockDeleteDeploymentPipelineParams satisfies deleteDeploymentPipelineParams.
type mockDeleteDeploymentPipelineParams struct{ Namespace, DeploymentPipelineName string }

func (m mockDeleteDeploymentPipelineParams) GetNamespace() string { return m.Namespace }
func (m mockDeleteDeploymentPipelineParams) GetDeploymentPipelineName() string {
	return m.DeploymentPipelineName
}

// mockDeleteWorkloadParams satisfies deleteWorkloadParams.
type mockDeleteWorkloadParams struct{ Namespace, WorkloadName string }

func (m mockDeleteWorkloadParams) GetNamespace() string    { return m.Namespace }
func (m mockDeleteWorkloadParams) GetWorkloadName() string { return m.WorkloadName }

// mockDeleteWorkflowPlaneParams satisfies deleteWorkflowPlaneParams.
type mockDeleteWorkflowPlaneParams struct{ Namespace, WorkflowPlaneName string }

func (m mockDeleteWorkflowPlaneParams) GetNamespace() string         { return m.Namespace }
func (m mockDeleteWorkflowPlaneParams) GetWorkflowPlaneName() string { return m.WorkflowPlaneName }

// mockDeleteObservabilityPlaneParams satisfies deleteObservabilityPlaneParams.
type mockDeleteObservabilityPlaneParams struct{ Namespace, ObservabilityPlaneName string }

func (m mockDeleteObservabilityPlaneParams) GetNamespace() string { return m.Namespace }
func (m mockDeleteObservabilityPlaneParams) GetObservabilityPlaneName() string {
	return m.ObservabilityPlaneName
}

// mockDeleteComponentTypeParams satisfies deleteComponentTypeParams.
type mockDeleteComponentTypeParams struct{ Namespace, ComponentTypeName string }

func (m mockDeleteComponentTypeParams) GetNamespace() string         { return m.Namespace }
func (m mockDeleteComponentTypeParams) GetComponentTypeName() string { return m.ComponentTypeName }

// mockDeleteTraitParams satisfies deleteTraitParams.
type mockDeleteTraitParams struct{ Namespace, TraitName string }

func (m mockDeleteTraitParams) GetNamespace() string { return m.Namespace }
func (m mockDeleteTraitParams) GetTraitName() string { return m.TraitName }

// mockDeleteWorkflowParams satisfies deleteWorkflowParams.
type mockDeleteWorkflowParams struct{ Namespace, WorkflowName string }

func (m mockDeleteWorkflowParams) GetNamespace() string    { return m.Namespace }
func (m mockDeleteWorkflowParams) GetWorkflowName() string { return m.WorkflowName }

// mockDeleteSecretReferenceParams satisfies deleteSecretReferenceParams.
type mockDeleteSecretReferenceParams struct{ Namespace, SecretReferenceName string }

func (m mockDeleteSecretReferenceParams) GetNamespace() string { return m.Namespace }
func (m mockDeleteSecretReferenceParams) GetSecretReferenceName() string {
	return m.SecretReferenceName
}

// mockDeleteComponentReleaseParams satisfies deleteComponentReleaseParams.
type mockDeleteComponentReleaseParams struct{ Namespace, ComponentReleaseName string }

func (m mockDeleteComponentReleaseParams) GetNamespace() string { return m.Namespace }
func (m mockDeleteComponentReleaseParams) GetComponentReleaseName() string {
	return m.ComponentReleaseName
}

// mockDeleteReleaseBindingParams satisfies deleteReleaseBindingParams.
type mockDeleteReleaseBindingParams struct{ Namespace, ReleaseBindingName string }

func (m mockDeleteReleaseBindingParams) GetNamespace() string { return m.Namespace }
func (m mockDeleteReleaseBindingParams) GetReleaseBindingName() string {
	return m.ReleaseBindingName
}

// mockDeleteObsAlertsNotifChannelParams satisfies deleteObservabilityAlertsNotificationChannelParams.
type mockDeleteObsAlertsNotifChannelParams struct{ Namespace, ChannelName string }

func (m mockDeleteObsAlertsNotifChannelParams) GetNamespace() string   { return m.Namespace }
func (m mockDeleteObsAlertsNotifChannelParams) GetChannelName() string { return m.ChannelName }

// --- Tests ---

func TestValidateParams(t *testing.T) {
	tests := []struct {
		name     string
		cmdType  CommandType
		resource ResourceType
		params   any
		wantErr  bool
		errMsg   string
	}{
		// --- Project ---
		{name: "project create valid", cmdType: CmdCreate, resource: ResourceProject,
			params: api.CreateProjectParams{Namespace: "ns", Name: "proj"}},
		{name: "project create missing name", cmdType: CmdCreate, resource: ResourceProject,
			params: api.CreateProjectParams{Namespace: "ns"}, wantErr: true, errMsg: "name"},
		{name: "project get valid", cmdType: CmdGet, resource: ResourceProject,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "project get missing namespace", cmdType: CmdGet, resource: ResourceProject,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "project list valid", cmdType: CmdList, resource: ResourceProject,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "project list missing namespace", cmdType: CmdList, resource: ResourceProject,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "project delete valid", cmdType: CmdDelete, resource: ResourceProject,
			params: mockDeleteProjectParams{Namespace: "ns", ProjectName: "proj"}},
		{name: "project delete missing fields", cmdType: CmdDelete, resource: ResourceProject,
			params: mockDeleteProjectParams{}, wantErr: true, errMsg: "namespace"},

		// --- Component ---
		{name: "component get valid", cmdType: CmdGet, resource: ResourceComponent,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "component get missing namespace", cmdType: CmdGet, resource: ResourceComponent,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "component list valid", cmdType: CmdList, resource: ResourceComponent,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "component list missing namespace", cmdType: CmdList, resource: ResourceComponent,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "component delete valid", cmdType: CmdDelete, resource: ResourceComponent,
			params: mockDeleteComponentParams{Namespace: "ns", ComponentName: "comp"}},
		{name: "component delete missing fields", cmdType: CmdDelete, resource: ResourceComponent,
			params: mockDeleteComponentParams{}, wantErr: true, errMsg: "namespace"},
		{name: "component deploy valid", cmdType: CmdDeploy, resource: ResourceComponent,
			params: mockDeployComponentParams{Namespace: "ns", Project: "proj", ComponentName: "comp"}},
		{name: "component deploy missing namespace", cmdType: CmdDeploy, resource: ResourceComponent,
			params: mockDeployComponentParams{Project: "proj", ComponentName: "comp"}, wantErr: true, errMsg: "namespace"},
		{name: "component deploy missing component", cmdType: CmdDeploy, resource: ResourceComponent,
			params: mockDeployComponentParams{Namespace: "ns", Project: "proj"}, wantErr: true, errMsg: "component name is required"},

		// --- Deployment ---
		{name: "deployment create valid", cmdType: CmdCreate, resource: ResourceDeployment,
			params: api.CreateDeploymentParams{Namespace: "ns", Project: "proj", Component: "comp"}},
		{name: "deployment create missing component", cmdType: CmdCreate, resource: ResourceDeployment,
			params: api.CreateDeploymentParams{Namespace: "ns", Project: "proj"}, wantErr: true, errMsg: "component"},
		{name: "deployment get valid", cmdType: CmdGet, resource: ResourceDeployment,
			params: api.GetDeploymentParams{Namespace: "ns", Project: "proj", Component: "comp"}},
		{name: "deployment get missing namespace", cmdType: CmdGet, resource: ResourceDeployment,
			params: api.GetDeploymentParams{Project: "proj", Component: "comp"}, wantErr: true, errMsg: "namespace"},

		// --- Environment ---
		{name: "environment create valid", cmdType: CmdCreate, resource: ResourceEnvironment,
			params: api.CreateEnvironmentParams{Namespace: "ns", Name: "env"}},
		{name: "environment create missing name", cmdType: CmdCreate, resource: ResourceEnvironment,
			params: api.CreateEnvironmentParams{Namespace: "ns"}, wantErr: true, errMsg: "name"},
		{name: "environment get valid", cmdType: CmdGet, resource: ResourceEnvironment,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "environment get missing namespace", cmdType: CmdGet, resource: ResourceEnvironment,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "environment list valid", cmdType: CmdList, resource: ResourceEnvironment,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "environment list missing namespace", cmdType: CmdList, resource: ResourceEnvironment,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "environment delete valid", cmdType: CmdDelete, resource: ResourceEnvironment,
			params: mockDeleteEnvironmentParams{Namespace: "ns", EnvironmentName: "env"}},
		{name: "environment delete missing fields", cmdType: CmdDelete, resource: ResourceEnvironment,
			params: mockDeleteEnvironmentParams{}, wantErr: true, errMsg: "namespace"},

		// --- DeployableArtifact ---
		{name: "deployable artifact create valid", cmdType: CmdCreate, resource: ResourceDeployableArtifact,
			params: api.CreateDeployableArtifactParams{Namespace: "ns", Project: "proj", Component: "comp"}},
		{name: "deployable artifact create missing project", cmdType: CmdCreate, resource: ResourceDeployableArtifact,
			params: api.CreateDeployableArtifactParams{Namespace: "ns", Component: "comp"}, wantErr: true, errMsg: "project"},
		{name: "deployable artifact get valid", cmdType: CmdGet, resource: ResourceDeployableArtifact,
			params: api.GetDeployableArtifactParams{Namespace: "ns", Project: "proj", Component: "comp"}},
		{name: "deployable artifact get missing namespace", cmdType: CmdGet, resource: ResourceDeployableArtifact,
			params: api.GetDeployableArtifactParams{Project: "proj", Component: "comp"}, wantErr: true, errMsg: "namespace"},

		// --- Logs ---
		{name: "log params build valid", cmdType: CmdLogs, resource: ResourceLogs,
			params: api.LogParams{Type: "build", Namespace: "ns", Build: "build-1"}},
		{name: "log params deployment valid", cmdType: CmdLogs, resource: ResourceLogs,
			params: api.LogParams{Type: "deployment", Namespace: "ns", Project: "proj", Component: "comp", Environment: "dev", Deployment: "dep"}},
		{name: "log params missing type", cmdType: CmdLogs, resource: ResourceLogs,
			params: api.LogParams{}, wantErr: true, errMsg: "type"},
		{name: "log params invalid type", cmdType: CmdLogs, resource: ResourceLogs,
			params: api.LogParams{Type: "unknown"}, wantErr: true, errMsg: "not supported"},
		{name: "log params build missing build", cmdType: CmdLogs, resource: ResourceLogs,
			params: api.LogParams{Type: "build", Namespace: "ns"}, wantErr: true, errMsg: "build"},
		{name: "log params deployment missing fields", cmdType: CmdLogs, resource: ResourceLogs,
			params: api.LogParams{Type: "deployment", Namespace: "ns"}, wantErr: true, errMsg: "project"},

		// --- DataPlane ---
		{name: "dataplane create valid", cmdType: CmdCreate, resource: ResourceDataPlane,
			params: api.CreateDataPlaneParams{Namespace: "ns", Name: "dp"}},
		{name: "dataplane create missing name", cmdType: CmdCreate, resource: ResourceDataPlane,
			params: api.CreateDataPlaneParams{Namespace: "ns"}, wantErr: true, errMsg: "name"},
		{name: "dataplane get valid", cmdType: CmdGet, resource: ResourceDataPlane,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "dataplane get missing namespace", cmdType: CmdGet, resource: ResourceDataPlane,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "dataplane list valid", cmdType: CmdList, resource: ResourceDataPlane,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "dataplane list missing namespace", cmdType: CmdList, resource: ResourceDataPlane,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "dataplane delete valid", cmdType: CmdDelete, resource: ResourceDataPlane,
			params: mockDeleteDataPlaneParams{Namespace: "ns", DataPlaneName: "dp"}},
		{name: "dataplane delete missing fields", cmdType: CmdDelete, resource: ResourceDataPlane,
			params: mockDeleteDataPlaneParams{}, wantErr: true, errMsg: "namespace"},

		// --- Namespace ---
		{name: "namespace create valid", cmdType: CmdCreate, resource: ResourceNamespace,
			params: api.CreateNamespaceParams{Name: "ns"}},
		{name: "namespace create missing name", cmdType: CmdCreate, resource: ResourceNamespace,
			params: api.CreateNamespaceParams{}, wantErr: true, errMsg: "name"},

		// --- Endpoint ---
		{name: "endpoint get valid", cmdType: CmdGet, resource: ResourceEndpoint,
			params: api.GetEndpointParams{Namespace: "ns", Project: "proj", Component: "comp"}},
		{name: "endpoint get missing component", cmdType: CmdGet, resource: ResourceEndpoint,
			params: api.GetEndpointParams{Namespace: "ns", Project: "proj"}, wantErr: true, errMsg: "component"},

		// --- DeploymentPipeline ---
		{name: "deployment pipeline create valid", cmdType: CmdCreate, resource: ResourceDeploymentPipeline,
			params: api.CreateDeploymentPipelineParams{Namespace: "ns", Name: "dp", EnvironmentOrder: []string{"dev", "prod"}}},
		{name: "deployment pipeline create missing env order", cmdType: CmdCreate, resource: ResourceDeploymentPipeline,
			params: api.CreateDeploymentPipelineParams{Namespace: "ns", Name: "dp"}, wantErr: true, errMsg: "environment-order"},
		{name: "deployment pipeline get valid", cmdType: CmdGet, resource: ResourceDeploymentPipeline,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "deployment pipeline get missing namespace", cmdType: CmdGet, resource: ResourceDeploymentPipeline,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "deployment pipeline list valid", cmdType: CmdList, resource: ResourceDeploymentPipeline,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "deployment pipeline list missing namespace", cmdType: CmdList, resource: ResourceDeploymentPipeline,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "deployment pipeline delete valid", cmdType: CmdDelete, resource: ResourceDeploymentPipeline,
			params: mockDeleteDeploymentPipelineParams{Namespace: "ns", DeploymentPipelineName: "dp"}},
		{name: "deployment pipeline delete missing fields", cmdType: CmdDelete, resource: ResourceDeploymentPipeline,
			params: mockDeleteDeploymentPipelineParams{}, wantErr: true, errMsg: "namespace"},

		// --- Workload ---
		{name: "workload create valid", cmdType: CmdCreate, resource: ResourceWorkload,
			params: api.CreateWorkloadParams{NamespaceName: "ns", ProjectName: "proj", ComponentName: "comp", ImageURL: "img:latest"}},
		{name: "workload create missing image", cmdType: CmdCreate, resource: ResourceWorkload,
			params: api.CreateWorkloadParams{NamespaceName: "ns", ProjectName: "proj", ComponentName: "comp"}, wantErr: true, errMsg: "image"},
		{name: "workload get valid", cmdType: CmdGet, resource: ResourceWorkload,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "workload get missing namespace", cmdType: CmdGet, resource: ResourceWorkload,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "workload list valid", cmdType: CmdList, resource: ResourceWorkload,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "workload list missing namespace", cmdType: CmdList, resource: ResourceWorkload,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "workload delete valid", cmdType: CmdDelete, resource: ResourceWorkload,
			params: mockDeleteWorkloadParams{Namespace: "ns", WorkloadName: "wl"}},
		{name: "workload delete missing fields", cmdType: CmdDelete, resource: ResourceWorkload,
			params: mockDeleteWorkloadParams{}, wantErr: true, errMsg: "namespace"},

		// --- WorkflowPlane ---
		{name: "workflow plane get valid", cmdType: CmdGet, resource: ResourceWorkflowPlane,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "workflow plane get missing namespace", cmdType: CmdGet, resource: ResourceWorkflowPlane,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "workflow plane list valid", cmdType: CmdList, resource: ResourceWorkflowPlane,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "workflow plane list missing namespace", cmdType: CmdList, resource: ResourceWorkflowPlane,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "workflow plane delete valid", cmdType: CmdDelete, resource: ResourceWorkflowPlane,
			params: mockDeleteWorkflowPlaneParams{Namespace: "ns", WorkflowPlaneName: "wp"}},
		{name: "workflow plane delete missing fields", cmdType: CmdDelete, resource: ResourceWorkflowPlane,
			params: mockDeleteWorkflowPlaneParams{}, wantErr: true, errMsg: "namespace"},

		// --- ObservabilityPlane ---
		{name: "observability plane get valid", cmdType: CmdGet, resource: ResourceObservabilityPlane,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "observability plane get missing namespace", cmdType: CmdGet, resource: ResourceObservabilityPlane,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "observability plane list valid", cmdType: CmdList, resource: ResourceObservabilityPlane,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "observability plane list missing namespace", cmdType: CmdList, resource: ResourceObservabilityPlane,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "observability plane delete valid", cmdType: CmdDelete, resource: ResourceObservabilityPlane,
			params: mockDeleteObservabilityPlaneParams{Namespace: "ns", ObservabilityPlaneName: "op"}},
		{name: "observability plane delete missing fields", cmdType: CmdDelete, resource: ResourceObservabilityPlane,
			params: mockDeleteObservabilityPlaneParams{}, wantErr: true, errMsg: "namespace"},

		// --- ComponentType ---
		{name: "component type get valid", cmdType: CmdGet, resource: ResourceComponentType,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "component type get missing namespace", cmdType: CmdGet, resource: ResourceComponentType,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "component type list valid", cmdType: CmdList, resource: ResourceComponentType,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "component type list missing namespace", cmdType: CmdList, resource: ResourceComponentType,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "component type delete valid", cmdType: CmdDelete, resource: ResourceComponentType,
			params: mockDeleteComponentTypeParams{Namespace: "ns", ComponentTypeName: "ct"}},
		{name: "component type delete missing fields", cmdType: CmdDelete, resource: ResourceComponentType,
			params: mockDeleteComponentTypeParams{}, wantErr: true, errMsg: "namespace"},

		// --- Trait ---
		{name: "trait get valid", cmdType: CmdGet, resource: ResourceTrait,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "trait get missing namespace", cmdType: CmdGet, resource: ResourceTrait,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "trait list valid", cmdType: CmdList, resource: ResourceTrait,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "trait list missing namespace", cmdType: CmdList, resource: ResourceTrait,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "trait delete valid", cmdType: CmdDelete, resource: ResourceTrait,
			params: mockDeleteTraitParams{Namespace: "ns", TraitName: "tr"}},
		{name: "trait delete missing fields", cmdType: CmdDelete, resource: ResourceTrait,
			params: mockDeleteTraitParams{}, wantErr: true, errMsg: "namespace"},

		// --- Workflow ---
		{name: "workflow list valid", cmdType: CmdList, resource: ResourceWorkflow,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "workflow list missing namespace", cmdType: CmdList, resource: ResourceWorkflow,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "workflow logs valid", cmdType: CmdLogs, resource: ResourceWorkflow,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "workflow logs missing namespace", cmdType: CmdLogs, resource: ResourceWorkflow,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "workflow delete valid", cmdType: CmdDelete, resource: ResourceWorkflow,
			params: mockDeleteWorkflowParams{Namespace: "ns", WorkflowName: "wf"}},
		{name: "workflow delete missing fields", cmdType: CmdDelete, resource: ResourceWorkflow,
			params: mockDeleteWorkflowParams{}, wantErr: true, errMsg: "namespace"},

		// --- SecretReference ---
		{name: "secret reference get valid", cmdType: CmdGet, resource: ResourceSecretReference,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "secret reference get missing namespace", cmdType: CmdGet, resource: ResourceSecretReference,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "secret reference list valid", cmdType: CmdList, resource: ResourceSecretReference,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "secret reference list missing namespace", cmdType: CmdList, resource: ResourceSecretReference,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "secret reference delete valid", cmdType: CmdDelete, resource: ResourceSecretReference,
			params: mockDeleteSecretReferenceParams{Namespace: "ns", SecretReferenceName: "sr"}},
		{name: "secret reference delete missing fields", cmdType: CmdDelete, resource: ResourceSecretReference,
			params: mockDeleteSecretReferenceParams{}, wantErr: true, errMsg: "namespace"},

		// --- ComponentRelease ---
		{name: "component release get valid", cmdType: CmdGet, resource: ResourceComponentRelease,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "component release get missing namespace", cmdType: CmdGet, resource: ResourceComponentRelease,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "component release list valid", cmdType: CmdList, resource: ResourceComponentRelease,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "component release list missing namespace", cmdType: CmdList, resource: ResourceComponentRelease,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "component release delete valid", cmdType: CmdDelete, resource: ResourceComponentRelease,
			params: mockDeleteComponentReleaseParams{Namespace: "ns", ComponentReleaseName: "cr"}},
		{name: "component release delete missing fields", cmdType: CmdDelete, resource: ResourceComponentRelease,
			params: mockDeleteComponentReleaseParams{}, wantErr: true, errMsg: "namespace"},

		// --- ReleaseBinding ---
		{name: "release binding get valid", cmdType: CmdGet, resource: ResourceReleaseBinding,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "release binding get missing namespace", cmdType: CmdGet, resource: ResourceReleaseBinding,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "release binding list valid", cmdType: CmdList, resource: ResourceReleaseBinding,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "release binding list missing namespace", cmdType: CmdList, resource: ResourceReleaseBinding,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "release binding delete valid", cmdType: CmdDelete, resource: ResourceReleaseBinding,
			params: mockDeleteReleaseBindingParams{Namespace: "ns", ReleaseBindingName: "rb"}},
		{name: "release binding delete missing fields", cmdType: CmdDelete, resource: ResourceReleaseBinding,
			params: mockDeleteReleaseBindingParams{}, wantErr: true, errMsg: "namespace"},

		// --- WorkflowRun ---
		{name: "workflow run get valid", cmdType: CmdGet, resource: ResourceWorkflowRun,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "workflow run get missing namespace", cmdType: CmdGet, resource: ResourceWorkflowRun,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "workflow run list valid", cmdType: CmdList, resource: ResourceWorkflowRun,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "workflow run list missing namespace", cmdType: CmdList, resource: ResourceWorkflowRun,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "workflow run logs valid", cmdType: CmdLogs, resource: ResourceWorkflowRun,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "workflow run logs missing namespace", cmdType: CmdLogs, resource: ResourceWorkflowRun,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},

		// --- ObservabilityAlertsNotificationChannel ---
		{name: "obs alerts channel get valid", cmdType: CmdGet, resource: ResourceObservabilityAlertsNotificationChannel,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "obs alerts channel get missing namespace", cmdType: CmdGet, resource: ResourceObservabilityAlertsNotificationChannel,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "obs alerts channel list valid", cmdType: CmdList, resource: ResourceObservabilityAlertsNotificationChannel,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "obs alerts channel list missing namespace", cmdType: CmdList, resource: ResourceObservabilityAlertsNotificationChannel,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "obs alerts channel delete valid", cmdType: CmdDelete, resource: ResourceObservabilityAlertsNotificationChannel,
			params: mockDeleteObsAlertsNotifChannelParams{Namespace: "ns", ChannelName: "ch"}},
		{name: "obs alerts channel delete missing fields", cmdType: CmdDelete, resource: ResourceObservabilityAlertsNotificationChannel,
			params: mockDeleteObsAlertsNotifChannelParams{}, wantErr: true, errMsg: "namespace"},

		// --- ClusterAuthzRole / ClusterAuthzRoleBinding (no-op validators) ---
		{name: "cluster authz role returns nil", cmdType: CmdGet, resource: ResourceClusterAuthzRole,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "cluster authz role binding returns nil", cmdType: CmdGet, resource: ResourceClusterAuthzRoleBinding,
			params: mockNamespaceParams{Namespace: "ns"}},

		// --- AuthzRole ---
		{name: "authz role get valid", cmdType: CmdGet, resource: ResourceAuthzRole,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "authz role get missing namespace", cmdType: CmdGet, resource: ResourceAuthzRole,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "authz role list valid", cmdType: CmdList, resource: ResourceAuthzRole,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "authz role list missing namespace", cmdType: CmdList, resource: ResourceAuthzRole,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "authz role delete valid", cmdType: CmdDelete, resource: ResourceAuthzRole,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "authz role delete missing namespace", cmdType: CmdDelete, resource: ResourceAuthzRole,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},

		// --- AuthzRoleBinding ---
		{name: "authz role binding get valid", cmdType: CmdGet, resource: ResourceAuthzRoleBinding,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "authz role binding get missing namespace", cmdType: CmdGet, resource: ResourceAuthzRoleBinding,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "authz role binding list valid", cmdType: CmdList, resource: ResourceAuthzRoleBinding,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "authz role binding list missing namespace", cmdType: CmdList, resource: ResourceAuthzRoleBinding,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},
		{name: "authz role binding delete valid", cmdType: CmdDelete, resource: ResourceAuthzRoleBinding,
			params: mockNamespaceParams{Namespace: "ns"}},
		{name: "authz role binding delete missing namespace", cmdType: CmdDelete, resource: ResourceAuthzRoleBinding,
			params: mockNamespaceParams{}, wantErr: true, errMsg: "namespace"},

		// --- Unknown resource ---
		{name: "unknown resource type", cmdType: CmdGet, resource: "foobar",
			params: nil, wantErr: true, errMsg: "unknown resource type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateParams(tt.cmdType, tt.resource, tt.params)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}
