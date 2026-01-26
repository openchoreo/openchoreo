// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package types

import "encoding/json"

// AlertingRuleRequest represents the request body for PUT /api/alerting/rule/{sourceType}/{ruleName}
type AlertingRuleRequest struct {
	Metadata  AlertingRuleMetadata  `json:"metadata"`
	Source    AlertingRuleSource    `json:"source"`
	Condition AlertingRuleCondition `json:"condition"`
}

// AlertingRuleMetadata contains metadata about an alerting rule
type AlertingRuleMetadata struct {
	Name                      string `json:"name"`
	Namespace                 string `json:"namespace"`
	ComponentUID              string `json:"component-uid"`
	ProjectUID                string `json:"project-uid"`
	EnvironmentUID            string `json:"environment-uid"`
	Severity                  string `json:"severity"`
	NotificationChannel       string `json:"notificationChannel"`
	EnableAiRootCauseAnalysis bool   `json:"enableAiRootCauseAnalysis"`
}

// AlertingRuleSource defines the source of data for the alerting rule
type AlertingRuleSource struct {
	Type   string `json:"type"`
	Query  string `json:"query"`  // For log-based alert rules
	Metric string `json:"metric"` // For metric-based alert rules (e.g., "cpu_usage", "memory_usage")
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

// AlertDetails represents alert information used for notifications and CEL template rendering
type AlertDetails struct {
	// Alert information
	AlertName                       string `json:"alertName"`
	AlertTimestamp                  string `json:"alertTimestamp"`
	AlertSeverity                   string `json:"alertSeverity"`
	AlertDescription                string `json:"alertDescription"`
	AlertThreshold                  string `json:"alertThreshold"`
	AlertValue                      string `json:"alertValue"`
	AlertType                       string `json:"alertType"`
	AlertAIRootCauseAnalysisEnabled bool   `json:"alertAIRootCauseAnalysisEnabled"`

	// Component information
	Component     string `json:"component"`
	Project       string `json:"project"`
	Environment   string `json:"environment"`
	ComponentID   string `json:"componentId"`
	ProjectID     string `json:"projectId"`
	EnvironmentID string `json:"environmentId"`

	// Notification channel name
	NotificationChannel string `json:"notificationChannel,omitempty"`
}

// ToMap converts AlertDetails to a map for CEL template rendering.
// Uses JSON marshaling to automatically convert struct fields to map keys based on JSON tags.
func (a *AlertDetails) ToMap() map[string]interface{} {
	// Marshal struct to JSON
	jsonBytes, err := json.Marshal(a)
	if err != nil {
		// Fallback to empty map if marshaling fails (shouldn't happen)
		return make(map[string]interface{})
	}

	// Unmarshal JSON into map - this automatically uses JSON tag names as keys
	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		// Fallback to empty map if unmarshaling fails (shouldn't happen)
		return make(map[string]interface{})
	}

	return result
}
