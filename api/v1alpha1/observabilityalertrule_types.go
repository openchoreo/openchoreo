// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ObservabilityAlertSeverity describes the seriousness of an alert.
type ObservabilityAlertSeverity string

const (
	// ObservabilityAlertSeverityInfo indicates informational alerts.
	ObservabilityAlertSeverityInfo ObservabilityAlertSeverity = "info"
	// ObservabilityAlertSeverityWarning indicates warning-level alerts.
	ObservabilityAlertSeverityWarning ObservabilityAlertSeverity = "warning"
	// ObservabilityAlertSeverityCritical indicates critical alerts.
	ObservabilityAlertSeverityCritical ObservabilityAlertSeverity = "critical"
)

// ObservabilityAlertSourceType identifies the origin of the telemetry data.
// +kubebuilder:validation:Enum=log;metric;budget
type ObservabilityAlertSourceType string

const (
	// ObservabilityAlertSourceTypeLog represents log-based alerting.
	ObservabilityAlertSourceTypeLog ObservabilityAlertSourceType = "log"
	// ObservabilityAlertSourceTypeMetric represents metric-based alerting.
	ObservabilityAlertSourceTypeMetric ObservabilityAlertSourceType = "metric"
	// ObservabilityAlertSourceTypeBudget represents budget-based alerting.
	ObservabilityAlertSourceTypeBudget ObservabilityAlertSourceType = "budget"
)

// ObservabilityAlertConditionOperator describes how a computed signal is evaluated.
// +kubebuilder:validation:Enum=gt;lt;gte;lte;eq
type ObservabilityAlertConditionOperator string

const (
	// ObservabilityAlertConditionOperatorGt triggers when value is greater than threshold.
	ObservabilityAlertConditionOperatorGt ObservabilityAlertConditionOperator = "gt"
	// ObservabilityAlertConditionOperatorLt triggers when value is less than threshold.
	ObservabilityAlertConditionOperatorLt ObservabilityAlertConditionOperator = "lt"
	// ObservabilityAlertConditionOperatorGte triggers when value is greater than or equal to threshold.
	ObservabilityAlertConditionOperatorGte ObservabilityAlertConditionOperator = "gte"
	// ObservabilityAlertConditionOperatorLte triggers when value is less than or equal to threshold.
	ObservabilityAlertConditionOperatorLte ObservabilityAlertConditionOperator = "lte"
	// ObservabilityAlertConditionOperatorEq triggers when value equals the threshold value.
	ObservabilityAlertConditionOperatorEq ObservabilityAlertConditionOperator = "eq"
)

// ObservabilityAlertSource describes where and how events are pulled for evaluation.
type ObservabilityAlertSource struct {
	// Type specifies the telemetry source type (log, metric).
	// +kubebuilder:validation:Required
	Type ObservabilityAlertSourceType `json:"type"`

	// Query defines the query or filter to locate relevant events.
	// This is required for log-based alerting.
	// +optional
	Query string `json:"query,omitempty"`

	// Metric specifies the metric to alert on.
	// This is required for metric-based alerting.
	// +optional
	Metric string `json:"metric,omitempty"`
}

// ObservabilityAlertCondition represents the evaluation window of the alert.
type ObservabilityAlertCondition struct {
	// Window is the time window that is aggregated before a comparison is made.
	// +kubebuilder:validation:Required
	Window metav1.Duration `json:"window"`

	// Interval dictates how often the alert rule is evaluated.
	// +kubebuilder:validation:Required
	Interval metav1.Duration `json:"interval"`

	// Operator describes the comparison used when evaluating the threshold.
	// +kubebuilder:validation:Required
	Operator ObservabilityAlertConditionOperator `json:"operator"`

	// Threshold is the trigger value for the configured operator.
	// +kubebuilder:validation:Required
	Threshold int64 `json:"threshold"`
}

// ObservabilityAlertRuleSpec defines the desired state of ObservabilityAlertRule.
type ObservabilityAlertRuleSpec struct {
	// Name identifies the alert rule when defined as a Trait.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Description is a human-friendly summary of the alert rule.
	// +optional
	Description string `json:"description,omitempty"`

	// Severity describes how urgent the alert is.
	// +optional
	Severity ObservabilityAlertSeverity `json:"severity,omitempty"`

	// Enabled toggles whether this alert rule should be evaluated.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Source specifies the origin and query that drives the rule.
	// +kubebuilder:validation:Required
	Source ObservabilityAlertSource `json:"source"`

	// Condition controls how the rule should be evaluated against the source.
	// +kubebuilder:validation:Required
	Condition ObservabilityAlertCondition `json:"condition"`

	// Actions defines the actions to take when the alert rule is triggered.
	// +kubebuilder:validation:Required
	Actions ObservabilityAlertActions `json:"actions"`
}

// ObservabilityAlertActions defines the actions to take when the alert rule is triggered.
type ObservabilityAlertActions struct {
	// Notifications defines the notifications to send when the alert rule is triggered.
	// +kubebuilder:validation:Required
	Notifications ObservabilityAlertNotifications `json:"notifications"`

	// Incident defines the incident to trigger when the alert rule is triggered.
	// +optional
	Incident *ObservabilityAlertIncident `json:"incident,omitempty"`
}

// ObservabilityAlertNotifications defines the notifications to send when the alert rule is triggered.
type ObservabilityAlertNotifications struct {
	// Channels defines the channels to send notifications to.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +listType=set
	Channels []NotificationChannelName `json:"channels"`
}

// NotificationChannelName defines a non-empty notification channel identifier.
// +kubebuilder:validation:MinLength=1
type NotificationChannelName string

// ObservabilityAlertIncident defines the incident to trigger when the alert rule is triggered.
// +kubebuilder:validation:XValidation:rule="self.triggerAiRca == true ? self.enabled == true : true",message="Incident must be enabled to trigger AI RCA"
// +kubebuilder:validation:XValidation:rule="self.triggerAiCostAnalysis == true ? self.enabled == true : true",message="Incident must be enabled to trigger AI cost analysis"
type ObservabilityAlertIncident struct {
	// Enabled toggles whether an incident should be triggered when the alert rule is triggered.
	// +optional
	// +kubebuilder:default:=false
	Enabled *bool `json:"enabled,omitempty"`

	// TriggerAiRca toggles whether an AI root cause analysis should be triggered when an incident is created.
	// To set this to true, Enabled must also be set to true.
	// +optional
	// +kubebuilder:default:=false
	TriggerAiRca *bool `json:"triggerAiRca,omitempty"`

	// TriggerAiCostAnalysis toggles whether an AI cost analysis should be triggered when an incident is created.
	// To set this to true, Enabled must also be set to true.
	// +optional
	// +kubebuilder:default:=false
	TriggerAiCostAnalysis *bool `json:"triggerAiCostAnalysis,omitempty"`
}

// ObservabilityAlertRulePhase represents the current phase of the rule.
type ObservabilityAlertRulePhase string

const (
	// ObservabilityAlertRulePhasePending means the rule is being reconciled.
	ObservabilityAlertRulePhasePending ObservabilityAlertRulePhase = "Pending"
	// ObservabilityAlertRulePhaseReady means the rule is active and ready.
	ObservabilityAlertRulePhaseReady ObservabilityAlertRulePhase = "Ready"
	// ObservabilityAlertRulePhaseError means the rule encountered an error.
	ObservabilityAlertRulePhaseError ObservabilityAlertRulePhase = "Error"
)

// ObservabilityAlertRuleStatus defines the observed state of ObservabilityAlertRule.
type ObservabilityAlertRuleStatus struct {
	// ObservedGeneration represents the .metadata.generation that the controller last handled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase indicates the current lifecycle phase.
	// +optional
	Phase ObservabilityAlertRulePhase `json:"phase,omitempty"`

	// LastReconcileTime records the last time the controller reconciled this object.
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// LastSyncTime records when the rule was pushed to the backend observability tool.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// BackendMonitorID is the identifier assigned by the backend observability tool.
	// +optional
	BackendMonitorID string `json:"backendMonitorId,omitempty"`

	// Conditions describe the latest observations of the rule's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Severity",type=string,JSONPath=`.spec.severity`
// +kubebuilder:printcolumn:name="Enabled",type=string,JSONPath=`.spec.enabled`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ObservabilityAlertRule is the Schema for the observabilityalertrules API.
type ObservabilityAlertRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ObservabilityAlertRuleSpec   `json:"spec,omitempty"`
	Status ObservabilityAlertRuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ObservabilityAlertRuleList contains a list of ObservabilityAlertRule.
type ObservabilityAlertRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ObservabilityAlertRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ObservabilityAlertRule{}, &ObservabilityAlertRuleList{})
}
