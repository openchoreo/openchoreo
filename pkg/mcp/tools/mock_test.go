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

func (m *MockCoreToolsetHandler) GetOrganization(ctx context.Context, name string) (any, error) {
	m.recordCall("GetOrganization", name)
	return `{"name":"test-org"}`, nil
}

func (m *MockCoreToolsetHandler) ListOrganizations(ctx context.Context) (any, error) {
	m.recordCall("ListOrganizations")
	return `[{"name":"test-org"}]`, nil
}

func (m *MockCoreToolsetHandler) ListSecretReferences(ctx context.Context, orgName string) (any, error) {
	m.recordCall("ListSecretReferences", orgName)
	return `[{"name":"secret-ref-1"}]`, nil
}

func (m *MockCoreToolsetHandler) ListProjects(ctx context.Context, orgName string) (any, error) {
	m.recordCall("ListProjects", orgName)
	return `[{"name":"project1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetProject(ctx context.Context, orgName, projectName string) (any, error) {
	m.recordCall("GetProject", orgName, projectName)
	return `{"name":"project1"}`, nil
}

func (m *MockCoreToolsetHandler) CreateProject(
	ctx context.Context, orgName string, req *models.CreateProjectRequest,
) (any, error) {
	m.recordCall("CreateProject", orgName, req)
	return `{"name":"new-project"}`, nil
}

func (m *MockCoreToolsetHandler) CreateComponent(
	ctx context.Context, orgName, projectName string, req *models.CreateComponentRequest,
) (any, error) {
	m.recordCall("CreateComponent", orgName, projectName, req)
	return `{"name":"new-component"}`, nil
}

func (m *MockCoreToolsetHandler) ListComponents(ctx context.Context, orgName, projectName string) (any, error) {
	m.recordCall("ListComponents", orgName, projectName)
	return `[{"name":"component1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetComponent(
	ctx context.Context, orgName, projectName, componentName string, additionalResources []string,
) (any, error) {
	m.recordCall("GetComponent", orgName, projectName, componentName, additionalResources)
	return `{"name":"component1"}`, nil
}

func (m *MockCoreToolsetHandler) GetComponentBinding(
	ctx context.Context, orgName, projectName, componentName, environment string,
) (any, error) {
	m.recordCall("GetComponentBinding", orgName, projectName, componentName, environment)
	return `{"environment":"dev"}`, nil
}

func (m *MockCoreToolsetHandler) UpdateComponentBinding(
	ctx context.Context, orgName, projectName, componentName, bindingName string,
	req *models.UpdateBindingRequest,
) (any, error) {
	m.recordCall("UpdateComponentBinding", orgName, projectName, componentName, bindingName, req)
	return `{"status":"updated"}`, nil
}

func (m *MockCoreToolsetHandler) GetComponentObserverURL(
	ctx context.Context, orgName, projectName, componentName, environmentName string,
) (any, error) {
	m.recordCall("GetComponentObserverURL", orgName, projectName, componentName, environmentName)
	return `{"url":"http://observer.example.com"}`, nil
}

func (m *MockCoreToolsetHandler) GetBuildObserverURL(
	ctx context.Context, orgName, projectName, componentName string,
) (any, error) {
	m.recordCall("GetBuildObserverURL", orgName, projectName, componentName)
	return `{"url":"http://build-observer.example.com"}`, nil
}

func (m *MockCoreToolsetHandler) GetComponentWorkloads(
	ctx context.Context, orgName, projectName, componentName string,
) (any, error) {
	m.recordCall("GetComponentWorkloads", orgName, projectName, componentName)
	return `[{"name":"workload1"}]`, nil
}

func (m *MockCoreToolsetHandler) ListEnvironments(ctx context.Context, orgName string) (any, error) {
	m.recordCall("ListEnvironments", orgName)
	return `[{"name":"dev"}]`, nil
}

func (m *MockCoreToolsetHandler) GetEnvironment(ctx context.Context, orgName, envName string) (any, error) {
	m.recordCall("GetEnvironment", orgName, envName)
	return `{"name":"dev"}`, nil
}

func (m *MockCoreToolsetHandler) CreateEnvironment(
	ctx context.Context, orgName string, req *models.CreateEnvironmentRequest,
) (any, error) {
	m.recordCall("CreateEnvironment", orgName, req)
	return `{"name":"new-env"}`, nil
}

func (m *MockCoreToolsetHandler) ListDataPlanes(ctx context.Context, orgName string) (any, error) {
	m.recordCall("ListDataPlanes", orgName)
	return `[{"name":"dp1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetDataPlane(ctx context.Context, orgName, dpName string) (any, error) {
	m.recordCall("GetDataPlane", orgName, dpName)
	return `{"name":"dp1"}`, nil
}

func (m *MockCoreToolsetHandler) CreateDataPlane(
	ctx context.Context, orgName string, req *models.CreateDataPlaneRequest,
) (any, error) {
	m.recordCall("CreateDataPlane", orgName, req)
	return `{"name":"new-dp"}`, nil
}

func (m *MockCoreToolsetHandler) ListBuildTemplates(ctx context.Context, orgName string) (any, error) {
	m.recordCall("ListBuildTemplates", orgName)
	return `[{"name":"template1"}]`, nil
}

func (m *MockCoreToolsetHandler) TriggerBuild(
	ctx context.Context, orgName, projectName, componentName, commit string,
) (any, error) {
	m.recordCall("TriggerBuild", orgName, projectName, componentName, commit)
	return `{"buildId":"build-123"}`, nil
}

func (m *MockCoreToolsetHandler) ListBuilds(
	ctx context.Context, orgName, projectName, componentName string,
) (any, error) {
	m.recordCall("ListBuilds", orgName, projectName, componentName)
	return `[{"id":"build-123"}]`, nil
}

func (m *MockCoreToolsetHandler) ListBuildPlanes(ctx context.Context, orgName string) (any, error) {
	m.recordCall("ListBuildPlanes", orgName)
	return `[{"name":"bp1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetProjectDeploymentPipeline(
	ctx context.Context, orgName, projectName string,
) (any, error) {
	m.recordCall("GetProjectDeploymentPipeline", orgName, projectName)
	return `{"stages":[]}`, nil
}

func (m *MockCoreToolsetHandler) ExplainSchema(ctx context.Context, kind, path string) (any, error) {
	m.recordCall("ExplainSchema", kind, path)
	return `{"group":"openchoreo.dev","kind":"Component","version":"v1alpha1","type":"Object"}`, nil
}

func (m *MockCoreToolsetHandler) ListComponentReleases(
	ctx context.Context, orgName, projectName, componentName string,
) (any, error) {
	m.recordCall("ListComponentReleases", orgName, projectName, componentName)
	return `[{"name":"release-1"}]`, nil
}

func (m *MockCoreToolsetHandler) CreateComponentRelease(
	ctx context.Context, orgName, projectName, componentName, releaseName string,
) (any, error) {
	m.recordCall("CreateComponentRelease", orgName, projectName, componentName, releaseName)
	return `{"name":"release-1"}`, nil
}

func (m *MockCoreToolsetHandler) GetComponentRelease(
	ctx context.Context, orgName, projectName, componentName, releaseName string,
) (any, error) {
	m.recordCall("GetComponentRelease", orgName, projectName, componentName, releaseName)
	return `{"name":"release-1"}`, nil
}

func (m *MockCoreToolsetHandler) ListReleaseBindings(
	ctx context.Context, orgName, projectName, componentName string, environments []string,
) (any, error) {
	m.recordCall("ListReleaseBindings", orgName, projectName, componentName, environments)
	return `[{"environment":"dev"}]`, nil
}

func (m *MockCoreToolsetHandler) PatchReleaseBinding(
	ctx context.Context, orgName, projectName, componentName, bindingName string,
	req *models.PatchReleaseBindingRequest,
) (any, error) {
	m.recordCall("PatchReleaseBinding", orgName, projectName, componentName, bindingName, req)
	return `{"status":"updated"}`, nil
}

func (m *MockCoreToolsetHandler) DeployRelease(
	ctx context.Context, orgName, projectName, componentName string, req *models.DeployReleaseRequest,
) (any, error) {
	m.recordCall("DeployRelease", orgName, projectName, componentName, req)
	return `{"environment":"dev"}`, nil
}

func (m *MockCoreToolsetHandler) PromoteComponent(
	ctx context.Context, orgName, projectName, componentName string, req *models.PromoteComponentRequest,
) (any, error) {
	m.recordCall("PromoteComponent", orgName, projectName, componentName, req)
	return `{"environment":"staging"}`, nil
}

func (m *MockCoreToolsetHandler) CreateWorkload(
	ctx context.Context, orgName, projectName, componentName string, workloadSpec interface{},
) (any, error) {
	m.recordCall("CreateWorkload", orgName, projectName, componentName, workloadSpec)
	return `{"name":"workload-1"}`, nil
}

func (m *MockCoreToolsetHandler) GetComponentSchema(
	ctx context.Context, orgName, projectName, componentName string,
) (any, error) {
	m.recordCall("GetComponentSchema", orgName, projectName, componentName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) GetComponentReleaseSchema(
	ctx context.Context, orgName, projectName, componentName, releaseName string,
) (any, error) {
	m.recordCall("GetComponentReleaseSchema", orgName, projectName, componentName, releaseName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) ListComponentTraits(
	ctx context.Context, orgName, projectName, componentName string,
) (any, error) {
	m.recordCall("ListComponentTraits", orgName, projectName, componentName)
	return `[{"name":"autoscaling","instanceName":"hpa-1","parameters":{"minReplicas":1,"maxReplicas":10}}]`, nil
}

func (m *MockCoreToolsetHandler) UpdateComponentTraits(
	ctx context.Context, orgName, projectName, componentName string, req *models.UpdateComponentTraitsRequest,
) (any, error) {
	m.recordCall("UpdateComponentTraits", orgName, projectName, componentName, req)
	return `[{"name":"autoscaling","instanceName":"hpa-1","parameters":{"minReplicas":2,"maxReplicas":20}}]`, nil
}

func (m *MockCoreToolsetHandler) GetEnvironmentRelease(
	ctx context.Context, orgName, projectName, componentName, environmentName string,
) (any, error) {
	m.recordCall("GetEnvironmentRelease", orgName, projectName, componentName, environmentName)
	return `{"spec":{"resources":[]},"status":{"phase":"Ready"}}`, nil
}

func (m *MockCoreToolsetHandler) PatchComponent(
	ctx context.Context, orgName, projectName, componentName string, req *models.PatchComponentRequest,
) (any, error) {
	m.recordCall("PatchComponent", orgName, projectName, componentName, req)
	return `{"name":"patched-component"}`, nil
}

func (m *MockCoreToolsetHandler) ListComponentWorkflows(ctx context.Context, orgName string) (any, error) {
	m.recordCall("ListComponentWorkflows", orgName)
	return `[{"name":"build-workflow"}]`, nil
}

func (m *MockCoreToolsetHandler) GetComponentWorkflowSchema(ctx context.Context, orgName, cwName string) (any, error) {
	m.recordCall("GetComponentWorkflowSchema", orgName, cwName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) TriggerComponentWorkflow(
	ctx context.Context, orgName, projectName, componentName, commit string,
) (any, error) {
	m.recordCall("TriggerComponentWorkflow", orgName, projectName, componentName, commit)
	return `{"runId":"workflow-run-1","status":"Running"}`, nil
}

func (m *MockCoreToolsetHandler) ListComponentWorkflowRuns(
	ctx context.Context, orgName, projectName, componentName string,
) (any, error) {
	m.recordCall("ListComponentWorkflowRuns", orgName, projectName, componentName)
	return `[{"runId":"workflow-run-1","status":"Completed"}]`, nil
}

func (m *MockCoreToolsetHandler) UpdateComponentWorkflowSchema(
	ctx context.Context, orgName, projectName, componentName string,
	req *models.UpdateComponentWorkflowSchemaRequest,
) (any, error) {
	m.recordCall("UpdateComponentWorkflowSchema", orgName, projectName, componentName, req)
	return `{"name":"component-1","workflowSchema":{}}`, nil
}

func (m *MockCoreToolsetHandler) ListComponentTypes(ctx context.Context, orgName string) (any, error) {
	m.recordCall("ListComponentTypes", orgName)
	return `[{"name":"WebApplication"}]`, nil
}

func (m *MockCoreToolsetHandler) GetComponentTypeSchema(ctx context.Context, orgName, ctName string) (any, error) {
	m.recordCall("GetComponentTypeSchema", orgName, ctName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) ListWorkflows(ctx context.Context, orgName string) (any, error) {
	m.recordCall("ListWorkflows", orgName)
	return `[{"name":"workflow-1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetWorkflowSchema(ctx context.Context, orgName, workflowName string) (any, error) {
	m.recordCall("GetWorkflowSchema", orgName, workflowName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) ListTraits(ctx context.Context, orgName string) (any, error) {
	m.recordCall("ListTraits", orgName)
	return `[{"name":"autoscaling"}]`, nil
}

func (m *MockCoreToolsetHandler) GetTraitSchema(ctx context.Context, orgName, traitName string) (any, error) {
	m.recordCall("GetTraitSchema", orgName, traitName)
	return emptyObjectSchema, nil
}

func (m *MockCoreToolsetHandler) ListObservabilityPlanes(ctx context.Context, orgName string) (any, error) {
	m.recordCall("ListObservabilityPlanes", orgName)
	return `[{"name":"observability-plane-1"}]`, nil
}

func (m *MockCoreToolsetHandler) ApplyResource(ctx context.Context, resource map[string]interface{}) (any, error) {
	m.recordCall("ApplyResource", resource)
	return `{"operation":"created"}`, nil
}

func (m *MockCoreToolsetHandler) DeleteResource(ctx context.Context, resource map[string]interface{}) (any, error) {
	m.recordCall("DeleteResource", resource)
	return `{"operation":"deleted"}`, nil
}
