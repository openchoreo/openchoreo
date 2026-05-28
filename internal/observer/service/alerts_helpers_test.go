// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
)

const (
	testTimestamp = "2026-03-07T10:00:00Z"
	testQuery     = "error"
)

func TestSourceTypeFromRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		req       gen.AlertRuleRequest
		expected  string
		expectErr bool
	}{
		{
			name:      "empty source type",
			req:       gen.AlertRuleRequest{},
			expectErr: true,
		},
		{
			name: "log type",
			req: gen.AlertRuleRequest{
				Source: struct {
					Metric *gen.AlertRuleRequestSourceMetric `json:"metric,omitempty"`
					Query  *string                           `json:"query,omitempty"`
					Type   gen.AlertRuleRequestSourceType    `json:"type"`
				}{Type: gen.AlertRuleRequestSourceTypeLog},
			},
			expected: "log",
		},
		{
			name: "metric type",
			req: gen.AlertRuleRequest{
				Source: struct {
					Metric *gen.AlertRuleRequestSourceMetric `json:"metric,omitempty"`
					Query  *string                           `json:"query,omitempty"`
					Type   gen.AlertRuleRequestSourceType    `json:"type"`
				}{Type: gen.AlertRuleRequestSourceTypeMetric},
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
		query := testQuery

		compUID := openapi_types.UUID{0x01}
		projUID := openapi_types.UUID{0x02}
		envUID := openapi_types.UUID{0x03}

		req := gen.AlertRuleRequest{
			//nolint:revive,staticcheck
			Metadata: struct {
				ComponentUid   openapi_types.UUID `json:"componentUid"`
				EnvironmentUid openapi_types.UUID `json:"environmentUid"`
				Name           string             `json:"name"`
				Namespace      string             `json:"namespace"`
				ProjectUid     openapi_types.UUID `json:"projectUid"`
			}{
				Name: "rule-1", Namespace: testNamespace,
				ComponentUid: compUID, ProjectUid: projUID, EnvironmentUid: envUID,
			},
			Source: struct {
				Metric *gen.AlertRuleRequestSourceMetric `json:"metric,omitempty"`
				Query  *string                           `json:"query,omitempty"`
				Type   gen.AlertRuleRequestSourceType    `json:"type"`
			}{Type: gen.AlertRuleRequestSourceTypeLog, Query: &query},
			Condition: struct {
				Enabled   bool                                  `json:"enabled"`
				Interval  string                                `json:"interval"`
				Operator  gen.AlertRuleRequestConditionOperator `json:"operator"`
				Threshold float32                               `json:"threshold"`
				Window    string                                `json:"window"`
			}{Enabled: true, Window: "5m", Interval: "1m", Operator: gen.AlertRuleRequestConditionOperatorGt, Threshold: float32(10)},
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

	t.Run("zero-value sub-fields", func(t *testing.T) {
		legacy := genRequestToLegacyRequest(gen.AlertRuleRequest{})
		assert.Equal(t, "", legacy.Metadata.Name)
		assert.Equal(t, "", legacy.Source.Type)
		assert.Equal(t, "", legacy.Condition.Window)
	})
}

func TestValidateAlertDurations(t *testing.T) {
	t.Parallel()

	t.Run("both empty", func(t *testing.T) {
		require.Error(t, validateAlertDurations("", ""))
	})

	t.Run("valid values", func(t *testing.T) {
		require.NoError(t, validateAlertDurations("1m", "5m"))
	})

	t.Run("invalid window", func(t *testing.T) {
		require.Error(t, validateAlertDurations("1m", "30s"))
	})

	t.Run("invalid interval", func(t *testing.T) {
		require.Error(t, validateAlertDurations("30s", "5m"))
	})
}
