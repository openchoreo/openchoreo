// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
)

// createMetricAlertRuleViaAdapter forwards a create-alert-rule request for metric alerts to the metrics adapter.
func (s *AlertService) createMetricAlertRuleViaAdapter(ctx context.Context, req gen.AlertRuleRequest) (*gen.AlertingRuleSyncResponse, error) {
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
func (s *AlertService) getMetricAlertRuleViaAdapter(ctx context.Context, ruleName string) (*gen.AlertRuleResponse, error) {
	url := s.metricsAdapterURL + "/api/v1alpha1/alerts/rules/" + ruleName
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
func (s *AlertService) updateMetricAlertRuleViaAdapter(ctx context.Context, ruleName string, req gen.AlertRuleRequest) (*gen.AlertingRuleSyncResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal alert rule request: %w", err)
	}

	url := s.metricsAdapterURL + "/api/v1alpha1/alerts/rules/" + ruleName
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
func (s *AlertService) deleteMetricAlertRuleViaAdapter(ctx context.Context, ruleName string) (*gen.AlertingRuleSyncResponse, error) {
	url := s.metricsAdapterURL + "/api/v1alpha1/alerts/rules/" + ruleName
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
