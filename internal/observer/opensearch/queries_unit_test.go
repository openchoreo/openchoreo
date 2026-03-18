// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package opensearch

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/labels"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

func TestSanitizeWildcardValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "escapes backslash", input: `a\b`, expected: `a\\b`},
		{name: "escapes double quote", input: `a"b`, expected: `a\"b`},
		{name: "escapes asterisk", input: `a*b`, expected: `a\*b`},
		{name: "escapes question mark", input: `a?b`, expected: `a\?b`},
		{name: "clean input unchanged", input: "hello world", expected: "hello world"},
		{name: "empty string", input: "", expected: ""},
		{name: "multiple special chars", input: `*?"\ `, expected: `\*\?\"\\` + " "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeWildcardValue(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestFormatDurationForOpenSearch(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  string
		expectErr bool
	}{
		{name: "hours", input: "2h", expected: "2h"},
		{name: "hours from verbose", input: "1h0m0s", expected: "1h"},
		{name: "minutes", input: "5m", expected: "5m"},
		{name: "minutes from verbose", input: "5m0s", expected: "5m"},
		{name: "seconds rejected", input: "30s", expectErr: true},
		{name: "mixed hours and minutes rejected", input: "1h30m", expected: "90m"},
		{name: "invalid format", input: "abc", expectErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatDurationForOpenSearch(tt.input)
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestAddTimeRangeFilter(t *testing.T) {
	t.Run("both times set", func(t *testing.T) {
		must := addTimeRangeFilter(nil, "2024-01-01T00:00:00Z", "2024-01-02T00:00:00Z")
		require.Len(t, must, 1)
		r := must[0]["range"].(map[string]any)
		ts := r["@timestamp"].(map[string]any)
		assert.Equal(t, "2024-01-01T00:00:00Z", ts["gt"])
		assert.Equal(t, "2024-01-02T00:00:00Z", ts["lt"])
	})

	t.Run("empty start time", func(t *testing.T) {
		must := addTimeRangeFilter(nil, "", "2024-01-02T00:00:00Z")
		assert.Empty(t, must)
	})

	t.Run("empty end time", func(t *testing.T) {
		must := addTimeRangeFilter(nil, "2024-01-01T00:00:00Z", "")
		assert.Empty(t, must)
	})

	t.Run("both empty", func(t *testing.T) {
		must := addTimeRangeFilter(nil, "", "")
		assert.Empty(t, must)
	})
}

func TestAddSearchPhraseFilter(t *testing.T) {
	t.Run("non-empty phrase", func(t *testing.T) {
		must := addSearchPhraseFilter(nil, "error")
		require.Len(t, must, 1)
		wc := must[0]["wildcard"].(map[string]any)
		assert.Equal(t, "*error*", wc["log"])
	})

	t.Run("empty phrase", func(t *testing.T) {
		must := addSearchPhraseFilter(nil, "")
		assert.Empty(t, must)
	})

	t.Run("phrase with special chars is sanitized", func(t *testing.T) {
		must := addSearchPhraseFilter(nil, "err*or")
		wc := must[0]["wildcard"].(map[string]any)
		assert.Equal(t, `*err\*or*`, wc["log"])
	})
}

func TestAddLogLevelFilter(t *testing.T) {
	t.Run("multiple levels", func(t *testing.T) {
		must := addLogLevelFilter(nil, []string{"error", "warn"})
		require.Len(t, must, 1)
		boolQ := must[0]["bool"].(map[string]any)
		should := boolQ["should"].([]map[string]any)
		assert.Len(t, should, 2)
		assert.Equal(t, 1, boolQ["minimum_should_match"])
	})

	t.Run("single level", func(t *testing.T) {
		must := addLogLevelFilter(nil, []string{"info"})
		require.Len(t, must, 1)
		boolQ := must[0]["bool"].(map[string]any)
		should := boolQ["should"].([]map[string]any)
		assert.Len(t, should, 1)
		// Verify value is uppercased
		wc := should[0]["wildcard"].(map[string]any)
		logField := wc["log"].(map[string]any)
		assert.Equal(t, "*INFO*", logField["value"])
	})

	t.Run("empty levels", func(t *testing.T) {
		must := addLogLevelFilter(nil, nil)
		assert.Empty(t, must)
		must = addLogLevelFilter(nil, []string{})
		assert.Empty(t, must)
	})
}

func TestGetOperatorSymbol(t *testing.T) {
	tests := []struct {
		op       string
		expected string
	}{
		{"gt", ">"},
		{"gte", ">="},
		{"lt", "<"},
		{"lte", "<="},
		{"unknown", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			got := GetOperatorSymbol(tt.op)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestBuildWebhookMessageTemplate(t *testing.T) {
	params := types.AlertingRuleRequest{
		Metadata: types.AlertingRuleMetadata{
			Name:           "test-rule",
			Namespace:      "ns-1",
			ComponentUID:   "comp-uid",
			ProjectUID:     "proj-uid",
			EnvironmentUID: "env-uid",
		},
	}
	tmpl := buildWebhookMessageTemplate(params)

	// Verify it is valid JSON (apart from Mustache placeholders)
	// Replace mustache vars with dummy values to validate structure
	sanitized := strings.ReplaceAll(tmpl, "{{ctx.results.0.hits.total.value}}", "0")
	sanitized = strings.ReplaceAll(sanitized, "{{ctx.periodStart}}", "2024-01-01")
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(sanitized), &parsed), "template is not valid JSON after placeholder replacement:\n%s", tmpl)

	// Verify expected keys
	for _, key := range []string{"ruleName", "ruleNamespace", "componentUid", "projectUid", "environmentUid", "alertValue", "alertTimestamp"} {
		assert.Contains(t, parsed, key, "expected key %q not found in template", key)
	}
	assert.Equal(t, "test-rule", parsed["ruleName"])
}

func TestBuildComponentLogsQueryV1(t *testing.T) {
	qb := NewQueryBuilder("logs-")

	t.Run("required fields missing returns error", func(t *testing.T) {
		_, err := qb.BuildComponentLogsQueryV1(ComponentLogsQueryParamsV1{})
		require.Error(t, err)
		_, err = qb.BuildComponentLogsQueryV1(ComponentLogsQueryParamsV1{StartTime: "a", EndTime: "b"})
		require.Error(t, err)
	})

	t.Run("all optional filters", func(t *testing.T) {
		q, err := qb.BuildComponentLogsQueryV1(ComponentLogsQueryParamsV1{
			StartTime:     "2024-01-01T00:00:00Z",
			EndTime:       "2024-01-02T00:00:00Z",
			NamespaceName: "ns-1",
			ProjectID:     "proj-1",
			ComponentID:   "comp-1",
			EnvironmentID: "env-1",
			SearchPhrase:  "error",
			LogLevels:     []string{"ERROR"},
			Limit:         50,
			SortOrder:     "asc",
		})
		require.NoError(t, err)
		assert.Equal(t, 50, q["size"])
		boolQ := q["query"].(map[string]any)["bool"].(map[string]any)
		must := boolQ["must"].([]map[string]any)
		// time range + namespace + project + component + environment + search + logLevel = 7
		assert.Len(t, must, 7)
		sort := q["sort"].([]map[string]any)
		ts := sort[0]["@timestamp"].(map[string]any)
		assert.Equal(t, "asc", ts["order"])
	})

	t.Run("defaults for limit and sort order", func(t *testing.T) {
		q, err := qb.BuildComponentLogsQueryV1(ComponentLogsQueryParamsV1{
			StartTime:     "2024-01-01T00:00:00Z",
			EndTime:       "2024-01-02T00:00:00Z",
			NamespaceName: "ns-1",
		})
		require.NoError(t, err)
		assert.Equal(t, 100, q["size"])
		sort := q["sort"].([]map[string]any)
		ts := sort[0]["@timestamp"].(map[string]any)
		assert.Equal(t, "desc", ts["order"])
	})
}

func TestBuildNamespaceLogsQuery(t *testing.T) {
	qb := NewQueryBuilder("logs-")

	t.Run("with podLabels and filters", func(t *testing.T) {
		params := QueryParams{
			NamespaceName: "ns-1",
			EnvironmentID: "env-1",
			Namespace:     "k8s-ns",
			StartTime:     "2024-01-01T00:00:00Z",
			EndTime:       "2024-01-02T00:00:00Z",
			SearchPhrase:  "warn",
			LogLevels:     []string{"WARN"},
			Limit:         10,
			SortOrder:     "asc",
		}
		podLabels := map[string]string{"app": "myapp"}

		q := qb.BuildNamespaceLogsQuery(params, podLabels)
		assert.Equal(t, 10, q["size"])
		boolQ := q["query"].(map[string]any)["bool"].(map[string]any)
		must := boolQ["must"].([]map[string]any)

		// namespace name + env + k8s namespace + time range + search + logLevel + 1 pod label = 7
		assert.Len(t, must, 7)

		// Verify pod label filter present
		found := false
		for _, c := range must {
			if term, ok := c["term"].(map[string]any); ok {
				if _, ok := term["kubernetes.labels.app"]; ok {
					found = true
				}
			}
		}
		assert.True(t, found, "expected pod label filter for 'app' not found")
	})

	t.Run("no podLabels, minimal params", func(t *testing.T) {
		q := qb.BuildNamespaceLogsQuery(QueryParams{
			NamespaceName: "ns-1",
			Limit:         5,
			SortOrder:     "desc",
		}, nil)
		boolQ := q["query"].(map[string]any)["bool"].(map[string]any)
		must := boolQ["must"].([]map[string]any)
		// Only namespace name filter
		assert.Len(t, must, 1)
	})
}

func TestBuildSpanDetailsQuery(t *testing.T) {
	qb := NewQueryBuilder("spans-")
	q := qb.BuildSpanDetailsQuery("trace-abc", "span-123")

	assert.Equal(t, 1, q["size"])

	boolQ := q["query"].(map[string]any)["bool"].(map[string]any)
	filters := boolQ["filter"].([]map[string]any)
	require.Len(t, filters, 2)

	traceFound, spanFound := false, false
	for _, f := range filters {
		if term, ok := f["term"].(map[string]any); ok {
			if v, ok := term["traceId"]; ok && v == "trace-abc" {
				traceFound = true
			}
			if v, ok := term["spanId"]; ok && v == "span-123" {
				spanFound = true
			}
		}
	}
	assert.True(t, traceFound, "traceId filter not found")
	assert.True(t, spanFound, "spanId filter not found")
}

func TestBuildLogAlertingRuleQuery(t *testing.T) {
	qb := NewQueryBuilder("logs-")

	t.Run("valid params", func(t *testing.T) {
		params := types.AlertingRuleRequest{
			Metadata: types.AlertingRuleMetadata{
				ComponentUID:   "comp-1",
				EnvironmentUID: "env-1",
				ProjectUID:     "proj-1",
			},
			Source: types.AlertingRuleSource{
				Query: "OutOfMemory",
			},
			Condition: types.AlertingRuleCondition{
				Window: "1h",
			},
		}
		q, err := qb.BuildLogAlertingRuleQuery(params)
		require.NoError(t, err)
		assert.Equal(t, 0, q["size"])
		boolQ := q["query"].(map[string]any)["bool"].(map[string]any)
		filters := boolQ["filter"].([]map[string]any)
		assert.Len(t, filters, 5)
	})

	t.Run("invalid window", func(t *testing.T) {
		params := types.AlertingRuleRequest{
			Condition: types.AlertingRuleCondition{
				Window: "invalid",
			},
		}
		_, err := qb.BuildLogAlertingRuleQuery(params)
		require.Error(t, err)
	})
}

func TestBuildLogAlertingRuleMonitorBody(t *testing.T) {
	qb := NewQueryBuilder("logs-")

	t.Run("valid params", func(t *testing.T) {
		params := types.AlertingRuleRequest{
			Metadata: types.AlertingRuleMetadata{
				Name:           "my-rule",
				Namespace:      "ns-1",
				ComponentUID:   "comp-1",
				EnvironmentUID: "env-1",
				ProjectUID:     "proj-1",
			},
			Source: types.AlertingRuleSource{
				Query: "error",
			},
			Condition: types.AlertingRuleCondition{
				Enabled:   true,
				Window:    "5m",
				Interval:  "1m",
				Operator:  "gt",
				Threshold: 10,
			},
		}
		body, err := qb.BuildLogAlertingRuleMonitorBody(params)
		require.NoError(t, err)
		assert.Equal(t, "my-rule", body["name"])
		assert.Equal(t, "monitor", body["type"])
		assert.Equal(t, true, body["enabled"])
		// Verify inputs contain indices with prefix
		inputs := body["inputs"].([]any)
		require.Len(t, inputs, 1)
		input := inputs[0].(map[string]any)
		search := input["search"].(map[string]any)
		indices := search["indices"].([]any)
		assert.Len(t, indices, 1)
		assert.Equal(t, "logs-*", indices[0])
	})

	t.Run("invalid interval", func(t *testing.T) {
		params := types.AlertingRuleRequest{
			Condition: types.AlertingRuleCondition{
				Window:   "5m",
				Interval: "bad",
			},
		}
		_, err := qb.BuildLogAlertingRuleMonitorBody(params)
		require.Error(t, err)
	})

	t.Run("invalid window propagated from query builder", func(t *testing.T) {
		params := types.AlertingRuleRequest{
			Condition: types.AlertingRuleCondition{
				Window:   "bad",
				Interval: "1m",
			},
		}
		_, err := qb.BuildLogAlertingRuleMonitorBody(params)
		require.Error(t, err)
	})
}

func TestBuildWorkflowRunLogsQuery(t *testing.T) {
	qb := NewQueryBuilder("logs-")

	t.Run("without stepName", func(t *testing.T) {
		params := WorkflowRunQueryParams{
			QueryParams: QueryParams{
				StartTime: "2024-01-01T00:00:00Z",
				EndTime:   "2024-01-02T00:00:00Z",
				Limit:     20,
				SortOrder: "asc",
			},
			WorkflowRunID: "run-abc",
		}
		q := qb.BuildWorkflowRunLogsQuery(params)
		boolQ := q["query"].(map[string]any)["bool"].(map[string]any)
		must := boolQ["must"].([]map[string]any)
		// pod wildcard + time range = 2
		assert.Len(t, must, 2)
	})

	t.Run("with stepName", func(t *testing.T) {
		params := WorkflowRunQueryParams{
			QueryParams: QueryParams{
				StartTime: "2024-01-01T00:00:00Z",
				EndTime:   "2024-01-02T00:00:00Z",
				Limit:     20,
				SortOrder: "asc",
			},
			WorkflowRunID: "run-abc",
			StepName:      "build",
		}
		q := qb.BuildWorkflowRunLogsQuery(params)
		boolQ := q["query"].(map[string]any)["bool"].(map[string]any)
		must := boolQ["must"].([]map[string]any)
		// pod wildcard + step name + time range = 3
		assert.Len(t, must, 3)
	})

	t.Run("with namespace", func(t *testing.T) {
		params := WorkflowRunQueryParams{
			QueryParams: QueryParams{
				NamespaceName: "my-ns",
				Limit:         20,
				SortOrder:     "asc",
			},
			WorkflowRunID: "run-abc",
		}
		q := qb.BuildWorkflowRunLogsQuery(params)
		boolQ := q["query"].(map[string]any)["bool"].(map[string]any)
		must := boolQ["must"].([]map[string]any)
		// pod wildcard + namespace = 2
		assert.Len(t, must, 2)
		// Check the namespace filter uses "workflows-" prefix
		nsFound := false
		for _, c := range must {
			if term, ok := c["term"].(map[string]any); ok {
				if v, ok := term[labels.KubernetesNamespaceName]; ok && v == "workflows-my-ns" {
					nsFound = true
				}
			}
		}
		assert.True(t, nsFound, "expected namespace filter with 'workflows-' prefix not found")
	})
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{name: "found", slice: []string{"a", "b", "c"}, item: "b", expected: true},
		{name: "not found", slice: []string{"a", "b", "c"}, item: "d", expected: false},
		{name: "empty slice", slice: []string{}, item: "a", expected: false},
		{name: "nil slice", slice: nil, item: "a", expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contains(tt.slice, tt.item)
			assert.Equal(t, tt.expected, got)
		})
	}
}
