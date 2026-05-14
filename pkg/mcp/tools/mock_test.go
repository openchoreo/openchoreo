// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const (
	emptyObjectSchema = `{"type":"object","properties":{}}`
	deletedResponse   = `{"action":"deleted"}`
)

// MockCoreToolsetHandler implements all toolset handler interfaces for testing.
type MockCoreToolsetHandler struct {
	// Track which methods were called and with what parameters
	calls map[string][]interface{}
}

func NewMockCoreToolsetHandler() *MockCoreToolsetHandler {
	return &MockCoreToolsetHandler{
		calls: make(map[string][]interface{}),
	}
}

func (m *MockCoreToolsetHandler) recordCall(method string, args ...interface{}) {
	m.calls[method] = append(m.calls[method], args)
}

// NamespaceToolsetHandler methods

func (m *MockCoreToolsetHandler) ListNamespaces(ctx context.Context, opts ListOpts) (any, error) {
	m.recordCall("ListNamespaces", opts)
	return `[{"name":"test-namespace"}]`, nil
}

func (m *MockCoreToolsetHandler) CreateNamespace(
	ctx context.Context, req *gen.CreateNamespaceJSONRequestBody,
) (any, error) {
	m.recordCall("CreateNamespace", req)
	return `{"name":"new-namespace"}`, nil
}

func (m *MockCoreToolsetHandler) ListSecretReferences(
	ctx context.Context, namespaceName string, opts ListOpts,
) (any, error) {
	m.recordCall("ListSecretReferences", namespaceName, opts)
	return `[{"name":"secret-ref-1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetSecretReference(
	ctx context.Context, namespaceName, secretReferenceName string,
) (any, error) {
	m.recordCall("GetSecretReference", namespaceName, secretReferenceName)
	return `{"name":"secret-ref-1"}`, nil
}

func (m *MockCoreToolsetHandler) CreateSecretReference(
	ctx context.Context, namespaceName string, req *gen.CreateSecretReferenceJSONRequestBody,
) (any, error) {
	m.recordCall("CreateSecretReference", namespaceName, req)
	return `{"name":"new-secret-ref"}`, nil
}

func (m *MockCoreToolsetHandler) UpdateSecretReference(
	ctx context.Context, namespaceName string, req *gen.UpdateSecretReferenceJSONRequestBody,
) (any, error) {
	m.recordCall("UpdateSecretReference", namespaceName, req)
	return `{"name":"updated-secret-ref"}`, nil
}

func (m *MockCoreToolsetHandler) DeleteSecretReference(
	ctx context.Context, namespaceName, secretReferenceName string,
) (any, error) {
	m.recordCall("DeleteSecretReference", namespaceName, secretReferenceName)
	return deletedResponse, nil
}

func (m *MockCoreToolsetHandler) DeleteProject(
	ctx context.Context, namespaceName, projectName string,
) (any, error) {
	m.recordCall("DeleteProject", namespaceName, projectName)
	return deletedResponse, nil
}

func (m *MockCoreToolsetHandler) DeleteComponent(
	ctx context.Context, namespaceName, componentName string,
) (any, error) {
	m.recordCall("DeleteComponent", namespaceName, componentName)
	return deletedResponse, nil
}

func (m *MockCoreToolsetHandler) DeleteWorkload(
	ctx context.Context, namespaceName, workloadName string,
) (any, error) {
	m.recordCall("DeleteWorkload", namespaceName, workloadName)
	return deletedResponse, nil
}

func (m *MockCoreToolsetHandler) DeleteReleaseBinding(
	ctx context.Context, namespaceName, bindingName string,
) (any, error) {
	m.recordCall("DeleteReleaseBinding", namespaceName, bindingName)
	return deletedResponse, nil
}

func (m *MockCoreToolsetHandler) DeleteComponentRelease(
	ctx context.Context, namespaceName, componentReleaseName string,
) (any, error) {
	m.recordCall("DeleteComponentRelease", namespaceName, componentReleaseName)
	return deletedResponse, nil
}

// ProjectToolsetHandler methods

func (m *MockCoreToolsetHandler) ListProjects(ctx context.Context, namespaceName string, opts ListOpts) (any, error) {
	m.recordCall("ListProjects", namespaceName, opts)
	return `[{"name":"project1"}]`, nil
}

func (m *MockCoreToolsetHandler) CreateProject(
	ctx context.Context, namespaceName string, req *gen.CreateProjectJSONRequestBody,
) (any, error) {
	m.recordCall("CreateProject", namespaceName, req)
	return `{"name":"new-project"}`, nil
}

func (m *MockCoreToolsetHandler) UpdateProject(
	ctx context.Context, namespaceName, projectName string, req *gen.PatchProjectRequest,
) (any, error) {
	m.recordCall("UpdateProject", namespaceName, projectName, req)
	return `{"name":"updated-project"}`, nil
}

// ComponentToolsetHandler methods

func (m *MockCoreToolsetHandler) CreateComponent(
	ctx context.Context, namespaceName, projectName string, req *gen.CreateComponentRequest,
) (any, error) {
	m.recordCall("CreateComponent", namespaceName, projectName, req)
	return `{"name":"new-component"}`, nil
}

func (m *MockCoreToolsetHandler) ListComponents(
	ctx context.Context, namespaceName, projectName string, opts ListOpts,
) (any, error) {
	m.recordCall("ListComponents", namespaceName, projectName, opts)
	return `[{"name":"component1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetComponent(
	ctx context.Context, namespaceName, componentName string,
) (any, error) {
	m.recordCall("GetComponent", namespaceName, componentName)
	return `{"name":"component1"}`, nil
}

func (m *MockCoreToolsetHandler) ListWorkloads(
	ctx context.Context, namespaceName, componentName string, opts ListOpts,
) (any, error) {
	m.recordCall("ListWorkloads", namespaceName, componentName, opts)
	return `[{"name":"workload1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetWorkload(
	ctx context.Context, namespaceName, workloadName string,
) (any, error) {
	m.recordCall("GetWorkload", namespaceName, workloadName)
	return `{"name":"workload1"}`, nil
}

func (m *MockCoreToolsetHandler) ListComponentReleases(
	ctx context.Context, namespaceName, componentName string, opts ListOpts,
) (any, error) {
	m.recordCall("ListComponentReleases", namespaceName, componentName, opts)
	return `[{"name":"release-1"}]`, nil
}

func (m *MockCoreToolsetHandler) CreateComponentRelease(
	ctx context.Context, namespaceName, componentName, releaseName string,
) (any, error) {
	m.recordCall("CreateComponentRelease", namespaceName, componentName, releaseName)
	return `{"name":"release-1"}`, nil
}

func (m *MockCoreToolsetHandler) GetComponentRelease(
	ctx context.Context, namespaceName, releaseName string,
) (any, error) {
	m.recordCall("GetComponentRelease", namespaceName, releaseName)
	return `{"name":"release-1"}`, nil
}

func (m *MockCoreToolsetHandler) ListReleaseBindings(
	ctx context.Context, namespaceName, componentName string, opts ListOpts,
) (any, error) {
	m.recordCall("ListReleaseBindings", namespaceName, componentName, opts)
	return `[{"environment":"dev"}]`, nil
}

func (m *MockCoreToolsetHandler) GetReleaseBinding(
	ctx context.Context, namespaceName, bindingName string,
) (any, error) {
	m.recordCall("GetReleaseBinding", namespaceName, bindingName)
	return `{"name":"binding-dev","environment":"dev"}`, nil
}

func (m *MockCoreToolsetHandler) CreateReleaseBinding(
	ctx context.Context, namespaceName string,
	req *gen.ReleaseBindingSpec,
) (any, error) {
	m.recordCall("CreateReleaseBinding", namespaceName, req)
	return `{"name":"binding-dev","action":"created"}`, nil
}

func (m *MockCoreToolsetHandler) UpdateReleaseBinding(
	ctx context.Context, namespaceName, bindingName string,
	req *gen.ReleaseBindingSpec,
) (any, error) {
	m.recordCall("UpdateReleaseBinding", namespaceName, bindingName, req)
	return `{"status":"updated"}`, nil
}

func (m *MockCoreToolsetHandler) CreateWorkload(
	ctx context.Context, namespaceName, componentName string, workloadSpec interface{},
) (any, error) {
	m.recordCall("CreateWorkload", namespaceName, componentName, workloadSpec)
	return `{"name":"workload-1"}`, nil
}

func (m *MockCoreToolsetHandler) UpdateWorkload(
	ctx context.Context, namespaceName, workloadName string, workloadSpec interface{},
) (any, error) {
	m.recordCall("UpdateWorkload", namespaceName, workloadName, workloadSpec)
	return `{"name":"workload-1"}`, nil
}

func (m *MockCoreToolsetHandler) GetWorkloadSchema(
	ctx context.Context,
) (any, error) {
	m.recordCall("GetWorkloadSchema")
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) GetComponentSchema(
	ctx context.Context, namespaceName, componentName string,
) (any, error) {
	m.recordCall("GetComponentSchema", namespaceName, componentName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) PatchComponent(
	ctx context.Context, namespaceName, componentName string, req *gen.PatchComponentRequest,
) (any, error) {
	m.recordCall("PatchComponent", namespaceName, componentName, req)
	return `{"name":"patched-component"}`, nil
}

func (m *MockCoreToolsetHandler) GetComponentReleaseSchema(
	ctx context.Context, namespaceName, componentName, releaseName string,
) (any, error) {
	m.recordCall("GetComponentReleaseSchema", namespaceName, componentName, releaseName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) TriggerWorkflowRun(
	ctx context.Context, namespaceName, projectName, componentName, commit string,
) (any, error) {
	m.recordCall("TriggerWorkflowRun", namespaceName, projectName, componentName, commit)
	return `{"name":"my-component-workflow-run","status":"Running"}`, nil
}

func (m *MockCoreToolsetHandler) ListComponentTypes(
	ctx context.Context, namespaceName string, opts ListOpts,
) (any, error) {
	m.recordCall("ListComponentTypes", namespaceName, opts)
	return `[{"name":"WebApplication"}]`, nil
}

func (m *MockCoreToolsetHandler) GetComponentType(
	ctx context.Context, namespaceName, ctName string,
) (any, error) {
	m.recordCall("GetComponentType", namespaceName, ctName)
	return `{"name":"WebApplication","spec":{"workloadType":"deployment"}}`, nil
}

func (m *MockCoreToolsetHandler) GetComponentTypeSchema(
	ctx context.Context, namespaceName, ctName string,
) (any, error) {
	m.recordCall("GetComponentTypeSchema", namespaceName, ctName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) ListTraits(ctx context.Context, namespaceName string, opts ListOpts) (any, error) {
	m.recordCall("ListTraits", namespaceName, opts)
	return `[{"name":"autoscaling"}]`, nil
}

func (m *MockCoreToolsetHandler) GetTrait(ctx context.Context, namespaceName, traitName string) (any, error) {
	m.recordCall("GetTrait", namespaceName, traitName)
	return `{"name":"autoscaling","spec":{}}`, nil
}

func (m *MockCoreToolsetHandler) GetTraitSchema(ctx context.Context, namespaceName, traitName string) (any, error) {
	m.recordCall("GetTraitSchema", namespaceName, traitName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) CreateWorkflowRun(
	ctx context.Context, namespaceName, workflowName string,
	parameters map[string]interface{},
) (any, error) {
	m.recordCall("CreateWorkflowRun", namespaceName, workflowName, parameters)
	return `{"name":"workflow-run-1"}`, nil
}

func (m *MockCoreToolsetHandler) ListWorkflowRuns(
	ctx context.Context, namespaceName, projectName, componentName string,
	opts ListOpts,
) (any, error) {
	m.recordCall("ListWorkflowRuns", namespaceName, projectName, componentName, opts)
	return `[{"name":"workflow-run-1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetWorkflowRun(ctx context.Context, namespaceName, runName string) (any, error) {
	m.recordCall("GetWorkflowRun", namespaceName, runName)
	return `{"name":"workflow-run-1"}`, nil
}

func (m *MockCoreToolsetHandler) GetWorkflowRunStatus(ctx context.Context, namespaceName, runName string) (any, error) {
	m.recordCall("GetWorkflowRunStatus", namespaceName, runName)
	return `{"status":"Running","steps":[]}`, nil
}

func (m *MockCoreToolsetHandler) GetWorkflowRunLogs(
	ctx context.Context, namespaceName, runName, taskName string, sinceSeconds *int64,
) (any, error) {
	m.recordCall("GetWorkflowRunLogs", namespaceName, runName, taskName, sinceSeconds)
	return `{"logs":[]}`, nil
}

func (m *MockCoreToolsetHandler) GetWorkflowRunEvents(
	ctx context.Context, namespaceName, runName, taskName string,
) (any, error) {
	m.recordCall("GetWorkflowRunEvents", namespaceName, runName, taskName)
	return `{"events":[]}`, nil
}

func (m *MockCoreToolsetHandler) ListClusterComponentTypes(ctx context.Context, opts ListOpts) (any, error) {
	m.recordCall("ListClusterComponentTypes", opts)
	return `[{"name":"go-service"}]`, nil
}

func (m *MockCoreToolsetHandler) GetClusterComponentType(ctx context.Context, cctName string) (any, error) {
	m.recordCall("GetClusterComponentType", cctName)
	return `{"name":"go-service"}`, nil
}

func (m *MockCoreToolsetHandler) GetClusterComponentTypeSchema(ctx context.Context, cctName string) (any, error) {
	m.recordCall("GetClusterComponentTypeSchema", cctName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) ListClusterTraits(ctx context.Context, opts ListOpts) (any, error) {
	m.recordCall("ListClusterTraits", opts)
	return `[{"name":"autoscaler"}]`, nil
}

func (m *MockCoreToolsetHandler) GetClusterTrait(ctx context.Context, ctName string) (any, error) {
	m.recordCall("GetClusterTrait", ctName)
	return `{"name":"autoscaler"}`, nil
}

func (m *MockCoreToolsetHandler) ListWorkflows(ctx context.Context, namespaceName string, opts ListOpts) (any, error) {
	m.recordCall("ListWorkflows", namespaceName, opts)
	return `[{"name":"build-workflow"}]`, nil
}

func (m *MockCoreToolsetHandler) GetWorkflow(ctx context.Context, namespaceName, workflowName string) (any, error) {
	m.recordCall("GetWorkflow", namespaceName, workflowName)
	return `{"name":"build-workflow","spec":{}}`, nil
}

func (m *MockCoreToolsetHandler) GetWorkflowSchema(
	ctx context.Context, namespaceName, workflowName string,
) (any, error) {
	m.recordCall("GetWorkflowSchema", namespaceName, workflowName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) GetClusterTraitSchema(ctx context.Context, ctName string) (any, error) {
	m.recordCall("GetClusterTraitSchema", ctName)
	return emptyObjectSchema, nil
}

// PEToolsetHandler methods

func (m *MockCoreToolsetHandler) ListEnvironments(
	ctx context.Context, namespaceName string, opts ListOpts,
) (any, error) {
	m.recordCall("ListEnvironments", namespaceName, opts)
	return `[{"name":"dev"}]`, nil
}

func (m *MockCoreToolsetHandler) CreateEnvironment(
	ctx context.Context, namespaceName string, req *gen.CreateEnvironmentJSONRequestBody,
) (any, error) {
	m.recordCall("CreateEnvironment", namespaceName, req)
	return `{"name":"new-env"}`, nil
}

func (m *MockCoreToolsetHandler) UpdateEnvironment(
	ctx context.Context, namespaceName string, req *gen.UpdateEnvironmentJSONRequestBody,
) (any, error) {
	m.recordCall("UpdateEnvironment", namespaceName, req)
	return `{"name":"updated-env"}`, nil
}

func (m *MockCoreToolsetHandler) DeleteEnvironment(
	ctx context.Context, namespaceName, envName string,
) (any, error) {
	m.recordCall("DeleteEnvironment", namespaceName, envName)
	return `{"name":"deleted-env"}`, nil
}

func (m *MockCoreToolsetHandler) CreateDeploymentPipeline(
	ctx context.Context, namespaceName string, req *gen.CreateDeploymentPipelineJSONRequestBody,
) (any, error) {
	m.recordCall("CreateDeploymentPipeline", namespaceName, req)
	return `{"name":"new-pipeline"}`, nil
}

func (m *MockCoreToolsetHandler) UpdateDeploymentPipeline(
	ctx context.Context, namespaceName string, req *gen.UpdateDeploymentPipelineJSONRequestBody,
) (any, error) {
	m.recordCall("UpdateDeploymentPipeline", namespaceName, req)
	return `{"name":"updated-pipeline"}`, nil
}

func (m *MockCoreToolsetHandler) DeleteDeploymentPipeline(
	ctx context.Context, namespaceName, dpName string,
) (any, error) {
	m.recordCall("DeleteDeploymentPipeline", namespaceName, dpName)
	return `{"name":"deleted-pipeline","action":"deleted"}`, nil
}

func (m *MockCoreToolsetHandler) ListDataPlanes(ctx context.Context, namespaceName string, opts ListOpts) (any, error) {
	m.recordCall("ListDataPlanes", namespaceName, opts)
	return `[{"name":"dp1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetDataPlane(ctx context.Context, namespaceName, dpName string) (any, error) {
	m.recordCall("GetDataPlane", namespaceName, dpName)
	return `{"name":"dp1"}`, nil
}

func (m *MockCoreToolsetHandler) ListObservabilityPlanes(
	ctx context.Context, namespaceName string, opts ListOpts,
) (any, error) {
	m.recordCall("ListObservabilityPlanes", namespaceName, opts)
	return `[{"name":"observability-plane-1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetObservabilityPlane(
	ctx context.Context, namespaceName, observabilityPlaneName string,
) (any, error) {
	m.recordCall("GetObservabilityPlane", namespaceName, observabilityPlaneName)
	return `{"name":"observability-plane-1"}`, nil
}

func (m *MockCoreToolsetHandler) ListDeploymentPipelines(
	ctx context.Context, namespaceName string, opts ListOpts,
) (any, error) {
	m.recordCall("ListDeploymentPipelines", namespaceName, opts)
	return `[{"name":"default-pipeline"}]`, nil
}

func (m *MockCoreToolsetHandler) GetDeploymentPipeline(
	ctx context.Context, namespaceName, pipelineName string,
) (any, error) {
	m.recordCall("GetDeploymentPipeline", namespaceName, pipelineName)
	return `{"name":"default-pipeline"}`, nil
}

func (m *MockCoreToolsetHandler) ListWorkflowPlanes(
	ctx context.Context, namespaceName string, opts ListOpts,
) (any, error) {
	m.recordCall("ListWorkflowPlanes", namespaceName, opts)
	return `[{"name":"wp1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetWorkflowPlane(
	ctx context.Context, namespaceName, workflowPlaneName string,
) (any, error) {
	m.recordCall("GetWorkflowPlane", namespaceName, workflowPlaneName)
	return `{"name":"wp1"}`, nil
}

// ClusterPlaneHandler methods

func (m *MockCoreToolsetHandler) ListClusterDataPlanes(ctx context.Context, opts ListOpts) (any, error) {
	m.recordCall("ListClusterDataPlanes", opts)
	return `[{"name":"cdp1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetClusterDataPlane(ctx context.Context, cdpName string) (any, error) {
	m.recordCall("GetClusterDataPlane", cdpName)
	return `{"name":"cdp1"}`, nil
}

func (m *MockCoreToolsetHandler) ListClusterWorkflowPlanes(ctx context.Context, opts ListOpts) (any, error) {
	m.recordCall("ListClusterWorkflowPlanes", opts)
	return `[{"name":"cwp1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetClusterWorkflowPlane(ctx context.Context, cbpName string) (any, error) {
	m.recordCall("GetClusterWorkflowPlane", cbpName)
	return `{"name":"cwp1"}`, nil
}

func (m *MockCoreToolsetHandler) ListClusterObservabilityPlanes(ctx context.Context, opts ListOpts) (any, error) {
	m.recordCall("ListClusterObservabilityPlanes", opts)
	return `[{"name":"cop1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetClusterObservabilityPlane(ctx context.Context, copName string) (any, error) {
	m.recordCall("GetClusterObservabilityPlane", copName)
	return `{"name":"cop1"}`, nil
}

func (m *MockCoreToolsetHandler) ListClusterWorkflows(ctx context.Context, opts ListOpts) (any, error) {
	m.recordCall("ListClusterWorkflows", opts)
	return `[{"name":"cluster-workflow-1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetClusterWorkflow(ctx context.Context, cwfName string) (any, error) {
	m.recordCall("GetClusterWorkflow", cwfName)
	return `{"name":"cluster-workflow-1"}`, nil
}

func (m *MockCoreToolsetHandler) GetClusterWorkflowSchema(ctx context.Context, cwfName string) (any, error) {
	m.recordCall("GetClusterWorkflowSchema", cwfName)
	return emptyObjectSchema, nil
}

// Platform standards write methods (namespace-scoped)

func (m *MockCoreToolsetHandler) CreateComponentType(
	ctx context.Context, namespaceName string, req *gen.CreateComponentTypeJSONRequestBody,
) (any, error) {
	m.recordCall("CreateComponentType", namespaceName, req)
	return `{"name":"new-component-type","action":"created"}`, nil
}

func (m *MockCoreToolsetHandler) UpdateComponentType(
	ctx context.Context, namespaceName string, req *gen.UpdateComponentTypeJSONRequestBody,
) (any, error) {
	m.recordCall("UpdateComponentType", namespaceName, req)
	return `{"name":"updated-component-type","action":"updated"}`, nil
}

func (m *MockCoreToolsetHandler) DeleteComponentType(
	ctx context.Context, namespaceName, ctName string,
) (any, error) {
	m.recordCall("DeleteComponentType", namespaceName, ctName)
	return `{"name":"deleted-component-type","action":"deleted"}`, nil
}

func (m *MockCoreToolsetHandler) CreateTrait(
	ctx context.Context, namespaceName string, req *gen.CreateTraitJSONRequestBody,
) (any, error) {
	m.recordCall("CreateTrait", namespaceName, req)
	return `{"name":"new-trait","action":"created"}`, nil
}

func (m *MockCoreToolsetHandler) UpdateTrait(
	ctx context.Context, namespaceName string, req *gen.UpdateTraitJSONRequestBody,
) (any, error) {
	m.recordCall("UpdateTrait", namespaceName, req)
	return `{"name":"updated-trait","action":"updated"}`, nil
}

func (m *MockCoreToolsetHandler) DeleteTrait(
	ctx context.Context, namespaceName, traitName string,
) (any, error) {
	m.recordCall("DeleteTrait", namespaceName, traitName)
	return `{"name":"deleted-trait","action":"deleted"}`, nil
}

func (m *MockCoreToolsetHandler) CreateWorkflow(
	ctx context.Context, namespaceName string, req *gen.CreateWorkflowJSONRequestBody,
) (any, error) {
	m.recordCall("CreateWorkflow", namespaceName, req)
	return `{"name":"new-workflow","action":"created"}`, nil
}

func (m *MockCoreToolsetHandler) UpdateWorkflow(
	ctx context.Context, namespaceName string, req *gen.UpdateWorkflowJSONRequestBody,
) (any, error) {
	m.recordCall("UpdateWorkflow", namespaceName, req)
	return `{"name":"updated-workflow","action":"updated"}`, nil
}

func (m *MockCoreToolsetHandler) DeleteWorkflow(
	ctx context.Context, namespaceName, workflowName string,
) (any, error) {
	m.recordCall("DeleteWorkflow", namespaceName, workflowName)
	return `{"name":"deleted-workflow","action":"deleted"}`, nil
}

// Platform standards write methods (cluster-scoped)

func (m *MockCoreToolsetHandler) CreateClusterComponentType(
	ctx context.Context, req *gen.CreateClusterComponentTypeJSONRequestBody,
) (any, error) {
	m.recordCall("CreateClusterComponentType", req)
	return `{"name":"new-cluster-component-type","action":"created"}`, nil
}

func (m *MockCoreToolsetHandler) UpdateClusterComponentType(
	ctx context.Context, req *gen.UpdateClusterComponentTypeJSONRequestBody,
) (any, error) {
	m.recordCall("UpdateClusterComponentType", req)
	return `{"name":"updated-cluster-component-type","action":"updated"}`, nil
}

func (m *MockCoreToolsetHandler) DeleteClusterComponentType(
	ctx context.Context, cctName string,
) (any, error) {
	m.recordCall("DeleteClusterComponentType", cctName)
	return `{"name":"deleted-cluster-component-type","action":"deleted"}`, nil
}

func (m *MockCoreToolsetHandler) CreateClusterTrait(
	ctx context.Context, req *gen.CreateClusterTraitJSONRequestBody,
) (any, error) {
	m.recordCall("CreateClusterTrait", req)
	return `{"name":"new-cluster-trait","action":"created"}`, nil
}

func (m *MockCoreToolsetHandler) UpdateClusterTrait(
	ctx context.Context, req *gen.UpdateClusterTraitJSONRequestBody,
) (any, error) {
	m.recordCall("UpdateClusterTrait", req)
	return `{"name":"updated-cluster-trait","action":"updated"}`, nil
}

func (m *MockCoreToolsetHandler) DeleteClusterTrait(
	ctx context.Context, clusterTraitName string,
) (any, error) {
	m.recordCall("DeleteClusterTrait", clusterTraitName)
	return `{"name":"deleted-cluster-trait","action":"deleted"}`, nil
}

func (m *MockCoreToolsetHandler) CreateClusterWorkflow(
	ctx context.Context, req *gen.CreateClusterWorkflowJSONRequestBody,
) (any, error) {
	m.recordCall("CreateClusterWorkflow", req)
	return `{"name":"new-cluster-workflow","action":"created"}`, nil
}

func (m *MockCoreToolsetHandler) UpdateClusterWorkflow(
	ctx context.Context, req *gen.UpdateClusterWorkflowJSONRequestBody,
) (any, error) {
	m.recordCall("UpdateClusterWorkflow", req)
	return `{"name":"updated-cluster-workflow","action":"updated"}`, nil
}

func (m *MockCoreToolsetHandler) DeleteClusterWorkflow(
	ctx context.Context, clusterWorkflowName string,
) (any, error) {
	m.recordCall("DeleteClusterWorkflow", clusterWorkflowName)
	return `{"name":"deleted-cluster-workflow","action":"deleted"}`, nil
}

// Diagnostics methods

func (m *MockCoreToolsetHandler) GetResourceTree(
	ctx context.Context, namespaceName, releaseBindingName string,
) (any, error) {
	m.recordCall("GetResourceTree", namespaceName, releaseBindingName)
	return `{"rendered_releases":[]}`, nil
}

func (m *MockCoreToolsetHandler) GetResourceEvents(
	ctx context.Context, namespaceName, releaseBindingName, group, version, kind, name string,
) (any, error) {
	m.recordCall("GetResourceEvents", namespaceName, releaseBindingName, group, version, kind, name)
	return `{"events":[]}`, nil
}

func (m *MockCoreToolsetHandler) GetResourceLogs(
	ctx context.Context, namespaceName, releaseBindingName, podName string, sinceSeconds *int64,
) (any, error) {
	m.recordCall("GetResourceLogs", namespaceName, releaseBindingName, podName, sinceSeconds)
	return `{"logEntries":[]}`, nil
}
