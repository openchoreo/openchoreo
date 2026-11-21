// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"

	"github.com/openchoreo/openchoreo/internal/observer/api/internalgen"
)

// createMetricAlertRuleViaAdapter forwards a create-alert-rule request for metric alerts to the metrics adapter.
func (s *AlertService) createMetricAlertRuleViaAdapter(ctx context.Context, req internalgen.AlertRuleRequest) (*internalgen.AlertingRuleSyncResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal alert rule request: %w", err)
	}

	url := s.metricsAdapterURL + "/api/v1alpha1/alerts/rules"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics adapter request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.metricsAdapterClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call metrics adapter create alert rule: %w", err)
	}
	defer resp.Body.Close()

	if err := mapAdapterHTTPError(resp, "metrics adapter"); err != nil {
		return nil, err
	}

	return decodeAdapterSyncResponse(resp)
}

// getMetricAlertRuleViaAdapter forwards a get-alert-rule request for metric alerts to the metrics adapter.
func (s *AlertService) getMetricAlertRuleViaAdapter(ctx context.Context, namespace, ruleName string) (*internalgen.AlertRuleResponse, error) {
	url := s.metricAlertRuleURL(namespace, ruleName)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics adapter request: %w", err)
	}

	resp, err := s.metricsAdapterClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call metrics adapter get alert rule: %w", err)
	}
	defer resp.Body.Close()

	if err := mapAdapterHTTPError(resp, "metrics adapter"); err != nil {
		return nil, err
	}

	return decodeAdapterAlertRuleResponse(resp)
}

// updateMetricAlertRuleViaAdapter forwards an update-alert-rule request for metric alerts to the metrics adapter.
func (s *AlertService) updateMetricAlertRuleViaAdapter(ctx context.Context, ruleName string, req internalgen.AlertRuleRequest) (*internalgen.AlertingRuleSyncResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal alert rule request: %w", err)
	}

	url := s.metricAlertRuleURL(req.Metadata.Namespace, ruleName)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics adapter request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.metricsAdapterClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call metrics adapter update alert rule: %w", err)
	}
	defer resp.Body.Close()

	if err := mapAdapterHTTPError(resp, "metrics adapter"); err != nil {
		return nil, err
	}

	return decodeAdapterSyncResponse(resp)
}

// deleteMetricAlertRuleViaAdapter forwards a delete-alert-rule request for metric alerts to the metrics adapter.
func (s *AlertService) deleteMetricAlertRuleViaAdapter(ctx context.Context, namespace, ruleName string) (*internalgen.AlertingRuleSyncResponse, error) {
	url := s.metricAlertRuleURL(namespace, ruleName)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics adapter request: %w", err)
	}

	resp, err := s.metricsAdapterClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call metrics adapter delete alert rule: %w", err)
	}
	defer resp.Body.Close()

	if err := mapAdapterHTTPError(resp, "metrics adapter"); err != nil {
		return nil, err
	}

	return decodeAdapterSyncResponse(resp)
}

// metricAlertRuleURL returns the metrics adapter URL of the alert rule with the given namespace and name.
func (s *AlertService) metricAlertRuleURL(namespace, ruleName string) string {
	return s.metricsAdapterURL + "/api/v1alpha1/alerts/rules/" + neturl.PathEscape(ruleName) +
		"?namespace=" + neturl.QueryEscape(namespace)
}
