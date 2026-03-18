// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
			assert.Equal(t, tt.expected, result, "prometheusLabelName(%q)", tt.kubernetesLabel)
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
				`label_openchoreo_dev_component_uid="comp-123"`,
				`label_openchoreo_dev_project_uid="proj-456"`,
				`label_openchoreo_dev_environment_uid="env-789"`,
			},
		},
		{
			name:          "IDs with special characters",
			componentID:   "comp_test-123",
			projectID:     "proj_test-456",
			environmentID: "env_test-789",
			expectedParts: []string{
				`label_openchoreo_dev_component_uid="comp_test-123"`,
				`label_openchoreo_dev_project_uid="proj_test-456"`,
				`label_openchoreo_dev_environment_uid="env_test-789"`,
			},
		},
		{
			name:          "empty IDs",
			componentID:   "",
			projectID:     "",
			environmentID: "",
			expectedParts: []string{
				`label_openchoreo_dev_component_uid=""`,
				`label_openchoreo_dev_project_uid=""`,
				`label_openchoreo_dev_environment_uid=""`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildLabelFilter(tt.componentID, tt.projectID, tt.environmentID)

			// Verify all expected parts are present
			for _, part := range tt.expectedParts {
				assert.Contains(t, result, part)
			}

			// Verify parts are comma-separated
			assert.Equal(t, 2, strings.Count(result, ","), "BuildLabelFilter() should have 2 commas, got: %q", result)
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
			labelFilter: `label_openchoreo_dev_component_uid="comp-123",label_openchoreo_dev_project_uid="proj-456"`,
			expectedParts: []string{
				"sum by (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid, container)",
				"rate(container_cpu_usage_seconds_total{container!=\"\"}[2m])",
				"kube_pod_labels{",
				`label_openchoreo_dev_component_uid="comp-123",label_openchoreo_dev_project_uid="proj-456"`,
			},
		},
		{
			name:        "empty label filter",
			labelFilter: "",
			expectedParts: []string{
				"sum by (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid, container)",
				"rate(container_cpu_usage_seconds_total{container!=\"\"}[2m])",
				"kube_pod_labels{",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildCPUUsageQuery(tt.labelFilter)

			for _, part := range tt.expectedParts {
				assert.Contains(t, result, part)
			}

			// Verify it contains the label filter
			if tt.labelFilter != "" {
				assert.Contains(t, result, tt.labelFilter)
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
			labelFilter: `label_openchoreo_dev_component_uid="comp-123"`,
			expectedParts: []string{
				"sum by (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid, container)",
				"container_memory_working_set_bytes{container!=\"\"}",
				"kube_pod_labels{",
				`label_openchoreo_dev_component_uid="comp-123"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildMemoryUsageQuery(tt.labelFilter)

			for _, part := range tt.expectedParts {
				assert.Contains(t, result, part)
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
			labelFilter: `label_openchoreo_dev_component_uid="comp-123"`,
			expectedParts: []string{
				"sum by (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid, resource)",
				"kube_pod_container_resource_requests{resource=\"cpu\"}",
				"kube_pod_status_phase{phase=\"Running\"} == 1",
				"kube_pod_labels{",
				`label_openchoreo_dev_component_uid="comp-123"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildCPURequestsQuery(tt.labelFilter)

			for _, part := range tt.expectedParts {
				assert.Contains(t, result, part)
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
			labelFilter: `label_openchoreo_dev_component_uid="comp-123"`,
			expectedParts: []string{
				"sum by (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid, resource)",
				"kube_pod_container_resource_limits{resource=\"cpu\"}",
				"kube_pod_status_phase{phase=\"Running\"} == 1",
				"kube_pod_labels{",
				`label_openchoreo_dev_component_uid="comp-123"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildCPULimitsQuery(tt.labelFilter)

			for _, part := range tt.expectedParts {
				assert.Contains(t, result, part)
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
			labelFilter: `label_openchoreo_dev_component_uid="comp-123"`,
			expectedParts: []string{
				"sum by (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid, resource)",
				"kube_pod_container_resource_requests{resource=\"memory\"}",
				"kube_pod_status_phase{phase=\"Running\"} == 1",
				"kube_pod_labels{",
				`label_openchoreo_dev_component_uid="comp-123"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildMemoryRequestsQuery(tt.labelFilter)

			for _, part := range tt.expectedParts {
				assert.Contains(t, result, part)
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
			labelFilter: `label_openchoreo_dev_component_uid="comp-123"`,
			expectedParts: []string{
				"sum by (label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid, resource)",
				"kube_pod_container_resource_limits{resource=\"memory\"}",
				"kube_pod_status_phase{phase=\"Running\"} == 1",
				"kube_pod_labels{",
				`label_openchoreo_dev_component_uid="comp-123"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildMemoryLimitsQuery(tt.labelFilter)

			for _, part := range tt.expectedParts {
				assert.Contains(t, result, part)
			}
		})
	}
}

func TestBuildHTTPRequestCountQuery(t *testing.T) {
	tests := []struct {
		name          string
		labelFilter   string
		expectedQuery string
	}{
		{
			name:        "valid label filter",
			labelFilter: `label_openchoreo_dev_component_uid="comp-123",label_openchoreo_dev_project_uid="proj-456"`,
			expectedQuery: `
				rate(hubble_http_requests_total{reporter="client"}[2m])
    				* on(destination_pod) group_left(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid)
	    			label_replace(
		    			kube_pod_labels{label_openchoreo_dev_component_uid="comp-123",label_openchoreo_dev_project_uid="proj-456"},
			    		"destination_pod",
				    	"$1",
					    "pod",
					    "(.*)"
			    	)
					>= 0
			`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildHTTPRequestCountQuery(tt.labelFilter)

			// Normalize whitespace for comparison
			normalizedResult := strings.Join(strings.Fields(result), " ")
			normalizedExpected := strings.Join(strings.Fields(tt.expectedQuery), " ")

			assert.Equal(t, normalizedExpected, normalizedResult)
		})
	}
}

func TestBuildSuccessfulHTTPRequestCountQuery(t *testing.T) {
	tests := []struct {
		name          string
		labelFilter   string
		expectedQuery string
	}{
		{
			name:        "valid label filter",
			labelFilter: `label_openchoreo_dev_component_uid="comp-123",label_openchoreo_dev_project_uid="proj-456"`,
			expectedQuery: `
				rate(hubble_http_requests_total{reporter="client", status=~"^[123]..?$"}[2m])
    				* on(destination_pod) group_left(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid)
	    			label_replace(
		    			kube_pod_labels{label_openchoreo_dev_component_uid="comp-123",label_openchoreo_dev_project_uid="proj-456"},
			    		"destination_pod",
				    	"$1",
					    "pod",
				    	"(.*)"
				    )
					>= 0
			`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildSuccessfulHTTPRequestCountQuery(tt.labelFilter)

			// Normalize whitespace for comparison
			normalizedResult := strings.Join(strings.Fields(result), " ")
			normalizedExpected := strings.Join(strings.Fields(tt.expectedQuery), " ")

			assert.Equal(t, normalizedExpected, normalizedResult)
		})
	}
}

func TestBuildUnsuccessfulHTTPRequestCountQuery(t *testing.T) {
	tests := []struct {
		name          string
		labelFilter   string
		expectedQuery string
	}{
		{
			name:        "valid label filter",
			labelFilter: `label_openchoreo_dev_component_uid="comp-123",label_openchoreo_dev_project_uid="proj-456"`,
			expectedQuery: `
				rate(hubble_http_requests_total{reporter="client", status=~"^[45]..?$"}[2m])
    				* on(destination_pod) group_left(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid)
	    			label_replace(
		    			kube_pod_labels{label_openchoreo_dev_component_uid="comp-123",label_openchoreo_dev_project_uid="proj-456"},
			    		"destination_pod",
				    	"$1",
					    "pod",
					    "(.*)"
				    )
					>= 0
			`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildUnsuccessfulHTTPRequestCountQuery(tt.labelFilter)

			// Normalize whitespace for comparison
			normalizedResult := strings.Join(strings.Fields(result), " ")
			normalizedExpected := strings.Join(strings.Fields(tt.expectedQuery), " ")

			assert.Equal(t, normalizedExpected, normalizedResult)
		})
	}
}

func TestBuildMeanHTTPRequestLatencyQuery(t *testing.T) {
	tests := []struct {
		name          string
		labelFilter   string
		expectedQuery string
	}{
		{
			name:        "valid label filter",
			labelFilter: `label_openchoreo_dev_component_uid="comp-123",label_openchoreo_dev_project_uid="proj-456"`,
			expectedQuery: `
				(
					sum by(destination_pod) (rate(hubble_http_request_duration_seconds_sum{reporter="client"}[2m]))
					/
					sum by(destination_pod) (rate(hubble_http_requests_total{reporter="client"}[2m]))
				)
				* on(destination_pod) group_left(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid)
    				label_replace(
	    				kube_pod_labels{label_openchoreo_dev_component_uid="comp-123",label_openchoreo_dev_project_uid="proj-456"},
		    			"destination_pod",
			    		"$1",
				    	"pod",
					    "(.*)"
				    )
				    >= 0
			`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildMeanHTTPRequestLatencyQuery(tt.labelFilter)

			// Normalize whitespace for comparison
			normalizedResult := strings.Join(strings.Fields(result), " ")
			normalizedExpected := strings.Join(strings.Fields(tt.expectedQuery), " ")

			assert.Equal(t, normalizedExpected, normalizedResult)
		})
	}
}

func TestBuild50thPercentileHTTPRequestLatencyQuery(t *testing.T) {
	tests := []struct {
		name          string
		labelFilter   string
		expectedQuery string
	}{
		{
			name:        "valid label filter",
			labelFilter: `label_openchoreo_dev_component_uid="comp-123",label_openchoreo_dev_project_uid="proj-456"`,
			expectedQuery: `
				histogram_quantile(
					0.5,
					sum by(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, le) (
						rate(hubble_http_request_duration_seconds_bucket{reporter="client"}[2m])
    						* on(destination_pod) group_left(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid)
	    					label_replace(
		    					kube_pod_labels{label_openchoreo_dev_component_uid="comp-123",label_openchoreo_dev_project_uid="proj-456"},
			    				"destination_pod",
				    			"$1",
					    		"pod",
						    	"(.*)"
						    )
					)
				)
				>= 0
			`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Build50thPercentileHTTPRequestLatencyQuery(tt.labelFilter)

			// Normalize whitespace for comparison
			normalizedResult := strings.Join(strings.Fields(result), " ")
			normalizedExpected := strings.Join(strings.Fields(tt.expectedQuery), " ")

			assert.Equal(t, normalizedExpected, normalizedResult)
		})
	}
}

func TestBuild90thPercentileHTTPRequestLatencyQuery(t *testing.T) {
	tests := []struct {
		name          string
		labelFilter   string
		expectedQuery string
	}{
		{
			name:        "valid label filter",
			labelFilter: `label_openchoreo_dev_component_uid="comp-123",label_openchoreo_dev_project_uid="proj-456"`,
			expectedQuery: `
				histogram_quantile(
					0.9,
					sum by(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, le) (
						rate(hubble_http_request_duration_seconds_bucket{reporter="client"}[2m])
    						* on(destination_pod) group_left(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid)
	    					label_replace(
		    					kube_pod_labels{label_openchoreo_dev_component_uid="comp-123",label_openchoreo_dev_project_uid="proj-456"},
			    				"destination_pod",
				    			"$1",
					    		"pod",
						    	"(.*)"
						    )
					)
				)
				>= 0
			`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Build90thPercentileHTTPRequestLatencyQuery(tt.labelFilter)

			// Normalize whitespace for comparison
			normalizedResult := strings.Join(strings.Fields(result), " ")
			normalizedExpected := strings.Join(strings.Fields(tt.expectedQuery), " ")

			assert.Equal(t, normalizedExpected, normalizedResult)
		})
	}
}

func TestBuild99thPercentileHTTPRequestLatencyQuery(t *testing.T) {
	tests := []struct {
		name          string
		labelFilter   string
		expectedQuery string
	}{
		{
			name:        "valid label filter",
			labelFilter: `label_openchoreo_dev_component_uid="comp-123",label_openchoreo_dev_project_uid="proj-456"`,
			expectedQuery: `
				histogram_quantile(
					0.99,
					sum by(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, le) (
						rate(hubble_http_request_duration_seconds_bucket{reporter="client"}[2m])
    						* on(destination_pod) group_left(label_openchoreo_dev_component_uid, label_openchoreo_dev_project_uid, label_openchoreo_dev_environment_uid)
    						label_replace(
	    						kube_pod_labels{label_openchoreo_dev_component_uid="comp-123",label_openchoreo_dev_project_uid="proj-456"},
							    "destination_pod",
    							"$1",
	    						"pod",
		    					"(.*)"
						    )
					)
				)
				>= 0
			`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Build99thPercentileHTTPRequestLatencyQuery(tt.labelFilter)

			// Normalize whitespace for comparison
			normalizedResult := strings.Join(strings.Fields(result), " ")
			normalizedExpected := strings.Join(strings.Fields(tt.expectedQuery), " ")

			assert.Equal(t, normalizedExpected, normalizedResult)
		})
	}
}
