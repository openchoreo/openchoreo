// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package types

// AlertingRuleRequest represents the request body for PUT /api/alerting/rule/{ruleName}
type AlertingRuleRequest struct {
	Metadata  AlertingRuleMetadata  `json:"metadata"`
	Source    AlertingRuleSource    `json:"source"`
	Condition AlertingRuleCondition `json:"condition"`
}

// AlertingRuleMetadata contains metadata about an alerting rule
type AlertingRuleMetadata struct {
	Name                      string `json:"name"`
	ComponentUID              string `json:"component-uid"`
	ProjectUID                string `json:"project-uid"`
	EnvironmentUID            string `json:"environment-uid"`
	Severity                  string `json:"severity"`
	EnableAiRootCauseAnalysis bool   `json:"enableAiRootCauseAnalysis"`
}

// AlertingRuleSource defines the source of data for the alerting rule
type AlertingRuleSource struct {
	Type  string `json:"type"`
	Query string `json:"query"` // For log-based alert rules
	// TODO: Add Metric field for metric-based alert rules
}

// AlertingRuleCondition defines the condition that triggers the alert
type AlertingRuleCondition struct {
	Enabled   bool    `json:"enabled"`
	Window    string  `json:"window"`
	Interval  string  `json:"interval"`
	Operator  string  `json:"operator"`
	Threshold float64 `json:"threshold"`
}

// AlertingRuleSyncResponse captures the result of an alert rule upsert.
type AlertingRuleSyncResponse struct {
	Status     string `json:"status"`
	LogicalID  string `json:"logicalId"`
	BackendID  string `json:"backendId"`
	Action     string `json:"action"`
	LastSynced string `json:"lastSynced"`
}
