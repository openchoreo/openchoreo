// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"strings"
	"testing"
)

func TestPrometheusLabelName(t *testing.T) {
	tests := []struct {
		name            string
		kubernetesLabel string
		expected        string
	}{
		{
			name:            "simple label with dashes",
			kubernetesLabel: "component-name",
			expected:        "label_component_name",
		},
		{
			name:            "label with multiple dashes",
			kubernetesLabel: "my-component-name",
			expected:        "label_my_component_name",
		},
		{
			name:            "label without dashes",
			kubernetesLabel: "componentname",
			expected:        "label_componentname",
		},
		{
			name:            "label with underscores preserved",
			kubernetesLabel: "component_name",
			expected:        "label_component_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := prometheusLabelName(tt.kubernetesLabel)
			if result != tt.expected {
				t.Errorf("prometheusLabelName(%q) = %q, want %q", tt.kubernetesLabel, result, tt.expected)
			}
		})
	}
}

func TestBuildLabelFilter(t *testing.T) {
	tests := []struct {
		name          string
		componentID   string
		projectID     string
		environmentID string
		expectedParts []string
	}{
		{
			name:          "all IDs provided",
			componentID:   "comp-123",
			projectID:     "proj-456",
			environmentID: "env-789",
			expectedParts: []string{
				`label_component_name="comp-123"`,
				`label_project_name="proj-456"`,
				`label_environment_name="env-789"`,
			},
		},
		{
			name:          "IDs with special characters",
			componentID:   "comp_test-123",
			projectID:     "proj_test-456",
			environmentID: "env_test-789",
			expectedParts: []string{
				`label_component_name="comp_test-123"`,
				`label_project_name="proj_test-456"`,
				`label_environment_name="env_test-789"`,
			},
		},
		{
			name:          "empty IDs",
			componentID:   "",
			projectID:     "",
			environmentID: "",
			expectedParts: []string{
				`label_component_name=""`,
				`label_project_name=""`,
				`label_environment_name=""`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildLabelFilter(tt.componentID, tt.projectID, tt.environmentID)

			// Verify all expected parts are present
			for _, part := range tt.expectedParts {
				if !strings.Contains(result, part) {
					t.Errorf("BuildLabelFilter() result missing expected part: %q\nGot: %q", part, result)
				}
			}

			// Verify parts are comma-separated
			if strings.Count(result, ",") != 2 {
				t.Errorf("BuildLabelFilter() should have 2 commas, got: %q", result)
			}
		})
	}
}

func TestBuildCPUUsageQuery(t *testing.T) {
	tests := []struct {
		name          string
		labelFilter   string
		expectedParts []string
	}{
		{
			name:        "valid label filter",
			labelFilter: `label_component_id="comp-123",label_project_id="proj-456"`,
			expectedParts: []string{
				"sum by (label_component_name, label_environment_name, label_project_name)",
				"rate(container_cpu_usage_seconds_total{container=\"main\"}[2m])",
				"kube_pod_labels{",
				`label_component_id="comp-123",label_project_id="proj-456"`,
			},
		},
		{
			name:        "empty label filter",
			labelFilter: "",
			expectedParts: []string{
				"sum by (label_component_name, label_environment_name, label_project_name)",
				"rate(container_cpu_usage_seconds_total{container=\"main\"}[2m])",
				"kube_pod_labels{",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildCPUUsageQuery(tt.labelFilter)

			for _, part := range tt.expectedParts {
				if !strings.Contains(result, part) {
					t.Errorf("BuildCPUUsageQuery() missing expected part: %q\nGot: %q", part, result)
				}
			}

			// Verify it contains the label filter
			if tt.labelFilter != "" && !strings.Contains(result, tt.labelFilter) {
				t.Errorf("BuildCPUUsageQuery() missing label filter: %q\nGot: %q", tt.labelFilter, result)
			}
		})
	}
}

func TestBuildMemoryUsageQuery(t *testing.T) {
	tests := []struct {
		name          string
		labelFilter   string
		expectedParts []string
	}{
		{
			name:        "valid label filter",
			labelFilter: `label_component_id="comp-123"`,
			expectedParts: []string{
				"sum by (label_component_name, label_environment_name, label_project_name)",
				"container_memory_working_set_bytes{container=\"main\"}",
				"kube_pod_labels{",
				`label_component_id="comp-123"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildMemoryUsageQuery(tt.labelFilter)

			for _, part := range tt.expectedParts {
				if !strings.Contains(result, part) {
					t.Errorf("BuildMemoryUsageQuery() missing expected part: %q\nGot: %q", part, result)
				}
			}
		})
	}
}

func TestBuildCPURequestsQuery(t *testing.T) {
	tests := []struct {
		name          string
		labelFilter   string
		expectedParts []string
	}{
		{
			name:        "valid label filter",
			labelFilter: `label_component_id="comp-123"`,
			expectedParts: []string{
				"sum by (label_component_name, label_environment_name, label_project_name, resource)",
				"kube_pod_container_resource_requests{resource=\"cpu\"}",
				"kube_pod_status_phase{phase=\"Running\"} == 1",
				"kube_pod_labels{",
				`label_component_id="comp-123"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildCPURequestsQuery(tt.labelFilter)

			for _, part := range tt.expectedParts {
				if !strings.Contains(result, part) {
					t.Errorf("BuildCPURequestsQuery() missing expected part: %q\nGot: %q", part, result)
				}
			}
		})
	}
}

func TestBuildCPULimitsQuery(t *testing.T) {
	tests := []struct {
		name          string
		labelFilter   string
		expectedParts []string
	}{
		{
			name:        "valid label filter",
			labelFilter: `label_component_id="comp-123"`,
			expectedParts: []string{
				"sum by (label_component_name, label_environment_name, label_project_name, resource)",
				"kube_pod_container_resource_limits{resource=\"cpu\"}",
				"kube_pod_status_phase{phase=\"Running\"} == 1",
				"kube_pod_labels{",
				`label_component_id="comp-123"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildCPULimitsQuery(tt.labelFilter)

			for _, part := range tt.expectedParts {
				if !strings.Contains(result, part) {
					t.Errorf("BuildCPULimitsQuery() missing expected part: %q\nGot: %q", part, result)
				}
			}
		})
	}
}

func TestBuildMemoryRequestsQuery(t *testing.T) {
	tests := []struct {
		name          string
		labelFilter   string
		expectedParts []string
	}{
		{
			name:        "valid label filter",
			labelFilter: `label_component_id="comp-123"`,
			expectedParts: []string{
				"sum by (label_component_name, label_environment_name, label_project_name, resource)",
				"kube_pod_container_resource_requests{resource=\"memory\"}",
				"kube_pod_status_phase{phase=\"Running\"} == 1",
				"kube_pod_labels{",
				`label_component_id="comp-123"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildMemoryRequestsQuery(tt.labelFilter)

			for _, part := range tt.expectedParts {
				if !strings.Contains(result, part) {
					t.Errorf("BuildMemoryRequestsQuery() missing expected part: %q\nGot: %q", part, result)
				}
			}
		})
	}
}

func TestBuildMemoryLimitsQuery(t *testing.T) {
	tests := []struct {
		name          string
		labelFilter   string
		expectedParts []string
	}{
		{
			name:        "valid label filter",
			labelFilter: `label_component_id="comp-123"`,
			expectedParts: []string{
				"sum by (label_component_name, label_environment_name, label_project_name, resource)",
				"kube_pod_container_resource_limits{resource=\"memory\"}",
				"kube_pod_status_phase{phase=\"Running\"} == 1",
				"kube_pod_labels{",
				`label_component_id="comp-123"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildMemoryLimitsQuery(tt.labelFilter)

			for _, part := range tt.expectedParts {
				if !strings.Contains(result, part) {
					t.Errorf("BuildMemoryLimitsQuery() missing expected part: %q\nGot: %q", part, result)
				}
			}
		})
	}
}
