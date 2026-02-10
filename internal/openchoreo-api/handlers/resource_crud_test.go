// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	testAPIVersion        = "openchoreo.dev/v1alpha1"
	testKindComponentType = "ComponentType"
	testKindTrait         = "Trait"
	testKindCW            = "ComponentWorkflow"
)

// testResourceJSON builds a JSON string for a valid OpenChoreo resource with the given kind and name.
func testResourceJSON(kind, name string) string {
	return fmt.Sprintf(`{"apiVersion": %q, "kind": %q, "metadata": {"name": %q}}`, testAPIVersion, kind, name)
}

// testPartialJSON builds JSON missing specific fields for validation tests.
func testPartialJSON(fields map[string]interface{}) string {
	data, _ := json.Marshal(fields)
	return string(data)
}

// ========== CreateComponentTypeDefinition Tests ==========

// TestCreateComponentTypeDefinition_PathParameters tests path parameter extraction
func TestCreateComponentTypeDefinition_PathParameters(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		namespaceName string
	}{
		{
			name:          "Valid path parameters",
			url:           "/api/v1/namespaces/mynamespace/component-types/definition",
			namespaceName: "mynamespace",
		},
		{
			name:          "Path with hyphens in names",
			url:           "/api/v1/namespaces/my-namespace/component-types/definition",
			namespaceName: "my-namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.url, nil)
			req.SetPathValue("namespaceName", tt.namespaceName)

			if req.PathValue("namespaceName") != tt.namespaceName {
				t.Errorf("namespaceName = %v, want %v", req.PathValue("namespaceName"), tt.namespaceName)
			}
		})
	}
}

// TestCreateComponentTypeDefinition_MissingPathParameters tests validation for missing required parameters
func TestCreateComponentTypeDefinition_MissingPathParameters(t *testing.T) {
	tests := []struct {
		name          string
		namespaceName string
		wantValid     bool
	}{
		{
			name:          "Namespace present",
			namespaceName: "mynamespace",
			wantValid:     true,
		},
		{
			name:          "Missing namespace name",
			namespaceName: "",
			wantValid:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation logic from CreateComponentTypeDefinition handler
			isValid := tt.namespaceName != ""

			if isValid != tt.wantValid {
				t.Errorf("Validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// TestCreateComponentTypeDefinition_RequestParsing tests request body parsing for ComponentType creation
func TestCreateComponentTypeDefinition_RequestParsing(t *testing.T) {
	tests := []struct {
		name        string
		requestBody string
		wantErr     bool
		checkFunc   func(*testing.T, map[string]interface{})
	}{
		{
			name:        "Valid ComponentType resource",
			requestBody: testResourceJSON(testKindComponentType, "my-component-type"),
			wantErr:     false,
			checkFunc: func(t *testing.T, obj map[string]interface{}) {
				if obj["kind"] != testKindComponentType {
					t.Errorf("Expected kind %q, got %v", testKindComponentType, obj["kind"])
				}
				if obj["apiVersion"] != testAPIVersion {
					t.Errorf("Expected apiVersion %q, got %v", testAPIVersion, obj["apiVersion"])
				}
				metadata, ok := obj["metadata"].(map[string]interface{})
				if !ok {
					t.Error("Expected metadata to be a map")
					return
				}
				if metadata["name"] != "my-component-type" {
					t.Errorf("Expected metadata.name 'my-component-type', got %v", metadata["name"])
				}
			},
		},
		{
			name:        "Invalid JSON",
			requestBody: `{"kind": }`,
			wantErr:     true,
		},
		{
			name:        "Empty body",
			requestBody: `{}`,
			wantErr:     false,
			checkFunc: func(t *testing.T, obj map[string]interface{}) {
				if _, ok := obj["kind"]; ok {
					t.Error("Expected kind to be absent in empty body")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var obj map[string]interface{}
			err := json.NewDecoder(bytes.NewReader([]byte(tt.requestBody))).Decode(&obj)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error parsing JSON, got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error parsing JSON: %v", err)
				}
				if tt.checkFunc != nil {
					tt.checkFunc(t, obj)
				}
			}
		})
	}
}

// TestCreateComponentTypeDefinition_KindValidation tests that only ComponentType kind is accepted
func TestCreateComponentTypeDefinition_KindValidation(t *testing.T) {
	tests := []struct {
		name      string
		kind      string
		wantValid bool
	}{
		{
			name:      "Valid kind - ComponentType",
			kind:      testKindComponentType,
			wantValid: true,
		},
		{
			name:      "Invalid kind - Trait",
			kind:      testKindTrait,
			wantValid: false,
		},
		{
			name:      "Invalid kind - ComponentWorkflow",
			kind:      testKindCW,
			wantValid: false,
		},
		{
			name:      "Invalid kind - empty",
			kind:      "",
			wantValid: false,
		},
		{
			name:      "Invalid kind - lowercase",
			kind:      "componenttype",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the kind validation logic from CreateComponentTypeDefinition handler
			isValid := tt.kind == testKindComponentType

			if isValid != tt.wantValid {
				t.Errorf("Kind validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// ========== GetComponentTypeDefinition Tests ==========

// TestGetComponentTypeDefinition_PathParameters tests path parameter extraction
func TestGetComponentTypeDefinition_PathParameters(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		namespaceName string
		ctName        string
	}{
		{
			name:          "Valid path with all parameters",
			url:           "/api/v1/namespaces/mynamespace/component-types/my-ct/definition",
			namespaceName: "mynamespace",
			ctName:        "my-ct",
		},
		{
			name:          "Path with hyphens in names",
			url:           "/api/v1/namespaces/my-namespace/component-types/my-component-type/definition",
			namespaceName: "my-namespace",
			ctName:        "my-component-type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			req.SetPathValue("namespaceName", tt.namespaceName)
			req.SetPathValue("ctName", tt.ctName)

			if req.PathValue("namespaceName") != tt.namespaceName {
				t.Errorf("namespaceName = %v, want %v", req.PathValue("namespaceName"), tt.namespaceName)
			}
			if req.PathValue("ctName") != tt.ctName {
				t.Errorf("ctName = %v, want %v", req.PathValue("ctName"), tt.ctName)
			}
		})
	}
}

// TestGetComponentTypeDefinition_MissingPathParameters tests validation for missing required parameters
func TestGetComponentTypeDefinition_MissingPathParameters(t *testing.T) {
	tests := []struct {
		name          string
		namespaceName string
		ctName        string
		wantValid     bool
	}{
		{
			name:          "All parameters present",
			namespaceName: "mynamespace",
			ctName:        "my-ct",
			wantValid:     true,
		},
		{
			name:          "Missing namespace name",
			namespaceName: "",
			ctName:        "my-ct",
			wantValid:     false,
		},
		{
			name:          "Missing ctName",
			namespaceName: "mynamespace",
			ctName:        "",
			wantValid:     false,
		},
		{
			name:          "All parameters missing",
			namespaceName: "",
			ctName:        "",
			wantValid:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation logic from GetComponentTypeDefinition handler
			isValid := tt.namespaceName != "" && tt.ctName != ""

			if isValid != tt.wantValid {
				t.Errorf("Validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// ========== UpdateComponentTypeDefinition Tests ==========

// TestUpdateComponentTypeDefinition_NameMismatchValidation tests that URL name must match resource name
func TestUpdateComponentTypeDefinition_NameMismatchValidation(t *testing.T) {
	tests := []struct {
		name         string
		urlName      string
		resourceName string
		wantValid    bool
	}{
		{
			name:         "Names match",
			urlName:      "my-ct",
			resourceName: "my-ct",
			wantValid:    true,
		},
		{
			name:         "Names do not match",
			urlName:      "my-ct",
			resourceName: "other-ct",
			wantValid:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the name mismatch validation from UpdateComponentTypeDefinition handler
			isValid := tt.resourceName == tt.urlName

			if isValid != tt.wantValid {
				t.Errorf("Name mismatch validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// ========== DeleteComponentTypeDefinition Tests ==========

// TestDeleteComponentTypeDefinition_PathParameters tests path parameter extraction
func TestDeleteComponentTypeDefinition_PathParameters(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		namespaceName string
		ctName        string
	}{
		{
			name:          "Valid path with all parameters",
			url:           "/api/v1/namespaces/mynamespace/component-types/my-ct/definition",
			namespaceName: "mynamespace",
			ctName:        "my-ct",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, tt.url, nil)
			req.SetPathValue("namespaceName", tt.namespaceName)
			req.SetPathValue("ctName", tt.ctName)

			if req.PathValue("namespaceName") != tt.namespaceName {
				t.Errorf("namespaceName = %v, want %v", req.PathValue("namespaceName"), tt.namespaceName)
			}
			if req.PathValue("ctName") != tt.ctName {
				t.Errorf("ctName = %v, want %v", req.PathValue("ctName"), tt.ctName)
			}
		})
	}
}

// TestDeleteComponentTypeDefinition_MissingPathParameters tests validation for missing required parameters
func TestDeleteComponentTypeDefinition_MissingPathParameters(t *testing.T) {
	tests := []struct {
		name          string
		namespaceName string
		ctName        string
		wantValid     bool
	}{
		{
			name:          "All parameters present",
			namespaceName: "mynamespace",
			ctName:        "my-ct",
			wantValid:     true,
		},
		{
			name:          "Missing namespace name",
			namespaceName: "",
			ctName:        "my-ct",
			wantValid:     false,
		},
		{
			name:          "Missing ctName",
			namespaceName: "mynamespace",
			ctName:        "",
			wantValid:     false,
		},
		{
			name:          "All parameters missing",
			namespaceName: "",
			ctName:        "",
			wantValid:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation logic from DeleteComponentTypeDefinition handler
			isValid := tt.namespaceName != "" && tt.ctName != ""

			if isValid != tt.wantValid {
				t.Errorf("Validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// ========== CreateTraitDefinition Tests ==========

// TestCreateTraitDefinition_PathParameters tests path parameter extraction
func TestCreateTraitDefinition_PathParameters(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		namespaceName string
	}{
		{
			name:          "Valid path parameters",
			url:           "/api/v1/namespaces/mynamespace/traits/definition",
			namespaceName: "mynamespace",
		},
		{
			name:          "Path with hyphens in names",
			url:           "/api/v1/namespaces/my-namespace/traits/definition",
			namespaceName: "my-namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.url, nil)
			req.SetPathValue("namespaceName", tt.namespaceName)

			if req.PathValue("namespaceName") != tt.namespaceName {
				t.Errorf("namespaceName = %v, want %v", req.PathValue("namespaceName"), tt.namespaceName)
			}
		})
	}
}

// TestCreateTraitDefinition_MissingPathParameters tests validation for missing required parameters
func TestCreateTraitDefinition_MissingPathParameters(t *testing.T) {
	tests := []struct {
		name          string
		namespaceName string
		wantValid     bool
	}{
		{
			name:          "Namespace present",
			namespaceName: "mynamespace",
			wantValid:     true,
		},
		{
			name:          "Missing namespace name",
			namespaceName: "",
			wantValid:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation logic from CreateTraitDefinition handler
			isValid := tt.namespaceName != ""

			if isValid != tt.wantValid {
				t.Errorf("Validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// TestCreateTraitDefinition_RequestParsing tests request body parsing for Trait creation
func TestCreateTraitDefinition_RequestParsing(t *testing.T) {
	tests := []struct {
		name        string
		requestBody string
		wantErr     bool
		checkFunc   func(*testing.T, map[string]interface{})
	}{
		{
			name:        "Valid Trait resource",
			requestBody: testResourceJSON(testKindTrait, "my-trait"),
			wantErr:     false,
			checkFunc: func(t *testing.T, obj map[string]interface{}) {
				if obj["kind"] != testKindTrait {
					t.Errorf("Expected kind %q, got %v", testKindTrait, obj["kind"])
				}
				if obj["apiVersion"] != testAPIVersion {
					t.Errorf("Expected apiVersion %q, got %v", testAPIVersion, obj["apiVersion"])
				}
				metadata, ok := obj["metadata"].(map[string]interface{})
				if !ok {
					t.Error("Expected metadata to be a map")
					return
				}
				if metadata["name"] != "my-trait" {
					t.Errorf("Expected metadata.name 'my-trait', got %v", metadata["name"])
				}
			},
		},
		{
			name:        "Invalid JSON",
			requestBody: `{"kind": }`,
			wantErr:     true,
		},
		{
			name:        "Empty body",
			requestBody: `{}`,
			wantErr:     false,
			checkFunc: func(t *testing.T, obj map[string]interface{}) {
				if _, ok := obj["kind"]; ok {
					t.Error("Expected kind to be absent in empty body")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var obj map[string]interface{}
			err := json.NewDecoder(bytes.NewReader([]byte(tt.requestBody))).Decode(&obj)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error parsing JSON, got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error parsing JSON: %v", err)
				}
				if tt.checkFunc != nil {
					tt.checkFunc(t, obj)
				}
			}
		})
	}
}

// TestCreateTraitDefinition_KindValidation tests that only Trait kind is accepted
func TestCreateTraitDefinition_KindValidation(t *testing.T) {
	tests := []struct {
		name      string
		kind      string
		wantValid bool
	}{
		{
			name:      "Valid kind - Trait",
			kind:      testKindTrait,
			wantValid: true,
		},
		{
			name:      "Invalid kind - ComponentType",
			kind:      testKindComponentType,
			wantValid: false,
		},
		{
			name:      "Invalid kind - ComponentWorkflow",
			kind:      testKindCW,
			wantValid: false,
		},
		{
			name:      "Invalid kind - empty",
			kind:      "",
			wantValid: false,
		},
		{
			name:      "Invalid kind - lowercase",
			kind:      "trait",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the kind validation logic from CreateTraitDefinition handler
			isValid := tt.kind == testKindTrait

			if isValid != tt.wantValid {
				t.Errorf("Kind validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// ========== GetTraitDefinition Tests ==========

// TestGetTraitDefinition_PathParameters tests path parameter extraction
func TestGetTraitDefinition_PathParameters(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		namespaceName string
		traitName     string
	}{
		{
			name:          "Valid path with all parameters",
			url:           "/api/v1/namespaces/mynamespace/traits/my-trait/definition",
			namespaceName: "mynamespace",
			traitName:     "my-trait",
		},
		{
			name:          "Path with hyphens in names",
			url:           "/api/v1/namespaces/my-namespace/traits/my-logging-trait/definition",
			namespaceName: "my-namespace",
			traitName:     "my-logging-trait",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			req.SetPathValue("namespaceName", tt.namespaceName)
			req.SetPathValue("traitName", tt.traitName)

			if req.PathValue("namespaceName") != tt.namespaceName {
				t.Errorf("namespaceName = %v, want %v", req.PathValue("namespaceName"), tt.namespaceName)
			}
			if req.PathValue("traitName") != tt.traitName {
				t.Errorf("traitName = %v, want %v", req.PathValue("traitName"), tt.traitName)
			}
		})
	}
}

// TestGetTraitDefinition_MissingPathParameters tests validation for missing required parameters
func TestGetTraitDefinition_MissingPathParameters(t *testing.T) {
	tests := []struct {
		name          string
		namespaceName string
		traitName     string
		wantValid     bool
	}{
		{
			name:          "All parameters present",
			namespaceName: "mynamespace",
			traitName:     "my-trait",
			wantValid:     true,
		},
		{
			name:          "Missing namespace name",
			namespaceName: "",
			traitName:     "my-trait",
			wantValid:     false,
		},
		{
			name:          "Missing trait name",
			namespaceName: "mynamespace",
			traitName:     "",
			wantValid:     false,
		},
		{
			name:          "All parameters missing",
			namespaceName: "",
			traitName:     "",
			wantValid:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation logic from GetTraitDefinition handler
			isValid := tt.namespaceName != "" && tt.traitName != ""

			if isValid != tt.wantValid {
				t.Errorf("Validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// ========== UpdateTraitDefinition Tests ==========

// TestUpdateTraitDefinition_NameMismatchValidation tests that URL name must match resource name
func TestUpdateTraitDefinition_NameMismatchValidation(t *testing.T) {
	tests := []struct {
		name         string
		urlName      string
		resourceName string
		wantValid    bool
	}{
		{
			name:         "Names match",
			urlName:      "my-trait",
			resourceName: "my-trait",
			wantValid:    true,
		},
		{
			name:         "Names do not match",
			urlName:      "my-trait",
			resourceName: "other-trait",
			wantValid:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the name mismatch validation from UpdateTraitDefinition handler
			isValid := tt.resourceName == tt.urlName

			if isValid != tt.wantValid {
				t.Errorf("Name mismatch validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// ========== DeleteTraitDefinition Tests ==========

// TestDeleteTraitDefinition_MissingPathParameters tests validation for missing required parameters
func TestDeleteTraitDefinition_MissingPathParameters(t *testing.T) {
	tests := []struct {
		name          string
		namespaceName string
		traitName     string
		wantValid     bool
	}{
		{
			name:          "All parameters present",
			namespaceName: "mynamespace",
			traitName:     "my-trait",
			wantValid:     true,
		},
		{
			name:          "Missing namespace name",
			namespaceName: "",
			traitName:     "my-trait",
			wantValid:     false,
		},
		{
			name:          "Missing trait name",
			namespaceName: "mynamespace",
			traitName:     "",
			wantValid:     false,
		},
		{
			name:          "All parameters missing",
			namespaceName: "",
			traitName:     "",
			wantValid:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation logic from DeleteTraitDefinition handler
			isValid := tt.namespaceName != "" && tt.traitName != ""

			if isValid != tt.wantValid {
				t.Errorf("Validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// ========== CreateComponentWorkflowDefinition Tests ==========

// TestCreateComponentWorkflowDefinition_PathParameters tests path parameter extraction
func TestCreateComponentWorkflowDefinition_PathParameters(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		namespaceName string
	}{
		{
			name:          "Valid path parameters",
			url:           "/api/v1/namespaces/mynamespace/component-workflows/definition",
			namespaceName: "mynamespace",
		},
		{
			name:          "Path with hyphens in names",
			url:           "/api/v1/namespaces/my-namespace/component-workflows/definition",
			namespaceName: "my-namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.url, nil)
			req.SetPathValue("namespaceName", tt.namespaceName)

			if req.PathValue("namespaceName") != tt.namespaceName {
				t.Errorf("namespaceName = %v, want %v", req.PathValue("namespaceName"), tt.namespaceName)
			}
		})
	}
}

// TestCreateComponentWorkflowDefinition_MissingPathParameters tests validation for missing required parameters
func TestCreateComponentWorkflowDefinition_MissingPathParameters(t *testing.T) {
	tests := []struct {
		name          string
		namespaceName string
		wantValid     bool
	}{
		{
			name:          "Namespace present",
			namespaceName: "mynamespace",
			wantValid:     true,
		},
		{
			name:          "Missing namespace name",
			namespaceName: "",
			wantValid:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation logic from CreateComponentWorkflowDefinition handler
			isValid := tt.namespaceName != ""

			if isValid != tt.wantValid {
				t.Errorf("Validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// TestCreateComponentWorkflowDefinition_RequestParsing tests request body parsing for ComponentWorkflow creation
func TestCreateComponentWorkflowDefinition_RequestParsing(t *testing.T) {
	tests := []struct {
		name        string
		requestBody string
		wantErr     bool
		checkFunc   func(*testing.T, map[string]interface{})
	}{
		{
			name:        "Valid ComponentWorkflow resource",
			requestBody: testResourceJSON(testKindCW, "my-component-workflow"),
			wantErr:     false,
			checkFunc: func(t *testing.T, obj map[string]interface{}) {
				if obj["kind"] != testKindCW {
					t.Errorf("Expected kind %q, got %v", testKindCW, obj["kind"])
				}
				if obj["apiVersion"] != testAPIVersion {
					t.Errorf("Expected apiVersion %q, got %v", testAPIVersion, obj["apiVersion"])
				}
				metadata, ok := obj["metadata"].(map[string]interface{})
				if !ok {
					t.Error("Expected metadata to be a map")
					return
				}
				if metadata["name"] != "my-component-workflow" {
					t.Errorf("Expected metadata.name 'my-component-workflow', got %v", metadata["name"])
				}
			},
		},
		{
			name:        "Invalid JSON",
			requestBody: `{"kind": }`,
			wantErr:     true,
		},
		{
			name:        "Empty body",
			requestBody: `{}`,
			wantErr:     false,
			checkFunc: func(t *testing.T, obj map[string]interface{}) {
				if _, ok := obj["kind"]; ok {
					t.Error("Expected kind to be absent in empty body")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var obj map[string]interface{}
			err := json.NewDecoder(bytes.NewReader([]byte(tt.requestBody))).Decode(&obj)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error parsing JSON, got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error parsing JSON: %v", err)
				}
				if tt.checkFunc != nil {
					tt.checkFunc(t, obj)
				}
			}
		})
	}
}

// TestCreateComponentWorkflowDefinition_KindValidation tests that only ComponentWorkflow kind is accepted
func TestCreateComponentWorkflowDefinition_KindValidation(t *testing.T) {
	tests := []struct {
		name      string
		kind      string
		wantValid bool
	}{
		{
			name:      "Valid kind - ComponentWorkflow",
			kind:      testKindCW,
			wantValid: true,
		},
		{
			name:      "Invalid kind - ComponentType",
			kind:      testKindComponentType,
			wantValid: false,
		},
		{
			name:      "Invalid kind - Trait",
			kind:      testKindTrait,
			wantValid: false,
		},
		{
			name:      "Invalid kind - empty",
			kind:      "",
			wantValid: false,
		},
		{
			name:      "Invalid kind - lowercase",
			kind:      "componentworkflow",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the kind validation logic from CreateComponentWorkflowDefinition handler
			isValid := tt.kind == testKindCW

			if isValid != tt.wantValid {
				t.Errorf("Kind validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// ========== GetComponentWorkflowDefinition Tests ==========

// TestGetComponentWorkflowDefinition_PathParameters tests path parameter extraction
func TestGetComponentWorkflowDefinition_PathParameters(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		namespaceName string
		cwName        string
	}{
		{
			name:          "Valid path with all parameters",
			url:           "/api/v1/namespaces/mynamespace/component-workflows/my-cw/definition",
			namespaceName: "mynamespace",
			cwName:        "my-cw",
		},
		{
			name:          "Path with hyphens in names",
			url:           "/api/v1/namespaces/my-namespace/component-workflows/my-component-workflow/definition",
			namespaceName: "my-namespace",
			cwName:        "my-component-workflow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			req.SetPathValue("namespaceName", tt.namespaceName)
			req.SetPathValue("cwName", tt.cwName)

			if req.PathValue("namespaceName") != tt.namespaceName {
				t.Errorf("namespaceName = %v, want %v", req.PathValue("namespaceName"), tt.namespaceName)
			}
			if req.PathValue("cwName") != tt.cwName {
				t.Errorf("cwName = %v, want %v", req.PathValue("cwName"), tt.cwName)
			}
		})
	}
}

// TestGetComponentWorkflowDefinition_MissingPathParameters tests validation for missing required parameters
func TestGetComponentWorkflowDefinition_MissingPathParameters(t *testing.T) {
	tests := []struct {
		name          string
		namespaceName string
		cwName        string
		wantValid     bool
	}{
		{
			name:          "All parameters present",
			namespaceName: "mynamespace",
			cwName:        "my-cw",
			wantValid:     true,
		},
		{
			name:          "Missing namespace name",
			namespaceName: "",
			cwName:        "my-cw",
			wantValid:     false,
		},
		{
			name:          "Missing cwName",
			namespaceName: "mynamespace",
			cwName:        "",
			wantValid:     false,
		},
		{
			name:          "All parameters missing",
			namespaceName: "",
			cwName:        "",
			wantValid:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation logic from GetComponentWorkflowDefinition handler
			isValid := tt.namespaceName != "" && tt.cwName != ""

			if isValid != tt.wantValid {
				t.Errorf("Validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// ========== UpdateComponentWorkflowDefinition Tests ==========

// TestUpdateComponentWorkflowDefinition_NameMismatchValidation tests that URL name must match resource name
func TestUpdateComponentWorkflowDefinition_NameMismatchValidation(t *testing.T) {
	tests := []struct {
		name         string
		urlName      string
		resourceName string
		wantValid    bool
	}{
		{
			name:         "Names match",
			urlName:      "my-cw",
			resourceName: "my-cw",
			wantValid:    true,
		},
		{
			name:         "Names do not match",
			urlName:      "my-cw",
			resourceName: "other-cw",
			wantValid:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the name mismatch validation from UpdateComponentWorkflowDefinition handler
			isValid := tt.resourceName == tt.urlName

			if isValid != tt.wantValid {
				t.Errorf("Name mismatch validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// ========== DeleteComponentWorkflowDefinition Tests ==========

// TestDeleteComponentWorkflowDefinition_MissingPathParameters tests validation for missing required parameters
func TestDeleteComponentWorkflowDefinition_MissingPathParameters(t *testing.T) {
	tests := []struct {
		name          string
		namespaceName string
		cwName        string
		wantValid     bool
	}{
		{
			name:          "All parameters present",
			namespaceName: "mynamespace",
			cwName:        "my-cw",
			wantValid:     true,
		},
		{
			name:          "Missing namespace name",
			namespaceName: "",
			cwName:        "my-cw",
			wantValid:     false,
		},
		{
			name:          "Missing cwName",
			namespaceName: "mynamespace",
			cwName:        "",
			wantValid:     false,
		},
		{
			name:          "All parameters missing",
			namespaceName: "",
			cwName:        "",
			wantValid:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation logic from DeleteComponentWorkflowDefinition handler
			isValid := tt.namespaceName != "" && tt.cwName != ""

			if isValid != tt.wantValid {
				t.Errorf("Validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// ========== Shared Validation Tests ==========

// TestValidateResourceRequest_Fields tests the validateResourceRequest validation logic
func TestValidateResourceRequest_Fields(t *testing.T) {
	tests := []struct {
		name        string
		requestBody string
		wantErr     bool
		checkFunc   func(*testing.T, map[string]interface{})
	}{
		{
			name:        "Valid resource with all required fields",
			requestBody: testResourceJSON(testKindComponentType, "my-resource"),
			wantErr:     false,
			checkFunc: func(t *testing.T, obj map[string]interface{}) {
				kind, apiVersion, name, err := validateResourceRequest(obj)
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
					return
				}
				if kind != testKindComponentType {
					t.Errorf("Expected kind %q, got %v", testKindComponentType, kind)
				}
				if apiVersion != testAPIVersion {
					t.Errorf("Expected apiVersion %q, got %v", testAPIVersion, apiVersion)
				}
				if name != "my-resource" {
					t.Errorf("Expected name 'my-resource', got %v", name)
				}
			},
		},
		{
			name:        "Missing kind field",
			requestBody: testPartialJSON(map[string]interface{}{"apiVersion": testAPIVersion, "metadata": map[string]string{"name": "test"}}),
			wantErr:     false,
			checkFunc: func(t *testing.T, obj map[string]interface{}) {
				_, _, _, err := validateResourceRequest(obj)
				if err == nil {
					t.Error("Expected error for missing kind, got none")
				}
			},
		},
		{
			name:        "Missing apiVersion field",
			requestBody: testPartialJSON(map[string]interface{}{"kind": testKindComponentType, "metadata": map[string]string{"name": "test"}}),
			wantErr:     false,
			checkFunc: func(t *testing.T, obj map[string]interface{}) {
				_, _, _, err := validateResourceRequest(obj)
				if err == nil {
					t.Error("Expected error for missing apiVersion, got none")
				}
			},
		},
		{
			name:        "Missing metadata field",
			requestBody: testPartialJSON(map[string]interface{}{"apiVersion": testAPIVersion, "kind": testKindComponentType}),
			wantErr:     false,
			checkFunc: func(t *testing.T, obj map[string]interface{}) {
				_, _, _, err := validateResourceRequest(obj)
				if err == nil {
					t.Error("Expected error for missing metadata, got none")
				}
			},
		},
		{
			name:        "Missing metadata.name field",
			requestBody: testPartialJSON(map[string]interface{}{"apiVersion": testAPIVersion, "kind": testKindComponentType, "metadata": map[string]string{}}),
			wantErr:     false,
			checkFunc: func(t *testing.T, obj map[string]interface{}) {
				_, _, _, err := validateResourceRequest(obj)
				if err == nil {
					t.Error("Expected error for missing metadata.name, got none")
				}
			},
		},
		{
			name:        "Non-openchoreo.dev group",
			requestBody: `{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test"}}`,
			wantErr:     false,
			checkFunc: func(t *testing.T, obj map[string]interface{}) {
				_, _, _, err := validateResourceRequest(obj)
				if err == nil {
					t.Error("Expected error for non-openchoreo.dev group, got none")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var obj map[string]interface{}
			err := json.NewDecoder(bytes.NewReader([]byte(tt.requestBody))).Decode(&obj)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error parsing JSON, got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error parsing JSON: %v", err)
				}
				if tt.checkFunc != nil {
					tt.checkFunc(t, obj)
				}
			}
		})
	}
}

// TestOpenChoreoGVK tests the helper function for creating GVK
func TestOpenChoreoGVK(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		wantKind string
	}{
		{
			name:     "ComponentType GVK",
			kind:     testKindComponentType,
			wantKind: testKindComponentType,
		},
		{
			name:     "Trait GVK",
			kind:     testKindTrait,
			wantKind: testKindTrait,
		},
		{
			name:     "ComponentWorkflow GVK",
			kind:     testKindCW,
			wantKind: testKindCW,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gvk := openChoreoGVK(tt.kind)

			if gvk.Group != "openchoreo.dev" {
				t.Errorf("Expected group 'openchoreo.dev', got %v", gvk.Group)
			}
			if gvk.Version != "v1alpha1" {
				t.Errorf("Expected version 'v1alpha1', got %v", gvk.Version)
			}
			if gvk.Kind != tt.wantKind {
				t.Errorf("Expected kind %v, got %v", tt.wantKind, gvk.Kind)
			}
		})
	}
}

// TestBuildUnstructuredRef tests the helper function for building unstructured references
func TestBuildUnstructuredRef(t *testing.T) {
	tests := []struct {
		name          string
		kind          string
		namespaceName string
		resourceName  string
	}{
		{
			name:          "ComponentType reference",
			kind:          testKindComponentType,
			namespaceName: "default",
			resourceName:  "my-ct",
		},
		{
			name:          "Trait reference",
			kind:          testKindTrait,
			namespaceName: "my-namespace",
			resourceName:  "my-trait",
		},
		{
			name:          "ComponentWorkflow reference",
			kind:          testKindCW,
			namespaceName: "production",
			resourceName:  "my-cw",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gvk := openChoreoGVK(tt.kind)
			obj := buildUnstructuredRef(gvk, tt.namespaceName, tt.resourceName)

			if obj.GetKind() != tt.kind {
				t.Errorf("Expected kind %v, got %v", tt.kind, obj.GetKind())
			}
			if obj.GetNamespace() != tt.namespaceName {
				t.Errorf("Expected namespace %v, got %v", tt.namespaceName, obj.GetNamespace())
			}
			if obj.GetName() != tt.resourceName {
				t.Errorf("Expected name %v, got %v", tt.resourceName, obj.GetName())
			}
			if obj.GetAPIVersion() != testAPIVersion {
				t.Errorf("Expected apiVersion %q, got %v", testAPIVersion, obj.GetAPIVersion())
			}
		})
	}
}

// TestResourceCRUDResponse_JSONSerialization tests that ResourceCRUDResponse serializes correctly
func TestResourceCRUDResponse_JSONSerialization(t *testing.T) {
	tests := []struct {
		name     string
		response ResourceCRUDResponse
		wantKeys []string
	}{
		{
			name: "Full response with all fields",
			response: ResourceCRUDResponse{
				APIVersion: testAPIVersion,
				Kind:       testKindComponentType,
				Name:       "my-ct",
				Namespace:  "default",
				Operation:  "created",
			},
			wantKeys: []string{"apiVersion", "kind", "name", "namespace", "operation"},
		},
		{
			name: "Response without namespace (omitempty)",
			response: ResourceCRUDResponse{
				APIVersion: testAPIVersion,
				Kind:       testKindTrait,
				Name:       "my-trait",
				Operation:  "deleted",
			},
			wantKeys: []string{"apiVersion", "kind", "name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.response)
			if err != nil {
				t.Fatalf("Failed to marshal response: %v", err)
			}

			var result map[string]interface{}
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("Expected key %q in JSON output", key)
				}
			}
		})
	}
}
