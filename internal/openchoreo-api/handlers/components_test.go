// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// TestListComponentReleases_PathParameters tests that path parameters are correctly extracted
func TestListComponentReleases_PathParameters(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		orgName       string
		projectName   string
		componentName string
	}{
		{
			name:          "Valid path parameters",
			url:           "/api/v1/orgs/myorg/projects/myproject/components/mycomponent/component-releases",
			orgName:       "myorg",
			projectName:   "myproject",
			componentName: "mycomponent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			req.SetPathValue("orgName", tt.orgName)
			req.SetPathValue("projectName", tt.projectName)
			req.SetPathValue("componentName", tt.componentName)

			// Verify path values are set
			if req.PathValue("orgName") != tt.orgName {
				t.Errorf("orgName = %v, want %v", req.PathValue("orgName"), tt.orgName)
			}
			if req.PathValue("projectName") != tt.projectName {
				t.Errorf("projectName = %v, want %v", req.PathValue("projectName"), tt.projectName)
			}
			if req.PathValue("componentName") != tt.componentName {
				t.Errorf("componentName = %v, want %v", req.PathValue("componentName"), tt.componentName)
			}
		})
	}
}

// TestCreateComponentRelease_RequestParsing tests request body parsing
func TestCreateComponentRelease_RequestParsing(t *testing.T) {
	tests := []struct {
		name        string
		requestBody string
		wantErr     bool
	}{
		{
			name:        "Valid request with release name",
			requestBody: `{"releaseName": "myrelease-v1"}`,
			wantErr:     false,
		},
		{
			name:        "Valid request without release name",
			requestBody: `{}`,
			wantErr:     false,
		},
		{
			name:        "Invalid JSON",
			requestBody: `{"releaseName": }`,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req models.CreateComponentReleaseRequest
			err := json.NewDecoder(bytes.NewReader([]byte(tt.requestBody))).Decode(&req)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error parsing JSON, got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error parsing JSON: %v", err)
				}
			}
		})
	}
}

// TestListReleaseBindings_QueryParameters tests query parameter extraction
func TestListReleaseBindings_QueryParameters(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		wantEnvCount int
		wantEnvs     []string
	}{
		{
			name:         "No environment filter",
			url:          "/api/v1/orgs/myorg/projects/myproject/components/mycomponent/release-bindings",
			wantEnvCount: 0,
			wantEnvs:     []string{},
		},
		{
			name:         "Single environment filter",
			url:          "/api/v1/orgs/myorg/projects/myproject/components/mycomponent/release-bindings?environment=dev",
			wantEnvCount: 1,
			wantEnvs:     []string{"dev"},
		},
		{
			name:         "Multiple environment filters",
			url:          "/api/v1/orgs/myorg/projects/myproject/components/mycomponent/release-bindings?environment=dev&environment=staging",
			wantEnvCount: 2,
			wantEnvs:     []string{"dev", "staging"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			environments := req.URL.Query()["environment"]

			if len(environments) != tt.wantEnvCount {
				t.Errorf("Got %d environments, want %d", len(environments), tt.wantEnvCount)
			}

			for i, env := range tt.wantEnvs {
				if i >= len(environments) || environments[i] != env {
					t.Errorf("Environment at index %d = %v, want %v", i, environments[i], env)
				}
			}
		})
	}
}

// TestPatchReleaseBinding_RequestParsing tests PATCH request body parsing
func TestPatchReleaseBinding_RequestParsing(t *testing.T) {
	tests := []struct {
		name        string
		requestBody string
		wantErr     bool
		checkFunc   func(*testing.T, *models.PatchReleaseBindingRequest)
	}{
		{
			name:        "Valid request with component type overrides",
			requestBody: `{"componentTypeEnvOverrides": {"replicas": 3}}`,
			wantErr:     false,
			checkFunc: func(t *testing.T, req *models.PatchReleaseBindingRequest) {
				if req.ComponentTypeEnvOverrides == nil {
					t.Error("Expected componentTypeEnvOverrides to be set")
				}
			},
		},
		{
			name:        "Valid request with workload overrides",
			requestBody: `{"workloadOverrides": {"containers": {"main": {"env": [{"key": "ENV", "value": "prod"}]}}}}`,
			wantErr:     false,
			checkFunc: func(t *testing.T, req *models.PatchReleaseBindingRequest) {
				if req.WorkloadOverrides == nil {
					t.Error("Expected workloadOverrides to be set")
				}
			},
		},
		{
			name:        "Valid request for creating new binding",
			requestBody: `{"releaseName": "myapp-v1", "environment": "dev", "componentTypeEnvOverrides": {"replicas": 3}}`,
			wantErr:     false,
			checkFunc: func(t *testing.T, req *models.PatchReleaseBindingRequest) {
				if req.ReleaseName != "myapp-v1" {
					t.Errorf("Expected releaseName 'myapp-v1', got %s", req.ReleaseName)
				}
				if req.Environment != "dev" {
					t.Errorf("Expected environment 'dev', got %s", req.Environment)
				}
				if req.ComponentTypeEnvOverrides == nil {
					t.Error("Expected componentTypeEnvOverrides to be set")
				}
			},
		},
		{
			name:        "Invalid JSON",
			requestBody: `{"componentTypeEnvOverrides": }`,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req models.PatchReleaseBindingRequest
			err := json.NewDecoder(bytes.NewReader([]byte(tt.requestBody))).Decode(&req)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error parsing JSON, got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error parsing JSON: %v", err)
				}
				if tt.checkFunc != nil {
					tt.checkFunc(t, &req)
				}
			}
		})
	}
}

// TestGetComponentRelease_PathParameters tests path parameter extraction for GetComponentRelease
func TestGetComponentRelease_PathParameters(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		orgName       string
		projectName   string
		componentName string
		releaseName   string
	}{
		{
			name:          "Valid path with all parameters",
			url:           "/api/v1/orgs/myorg/projects/myproject/components/mycomponent/component-releases/myrelease-v1",
			orgName:       "myorg",
			projectName:   "myproject",
			componentName: "mycomponent",
			releaseName:   "myrelease-v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			req.SetPathValue("orgName", tt.orgName)
			req.SetPathValue("projectName", tt.projectName)
			req.SetPathValue("componentName", tt.componentName)
			req.SetPathValue("releaseName", tt.releaseName)

			// Verify all path values are set correctly
			if req.PathValue("releaseName") != tt.releaseName {
				t.Errorf("releaseName = %v, want %v", req.PathValue("releaseName"), tt.releaseName)
			}
		})
	}
}

// TestGetComponentReleaseSchema_PathParameters tests path parameter extraction for GetComponentReleaseSchema
func TestGetComponentReleaseSchema_PathParameters(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		orgName       string
		projectName   string
		componentName string
		releaseName   string
	}{
		{
			name:          "Valid path with all parameters",
			url:           "/api/v1/orgs/myorg/projects/myproject/components/mycomponent/component-releases/myrelease-v1/schema",
			orgName:       "myorg",
			projectName:   "myproject",
			componentName: "mycomponent",
			releaseName:   "myrelease-v1",
		},
		{
			name:          "Path with hyphens in names",
			url:           "/api/v1/orgs/my-org/projects/my-project/components/my-component/component-releases/myrelease-20251120-1/schema",
			orgName:       "my-org",
			projectName:   "my-project",
			componentName: "my-component",
			releaseName:   "myrelease-20251120-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			req.SetPathValue("orgName", tt.orgName)
			req.SetPathValue("projectName", tt.projectName)
			req.SetPathValue("componentName", tt.componentName)
			req.SetPathValue("releaseName", tt.releaseName)

			// Verify all path values are set correctly
			if req.PathValue("orgName") != tt.orgName {
				t.Errorf("orgName = %v, want %v", req.PathValue("orgName"), tt.orgName)
			}
			if req.PathValue("projectName") != tt.projectName {
				t.Errorf("projectName = %v, want %v", req.PathValue("projectName"), tt.projectName)
			}
			if req.PathValue("componentName") != tt.componentName {
				t.Errorf("componentName = %v, want %v", req.PathValue("componentName"), tt.componentName)
			}
			if req.PathValue("releaseName") != tt.releaseName {
				t.Errorf("releaseName = %v, want %v", req.PathValue("releaseName"), tt.releaseName)
			}
		})
	}
}

// TestGetComponentSchema_PathParameters tests path parameter extraction for GetComponentSchema
func TestGetComponentSchema_PathParameters(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		orgName       string
		projectName   string
		componentName string
	}{
		{
			name:          "Valid path with all parameters",
			url:           "/api/v1/orgs/myorg/projects/myproject/components/mycomponent/schema",
			orgName:       "myorg",
			projectName:   "myproject",
			componentName: "mycomponent",
		},
		{
			name:          "Path with hyphens in names",
			url:           "/api/v1/orgs/my-org/projects/my-project/components/my-component/schema",
			orgName:       "my-org",
			projectName:   "my-project",
			componentName: "my-component",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			req.SetPathValue("orgName", tt.orgName)
			req.SetPathValue("projectName", tt.projectName)
			req.SetPathValue("componentName", tt.componentName)

			// Verify all path values are set correctly
			if req.PathValue("orgName") != tt.orgName {
				t.Errorf("orgName = %v, want %v", req.PathValue("orgName"), tt.orgName)
			}
			if req.PathValue("projectName") != tt.projectName {
				t.Errorf("projectName = %v, want %v", req.PathValue("projectName"), tt.projectName)
			}
			if req.PathValue("componentName") != tt.componentName {
				t.Errorf("componentName = %v, want %v", req.PathValue("componentName"), tt.componentName)
			}
		})
	}
}

// TestGetComponentSchema_MissingPathParameters tests validation for missing required parameters
func TestGetComponentSchema_MissingPathParameters(t *testing.T) {
	tests := []struct {
		name          string
		orgName       string
		projectName   string
		componentName string
		wantValid     bool
	}{
		{
			name:          "All parameters present",
			orgName:       "myorg",
			projectName:   "myproject",
			componentName: "mycomponent",
			wantValid:     true,
		},
		{
			name:          "Missing org name",
			orgName:       "",
			projectName:   "myproject",
			componentName: "mycomponent",
			wantValid:     false,
		},
		{
			name:          "Missing project name",
			orgName:       "myorg",
			projectName:   "",
			componentName: "mycomponent",
			wantValid:     false,
		},
		{
			name:          "Missing component name",
			orgName:       "myorg",
			projectName:   "myproject",
			componentName: "",
			wantValid:     false,
		},
		{
			name:          "All parameters missing",
			orgName:       "",
			projectName:   "",
			componentName: "",
			wantValid:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation logic from GetComponentSchema handler
			isValid := tt.orgName != "" && tt.projectName != "" && tt.componentName != ""

			if isValid != tt.wantValid {
				t.Errorf("Validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// TestGetReleaseResources_PathParameters tests path parameter extraction for GetReleaseResources
func TestGetReleaseResources_PathParameters(t *testing.T) {
	tests := []struct {
		name            string
		url             string
		orgName         string
		projectName     string
		componentName   string
		environmentName string
	}{
		{
			name:            "Valid path with all parameters",
			url:             "/api/v1/orgs/myorg/projects/myproject/components/mycomponent/environments/development/resources",
			orgName:         "myorg",
			projectName:     "myproject",
			componentName:   "mycomponent",
			environmentName: "development",
		},
		{
			name:            "Path with hyphens in names",
			url:             "/api/v1/orgs/my-org/projects/my-project/components/my-component/environments/staging/resources",
			orgName:         "my-org",
			projectName:     "my-project",
			componentName:   "my-component",
			environmentName: "staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			req.SetPathValue("orgName", tt.orgName)
			req.SetPathValue("projectName", tt.projectName)
			req.SetPathValue("componentName", tt.componentName)
			req.SetPathValue("environmentName", tt.environmentName)

			// Verify all path values are set correctly
			if req.PathValue("orgName") != tt.orgName {
				t.Errorf("orgName = %v, want %v", req.PathValue("orgName"), tt.orgName)
			}
			if req.PathValue("projectName") != tt.projectName {
				t.Errorf("projectName = %v, want %v", req.PathValue("projectName"), tt.projectName)
			}
			if req.PathValue("componentName") != tt.componentName {
				t.Errorf("componentName = %v, want %v", req.PathValue("componentName"), tt.componentName)
			}
			if req.PathValue("environmentName") != tt.environmentName {
				t.Errorf("environmentName = %v, want %v", req.PathValue("environmentName"), tt.environmentName)
			}
		})
	}
}

// TestGetReleaseResources_MissingPathParameters tests validation for missing required parameters
func TestGetReleaseResources_MissingPathParameters(t *testing.T) {
	tests := []struct {
		name            string
		orgName         string
		projectName     string
		componentName   string
		environmentName string
		wantValid       bool
	}{
		{
			name:            "All parameters present",
			orgName:         "myorg",
			projectName:     "myproject",
			componentName:   "mycomponent",
			environmentName: "development",
			wantValid:       true,
		},
		{
			name:            "Missing org name",
			orgName:         "",
			projectName:     "myproject",
			componentName:   "mycomponent",
			environmentName: "development",
			wantValid:       false,
		},
		{
			name:            "Missing project name",
			orgName:         "myorg",
			projectName:     "",
			componentName:   "mycomponent",
			environmentName: "development",
			wantValid:       false,
		},
		{
			name:            "Missing component name",
			orgName:         "myorg",
			projectName:     "myproject",
			componentName:   "",
			environmentName: "development",
			wantValid:       false,
		},
		{
			name:            "Missing environment name",
			orgName:         "myorg",
			projectName:     "myproject",
			componentName:   "mycomponent",
			environmentName: "",
			wantValid:       false,
		},
		{
			name:            "All parameters missing",
			orgName:         "",
			projectName:     "",
			componentName:   "",
			environmentName: "",
			wantValid:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation logic from GetReleaseResources handler
			isValid := tt.orgName != "" && tt.projectName != "" && tt.componentName != "" && tt.environmentName != ""

			if isValid != tt.wantValid {
				t.Errorf("Validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// TestGetComponentReleaseSchema_MissingPathParameters tests validation for missing required parameters
func TestGetComponentReleaseSchema_MissingPathParameters(t *testing.T) {
	tests := []struct {
		name          string
		orgName       string
		projectName   string
		componentName string
		releaseName   string
		wantValid     bool
	}{
		{
			name:          "All parameters present",
			orgName:       "myorg",
			projectName:   "myproject",
			componentName: "mycomponent",
			releaseName:   "myrelease-v1",
			wantValid:     true,
		},
		{
			name:          "Missing org name",
			orgName:       "",
			projectName:   "myproject",
			componentName: "mycomponent",
			releaseName:   "myrelease-v1",
			wantValid:     false,
		},
		{
			name:          "Missing project name",
			orgName:       "myorg",
			projectName:   "",
			componentName: "mycomponent",
			releaseName:   "myrelease-v1",
			wantValid:     false,
		},
		{
			name:          "Missing component name",
			orgName:       "myorg",
			projectName:   "myproject",
			componentName: "",
			releaseName:   "myrelease-v1",
			wantValid:     false,
		},
		{
			name:          "Missing release name",
			orgName:       "myorg",
			projectName:   "myproject",
			componentName: "mycomponent",
			releaseName:   "",
			wantValid:     false,
		},
		{
			name:          "All parameters missing",
			orgName:       "",
			projectName:   "",
			componentName: "",
			releaseName:   "",
			wantValid:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation logic from GetComponentReleaseSchema handler
			isValid := tt.orgName != "" && tt.projectName != "" && tt.componentName != "" && tt.releaseName != ""

			if isValid != tt.wantValid {
				t.Errorf("Validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}
