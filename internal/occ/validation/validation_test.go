// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"strings"
	"testing"

	configContext "github.com/openchoreo/openchoreo/pkg/cli/cmd/config"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// ---- ValidateName tests ----

func TestValidateName_Valid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"simple lowercase", "myproject"},
		{"with hyphen", "my-project"},
		{"with numbers", "project123"},
		{"alphanumeric start and end", "a1b2c3"},
		{"single hyphen in middle", "a-b"},
		{"long name", "my-long-project-name-with-many-hyphens-123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName("project", tt.input)
			if err != nil {
				t.Errorf("ValidateName(%q) returned unexpected error: %v", tt.input, err)
			}
		})
	}
}

func TestValidateName_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"uppercase", "MyProject"},
		{"starts with hyphen", "-myproject"},
		{"ends with hyphen", "myproject-"},
		{"with underscore", "my_project"},
		{"with space", "my project"},
		{"with dot", "my.project"},
		{"only number start end hyphen", "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName("project", tt.input)
			if err == nil {
				t.Errorf("ValidateName(%q) expected error, got nil", tt.input)
			}
		})
	}
}

func TestValidateName_NonStringType(t *testing.T) {
	err := ValidateName("project", 123)
	if err == nil {
		t.Error("ValidateName with non-string input should return error")
	}
}

func TestValidateNamespaceName(t *testing.T) {
	if err := ValidateNamespaceName("my-namespace"); err != nil {
		t.Errorf("ValidateNamespaceName(valid) = %v, want nil", err)
	}
	if err := ValidateNamespaceName(""); err == nil {
		t.Error("ValidateNamespaceName(empty) should return error")
	}
}

func TestValidateProjectName(t *testing.T) {
	if err := ValidateProjectName("my-project"); err != nil {
		t.Errorf("ValidateProjectName(valid) = %v, want nil", err)
	}
	if err := ValidateProjectName(""); err == nil {
		t.Error("ValidateProjectName(empty) should return error")
	}
}

func TestValidateComponentName(t *testing.T) {
	if err := ValidateComponentName("my-component"); err != nil {
		t.Errorf("ValidateComponentName(valid) = %v, want nil", err)
	}
	if err := ValidateComponentName("Bad_Name"); err == nil {
		t.Error("ValidateComponentName(invalid) should return error")
	}
}

// ---- ValidateURL tests ----

func TestValidateURL_Valid(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"http URL", "http://example.com"},
		{"https URL", "https://example.com/path"},
		{"URL with query", "https://api.example.com/v1?key=val"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url)
			if err != nil {
				t.Errorf("ValidateURL(%q) = %v, want nil", tt.url, err)
			}
		})
	}
}

func TestValidateURL_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
	}{
		{"empty string", ""},
		{"non-string type", 123},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.input)
			if err == nil {
				t.Errorf("ValidateURL(%v) expected error, got nil", tt.input)
			}
		})
	}
}

// ---- ValidateGitHubURL tests ----

func TestValidateGitHubURL_Valid(t *testing.T) {
	tests := []string{
		"https://github.com/owner/repo",
		"https://github.com/my-org/my-repo",
		"https://github.com/org123/repo-name",
	}
	for _, url := range tests {
		t.Run(url, func(t *testing.T) {
			err := ValidateGitHubURL(url)
			if err != nil {
				t.Errorf("ValidateGitHubURL(%q) = %v, want nil", url, err)
			}
		})
	}
}

func TestValidateGitHubURL_Invalid(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"empty", ""},
		{"not github", "https://gitlab.com/owner/repo"},
		{"too many segments", "https://github.com/owner/repo/extra"},
		{"no repo", "https://github.com/owner"},
		{"http not https", "http://github.com/owner/repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGitHubURL(tt.url)
			if err == nil {
				t.Errorf("ValidateGitHubURL(%q) expected error, got nil", tt.url)
			}
		})
	}
}

// ---- checkRequiredFields tests ----

func TestCheckRequiredFields_AllPresent(t *testing.T) {
	fields := map[string]string{
		"name":      "my-project",
		"namespace": "default",
	}
	if !checkRequiredFields(fields) {
		t.Error("checkRequiredFields should return true when all fields are present")
	}
}

func TestCheckRequiredFields_OneMissing(t *testing.T) {
	fields := map[string]string{
		"name":      "my-project",
		"namespace": "", // missing
	}
	if checkRequiredFields(fields) {
		t.Error("checkRequiredFields should return false when a field is missing")
	}
}

func TestCheckRequiredFields_AllMissing(t *testing.T) {
	fields := map[string]string{
		"name":      "",
		"namespace": "",
	}
	if checkRequiredFields(fields) {
		t.Error("checkRequiredFields should return false when all fields are missing")
	}
}

func TestCheckRequiredFields_Empty(t *testing.T) {
	if !checkRequiredFields(map[string]string{}) {
		t.Error("checkRequiredFields should return true for empty fields map")
	}
}

// ---- generateHelpError tests ----

func TestGenerateHelpError_SingleMissing(t *testing.T) {
	fields := map[string]string{
		"name":      "",
		"namespace": "default",
	}
	err := generateHelpError(CmdCreate, ResourceProject, fields)
	if err == nil {
		t.Fatal("generateHelpError should return an error")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "name") {
		t.Errorf("generateHelpError error = %q, should contain missing field 'name'", errMsg)
	}
}

func TestGenerateHelpError_MultipleMissing(t *testing.T) {
	fields := map[string]string{
		"name":      "",
		"namespace": "",
	}
	err := generateHelpError(CmdCreate, ResourceProject, fields)
	if err == nil {
		t.Fatal("generateHelpError should return an error")
	}
	// Should mention "parameters" (plural)
	errMsg := err.Error()
	if !strings.Contains(errMsg, "parameters") {
		t.Errorf("generateHelpError error = %q, should say 'parameters' for multiple missing fields", errMsg)
	}
}

func TestGenerateHelpError_WithEmptyResource(t *testing.T) {
	fields := map[string]string{"field": ""}
	err := generateHelpError(CmdCreate, "", fields)
	if err == nil {
		t.Fatal("generateHelpError should return an error")
	}
}

// ---- pluralS tests ----

func TestPluralS_One(t *testing.T) {
	if pluralS(1) != "" {
		t.Errorf("pluralS(1) = %q, want %q", pluralS(1), "")
	}
}

func TestPluralS_Many(t *testing.T) {
	if pluralS(2) != "s" {
		t.Errorf("pluralS(2) = %q, want %q", pluralS(2), "s")
	}
	if pluralS(10) != "s" {
		t.Errorf("pluralS(10) = %q, want %q", pluralS(10), "s")
	}
}

// ---- ValidateParams tests ----

func TestValidateParams_ProjectCreate_Valid(t *testing.T) {
	params := api.CreateProjectParams{
		Namespace: "default",
		Name:      "my-project",
	}
	err := ValidateParams(CmdCreate, ResourceProject, params)
	if err != nil {
		t.Errorf("ValidateParams project create = %v, want nil", err)
	}
}

func TestValidateParams_ProjectCreate_MissingNamespace(t *testing.T) {
	params := api.CreateProjectParams{
		Namespace: "",
		Name:      "my-project",
	}
	err := ValidateParams(CmdCreate, ResourceProject, params)
	if err == nil {
		t.Error("ValidateParams project create missing namespace should return error")
	}
}

func TestValidateParams_ProjectCreate_MissingName(t *testing.T) {
	params := api.CreateProjectParams{
		Namespace: "default",
		Name:      "",
	}
	err := ValidateParams(CmdCreate, ResourceProject, params)
	if err == nil {
		t.Error("ValidateParams project create missing name should return error")
	}
}

func TestValidateParams_ProjectGet_Valid(t *testing.T) {
	params := api.GetProjectParams{
		Namespace: "default",
	}
	err := ValidateParams(CmdGet, ResourceProject, params)
	if err != nil {
		t.Errorf("ValidateParams project get = %v, want nil", err)
	}
}

func TestValidateParams_ProjectGet_MissingNamespace(t *testing.T) {
	params := api.GetProjectParams{
		Namespace: "",
	}
	err := ValidateParams(CmdGet, ResourceProject, params)
	if err == nil {
		t.Error("ValidateParams project get missing namespace should return error")
	}
}

func TestValidateParams_ComponentCreate_Valid(t *testing.T) {
	params := api.CreateComponentParams{
		Namespace:        "default",
		Project:          "my-project",
		Name:             "my-component",
		GitRepositoryURL: "https://github.com/owner/repo",
	}
	err := ValidateParams(CmdCreate, ResourceComponent, params)
	if err != nil {
		t.Errorf("ValidateParams component create = %v, want nil", err)
	}
}

func TestValidateParams_ComponentCreate_InvalidGitURL(t *testing.T) {
	params := api.CreateComponentParams{
		Namespace:        "default",
		Project:          "my-project",
		Name:             "my-component",
		GitRepositoryURL: "https://gitlab.com/owner/repo", // not GitHub
	}
	err := ValidateParams(CmdCreate, ResourceComponent, params)
	if err == nil {
		t.Error("ValidateParams component create with invalid git URL should return error")
	}
}

func TestValidateParams_UnknownResource(t *testing.T) {
	err := ValidateParams(CmdCreate, "unknown-resource", nil)
	if err == nil {
		t.Error("ValidateParams with unknown resource should return error")
	}
}

func TestValidateParams_ProjectList_ReturnsNil(t *testing.T) {
	err := ValidateParams(CmdList, ResourceProject, nil)
	// List for project should not error
	if err != nil {
		t.Errorf("ValidateParams project list = %v, want nil", err)
	}
}

// ---- Build params ----

func TestValidateParams_BuildCreate_Valid(t *testing.T) {
	params := api.CreateBuildParams{
		Namespace: "default",
		Project:   "my-project",
		Component: "my-component",
		Name:      "my-build",
	}
	if err := ValidateParams(CmdCreate, ResourceBuild, params); err != nil {
		t.Errorf("ValidateParams build create valid: %v", err)
	}
}

func TestValidateParams_BuildCreate_MissingFields(t *testing.T) {
	params := api.CreateBuildParams{Namespace: "default"}
	if err := ValidateParams(CmdCreate, ResourceBuild, params); err == nil {
		t.Error("ValidateParams build create missing fields: expected error")
	}
}

func TestValidateParams_BuildGet_Valid(t *testing.T) {
	params := api.GetBuildParams{
		Namespace: "default",
		Project:   "my-project",
		Component: "my-component",
	}
	if err := ValidateParams(CmdGet, ResourceBuild, params); err != nil {
		t.Errorf("ValidateParams build get valid: %v", err)
	}
}

func TestValidateParams_BuildGet_MissingFields(t *testing.T) {
	params := api.GetBuildParams{Namespace: "default"}
	if err := ValidateParams(CmdGet, ResourceBuild, params); err == nil {
		t.Error("ValidateParams build get missing fields: expected error")
	}
}

// ---- Deployment params ----

func TestValidateParams_DeploymentCreate_Valid(t *testing.T) {
	params := api.CreateDeploymentParams{
		Namespace: "default",
		Project:   "my-project",
		Component: "my-component",
	}
	if err := ValidateParams(CmdCreate, ResourceDeployment, params); err != nil {
		t.Errorf("ValidateParams deployment create valid: %v", err)
	}
}

func TestValidateParams_DeploymentCreate_MissingFields(t *testing.T) {
	params := api.CreateDeploymentParams{Namespace: "default"}
	if err := ValidateParams(CmdCreate, ResourceDeployment, params); err == nil {
		t.Error("ValidateParams deployment create missing fields: expected error")
	}
}

func TestValidateParams_DeploymentGet_Valid(t *testing.T) {
	params := api.GetDeploymentParams{
		Namespace: "default",
		Project:   "my-project",
		Component: "my-component",
	}
	if err := ValidateParams(CmdGet, ResourceDeployment, params); err != nil {
		t.Errorf("ValidateParams deployment get valid: %v", err)
	}
}

// ---- DeploymentTrack params ----

func TestValidateParams_DeploymentTrackCreate_Valid(t *testing.T) {
	params := api.CreateDeploymentTrackParams{
		Namespace: "default",
		Project:   "my-project",
		Component: "my-component",
	}
	if err := ValidateParams(CmdCreate, ResourceDeploymentTrack, params); err != nil {
		t.Errorf("ValidateParams deployment track create valid: %v", err)
	}
}

func TestValidateParams_DeploymentTrackCreate_Missing(t *testing.T) {
	params := api.CreateDeploymentTrackParams{Namespace: "default"}
	if err := ValidateParams(CmdCreate, ResourceDeploymentTrack, params); err == nil {
		t.Error("ValidateParams deployment track create missing: expected error")
	}
}

func TestValidateParams_DeploymentTrackGet_Valid(t *testing.T) {
	params := api.GetDeploymentTrackParams{
		Namespace: "default",
		Project:   "my-project",
		Component: "my-component",
	}
	if err := ValidateParams(CmdGet, ResourceDeploymentTrack, params); err != nil {
		t.Errorf("ValidateParams deployment track get valid: %v", err)
	}
}

// ---- Environment params ----

func TestValidateParams_EnvironmentCreate_Valid(t *testing.T) {
	params := api.CreateEnvironmentParams{
		Namespace: "default",
		Name:      "my-env",
	}
	if err := ValidateParams(CmdCreate, ResourceEnvironment, params); err != nil {
		t.Errorf("ValidateParams environment create valid: %v", err)
	}
}

func TestValidateParams_EnvironmentCreate_Missing(t *testing.T) {
	params := api.CreateEnvironmentParams{Namespace: "default"}
	if err := ValidateParams(CmdCreate, ResourceEnvironment, params); err == nil {
		t.Error("ValidateParams environment create missing name: expected error")
	}
}

func TestValidateParams_EnvironmentGet_Valid(t *testing.T) {
	params := api.GetEnvironmentParams{Namespace: "default"}
	if err := ValidateParams(CmdGet, ResourceEnvironment, params); err != nil {
		t.Errorf("ValidateParams environment get valid: %v", err)
	}
}

func TestValidateParams_EnvironmentGet_Missing(t *testing.T) {
	params := api.GetEnvironmentParams{Namespace: ""}
	if err := ValidateParams(CmdGet, ResourceEnvironment, params); err == nil {
		t.Error("ValidateParams environment get missing namespace: expected error")
	}
}

func TestValidateParams_EnvironmentList_Valid(t *testing.T) {
	params := api.ListEnvironmentsParams{Namespace: "default"}
	if err := ValidateParams(CmdList, ResourceEnvironment, params); err != nil {
		t.Errorf("ValidateParams environment list valid: %v", err)
	}
}

func TestValidateParams_EnvironmentList_Missing(t *testing.T) {
	params := api.ListEnvironmentsParams{Namespace: ""}
	if err := ValidateParams(CmdList, ResourceEnvironment, params); err == nil {
		t.Error("ValidateParams environment list missing namespace: expected error")
	}
}

// ---- DeployableArtifact params ----

func TestValidateParams_DeployableArtifactCreate_Valid(t *testing.T) {
	params := api.CreateDeployableArtifactParams{
		Namespace: "default",
		Project:   "my-project",
		Component: "my-component",
	}
	if err := ValidateParams(CmdCreate, ResourceDeployableArtifact, params); err != nil {
		t.Errorf("ValidateParams deployable artifact create valid: %v", err)
	}
}

func TestValidateParams_DeployableArtifactCreate_Missing(t *testing.T) {
	params := api.CreateDeployableArtifactParams{Namespace: "default"}
	if err := ValidateParams(CmdCreate, ResourceDeployableArtifact, params); err == nil {
		t.Error("ValidateParams deployable artifact create missing: expected error")
	}
}

// ---- Namespace params ----

func TestValidateParams_NamespaceCreate_Valid(t *testing.T) {
	params := api.CreateNamespaceParams{Name: "my-namespace"}
	if err := ValidateParams(CmdCreate, ResourceNamespace, params); err != nil {
		t.Errorf("ValidateParams namespace create valid: %v", err)
	}
}

func TestValidateParams_NamespaceCreate_Missing(t *testing.T) {
	params := api.CreateNamespaceParams{Name: ""}
	if err := ValidateParams(CmdCreate, ResourceNamespace, params); err == nil {
		t.Error("ValidateParams namespace create missing name: expected error")
	}
}

// ---- DataPlane params ----

func TestValidateParams_DataPlaneCreate_Valid(t *testing.T) {
	params := api.CreateDataPlaneParams{
		Namespace: "default",
		Name:      "my-dp",
	}
	if err := ValidateParams(CmdCreate, ResourceDataPlane, params); err != nil {
		t.Errorf("ValidateParams dataplane create valid: %v", err)
	}
}

func TestValidateParams_DataPlaneCreate_Missing(t *testing.T) {
	params := api.CreateDataPlaneParams{Namespace: "default"}
	if err := ValidateParams(CmdCreate, ResourceDataPlane, params); err == nil {
		t.Error("ValidateParams dataplane create missing name: expected error")
	}
}

func TestValidateParams_DataPlaneGet_Valid(t *testing.T) {
	params := api.GetDataPlaneParams{Namespace: "default"}
	if err := ValidateParams(CmdGet, ResourceDataPlane, params); err != nil {
		t.Errorf("ValidateParams dataplane get valid: %v", err)
	}
}

func TestValidateParams_DataPlaneList_Valid(t *testing.T) {
	params := api.ListDataPlanesParams{Namespace: "default"}
	if err := ValidateParams(CmdList, ResourceDataPlane, params); err != nil {
		t.Errorf("ValidateParams dataplane list valid: %v", err)
	}
}

func TestValidateParams_DataPlaneList_Missing(t *testing.T) {
	params := api.ListDataPlanesParams{Namespace: ""}
	if err := ValidateParams(CmdList, ResourceDataPlane, params); err == nil {
		t.Error("ValidateParams dataplane list missing namespace: expected error")
	}
}

// ---- Endpoint params ----

func TestValidateParams_EndpointGet_Valid(t *testing.T) {
	params := api.GetEndpointParams{
		Namespace: "default",
		Project:   "my-project",
		Component: "my-component",
	}
	if err := ValidateParams(CmdGet, ResourceEndpoint, params); err != nil {
		t.Errorf("ValidateParams endpoint get valid: %v", err)
	}
}

func TestValidateParams_EndpointGet_Missing(t *testing.T) {
	params := api.GetEndpointParams{Namespace: "default"}
	if err := ValidateParams(CmdGet, ResourceEndpoint, params); err == nil {
		t.Error("ValidateParams endpoint get missing fields: expected error")
	}
}

// ---- Apply/Delete params ----

func TestValidateParams_Apply_Valid(t *testing.T) {
	params := api.ApplyParams{FilePath: "/path/to/file.yaml"}
	if err := ValidateParams(CmdApply, ResourceApply, params); err != nil {
		t.Errorf("ValidateParams apply valid: %v", err)
	}
}

func TestValidateParams_Apply_MissingFile(t *testing.T) {
	params := api.ApplyParams{FilePath: ""}
	if err := ValidateParams(CmdApply, ResourceApply, params); err == nil {
		t.Error("ValidateParams apply missing file: expected error")
	}
}

func TestValidateParams_Delete_Valid(t *testing.T) {
	params := api.DeleteParams{FilePath: "/path/to/file.yaml"}
	if err := ValidateParams(CmdDelete, ResourceDelete, params); err != nil {
		t.Errorf("ValidateParams delete valid: %v", err)
	}
}

func TestValidateParams_Delete_MissingFile(t *testing.T) {
	params := api.DeleteParams{FilePath: ""}
	if err := ValidateParams(CmdDelete, ResourceDelete, params); err == nil {
		t.Error("ValidateParams delete missing file: expected error")
	}
}

// ---- Log params ----

func TestValidateParams_Logs_Build_Valid(t *testing.T) {
	params := api.LogParams{
		Type:      "build",
		Namespace: "default",
		Build:     "my-build",
	}
	if err := ValidateParams(CmdLogs, ResourceLogs, params); err != nil {
		t.Errorf("ValidateParams logs build valid: %v", err)
	}
}

func TestValidateParams_Logs_Build_MissingType(t *testing.T) {
	params := api.LogParams{Namespace: "default", Build: "my-build"}
	if err := ValidateParams(CmdLogs, ResourceLogs, params); err == nil {
		t.Error("ValidateParams logs missing type: expected error")
	}
}

func TestValidateParams_Logs_Build_MissingBuild(t *testing.T) {
	params := api.LogParams{Type: "build", Namespace: "default"}
	if err := ValidateParams(CmdLogs, ResourceLogs, params); err == nil {
		t.Error("ValidateParams logs build missing build: expected error")
	}
}

func TestValidateParams_Logs_Deployment_Valid(t *testing.T) {
	params := api.LogParams{
		Type:        "deployment",
		Namespace:   "default",
		Project:     "my-project",
		Component:   "my-component",
		Environment: "production",
		Deployment:  "my-deployment",
	}
	if err := ValidateParams(CmdLogs, ResourceLogs, params); err != nil {
		t.Errorf("ValidateParams logs deployment valid: %v", err)
	}
}

func TestValidateParams_Logs_Deployment_Missing(t *testing.T) {
	params := api.LogParams{Type: "deployment", Namespace: "default"}
	if err := ValidateParams(CmdLogs, ResourceLogs, params); err == nil {
		t.Error("ValidateParams logs deployment missing fields: expected error")
	}
}

func TestValidateParams_Logs_InvalidType(t *testing.T) {
	params := api.LogParams{Type: "invalid-type", Namespace: "default"}
	if err := ValidateParams(CmdLogs, ResourceLogs, params); err == nil {
		t.Error("ValidateParams logs invalid type: expected error")
	}
}

// ---- DeploymentPipeline params ----

func TestValidateParams_DeploymentPipelineGet_Valid(t *testing.T) {
	params := api.GetDeploymentPipelineParams{Namespace: "default"}
	if err := ValidateParams(CmdGet, ResourceDeploymentPipeline, params); err != nil {
		t.Errorf("ValidateParams deployment pipeline get valid: %v", err)
	}
}

func TestValidateParams_DeploymentPipelineCreate_Valid(t *testing.T) {
	params := api.CreateDeploymentPipelineParams{
		Namespace:        "default",
		Name:             "my-pipeline",
		EnvironmentOrder: []string{"dev", "prod"},
	}
	if err := ValidateParams(CmdCreate, ResourceDeploymentPipeline, params); err != nil {
		t.Errorf("ValidateParams deployment pipeline create valid: %v", err)
	}
}

func TestValidateParams_DeploymentPipelineCreate_Missing(t *testing.T) {
	params := api.CreateDeploymentPipelineParams{Namespace: "default"}
	if err := ValidateParams(CmdCreate, ResourceDeploymentPipeline, params); err == nil {
		t.Error("ValidateParams deployment pipeline create missing: expected error")
	}
}

// ---- ConfigurationGroup params ----

func TestValidateParams_ConfigurationGroupGet_Valid(t *testing.T) {
	params := api.GetConfigurationGroupParams{Namespace: "default"}
	if err := ValidateParams(CmdGet, ResourceConfigurationGroup, params); err != nil {
		t.Errorf("ValidateParams configuration group get valid: %v", err)
	}
}

func TestValidateParams_ConfigurationGroupGet_Missing(t *testing.T) {
	params := api.GetConfigurationGroupParams{Namespace: ""}
	if err := ValidateParams(CmdGet, ResourceConfigurationGroup, params); err == nil {
		t.Error("ValidateParams configuration group get missing: expected error")
	}
}

// ---- Workload params ----

func TestValidateParams_WorkloadCreate_Valid(t *testing.T) {
	params := api.CreateWorkloadParams{
		NamespaceName: "default",
		ProjectName:   "my-project",
		ComponentName: "my-component",
		ImageURL:      "gcr.io/my-image:latest",
	}
	if err := ValidateParams(CmdCreate, ResourceWorkload, params); err != nil {
		t.Errorf("ValidateParams workload create valid: %v", err)
	}
}

func TestValidateParams_WorkloadCreate_Missing(t *testing.T) {
	params := api.CreateWorkloadParams{NamespaceName: "default"}
	if err := ValidateParams(CmdCreate, ResourceWorkload, params); err == nil {
		t.Error("ValidateParams workload create missing fields: expected error")
	}
}

// ---- List params for namespace-only resources ----

func TestValidateParams_BuildPlaneList_Valid(t *testing.T) {
	params := api.ListBuildPlanesParams{Namespace: "default"}
	if err := ValidateParams(CmdList, ResourceBuildPlane, params); err != nil {
		t.Errorf("ValidateParams build plane list valid: %v", err)
	}
}

func TestValidateParams_BuildPlaneList_Missing(t *testing.T) {
	params := api.ListBuildPlanesParams{Namespace: ""}
	if err := ValidateParams(CmdList, ResourceBuildPlane, params); err == nil {
		t.Error("ValidateParams build plane list missing: expected error")
	}
}

func TestValidateParams_ObservabilityPlaneList_Valid(t *testing.T) {
	params := api.ListObservabilityPlanesParams{Namespace: "default"}
	if err := ValidateParams(CmdList, ResourceObservabilityPlane, params); err != nil {
		t.Errorf("ValidateParams observability plane list valid: %v", err)
	}
}

func TestValidateParams_ComponentTypeList_Valid(t *testing.T) {
	params := api.ListComponentTypesParams{Namespace: "default"}
	if err := ValidateParams(CmdList, ResourceComponentType, params); err != nil {
		t.Errorf("ValidateParams component type list valid: %v", err)
	}
}

func TestValidateParams_TraitList_Valid(t *testing.T) {
	params := api.ListTraitsParams{Namespace: "default"}
	if err := ValidateParams(CmdList, ResourceTrait, params); err != nil {
		t.Errorf("ValidateParams trait list valid: %v", err)
	}
}

func TestValidateParams_WorkflowList_Valid(t *testing.T) {
	params := api.ListWorkflowsParams{Namespace: "default"}
	if err := ValidateParams(CmdList, ResourceWorkflow, params); err != nil {
		t.Errorf("ValidateParams workflow list valid: %v", err)
	}
}

func TestValidateParams_WorkflowList_Missing(t *testing.T) {
	params := api.ListWorkflowsParams{Namespace: ""}
	if err := ValidateParams(CmdList, ResourceWorkflow, params); err == nil {
		t.Error("ValidateParams workflow list missing namespace: expected error")
	}
}

func TestValidateParams_ComponentWorkflowList_Valid(t *testing.T) {
	params := api.ListComponentWorkflowsParams{Namespace: "default"}
	if err := ValidateParams(CmdList, ResourceComponentWorkflow, params); err != nil {
		t.Errorf("ValidateParams component workflow list valid: %v", err)
	}
}

func TestValidateParams_SecretReferenceList_Valid(t *testing.T) {
	params := api.ListSecretReferencesParams{Namespace: "default"}
	if err := ValidateParams(CmdList, ResourceSecretReference, params); err != nil {
		t.Errorf("ValidateParams secret reference list valid: %v", err)
	}
}

func TestValidateParams_ComponentReleaseList_Valid(t *testing.T) {
	params := api.ListComponentReleasesParams{
		Namespace: "default",
		Project:   "my-project",
		Component: "my-component",
	}
	if err := ValidateParams(CmdList, ResourceComponentRelease, params); err != nil {
		t.Errorf("ValidateParams component release list valid: %v", err)
	}
}

func TestValidateParams_ComponentReleaseList_Missing(t *testing.T) {
	params := api.ListComponentReleasesParams{Namespace: "default"}
	if err := ValidateParams(CmdList, ResourceComponentRelease, params); err == nil {
		t.Error("ValidateParams component release list missing: expected error")
	}
}

func TestValidateParams_ReleaseBindingList_Valid(t *testing.T) {
	params := api.ListReleaseBindingsParams{
		Namespace: "default",
		Project:   "my-project",
		Component: "my-component",
	}
	if err := ValidateParams(CmdList, ResourceReleaseBinding, params); err != nil {
		t.Errorf("ValidateParams release binding list valid: %v", err)
	}
}

func TestValidateParams_WorkflowRunList_Valid(t *testing.T) {
	params := api.ListWorkflowRunsParams{Namespace: "default"}
	if err := ValidateParams(CmdList, ResourceWorkflowRun, params); err != nil {
		t.Errorf("ValidateParams workflow run list valid: %v", err)
	}
}

func TestValidateParams_ComponentWorkflowRunList_Valid(t *testing.T) {
	params := api.ListComponentWorkflowRunsParams{
		Namespace: "default",
		Project:   "my-project",
		Component: "my-component",
	}
	if err := ValidateParams(CmdList, ResourceComponentWorkflowRun, params); err != nil {
		t.Errorf("ValidateParams component workflow run list valid: %v", err)
	}
}

func TestValidateParams_ComponentWorkflowRunList_Missing(t *testing.T) {
	params := api.ListComponentWorkflowRunsParams{Namespace: "default"}
	if err := ValidateParams(CmdList, ResourceComponentWorkflowRun, params); err == nil {
		t.Error("ValidateParams component workflow run list missing: expected error")
	}
}

// ---- Component list/deploy params ----

func TestValidateParams_ComponentList_Valid(t *testing.T) {
	params := api.ListComponentsParams{
		Namespace: "default",
		Project:   "my-project",
	}
	if err := ValidateParams(CmdList, ResourceComponent, params); err != nil {
		t.Errorf("ValidateParams component list valid: %v", err)
	}
}

func TestValidateParams_ComponentList_Missing(t *testing.T) {
	params := api.ListComponentsParams{Namespace: "default"}
	if err := ValidateParams(CmdList, ResourceComponent, params); err == nil {
		t.Error("ValidateParams component list missing project: expected error")
	}
}

func TestValidateParams_ComponentDeploy_Valid(t *testing.T) {
	params := api.DeployComponentParams{
		Namespace:     "default",
		Project:       "my-project",
		ComponentName: "my-component",
	}
	if err := ValidateParams(CmdDeploy, ResourceComponent, params); err != nil {
		t.Errorf("ValidateParams component deploy valid: %v", err)
	}
}

func TestValidateParams_ComponentDeploy_MissingComponentName(t *testing.T) {
	params := api.DeployComponentParams{
		Namespace: "default",
		Project:   "my-project",
	}
	if err := ValidateParams(CmdDeploy, ResourceComponent, params); err == nil {
		t.Error("ValidateParams component deploy missing component name: expected error")
	}
}

// ---- Project list params ----

func TestValidateParams_ProjectList_WithNamespace(t *testing.T) {
	params := api.ListProjectsParams{Namespace: "default"}
	if err := ValidateParams(CmdList, ResourceProject, params); err != nil {
		t.Errorf("ValidateParams project list with namespace: %v", err)
	}
}

func TestValidateParams_ProjectList_MissingNamespace(t *testing.T) {
	params := api.ListProjectsParams{Namespace: ""}
	if err := ValidateParams(CmdList, ResourceProject, params); err == nil {
		t.Error("ValidateParams project list missing namespace: expected error")
	}
}

// ---- ValidateAddContextParams ----

func TestValidateAddContextParams_Valid(t *testing.T) {
	params := api.AddContextParams{
		ControlPlane: "my-cp",
		Credentials:  "my-creds",
	}
	if err := ValidateAddContextParams(params); err != nil {
		t.Errorf("ValidateAddContextParams valid: %v", err)
	}
}

func TestValidateAddContextParams_MissingControlPlane(t *testing.T) {
	params := api.AddContextParams{Credentials: "my-creds"}
	if err := ValidateAddContextParams(params); err == nil {
		t.Error("ValidateAddContextParams missing control plane: expected error")
	}
}

func TestValidateAddContextParams_MissingCredentials(t *testing.T) {
	params := api.AddContextParams{ControlPlane: "my-cp"}
	if err := ValidateAddContextParams(params); err == nil {
		t.Error("ValidateAddContextParams missing credentials: expected error")
	}
}

// ---- ValidateContextNameUniqueness ----

func TestValidateContextNameUniqueness_Unique(t *testing.T) {
	cfg := &configContext.StoredConfig{
		Contexts: []configContext.Context{
			{Name: "ctx-a"},
			{Name: "ctx-b"},
		},
	}
	if err := ValidateContextNameUniqueness(cfg, "ctx-c"); err != nil {
		t.Errorf("ValidateContextNameUniqueness unique: %v", err)
	}
}

func TestValidateContextNameUniqueness_Duplicate(t *testing.T) {
	cfg := &configContext.StoredConfig{
		Contexts: []configContext.Context{
			{Name: "ctx-a"},
		},
	}
	if err := ValidateContextNameUniqueness(cfg, "ctx-a"); err == nil {
		t.Error("ValidateContextNameUniqueness duplicate: expected error")
	}
}

// ---- ValidateControlPlaneNameUniqueness ----

func TestValidateControlPlaneNameUniqueness_Unique(t *testing.T) {
	cfg := &configContext.StoredConfig{
		ControlPlanes: []configContext.ControlPlane{
			{Name: "cp-a"},
		},
	}
	if err := ValidateControlPlaneNameUniqueness(cfg, "cp-b"); err != nil {
		t.Errorf("ValidateControlPlaneNameUniqueness unique: %v", err)
	}
}

func TestValidateControlPlaneNameUniqueness_Duplicate(t *testing.T) {
	cfg := &configContext.StoredConfig{
		ControlPlanes: []configContext.ControlPlane{
			{Name: "cp-a"},
		},
	}
	if err := ValidateControlPlaneNameUniqueness(cfg, "cp-a"); err == nil {
		t.Error("ValidateControlPlaneNameUniqueness duplicate: expected error")
	}
}

// ---- ValidateCredentialsNameUniqueness ----

func TestValidateCredentialsNameUniqueness_Unique(t *testing.T) {
	cfg := &configContext.StoredConfig{
		Credentials: []configContext.Credential{
			{Name: "cred-a"},
		},
	}
	if err := ValidateCredentialsNameUniqueness(cfg, "cred-b"); err != nil {
		t.Errorf("ValidateCredentialsNameUniqueness unique: %v", err)
	}
}

func TestValidateCredentialsNameUniqueness_Duplicate(t *testing.T) {
	cfg := &configContext.StoredConfig{
		Credentials: []configContext.Credential{
			{Name: "cred-a"},
		},
	}
	if err := ValidateCredentialsNameUniqueness(cfg, "cred-a"); err == nil {
		t.Error("ValidateCredentialsNameUniqueness duplicate: expected error")
	}
}
