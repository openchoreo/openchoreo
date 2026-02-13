// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

const emptyObjectSchema = `{"type":"object","properties":{}}`

// MockCoreToolsetHandler implements CoreToolsetHandler for testing
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

func (m *MockCoreToolsetHandler) GetNamespace(ctx context.Context, name string) (any, error) {
	m.recordCall("GetNamespace", name)
	return `{"name":"test-namespace"}`, nil
}

func (m *MockCoreToolsetHandler) ListNamespaces(ctx context.Context) (any, error) {
	m.recordCall("ListNamespaces")
	return `[{"name":"test-namespace"}]`, nil
}

func (m *MockCoreToolsetHandler) CreateNamespace(
	ctx context.Context, req *models.CreateNamespaceRequest,
) (any, error) {
	m.recordCall("CreateNamespace", req)
	return `{"name":"new-namespace"}`, nil
}

func (m *MockCoreToolsetHandler) ListSecretReferences(ctx context.Context, namespaceName string) (any, error) {
	m.recordCall("ListSecretReferences", namespaceName)
	return `[{"name":"secret-ref-1"}]`, nil
}

func (m *MockCoreToolsetHandler) ListProjects(ctx context.Context, namespaceName string) (any, error) {
	m.recordCall("ListProjects", namespaceName)
	return `[{"name":"project1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetProject(ctx context.Context, namespaceName, projectName string) (any, error) {
	m.recordCall("GetProject", namespaceName, projectName)
	return `{"name":"project1"}`, nil
}

func (m *MockCoreToolsetHandler) CreateProject(
	ctx context.Context, namespaceName string, req *models.CreateProjectRequest,
) (any, error) {
	m.recordCall("CreateProject", namespaceName, req)
	return `{"name":"new-project"}`, nil
}

func (m *MockCoreToolsetHandler) CreateComponent(
	ctx context.Context, namespaceName, projectName string, req *models.CreateComponentRequest,
) (any, error) {
	m.recordCall("CreateComponent", namespaceName, projectName, req)
	return `{"name":"new-component"}`, nil
}

func (m *MockCoreToolsetHandler) ListComponents(ctx context.Context, namespaceName, projectName string) (any, error) {
	m.recordCall("ListComponents", namespaceName, projectName)
	return `[{"name":"component1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetComponent(
	ctx context.Context, namespaceName, projectName, componentName string, additionalResources []string,
) (any, error) {
	m.recordCall("GetComponent", namespaceName, projectName, componentName, additionalResources)
	return `{"name":"component1"}`, nil
}

func (m *MockCoreToolsetHandler) GetComponentBinding(
	ctx context.Context, namespaceName, projectName, componentName, environment string,
) (any, error) {
	m.recordCall("GetComponentBinding", namespaceName, projectName, componentName, environment)
	return `{"environment":"dev"}`, nil
}

func (m *MockCoreToolsetHandler) UpdateComponentBinding(
	ctx context.Context, namespaceName, projectName, componentName, bindingName string,
	req *models.UpdateBindingRequest,
) (any, error) {
	m.recordCall("UpdateComponentBinding", namespaceName, projectName, componentName, bindingName, req)
	return `{"status":"updated"}`, nil
}

func (m *MockCoreToolsetHandler) GetComponentObserverURL(
	ctx context.Context, namespaceName, projectName, componentName, environmentName string,
) (any, error) {
	m.recordCall("GetComponentObserverURL", namespaceName, projectName, componentName, environmentName)
	return `{"url":"http://observer.example.com"}`, nil
}

func (m *MockCoreToolsetHandler) GetBuildObserverURL(
	ctx context.Context, namespaceName, projectName, componentName string,
) (any, error) {
	m.recordCall("GetBuildObserverURL", namespaceName, projectName, componentName)
	return `{"url":"http://build-observer.example.com"}`, nil
}

func (m *MockCoreToolsetHandler) GetComponentWorkloads(
	ctx context.Context, namespaceName, projectName, componentName string,
) (any, error) {
	m.recordCall("GetComponentWorkloads", namespaceName, projectName, componentName)
	return `[{"name":"workload1"}]`, nil
}

func (m *MockCoreToolsetHandler) ListEnvironments(ctx context.Context, namespaceName string) (any, error) {
	m.recordCall("ListEnvironments", namespaceName)
	return `[{"name":"dev"}]`, nil
}

func (m *MockCoreToolsetHandler) GetEnvironment(ctx context.Context, namespaceName, envName string) (any, error) {
	m.recordCall("GetEnvironment", namespaceName, envName)
	return `{"name":"dev"}`, nil
}

func (m *MockCoreToolsetHandler) CreateEnvironment(
	ctx context.Context, namespaceName string, req *models.CreateEnvironmentRequest,
) (any, error) {
	m.recordCall("CreateEnvironment", namespaceName, req)
	return `{"name":"new-env"}`, nil
}

func (m *MockCoreToolsetHandler) ListDataPlanes(ctx context.Context, namespaceName string) (any, error) {
	m.recordCall("ListDataPlanes", namespaceName)
	return `[{"name":"dp1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetDataPlane(ctx context.Context, namespaceName, dpName string) (any, error) {
	m.recordCall("GetDataPlane", namespaceName, dpName)
	return `{"name":"dp1"}`, nil
}

func (m *MockCoreToolsetHandler) CreateDataPlane(
	ctx context.Context, namespaceName string, req *models.CreateDataPlaneRequest,
) (any, error) {
	m.recordCall("CreateDataPlane", namespaceName, req)
	return `{"name":"new-dp"}`, nil
}

func (m *MockCoreToolsetHandler) ListBuildTemplates(ctx context.Context, namespaceName string) (any, error) {
	m.recordCall("ListBuildTemplates", namespaceName)
	return `[{"name":"template1"}]`, nil
}

func (m *MockCoreToolsetHandler) TriggerBuild(
	ctx context.Context, namespaceName, projectName, componentName, commit string,
) (any, error) {
	m.recordCall("TriggerBuild", namespaceName, projectName, componentName, commit)
	return `{"buildId":"build-123"}`, nil
}

func (m *MockCoreToolsetHandler) ListBuilds(
	ctx context.Context, namespaceName, projectName, componentName string,
) (any, error) {
	m.recordCall("ListBuilds", namespaceName, projectName, componentName)
	return `[{"id":"build-123"}]`, nil
}

func (m *MockCoreToolsetHandler) ListBuildPlanes(ctx context.Context, namespaceName string) (any, error) {
	m.recordCall("ListBuildPlanes", namespaceName)
	return `[{"name":"bp1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetProjectDeploymentPipeline(
	ctx context.Context, namespaceName, projectName string,
) (any, error) {
	m.recordCall("GetProjectDeploymentPipeline", namespaceName, projectName)
	return `{"stages":[]}`, nil
}

func (m *MockCoreToolsetHandler) ExplainSchema(ctx context.Context, kind, path string) (any, error) {
	m.recordCall("ExplainSchema", kind, path)
	return `{"group":"openchoreo.dev","kind":"Component","version":"v1alpha1","type":"Object"}`, nil
}

func (m *MockCoreToolsetHandler) ListComponentReleases(
	ctx context.Context, namespaceName, projectName, componentName string,
) (any, error) {
	m.recordCall("ListComponentReleases", namespaceName, projectName, componentName)
	return `[{"name":"release-1"}]`, nil
}

func (m *MockCoreToolsetHandler) CreateComponentRelease(
	ctx context.Context, namespaceName, projectName, componentName, releaseName string,
) (any, error) {
	m.recordCall("CreateComponentRelease", namespaceName, projectName, componentName, releaseName)
	return `{"name":"release-1"}`, nil
}

func (m *MockCoreToolsetHandler) GetComponentRelease(
	ctx context.Context, namespaceName, projectName, componentName, releaseName string,
) (any, error) {
	m.recordCall("GetComponentRelease", namespaceName, projectName, componentName, releaseName)
	return `{"name":"release-1"}`, nil
}

func (m *MockCoreToolsetHandler) ListReleaseBindings(
	ctx context.Context, namespaceName, projectName, componentName string, environments []string,
) (any, error) {
	m.recordCall("ListReleaseBindings", namespaceName, projectName, componentName, environments)
	return `[{"environment":"dev"}]`, nil
}

func (m *MockCoreToolsetHandler) PatchReleaseBinding(
	ctx context.Context, namespaceName, projectName, componentName, bindingName string,
	req *models.PatchReleaseBindingRequest,
) (any, error) {
	m.recordCall("PatchReleaseBinding", namespaceName, projectName, componentName, bindingName, req)
	return `{"status":"updated"}`, nil
}

func (m *MockCoreToolsetHandler) DeployRelease(
	ctx context.Context, namespaceName, projectName, componentName string, req *models.DeployReleaseRequest,
) (any, error) {
	m.recordCall("DeployRelease", namespaceName, projectName, componentName, req)
	return `{"environment":"dev"}`, nil
}

func (m *MockCoreToolsetHandler) PromoteComponent(
	ctx context.Context, namespaceName, projectName, componentName string, req *models.PromoteComponentRequest,
) (any, error) {
	m.recordCall("PromoteComponent", namespaceName, projectName, componentName, req)
	return `{"environment":"staging"}`, nil
}

func (m *MockCoreToolsetHandler) CreateWorkload(
	ctx context.Context, namespaceName, projectName, componentName string, workloadSpec interface{},
) (any, error) {
	m.recordCall("CreateWorkload", namespaceName, projectName, componentName, workloadSpec)
	return `{"name":"workload-1"}`, nil
}

func (m *MockCoreToolsetHandler) GetComponentSchema(
	ctx context.Context, namespaceName, projectName, componentName string,
) (any, error) {
	m.recordCall("GetComponentSchema", namespaceName, projectName, componentName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) GetComponentReleaseSchema(
	ctx context.Context, namespaceName, projectName, componentName, releaseName string,
) (any, error) {
	m.recordCall("GetComponentReleaseSchema", namespaceName, projectName, componentName, releaseName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) ListComponentTraits(
	ctx context.Context, namespaceName, projectName, componentName string,
) (any, error) {
	m.recordCall("ListComponentTraits", namespaceName, projectName, componentName)
	return `[{"name":"autoscaling","instanceName":"hpa-1","parameters":{"minReplicas":1,"maxReplicas":10}}]`, nil
}

func (m *MockCoreToolsetHandler) UpdateComponentTraits(
	ctx context.Context, namespaceName, projectName, componentName string, req *models.UpdateComponentTraitsRequest,
) (any, error) {
	m.recordCall("UpdateComponentTraits", namespaceName, projectName, componentName, req)
	return `[{"name":"autoscaling","instanceName":"hpa-1","parameters":{"minReplicas":2,"maxReplicas":20}}]`, nil
}

func (m *MockCoreToolsetHandler) GetEnvironmentRelease(
	ctx context.Context, namespaceName, projectName, componentName, environmentName string,
) (any, error) {
	m.recordCall("GetEnvironmentRelease", namespaceName, projectName, componentName, environmentName)
	return `{"spec":{"resources":[]},"status":{"phase":"Ready"}}`, nil
}

func (m *MockCoreToolsetHandler) PatchComponent(
	ctx context.Context, namespaceName, projectName, componentName string, req *models.PatchComponentRequest,
) (any, error) {
	m.recordCall("PatchComponent", namespaceName, projectName, componentName, req)
	return `{"name":"patched-component"}`, nil
}

func (m *MockCoreToolsetHandler) ListComponentWorkflows(ctx context.Context, namespaceName string) (any, error) {
	m.recordCall("ListComponentWorkflows", namespaceName)
	return `[{"name":"build-workflow"}]`, nil
}

func (m *MockCoreToolsetHandler) GetComponentWorkflowSchema(
	ctx context.Context, namespaceName, cwName string,
) (any, error) {
	m.recordCall("GetComponentWorkflowSchema", namespaceName, cwName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) TriggerComponentWorkflow(
	ctx context.Context, namespaceName, projectName, componentName, commit string,
) (any, error) {
	m.recordCall("TriggerComponentWorkflow", namespaceName, projectName, componentName, commit)
	return `{"runId":"workflow-run-1","status":"Running"}`, nil
}

func (m *MockCoreToolsetHandler) ListComponentWorkflowRuns(
	ctx context.Context, namespaceName, projectName, componentName string,
) (any, error) {
	m.recordCall("ListComponentWorkflowRuns", namespaceName, projectName, componentName)
	return `[{"runId":"workflow-run-1","status":"Completed"}]`, nil
}

func (m *MockCoreToolsetHandler) UpdateComponentWorkflowSchema(
	ctx context.Context, namespaceName, projectName, componentName string,
	req *models.UpdateComponentWorkflowRequest,
) (any, error) {
	m.recordCall("UpdateComponentWorkflowSchema", namespaceName, projectName, componentName, req)
	return `{"name":"component-1","workflowSchema":{}}`, nil
}

func (m *MockCoreToolsetHandler) ListComponentTypes(ctx context.Context, namespaceName string) (any, error) {
	m.recordCall("ListComponentTypes", namespaceName)
	return `[{"name":"WebApplication"}]`, nil
}

func (m *MockCoreToolsetHandler) GetComponentTypeSchema(
	ctx context.Context, namespaceName, ctName string,
) (any, error) {
	m.recordCall("GetComponentTypeSchema", namespaceName, ctName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) ListWorkflows(ctx context.Context, namespaceName string) (any, error) {
	m.recordCall("ListWorkflows", namespaceName)
	return `[{"name":"workflow-1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetWorkflowSchema(
	ctx context.Context, namespaceName, workflowName string,
) (any, error) {
	m.recordCall("GetWorkflowSchema", namespaceName, workflowName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) ListTraits(ctx context.Context, namespaceName string) (any, error) {
	m.recordCall("ListTraits", namespaceName)
	return `[{"name":"autoscaling"}]`, nil
}

func (m *MockCoreToolsetHandler) GetTraitSchema(ctx context.Context, namespaceName, traitName string) (any, error) {
	m.recordCall("GetTraitSchema", namespaceName, traitName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) ListObservabilityPlanes(ctx context.Context, namespaceName string) (any, error) {
	m.recordCall("ListObservabilityPlanes", namespaceName)
	return `[{"name":"observability-plane-1"}]`, nil
}

func (m *MockCoreToolsetHandler) ListClusterDataPlanes(ctx context.Context) (any, error) {
	m.recordCall("ListClusterDataPlanes")
	return `[{"name":"cdp1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetClusterDataPlane(ctx context.Context, cdpName string) (any, error) {
	m.recordCall("GetClusterDataPlane", cdpName)
	return `{"name":"cdp1"}`, nil
}

func (m *MockCoreToolsetHandler) CreateClusterDataPlane(
	ctx context.Context, req *models.CreateClusterDataPlaneRequest,
) (any, error) {
	m.recordCall("CreateClusterDataPlane", req)
	return `{"name":"new-cdp"}`, nil
}

func (m *MockCoreToolsetHandler) ListClusterBuildPlanes(ctx context.Context) (any, error) {
	m.recordCall("ListClusterBuildPlanes")
	return `[{"name":"cbp1"}]`, nil
}

func (m *MockCoreToolsetHandler) ListClusterObservabilityPlanes(ctx context.Context) (any, error) {
	m.recordCall("ListClusterObservabilityPlanes")
	return `[{"name":"cop1"}]`, nil
}

func (m *MockCoreToolsetHandler) ApplyResource(ctx context.Context, resource map[string]interface{}) (any, error) {
	m.recordCall("ApplyResource", resource)
	return `{"operation":"created"}`, nil
}

func (m *MockCoreToolsetHandler) DeleteResource(ctx context.Context, resource map[string]interface{}) (any, error) {
	m.recordCall("DeleteResource", resource)
	return `{"operation":"deleted"}`, nil
}

func (m *MockCoreToolsetHandler) GetResource(
	ctx context.Context, namespaceName, kind, resourceName string,
) (any, error) {
	m.recordCall("GetResource", namespaceName, kind, resourceName)
	return `{"kind":"Component","metadata":{"name":"test-component"}}`, nil
}
