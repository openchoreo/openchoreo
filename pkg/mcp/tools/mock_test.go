// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const emptyObjectSchema = `{"type":"object","properties":{}}`

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
	ctx context.Context, namespaceName, componentName string,
) (any, error) {
	m.recordCall("ListWorkloads", namespaceName, componentName)
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

func (m *MockCoreToolsetHandler) PatchReleaseBinding(
	ctx context.Context, namespaceName, bindingName string,
	req *gen.ReleaseBindingSpec,
) (any, error) {
	m.recordCall("PatchReleaseBinding", namespaceName, bindingName, req)
	return `{"status":"updated"}`, nil
}

func (m *MockCoreToolsetHandler) DeployRelease(
	ctx context.Context, namespaceName, componentName string, req *gen.DeployReleaseRequest,
) (any, error) {
	m.recordCall("DeployRelease", namespaceName, componentName, req)
	return `{"environment":"dev"}`, nil
}

func (m *MockCoreToolsetHandler) PromoteComponent(
	ctx context.Context, namespaceName, componentName string, req *gen.PromoteComponentRequest,
) (any, error) {
	m.recordCall("PromoteComponent", namespaceName, componentName, req)
	return `{"environment":"staging"}`, nil
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

func (m *MockCoreToolsetHandler) UpdateReleaseBindingState(
	ctx context.Context, namespaceName, bindingName string,
	state *gen.ReleaseBindingSpecState,
) (any, error) {
	m.recordCall("UpdateReleaseBindingState", namespaceName, bindingName, state)
	if state == nil {
		return `{"status":"updated"}`, nil
	}
	return `{"status":"updated","state":"` + string(*state) + `"}`, nil
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

// InfrastructureToolsetHandler methods

func (m *MockCoreToolsetHandler) ListEnvironments(
	ctx context.Context, namespaceName string, opts ListOpts,
) (any, error) {
	m.recordCall("ListEnvironments", namespaceName, opts)
	return `[{"name":"dev"}]`, nil
}

func (m *MockCoreToolsetHandler) GetEnvironment(ctx context.Context, namespaceName, envName string) (any, error) {
	m.recordCall("GetEnvironment", namespaceName, envName)
	return `{"name":"dev"}`, nil
}

func (m *MockCoreToolsetHandler) CreateEnvironment(
	ctx context.Context, namespaceName string, req *gen.CreateEnvironmentJSONRequestBody,
) (any, error) {
	m.recordCall("CreateEnvironment", namespaceName, req)
	return `{"name":"new-env"}`, nil
}

func (m *MockCoreToolsetHandler) ListDataPlanes(ctx context.Context, namespaceName string, opts ListOpts) (any, error) {
	m.recordCall("ListDataPlanes", namespaceName, opts)
	return `[{"name":"dp1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetDataPlane(ctx context.Context, namespaceName, dpName string) (any, error) {
	m.recordCall("GetDataPlane", namespaceName, dpName)
	return `{"name":"dp1"}`, nil
}

func (m *MockCoreToolsetHandler) CreateDataPlane(
	ctx context.Context, namespaceName string, req *gen.CreateDataPlaneJSONRequestBody,
) (any, error) {
	m.recordCall("CreateDataPlane", namespaceName, req)
	return `{"name":"new-dp"}`, nil
}

func (m *MockCoreToolsetHandler) ListObservabilityPlanes(
	ctx context.Context, namespaceName string, opts ListOpts,
) (any, error) {
	m.recordCall("ListObservabilityPlanes", namespaceName, opts)
	return `[{"name":"observability-plane-1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetDeploymentPipeline(
	ctx context.Context, namespaceName, pipelineName string,
) (any, error) {
	m.recordCall("GetDeploymentPipeline", namespaceName, pipelineName)
	return `{"name":"default-pipeline"}`, nil
}

func (m *MockCoreToolsetHandler) ListDeploymentPipelines(
	ctx context.Context, namespaceName string, opts ListOpts,
) (any, error) {
	m.recordCall("ListDeploymentPipelines", namespaceName, opts)
	return `[{"name":"default-pipeline"}]`, nil
}

func (m *MockCoreToolsetHandler) ListBuildPlanes(
	ctx context.Context, namespaceName string, opts ListOpts,
) (any, error) {
	m.recordCall("ListBuildPlanes", namespaceName, opts)
	return `[{"name":"bp1"}]`, nil
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

func (m *MockCoreToolsetHandler) CreateClusterDataPlane(
	ctx context.Context, req *gen.CreateClusterDataPlaneJSONRequestBody,
) (any, error) {
	m.recordCall("CreateClusterDataPlane", req)
	return `{"name":"new-cdp"}`, nil
}

func (m *MockCoreToolsetHandler) ListClusterBuildPlanes(ctx context.Context, opts ListOpts) (any, error) {
	m.recordCall("ListClusterBuildPlanes", opts)
	return `[{"name":"cbp1"}]`, nil
}

func (m *MockCoreToolsetHandler) ListClusterObservabilityPlanes(ctx context.Context, opts ListOpts) (any, error) {
	m.recordCall("ListClusterObservabilityPlanes", opts)
	return `[{"name":"cop1"}]`, nil
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
