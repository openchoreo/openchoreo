// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// projectToolSpecs returns test specs for project toolset
func projectToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_projects",
			toolset:             "project",
			descriptionKeywords: []string{"list", "project"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs:            map[string]any{"namespace_name": testNamespaceName},
			expectedMethod:      "ListProjects",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "create_project",
			toolset:             "project",
			descriptionKeywords: []string{"create", "project"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			optionalParams:      []string{"description", "deployment_pipeline", "type_name", "type_kind", "parameters"},
			testArgs: map[string]any{
				"namespace_name":      testNamespaceName,
				"name":                "new-project",
				"description":         "test project",
				"deployment_pipeline": "custom-pipeline",
				"type_name":           "standard-project",
				"type_kind":           "ProjectType",
				"parameters":          map[string]any{"tier": "premium"},
			},
			expectedMethod: "CreateProject",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
				req, ok := args[1].(*gen.CreateProjectJSONRequestBody)
				if !ok {
					t.Fatalf("Expected args[1] to be *gen.CreateProjectJSONRequestBody, got %T", args[1])
				}
				if req.Metadata.Name != "new-project" {
					t.Errorf("Expected name %q, got %q", "new-project", req.Metadata.Name)
				}
				if req.Spec == nil {
					t.Fatal("Expected spec to be populated from optional args")
				}
				if req.Spec.DeploymentPipelineRef == nil || req.Spec.DeploymentPipelineRef.Name != "custom-pipeline" {
					t.Errorf("Expected deploymentPipelineRef name %q, got %v", "custom-pipeline", req.Spec.DeploymentPipelineRef)
				}
				if req.Spec.Type == nil || req.Spec.Type.Name != "standard-project" {
					t.Errorf("Expected type name %q, got %v", "standard-project", req.Spec.Type)
				}
				if req.Spec.Type == nil || req.Spec.Type.Kind == nil || *req.Spec.Type.Kind != gen.ProjectType {
					t.Errorf("Expected type kind %q, got %v", gen.ProjectType, req.Spec.Type)
				}
				if req.Spec.Parameters == nil || (*req.Spec.Parameters)["tier"] != "premium" {
					t.Errorf("Expected parameters tier=premium, got %v", req.Spec.Parameters)
				}
			},
		},
		{
			name:                "update_project",
			toolset:             "project",
			descriptionKeywords: []string{"update", "project"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name"},
			optionalParams:      []string{"deployment_pipeline", "display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"display_name":   "Updated Project",
				"description":    "Updated project description",
			},
			expectedMethod: "UpdateProject",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
				if args[1] != testProjectName {
					t.Errorf("Expected project %q, got %v", testProjectName, args[1])
				}
				req, ok := args[2].(*gen.PatchProjectRequest)
				if !ok {
					t.Fatalf("Expected *gen.PatchProjectRequest, got %T", args[2])
				}
				if req.DeploymentPipeline != nil {
					t.Errorf("Expected deployment pipeline to be empty, got %v", *req.DeploymentPipeline)
				}
				if req.DisplayName == nil || *req.DisplayName != "Updated Project" {
					t.Errorf("Expected display name %q, got %v", "Updated Project", req.DisplayName)
				}
				if req.Description == nil || *req.Description != "Updated project description" {
					t.Errorf("Expected description %q, got %v", "Updated project description", req.Description)
				}
			},
		},
		{
			name:                "delete_project",
			toolset:             "project",
			descriptionKeywords: []string{"delete", "project"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
			},
			expectedMethod: "DeleteProject",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testProjectName, args[0], args[1])
				}
			},
		},
	}
}
