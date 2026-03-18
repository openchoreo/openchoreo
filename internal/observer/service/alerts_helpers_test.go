// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
)

const (
	testTimestamp = "2026-03-07T10:00:00Z"
	testQuery     = "error"
)

func TestSourceTypeFromRequest(t *testing.T) {
	t.Parallel()

	logType := gen.AlertRuleRequestSourceTypeLog
	metricType := gen.AlertRuleRequestSourceTypeMetric

	tests := []struct {
		name      string
		req       gen.AlertRuleRequest
		expected  string
		expectErr bool
	}{
		{
			name:      "nil source",
			req:       gen.AlertRuleRequest{},
			expectErr: true,
		},
		{
			name: "nil source type",
			req: gen.AlertRuleRequest{
				Source: &struct {
					Metric *gen.AlertRuleRequestSourceMetric `json:"metric,omitempty"`
					Query  *string                           `json:"query,omitempty"`
					Type   *gen.AlertRuleRequestSourceType   `json:"type,omitempty"`
				}{},
			},
			expectErr: true,
		},
		{
			name: "log type",
			req: gen.AlertRuleRequest{
				Source: &struct {
					Metric *gen.AlertRuleRequestSourceMetric `json:"metric,omitempty"`
					Query  *string                           `json:"query,omitempty"`
					Type   *gen.AlertRuleRequestSourceType   `json:"type,omitempty"`
				}{Type: &logType},
			},
			expected: "log",
		},
		{
			name: "metric type",
			req: gen.AlertRuleRequest{
				Source: &struct {
					Metric *gen.AlertRuleRequestSourceMetric `json:"metric,omitempty"`
					Query  *string                           `json:"query,omitempty"`
					Type   *gen.AlertRuleRequestSourceType   `json:"type,omitempty"`
				}{Type: &metricType},
			},
			expected: "metric",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sourceTypeFromRequest(tt.req)
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestStringPtrVal(t *testing.T) {
	t.Parallel()

	t.Run("nil pointer", func(t *testing.T) {
		assert.Equal(t, "", stringPtrVal(nil))
	})

	t.Run("non-nil pointer", func(t *testing.T) {
		s := "hello"
		assert.Equal(t, "hello", stringPtrVal(&s))
	})
}

func TestBuildSyncResponse(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)
	resp := buildSyncResponse("created", "my-rule", "backend-123", ts)

	require.NotNil(t, resp.Status)
	assert.Equal(t, "synced", string(*resp.Status))
	require.NotNil(t, resp.Action)
	assert.Equal(t, "created", string(*resp.Action))
	require.NotNil(t, resp.RuleLogicalId)
	assert.Equal(t, "my-rule", *resp.RuleLogicalId)
	require.NotNil(t, resp.RuleBackendId)
	assert.Equal(t, "backend-123", *resp.RuleBackendId)
	require.NotNil(t, resp.LastSyncedAt)
	assert.Equal(t, testTimestamp, *resp.LastSyncedAt)
}

func TestGenRequestToLegacyRequest(t *testing.T) {
	t.Parallel()

	t.Run("full request", func(t *testing.T) {
		logType := gen.AlertRuleRequestSourceTypeLog
		name := "rule-1"
		ns := testNamespace
		query := testQuery
		window := "5m"
		interval := "1m"
		operator := gen.AlertRuleRequestConditionOperatorGt
		threshold := float32(10)
		enabled := true

		compUID := openapi_types.UUID{0x01}
		projUID := openapi_types.UUID{0x02}
		envUID := openapi_types.UUID{0x03}

		req := gen.AlertRuleRequest{
			//nolint:revive,staticcheck
			Metadata: &struct {
				ComponentUid   *openapi_types.UUID `json:"componentUid,omitempty"`
				EnvironmentUid *openapi_types.UUID `json:"environmentUid,omitempty"`
				Name           *string             `json:"name,omitempty"`
				Namespace      *string             `json:"namespace,omitempty"`
				ProjectUid     *openapi_types.UUID `json:"projectUid,omitempty"`
			}{
				Name: &name, Namespace: &ns,
				ComponentUid: &compUID, ProjectUid: &projUID, EnvironmentUid: &envUID,
			},
			Source: &struct {
				Metric *gen.AlertRuleRequestSourceMetric `json:"metric,omitempty"`
				Query  *string                           `json:"query,omitempty"`
				Type   *gen.AlertRuleRequestSourceType   `json:"type,omitempty"`
			}{Type: &logType, Query: &query},
			Condition: &struct {
				Enabled   *bool                                  `json:"enabled,omitempty"`
				Interval  *string                                `json:"interval,omitempty"`
				Operator  *gen.AlertRuleRequestConditionOperator `json:"operator,omitempty"`
				Threshold *float32                               `json:"threshold,omitempty"`
				Window    *string                                `json:"window,omitempty"`
			}{Enabled: &enabled, Window: &window, Interval: &interval, Operator: &operator, Threshold: &threshold},
		}

		legacy := genRequestToLegacyRequest(req)
		assert.Equal(t, "rule-1", legacy.Metadata.Name)
		assert.Equal(t, testNamespace, legacy.Metadata.Namespace)
		assert.Equal(t, "log", legacy.Source.Type)
		assert.Equal(t, testQuery, legacy.Source.Query)
		assert.True(t, legacy.Condition.Enabled)
		assert.Equal(t, "5m", legacy.Condition.Window)
		assert.InDelta(t, float64(10), legacy.Condition.Threshold, 0.001)
	})

	t.Run("nil sub-fields", func(t *testing.T) {
		legacy := genRequestToLegacyRequest(gen.AlertRuleRequest{})
		assert.Equal(t, "", legacy.Metadata.Name)
		assert.Equal(t, "", legacy.Source.Type)
		assert.Equal(t, "", legacy.Condition.Window)
	})
}

func TestExtractWebhookTemplateData(t *testing.T) {
	t.Parallel()

	t.Run("valid template", func(t *testing.T) {
		monitor := opensearch.MonitorBody{
			Triggers: []opensearch.MonitorTrigger{
				{
					QueryLevelTrigger: &opensearch.MonitorTriggerQueryLevelTrigger{
						Actions: []opensearch.MonitorTriggerAction{
							{
								MessageTemplate: opensearch.MonitorMessageTemplate{
									Source: `{"ruleName":"my-rule","ruleNamespace":"ns-1","componentUid":"c-uid","projectUid":"p-uid","environmentUid":"e-uid","alertValue":{{ctx.results.0.hits.total.value}},"alertTimestamp":"{{ctx.periodStart}}"}`,
								},
							},
						},
					},
				},
			},
		}
		data := extractWebhookTemplateData(monitor)
		assert.Equal(t, "my-rule", data.RuleName)
		assert.Equal(t, "ns-1", data.RuleNamespace)
		assert.Equal(t, "c-uid", data.ComponentUID)
	})

	t.Run("empty triggers", func(t *testing.T) {
		data := extractWebhookTemplateData(opensearch.MonitorBody{})
		assert.Equal(t, "", data.RuleName)
	})

	t.Run("nil query level trigger", func(t *testing.T) {
		monitor := opensearch.MonitorBody{
			Triggers: []opensearch.MonitorTrigger{{}},
		}
		data := extractWebhookTemplateData(monitor)
		assert.Equal(t, "", data.RuleName)
	})
}

func TestExtractQueryFromMonitor(t *testing.T) {
	t.Parallel()

	t.Run("valid query with wildcard", func(t *testing.T) {
		monitor := opensearch.MonitorBody{
			Inputs: []opensearch.MonitorInput{
				{
					Search: opensearch.MonitorInputSearch{
						Query: map[string]any{
							"query": map[string]any{
								"bool": map[string]any{
									"filter": []any{
										map[string]any{
											"wildcard": map[string]any{
												"log": map[string]any{
													"wildcard": "*OutOfMemory*",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		assert.Equal(t, "OutOfMemory", extractQueryFromMonitor(monitor))
	})

	t.Run("no inputs", func(t *testing.T) {
		assert.Equal(t, "", extractQueryFromMonitor(opensearch.MonitorBody{}))
	})

	t.Run("no wildcard filter", func(t *testing.T) {
		monitor := opensearch.MonitorBody{
			Inputs: []opensearch.MonitorInput{
				{
					Search: opensearch.MonitorInputSearch{
						Query: map[string]any{
							"query": map[string]any{
								"bool": map[string]any{
									"filter": []any{
										map[string]any{
											"term": map[string]any{"field": "value"},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		assert.Equal(t, "", extractQueryFromMonitor(monitor))
	})
}

func TestExtractTriggerCondition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		script      string
		expectedOp  string
		expectedVal *float32
	}{
		{"gt", "ctx.results[0].hits.total.value > 100", "gt", ptrFloat32(100)},
		{"gte", "ctx.results[0].hits.total.value >= 50", "gte", ptrFloat32(50)},
		{"lt", "ctx.results[0].hits.total.value < 10", "lt", ptrFloat32(10)},
		{"lte", "ctx.results[0].hits.total.value <= 5", "lte", ptrFloat32(5)},
		{"eq", "ctx.results[0].hits.total.value == 0", "eq", ptrFloat32(0)},
		{"neq", "ctx.results[0].hits.total.value != 42", "neq", ptrFloat32(42)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := opensearch.MonitorBody{
				Triggers: []opensearch.MonitorTrigger{
					{
						QueryLevelTrigger: &opensearch.MonitorTriggerQueryLevelTrigger{
							Condition: opensearch.MonitorTriggerCondition{
								Script: opensearch.MonitorTriggerConditionScript{Source: tt.script},
							},
						},
					},
				},
			}
			op, val := extractTriggerCondition(monitor)
			assert.Equal(t, tt.expectedOp, op)
			if tt.expectedVal != nil {
				require.NotNil(t, val)
				assert.Equal(t, *tt.expectedVal, *val)
			}
		})
	}

	t.Run("no triggers", func(t *testing.T) {
		op, val := extractTriggerCondition(opensearch.MonitorBody{})
		assert.Equal(t, "", op)
		assert.Nil(t, val)
	})
}

func TestExtractWindowFromQuery(t *testing.T) {
	t.Parallel()

	t.Run("valid window", func(t *testing.T) {
		monitor := opensearch.MonitorBody{
			Inputs: []opensearch.MonitorInput{
				{
					Search: opensearch.MonitorInputSearch{
						Query: map[string]any{
							"query": map[string]any{
								"bool": map[string]any{
									"filter": []any{
										map[string]any{
											"range": map[string]any{
												"@timestamp": map[string]any{
													"from": "{{period_end}}||-5m",
													"to":   "{{period_end}}",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		assert.Equal(t, "5m", extractWindowFromQuery(monitor))
	})

	t.Run("no inputs", func(t *testing.T) {
		assert.Equal(t, "", extractWindowFromQuery(opensearch.MonitorBody{}))
	})

	t.Run("no range filter", func(t *testing.T) {
		monitor := opensearch.MonitorBody{
			Inputs: []opensearch.MonitorInput{
				{
					Search: opensearch.MonitorInputSearch{
						Query: map[string]any{
							"query": map[string]any{
								"bool": map[string]any{
									"filter": []any{},
								},
							},
						},
					},
				},
			},
		}
		assert.Equal(t, "", extractWindowFromQuery(monitor))
	})
}

func TestExtractBoolFilters(t *testing.T) {
	t.Parallel()

	t.Run("valid query", func(t *testing.T) {
		q := map[string]any{
			"query": map[string]any{
				"bool": map[string]any{
					"filter": []any{
						map[string]any{"term": "a"},
						map[string]any{"term": "b"},
					},
				},
			},
		}
		assert.Len(t, extractBoolFilters(q), 2)
	})

	t.Run("missing query key", func(t *testing.T) {
		assert.Nil(t, extractBoolFilters(map[string]any{}))
	})

	t.Run("missing bool key", func(t *testing.T) {
		assert.Nil(t, extractBoolFilters(map[string]any{"query": map[string]any{}}))
	})

	t.Run("missing filter key", func(t *testing.T) {
		assert.Nil(t, extractBoolFilters(map[string]any{
			"query": map[string]any{"bool": map[string]any{}},
		}))
	})
}

func TestFormatMinutesDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    float64
		expected string
	}{
		{5, "5m0s"},
		{60, "1h0m0s"},
		{1.5, "1m30s"},
		{0, "0s"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, formatMinutesDuration(tt.input))
	}
}

func TestExtractPromLabelValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expr     string
		label    string
		expected string
	}{
		{
			name:     "found",
			expr:     `rate(container_cpu_usage_seconds_total{label_openchoreo_dev_component_uid="abc-123"}[5m])`,
			label:    "label_openchoreo_dev_component_uid",
			expected: "abc-123",
		},
		{
			name:     "not found",
			expr:     `rate(container_cpu_usage_seconds_total[5m])`,
			label:    "label_openchoreo_dev_component_uid",
			expected: "",
		},
		{
			name:     "malformed no closing quote",
			expr:     `label_openchoreo_dev_component_uid="abc`,
			label:    "label_openchoreo_dev_component_uid",
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractPromLabelValue(tt.expr, tt.label))
		})
	}
}

func TestDetectMetricType(t *testing.T) {
	t.Parallel()

	t.Run("cpu", func(t *testing.T) {
		assert.Equal(t, gen.AlertRuleResponseSourceMetricCpuUsage, detectMetricType("rate(container_cpu_usage_seconds_total[5m]) * 100 >= 80"))
	})

	t.Run("memory (default)", func(t *testing.T) {
		assert.Equal(t, gen.AlertRuleResponseSourceMetricMemoryUsage, detectMetricType("container_memory_working_set_bytes > 1000000"))
	})

	t.Run("unknown defaults to memory", func(t *testing.T) {
		assert.Equal(t, gen.AlertRuleResponseSourceMetricMemoryUsage, detectMetricType("some_custom_metric > 5"))
	})
}

func TestExtractPromOperatorAndThreshold(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		expr        string
		expectedOp  string
		expectedVal *float32
	}{
		{"gt", "metric > 80", "gt", ptrFloat32(80)},
		{"gte", "metric >= 80", "gte", ptrFloat32(80)},
		{"lt", "metric < 10", "lt", ptrFloat32(10)},
		{"lte", "metric <= 5", "lte", ptrFloat32(5)},
		{"eq", "metric == 0", "eq", ptrFloat32(0)},
		{"neq", "metric != 42", "neq", ptrFloat32(42)},
		{"none", "metric", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op, val := extractPromOperatorAndThreshold(tt.expr)
			assert.Equal(t, tt.expectedOp, op)
			if tt.expectedVal == nil {
				assert.Nil(t, val)
			} else {
				require.NotNil(t, val)
				assert.Equal(t, *tt.expectedVal, *val)
			}
		})
	}
}

func TestMonitorsAreEqual(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("equal monitors", func(t *testing.T) {
		m := map[string]any{"name": "rule-1", "type": "monitor", "enabled": true}
		assert.True(t, monitorsAreEqual(logger, m, m))
	})

	t.Run("different monitors", func(t *testing.T) {
		m1 := map[string]any{"name": "rule-1", "type": "monitor", "enabled": true}
		m2 := map[string]any{"name": "rule-2", "type": "monitor", "enabled": false}
		assert.False(t, monitorsAreEqual(logger, m1, m2))
	})
}

func TestMapMonitorToAlertRuleResponse(t *testing.T) {
	t.Parallel()

	t.Run("full monitor", func(t *testing.T) {
		// Build a realistic monitor via JSON round-trip
		monitor := opensearch.MonitorBody{
			Name:    "test-rule",
			Enabled: true,
			Schedule: opensearch.MonitorSchedule{
				Period: opensearch.MonitorSchedulePeriod{Interval: 1, Unit: "MINUTES"},
			},
			Inputs: []opensearch.MonitorInput{
				{
					Search: opensearch.MonitorInputSearch{
						Indices: []string{"logs-*"},
						Query: map[string]any{
							"query": map[string]any{
								"bool": map[string]any{
									"filter": []any{
										map[string]any{
											"range": map[string]any{
												"@timestamp": map[string]any{
													"from": "{{period_end}}||-5m",
													"to":   "{{period_end}}",
												},
											},
										},
										map[string]any{
											"wildcard": map[string]any{
												"log": map[string]any{
													"wildcard": "*error*",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			Triggers: []opensearch.MonitorTrigger{
				{
					QueryLevelTrigger: &opensearch.MonitorTriggerQueryLevelTrigger{
						Name: "trigger-test-rule",
						Condition: opensearch.MonitorTriggerCondition{
							Script: opensearch.MonitorTriggerConditionScript{
								Source: "ctx.results[0].hits.total.value > 10",
							},
						},
						Actions: []opensearch.MonitorTriggerAction{
							{
								MessageTemplate: opensearch.MonitorMessageTemplate{
									Source: `{"ruleName":"test-rule","ruleNamespace":"ns-1","componentUid":"comp","projectUid":"proj","environmentUid":"env","alertValue":{{ctx.results.0.hits.total.value}},"alertTimestamp":"{{ctx.periodStart}}"}`,
								},
							},
						},
					},
				},
			},
		}

		// Marshal to map for the function input
		b, _ := json.Marshal(monitor)
		var raw map[string]any
		_ = json.Unmarshal(b, &raw)

		resp, err := mapMonitorToAlertRuleResponse(raw, "test-rule")
		require.NoError(t, err)
		require.NotNil(t, resp.Metadata)
		assert.Equal(t, "test-rule", *resp.Metadata.Name)
		require.NotNil(t, resp.Source)
		assert.Equal(t, testQuery, *resp.Source.Query)
		require.NotNil(t, resp.Condition)
		require.NotNil(t, resp.Condition.Operator)
		assert.Equal(t, "gt", string(*resp.Condition.Operator))
		require.NotNil(t, resp.Condition.Threshold)
		assert.Equal(t, float32(10), *resp.Condition.Threshold)
		require.NotNil(t, resp.Condition.Window)
		assert.Equal(t, "5m", *resp.Condition.Window)
	})

	t.Run("empty monitor", func(t *testing.T) {
		raw := map[string]any{}
		resp, err := mapMonitorToAlertRuleResponse(raw, "empty")
		require.NoError(t, err)
		require.NotNil(t, resp.Metadata)
		assert.Equal(t, "empty", *resp.Metadata.Name)
	})
}

func TestValidateAlertDurations(t *testing.T) {
	t.Parallel()

	t.Run("both nil", func(t *testing.T) {
		require.Error(t, validateAlertDurations(nil, nil))
	})

	t.Run("valid values", func(t *testing.T) {
		interval := "1m"
		window := "5m"
		require.NoError(t, validateAlertDurations(&interval, &window))
	})

	t.Run("invalid window", func(t *testing.T) {
		interval := "1m"
		window := "30s"
		require.Error(t, validateAlertDurations(&interval, &window))
	})

	t.Run("invalid interval", func(t *testing.T) {
		interval := "30s"
		window := "5m"
		require.Error(t, validateAlertDurations(&interval, &window))
	})
}

func ptrFloat32(v float32) *float32 {
	return &v
}
