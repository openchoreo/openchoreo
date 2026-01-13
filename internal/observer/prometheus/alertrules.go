// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"fmt"
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/openchoreo/openchoreo/internal/observer/types"
)

const (
	// MetricTypeCPUUsage is the metric type for CPU usage alerts
	MetricTypeCPUUsage = "cpu_usage"
	// MetricTypeMemoryUsage is the metric type for memory usage alerts
	MetricTypeMemoryUsage = "memory_usage"
)

// AlertRuleBuilder builds PrometheusRule CRs from alerting rule requests
type AlertRuleBuilder struct {
	namespace string
}

// NewAlertRuleBuilder creates a new AlertRuleBuilder
func NewAlertRuleBuilder(namespace string) *AlertRuleBuilder {
	return &AlertRuleBuilder{
		namespace: namespace,
	}
}

// BuildPrometheusRule builds a PrometheusRule CR from an alerting rule request
func (b *AlertRuleBuilder) BuildPrometheusRule(rule types.AlertingRuleRequest) (*monitoringv1.PrometheusRule, error) {
	// Validate metric type
	metricType := rule.Source.Metric
	if metricType != MetricTypeCPUUsage && metricType != MetricTypeMemoryUsage {
		return nil, fmt.Errorf("unsupported metric type: %s (supported: %s, %s)", metricType, MetricTypeCPUUsage, MetricTypeMemoryUsage)
	}

	// Build the PromQL expression based on metric type
	expr, err := b.buildAlertExpression(rule)
	if err != nil {
		return nil, fmt.Errorf("failed to build alert expression: %w", err)
	}

	// Parse duration for the alert window (used as 'for' duration)
	forDuration, err := parseDuration(rule.Condition.Window)
	if err != nil {
		return nil, fmt.Errorf("failed to parse window duration: %w", err)
	}

	// Build alert annotations
	alertAnnotations := map[string]string{
		"rule_name":      rule.Metadata.Name,
		"rule_namespace": rule.Metadata.Namespace,
		"alert_value":    "{{ $value | printf \"%.2f\" }}",
	}

	alertLabels := map[string]string{
		"openchoreo_alert": "true", // Simple label for routing alerts to openchoreo observer alerting webhook
	}
	// Convert durations to monitoringv1.Duration pointers
	interval := monitoringv1.Duration(rule.Condition.Interval)
	forDur := monitoringv1.Duration(forDuration)

	// Create the PrometheusRule CR
	prometheusRule := &monitoringv1.PrometheusRule{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "monitoring.coreos.com/v1",
			Kind:       "PrometheusRule",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      rule.Metadata.Name,
			Namespace: b.namespace,
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{
					Name:     rule.Metadata.Name,
					Interval: &interval,
					Rules: []monitoringv1.Rule{
						{
							Alert:       rule.Metadata.Name,
							Expr:        intstr.FromString(expr),
							For:         &forDur,
							Annotations: alertAnnotations,
							Labels:      alertLabels,
						},
					},
				},
			},
		},
	}

	return prometheusRule, nil
}

// buildAlertExpression builds the PromQL alert expression based on the metric type
func (b *AlertRuleBuilder) buildAlertExpression(rule types.AlertingRuleRequest) (string, error) {
	// Build the comparison expression based on the operator
	operator, err := convertOperator(rule.Condition.Operator)
	if err != nil {
		return "", err
	}

	// Build alert-specific PromQL expressions (simpler than dashboard queries)
	var expr string
	switch rule.Source.Metric {
	case MetricTypeCPUUsage:
		expr = buildCPUUsageAlertExpression(rule.Metadata.ComponentUID, rule.Metadata.ProjectUID, rule.Metadata.EnvironmentUID, operator, rule.Condition.Threshold)
	case MetricTypeMemoryUsage:
		expr = buildMemoryUsageAlertExpression(rule.Metadata.ComponentUID, rule.Metadata.ProjectUID, rule.Metadata.EnvironmentUID, operator, rule.Condition.Threshold)
	default:
		return "", fmt.Errorf("unsupported metric type: %s", rule.Source.Metric)
	}

	return expr, nil
}

// buildCPUUsageAlertExpression builds a PromQL expression for CPU usage alerts
// The threshold is expected as a percentage (e.g., 80 for 80% of requested CPU)
func buildCPUUsageAlertExpression(componentUID, projectUID, environmentUID, operator string, threshold float64) string {
	// Build the label filter for component identification
	labelFilter := BuildLabelFilter(componentUID, projectUID, environmentUID)

	// Calculate CPU usage as percentage of limits
	// Uses the same pattern as BuildCPUUsageQuery but compares against CPU limits
	// Formula: (CPU usage / CPU limits) * 100 > threshold
	expr := fmt.Sprintf(
		`(sum(rate(container_cpu_usage_seconds_total{container!=""}[2m]) * on (pod) group_left(label_openchoreo_dev_component_uid,label_openchoreo_dev_project_uid,label_openchoreo_dev_environment_uid) kube_pod_labels{%s}) / sum(kube_pod_container_resource_limits{resource="cpu"} * on (pod) group_left(label_openchoreo_dev_component_uid,label_openchoreo_dev_project_uid,label_openchoreo_dev_environment_uid) kube_pod_labels{%s})) * 100 %s %v`,
		labelFilter, labelFilter, operator, threshold,
	)
	return expr
}

// buildMemoryUsageAlertExpression builds a PromQL expression for memory usage alerts
// The threshold is expected as a percentage (e.g., 80 for 80% of requested memory)
func buildMemoryUsageAlertExpression(componentUID, projectUID, environmentUID, operator string, threshold float64) string {
	// Build the label filter for component identification
	labelFilter := BuildLabelFilter(componentUID, projectUID, environmentUID)

	// Calculate memory usage as percentage of limits
	// Uses the same pattern as BuildMemoryUsageQuery but compares against memory limits
	// Formula: (Memory usage / Memory limits) * 100 > threshold
	expr := fmt.Sprintf(
		`(sum(container_memory_working_set_bytes{container!=""} * on (pod) group_left(label_openchoreo_dev_component_uid,label_openchoreo_dev_project_uid,label_openchoreo_dev_environment_uid) kube_pod_labels{%s}) / sum(kube_pod_container_resource_limits{resource="memory"} * on (pod) group_left(label_openchoreo_dev_component_uid,label_openchoreo_dev_project_uid,label_openchoreo_dev_environment_uid) kube_pod_labels{%s})) * 100 %s %v`,
		labelFilter, labelFilter, operator, threshold,
	)
	return expr
}

// convertOperator converts the alert rule operator to PromQL comparison operator
func convertOperator(op string) (string, error) {
	switch op {
	case "gt":
		return ">", nil
	case "lt":
		return "<", nil
	case "gte":
		return ">=", nil
	case "lte":
		return "<=", nil
	case "eq":
		return "==", nil
	default:
		return "", fmt.Errorf("unsupported operator: %s", op)
	}
}

// parseDuration parses a duration string (e.g., "5m", "1h") and returns it as a string
// suitable for PrometheusRule 'for' field
func parseDuration(durationStr string) (string, error) {
	_, err := time.ParseDuration(durationStr)
	if err != nil {
		return "", fmt.Errorf("invalid duration format: %s", durationStr)
	}
	// Return as-is since Prometheus accepts the same format
	return durationStr, nil
}
