// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package api

// AnalyzeRequest is the request body for POST /api/v1alpha1/rca-agent/analyze.
type AnalyzeRequest struct {
	Namespace   string         `json:"namespace"`
	Project     string         `json:"project"`
	Component   string         `json:"component"`
	Environment string         `json:"environment"`
	Alert       AlertContext   `json:"alert"`
	Meta        map[string]any `json:"meta,omitempty"`
}

// AlertContext describes the alert that triggered the analysis.
type AlertContext struct {
	ID        string        `json:"id"`
	Value     any           `json:"value"`
	Timestamp string        `json:"timestamp"`
	Rule      AlertRuleInfo `json:"rule"`
}

// AlertRuleInfo describes the alert rule.
type AlertRuleInfo struct {
	Name        string              `json:"name"`
	Description *string             `json:"description,omitempty"`
	Severity    *string             `json:"severity,omitempty"`
	Source      *AlertRuleSource    `json:"source,omitempty"`
	Condition   *AlertRuleCondition `json:"condition,omitempty"`
}

// AlertRuleSource describes the alert rule source.
type AlertRuleSource struct {
	Type   string  `json:"type"`
	Query  *string `json:"query,omitempty"`
	Metric *string `json:"metric,omitempty"`
}

// AlertRuleCondition describes the alert rule condition.
type AlertRuleCondition struct {
	Window    string `json:"window"`
	Interval  string `json:"interval"`
	Operator  string `json:"operator"`
	Threshold int    `json:"threshold"`
}

// RCAResponse is the response for a triggered analysis.
type RCAResponse struct {
	ReportID string `json:"reportId"`
	Status   string `json:"status"`
}

// ChatRequest is the request body for POST /api/v1alpha1/rca-agent/chat.
type ChatRequest struct {
	ReportID    string        `json:"reportId"`
	Namespace   string        `json:"namespace"`
	Project     string        `json:"project"`
	Environment string        `json:"environment"`
	Messages    []ChatMessage `json:"messages"`
}

// ChatMessage is a single message in a chat conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// StreamEvent is a streaming NDJSON event.
type StreamEvent struct {
	Type       string `json:"type"`
	Content    string `json:"content,omitempty"`
	Tool       string `json:"tool,omitempty"`
	ActiveForm string `json:"activeForm,omitempty"`
	Args       string `json:"args,omitempty"`
	Actions    []any  `json:"actions,omitempty"`
	Message    string `json:"message,omitempty"`
}

// RCAReportsResponse is the response for listing reports.
type RCAReportsResponse struct {
	Reports    []RCAReportSummary `json:"reports"`
	TotalCount int                `json:"totalCount"`
}

// RCAReportSummary is a summary of an RCA report.
type RCAReportSummary struct {
	AlertID   string  `json:"alertId"`
	ReportID  string  `json:"reportId"`
	Timestamp string  `json:"timestamp"`
	Summary   *string `json:"summary"`
	Status    string  `json:"status"`
}

// RCAReportDetailed is the full RCA report response.
type RCAReportDetailed struct {
	AlertID   string     `json:"alertId"`
	ReportID  string     `json:"reportId"`
	Timestamp string     `json:"timestamp"`
	Status    string     `json:"status"`
	Report    *RCAReport `json:"report,omitempty"`
}

// RCAReport is the full RCA report content.
type RCAReport struct {
	AlertContext      ReportAlertContext  `json:"alert_context"`
	Summary           string              `json:"summary"`
	Result            any                 `json:"result"`
	InvestigationPath []InvestigationStep `json:"investigation_path"`
}

// ReportAlertContext is the alert context as stored in the report.
type ReportAlertContext struct {
	AlertID          string                `json:"alert_id"`
	AlertName        string                `json:"alert_name"`
	AlertDescription *string               `json:"alert_description,omitempty"`
	Severity         *string               `json:"severity,omitempty"`
	TriggeredAt      string                `json:"triggered_at"`
	TriggerValue     float64               `json:"trigger_value"`
	SourceType       *string               `json:"source_type,omitempty"`
	SourceQuery      *string               `json:"source_query,omitempty"`
	SourceMetric     *string               `json:"source_metric,omitempty"`
	Condition        *ReportAlertCondition `json:"condition,omitempty"`
	Component        string                `json:"component"`
	Project          string                `json:"project"`
	Environment      string                `json:"environment"`
}

// ReportAlertCondition is the alert condition as stored in the report.
type ReportAlertCondition struct {
	Window    string `json:"window"`
	Interval  string `json:"interval"`
	Operator  string `json:"operator"`
	Threshold int    `json:"threshold"`
}

// RootCauseIdentified is the result when root causes are found.
type RootCauseIdentified struct {
	Type            string           `json:"type"`
	RootCauses      []RootCause      `json:"root_causes"`
	Timeline        []TimelineEvent  `json:"timeline"`
	ExcludedCauses  []ExcludedCause  `json:"excluded_causes,omitempty"`
	Recommendations *Recommendations `json:"recommendations"`
}

// NoRootCauseIdentified is the result when no root cause is found.
type NoRootCauseIdentified struct {
	Type            string           `json:"type"`
	Outcome         string           `json:"outcome"`
	Explanation     string           `json:"explanation"`
	Recommendations *Recommendations `json:"recommendations,omitempty"`
}

// RootCause describes a single root cause.
type RootCause struct {
	Summary            string    `json:"summary"`
	Confidence         string    `json:"confidence"`
	Analysis           string    `json:"analysis"`
	SupportingFindings []Finding `json:"supporting_findings"`
}

// Finding is a piece of evidence supporting a root cause.
type Finding struct {
	Observation string    `json:"observation"`
	Component   string    `json:"component"`
	TimeRange   TimeRange `json:"time_range"`
	Evidence    any       `json:"evidence"`
}

// TimeRange is a time range.
type TimeRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// LogEvidence is log-based evidence.
type LogEvidence struct {
	Type       string    `json:"type"`
	LogLines   []LogLine `json:"log_lines"`
	Repetition *string   `json:"repetition,omitempty"`
}

// LogLine is a single log line.
type LogLine struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Log       string `json:"log"`
}

// MetricEvidence is metric-based evidence.
type MetricEvidence struct {
	Type    string `json:"type"`
	Summary string `json:"summary"`
}

// TraceEvidence is trace-based evidence.
type TraceEvidence struct {
	Type         string  `json:"type"`
	TraceID      string  `json:"trace_id"`
	SpanID       *string `json:"span_id,omitempty"`
	Summary      string  `json:"summary"`
	IsError      bool    `json:"is_error"`
	ErrorMessage *string `json:"error_message,omitempty"`
	Repetition   *string `json:"repetition,omitempty"`
}

// TimelineEvent is a chronological event.
type TimelineEvent struct {
	Timestamp string  `json:"timestamp"`
	Component *string `json:"component,omitempty"`
	Event     string  `json:"event"`
}

// InvestigationStep is a step taken during analysis.
type InvestigationStep struct {
	Action    string  `json:"action"`
	Outcome   string  `json:"outcome"`
	Rationale *string `json:"rationale,omitempty"`
}

// ExcludedCause is a ruled-out cause.
type ExcludedCause struct {
	Description string `json:"description"`
	Rationale   string `json:"rationale"`
}

// Recommendations holds recommended and observability actions.
type Recommendations struct {
	RecommendedActions           []RecommendedAction `json:"recommended_actions,omitempty"`
	ObservabilityRecommendations []Action            `json:"observability_recommendations,omitempty"`
}

// Action is a generic action.
type Action struct {
	Description string  `json:"description"`
	Rationale   *string `json:"rationale,omitempty"`
}

// RecommendedAction is an action with status and optional resource change.
type RecommendedAction struct {
	Description string          `json:"description"`
	Rationale   *string         `json:"rationale,omitempty"`
	Status      string          `json:"status"`
	Change      *ResourceChange `json:"change,omitempty"`
}

// ResourceChange describes a change to a resource.
type ResourceChange struct {
	ReleaseBinding string         `json:"release_binding"`
	Env            []EnvVarChange `json:"env,omitempty"`
	Files          []FileChange   `json:"files,omitempty"`
	Fields         []FieldChange  `json:"fields,omitempty"`
}

// EnvVarChange describes an environment variable change.
type EnvVarChange struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// FileChange describes a file change.
type FileChange struct {
	Key       string `json:"key"`
	MountPath string `json:"mount_path"`
	Value     string `json:"value"`
}

// FieldChange describes a field change.
type FieldChange struct {
	JSONPointer string `json:"json_pointer"`
	Value       any    `json:"value"`
}

// ReportUpdateRequest is the request body for PUT /api/v1/rca-agent/reports/{report_id}.
type ReportUpdateRequest struct {
	AppliedIndices   []int `json:"appliedIndices,omitempty"`
	DismissedIndices []int `json:"dismissedIndices,omitempty"`
}

// StatusResponse is a generic status response.
type StatusResponse struct {
	Status string `json:"status"`
}

// ErrorResponse is the standard error response.
type ErrorResponse struct {
	Detail string `json:"detail"`
}
