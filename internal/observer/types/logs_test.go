// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"encoding/json"
	"testing"
)

const testNS1 = "ns1"

func TestSearchScope_UnmarshalJSON_ComponentScope(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantIsComp    bool
		wantIsWf      bool
		wantErr       bool
		wantNamespace string
		wantProject   string
	}{
		{
			name:          "component scope with all fields",
			input:         `{"namespace":"ns1","project":"proj1","component":"comp1","environment":"env1"}`,
			wantIsComp:    true,
			wantIsWf:      false,
			wantNamespace: testNS1,
			wantProject:   "proj1",
		},
		{
			name:          "component scope - namespace and project only",
			input:         `{"namespace":"ns1","project":"proj1"}`,
			wantIsComp:    true,
			wantIsWf:      false,
			wantNamespace: testNS1,
			wantProject:   "proj1",
		},
		{
			name:          "component scope - namespace only (defaults to component scope)",
			input:         `{"namespace":"ns1"}`,
			wantIsComp:    true,
			wantIsWf:      false,
			wantNamespace: testNS1,
		},
		{
			name:       "component scope via component field",
			input:      `{"namespace":"ns1","component":"comp1"}`,
			wantIsComp: true,
			wantIsWf:   false,
		},
		{
			name:       "component scope via environment field",
			input:      `{"namespace":"ns1","environment":"env1"}`,
			wantIsComp: true,
			wantIsWf:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s SearchScope
			err := json.Unmarshal([]byte(tt.input), &s)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.wantIsComp && s.Component == nil {
				t.Error("Expected Component scope to be set, got nil")
			}
			if !tt.wantIsComp && s.Component != nil {
				t.Error("Expected Component scope to be nil")
			}
			if tt.wantIsWf && s.Workflow == nil {
				t.Error("Expected Workflow scope to be set, got nil")
			}
			if !tt.wantIsWf && s.Workflow != nil {
				t.Error("Expected Workflow scope to be nil")
			}
			if tt.wantIsComp && s.Component != nil {
				if tt.wantNamespace != "" && s.Component.Namespace != tt.wantNamespace {
					t.Errorf("Expected Namespace=%q, got %q", tt.wantNamespace, s.Component.Namespace)
				}
				if tt.wantProject != "" && s.Component.Project != tt.wantProject {
					t.Errorf("Expected Project=%q, got %q", tt.wantProject, s.Component.Project)
				}
			}
		})
	}
}

func TestSearchScope_UnmarshalJSON_WorkflowScope(t *testing.T) {
	tests := []struct {
		name                string
		input               string
		wantErr             bool
		wantNamespace       string
		wantWorkflowRunName string
	}{
		{
			name:                "workflow scope with run name",
			input:               `{"namespace":"ns1","workflowRunName":"run-123"}`,
			wantNamespace:       testNS1,
			wantWorkflowRunName: "run-123",
		},
		{
			name:          "workflow scope - namespace and workflowRunName empty string",
			input:         `{"namespace":"ns1","workflowRunName":""}`,
			wantNamespace: testNS1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s SearchScope
			err := json.Unmarshal([]byte(tt.input), &s)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if s.Workflow == nil {
				t.Fatal("Expected Workflow scope, got nil")
			}
			if s.Component != nil {
				t.Error("Expected Component scope to be nil for workflow input")
			}
			if s.Workflow.Namespace != tt.wantNamespace {
				t.Errorf("Expected Namespace=%q, got %q", tt.wantNamespace, s.Workflow.Namespace)
			}
			if tt.wantWorkflowRunName != "" && s.Workflow.WorkflowRunName != tt.wantWorkflowRunName {
				t.Errorf("Expected WorkflowRunName=%q, got %q", tt.wantWorkflowRunName, s.Workflow.WorkflowRunName)
			}
		})
	}
}

func TestSearchScope_UnmarshalJSON_InvalidJSON(t *testing.T) {
	var s SearchScope
	err := json.Unmarshal([]byte(`not-valid-json`), &s)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestSearchScope_MarshalJSON_ComponentScope(t *testing.T) {
	s := SearchScope{
		Component: &ComponentSearchScope{
			Namespace: testNS1,
			Project:   "proj1",
			Component: "comp1",
		},
	}

	data, err := json.Marshal(&s)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	// Re-unmarshal and verify
	var result ComponentSearchScope
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal marshaled data: %v", err)
	}
	if result.Namespace != testNS1 {
		t.Errorf("Expected Namespace=ns1, got %s", result.Namespace)
	}
	if result.Project != "proj1" {
		t.Errorf("Expected Project=proj1, got %s", result.Project)
	}
	if result.Component != "comp1" {
		t.Errorf("Expected Component=comp1, got %s", result.Component)
	}
}

func TestSearchScope_MarshalJSON_WorkflowScope(t *testing.T) {
	s := SearchScope{
		Workflow: &WorkflowSearchScope{
			Namespace:       testNS1,
			WorkflowRunName: "run-456",
		},
	}

	data, err := json.Marshal(&s)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	var result WorkflowSearchScope
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal marshaled data: %v", err)
	}
	if result.Namespace != testNS1 {
		t.Errorf("Expected Namespace=ns1, got %s", result.Namespace)
	}
	if result.WorkflowRunName != "run-456" {
		t.Errorf("Expected WorkflowRunName=run-456, got %s", result.WorkflowRunName)
	}
}

func TestSearchScope_MarshalJSON_NilScope(t *testing.T) {
	s := SearchScope{} // both Component and Workflow are nil

	data, err := json.Marshal(&s)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if string(data) != "null" {
		t.Errorf("Expected 'null' for empty scope, got %s", string(data))
	}
}

func TestSearchScope_RoundTrip_ComponentScope(t *testing.T) {
	original := `{"namespace":"my-ns","project":"my-proj","component":"my-comp","environment":"my-env"}`

	var s SearchScope
	if err := json.Unmarshal([]byte(original), &s); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if s.Component == nil {
		t.Fatal("Expected Component scope")
	}
	if s.Component.Namespace != "my-ns" {
		t.Errorf("Namespace mismatch: got %s", s.Component.Namespace)
	}

	data, err := json.Marshal(&s)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Re-unmarshal and verify fields are preserved
	var s2 SearchScope
	if err := json.Unmarshal(data, &s2); err != nil {
		t.Fatalf("Second unmarshal failed: %v", err)
	}
	if s2.Component == nil {
		t.Fatal("Expected Component scope after round-trip")
	}
	if s2.Component.Namespace != s.Component.Namespace {
		t.Errorf("Namespace mismatch after round-trip: got %s", s2.Component.Namespace)
	}
	if s2.Component.Project != s.Component.Project {
		t.Errorf("Project mismatch after round-trip: got %s", s2.Component.Project)
	}
}

func TestLogsQueryRequest_JSONSerialization(t *testing.T) {
	req := LogsQueryRequest{
		SearchScope: &SearchScope{
			Component: &ComponentSearchScope{
				Namespace: testNS1,
			},
		},
		StartTime:    "2024-01-01T00:00:00Z",
		EndTime:      "2024-01-02T00:00:00Z",
		SearchPhrase: "error",
		LogLevels:    []string{"ERROR", "WARN"},
		Limit:        100,
		SortOrder:    "desc",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded LogsQueryRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.StartTime != req.StartTime {
		t.Errorf("StartTime mismatch: got %s", decoded.StartTime)
	}
	if decoded.Limit != req.Limit {
		t.Errorf("Limit mismatch: got %d", decoded.Limit)
	}
	if decoded.SearchScope == nil || decoded.SearchScope.Component == nil {
		t.Fatal("Expected component search scope after deserialization")
	}
	if decoded.SearchScope.Component.Namespace != testNS1 {
		t.Errorf("Namespace mismatch: got %s", decoded.SearchScope.Component.Namespace)
	}
}

func TestErrorResponse_Constants(t *testing.T) {
	// Verify error constants have expected values
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"ErrorTypeValidation", ErrorTypeValidation, "validation_error"},
		{"ErrorTypeInternal", ErrorTypeInternal, "internal_error"},
		{"ErrorTypeForbidden", ErrorTypeForbidden, "forbidden"},
		{"ErrorTypeNotFound", ErrorTypeNotFound, "not_found"},
		{"ErrorCodeInvalidRequest", ErrorCodeInvalidRequest, "OBS-V1-001"},
		{"ErrorCodeMissingField", ErrorCodeMissingField, "OBS-V1-002"},
		{"ErrorCodeInvalidTimeRange", ErrorCodeInvalidTimeRange, "OBS-V1-003"},
		{"ErrorCodeInternalError", ErrorCodeInternalError, "OBS-V1-100"},
		{"ErrorCodeForbidden", ErrorCodeForbidden, "OBS-V1-403"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}
