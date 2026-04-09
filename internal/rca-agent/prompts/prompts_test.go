// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package prompts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderRCAPrompt(t *testing.T) {
	t.Parallel()
	result, err := RenderRCAPrompt(&RCAPromptData{
		ObservabilityTools: []ToolInfo{{Name: "query_logs"}, {Name: "query_metrics"}},
		OpenchoreoTools:    []ToolInfo{{Name: "list_components"}},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "query_logs")
	assert.Contains(t, result, "list_components")
}

func TestRenderChatPrompt(t *testing.T) {
	t.Parallel()
	result, err := RenderChatPrompt(&ChatPromptData{
		ObservabilityTools: []ToolInfo{{Name: "query_traces"}},
		OpenchoreoTools:    []ToolInfo{{Name: "list_components"}},
		Scope: &Scope{
			Namespace:   "default",
			Project:     "myproj",
			Environment: "prod",
		},
		ReportContext: map[string]any{"summary": "OOM detected"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "query_traces")
}

func TestRenderRCARequest(t *testing.T) {
	t.Parallel()
	result, err := RenderRCARequest(&RCARequestData{
		Scope: &Scope{
			Namespace:   "default",
			Project:     "myproj",
			Environment: "prod",
			Component:   "api-server",
		},
		Alert: &AlertData{
			ID:        "alert-1",
			Value:     95.0,
			Timestamp: "2026-03-07T10:00:00Z",
			Rule: AlertRuleData{
				Name:     "high-error-rate",
				Severity: "critical",
			},
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "api-server")
	assert.Contains(t, result, "alert-1")
}

func TestRenderRemedPrompt(t *testing.T) {
	t.Parallel()
	result, err := RenderRemedPrompt(&RemedPromptData{
		Scope: &Scope{
			Namespace:   "default",
			Project:     "myproj",
			Environment: "prod",
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

func TestRenderRCAPrompt_EmptyTools(t *testing.T) {
	t.Parallel()
	result, err := RenderRCAPrompt(&RCAPromptData{})
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}
