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
			name:        "Valid request with configuration overrides",
			requestBody: `{"configurationOverrides": {"env": [{"key": "ENV", "value": "prod"}]}}`,
			wantErr:     false,
			checkFunc: func(t *testing.T, req *models.PatchReleaseBindingRequest) {
				if req.ConfigurationOverrides == nil {
					t.Error("Expected configurationOverrides to be set")
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
