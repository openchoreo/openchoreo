// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/api/logsadapterclientgen"
)

// createLogAlertRuleViaAdapter forwards a create-alert-rule request to the logs adapter.
func (s *AlertService) createLogAlertRuleViaAdapter(ctx context.Context, req gen.AlertRuleRequest) (*gen.AlertingRuleSyncResponse, error) {
	adapterReq, err := toAdapterAlertRuleRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to convert alert rule request: %w", err)
	}

	resp, err := s.logsAdapter.adapterClient.CreateAlertRule(ctx, adapterReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call logs adapter create alert rule: %w", err)
	}
	defer resp.Body.Close()

	if err := mapAdapterHTTPError(resp, "logs adapter"); err != nil {
		return nil, err
	}

	return decodeAdapterSyncResponse(resp)
}

// getLogAlertRuleViaAdapter forwards a get-alert-rule request to the logs adapter.
func (s *AlertService) getLogAlertRuleViaAdapter(ctx context.Context, ruleName string) (*gen.AlertRuleResponse, error) {
	resp, err := s.logsAdapter.adapterClient.GetAlertRule(ctx, ruleName)
	if err != nil {
		return nil, fmt.Errorf("failed to call logs adapter get alert rule: %w", err)
	}
	defer resp.Body.Close()

	if err := mapAdapterHTTPError(resp, "logs adapter"); err != nil {
		return nil, err
	}

	return decodeAdapterAlertRuleResponse(resp)
}

// updateLogAlertRuleViaAdapter forwards an update-alert-rule request to the logs adapter.
func (s *AlertService) updateLogAlertRuleViaAdapter(ctx context.Context, ruleName string, req gen.AlertRuleRequest) (*gen.AlertingRuleSyncResponse, error) {
	adapterReq, err := toAdapterAlertRuleRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to convert alert rule request: %w", err)
	}

	resp, err := s.logsAdapter.adapterClient.UpdateAlertRule(ctx, ruleName, adapterReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call logs adapter update alert rule: %w", err)
	}
	defer resp.Body.Close()

	if err := mapAdapterHTTPError(resp, "logs adapter"); err != nil {
		return nil, err
	}

	return decodeAdapterSyncResponse(resp)
}

// deleteLogAlertRuleViaAdapter forwards a delete-alert-rule request to the logs adapter.
func (s *AlertService) deleteLogAlertRuleViaAdapter(ctx context.Context, ruleName string) (*gen.AlertingRuleSyncResponse, error) {
	resp, err := s.logsAdapter.adapterClient.DeleteAlertRule(ctx, ruleName)
	if err != nil {
		return nil, fmt.Errorf("failed to call logs adapter delete alert rule: %w", err)
	}
	defer resp.Body.Close()

	if err := mapAdapterHTTPError(resp, "logs adapter"); err != nil {
		return nil, err
	}

	return decodeAdapterSyncResponse(resp)
}

// toAdapterAlertRuleRequest converts a gen.AlertRuleRequest to the adapter client type
// via JSON round-trip. The schemas are not identical: logsadapterclientgen.AlertRuleRequest's
// Source struct omits the Type field because the adapter is log-only. Dropping Source.Type
// during the round-trip is intentional and safe because sourceTypeFromRequest and
// createLogAlertRuleViaAdapter validate the type before this conversion is reached.
func toAdapterAlertRuleRequest(req gen.AlertRuleRequest) (logsadapterclientgen.AlertRuleRequest, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return logsadapterclientgen.AlertRuleRequest{}, fmt.Errorf("failed to marshal request: %w", err)
	}
	var adapterReq logsadapterclientgen.AlertRuleRequest
	if err := json.Unmarshal(data, &adapterReq); err != nil {
		return logsadapterclientgen.AlertRuleRequest{}, fmt.Errorf("failed to unmarshal request: %w", err)
	}
	return adapterReq, nil
}

// mapAdapterHTTPError maps adapter HTTP error responses to sentinel errors.
// Returns nil for success status codes.
func mapAdapterHTTPError(resp *http.Response, adapterName string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusNotFound:
		return fmt.Errorf("%w: adapter returned 404", ErrAlertRuleNotFound)
	case http.StatusConflict:
		return fmt.Errorf("%w: adapter returned 409", ErrAlertRuleAlreadyExists)
	default:
		return fmt.Errorf("%s returned HTTP %d: %s", adapterName, resp.StatusCode, string(body))
	}
}

// decodeAdapterSyncResponse decodes the adapter's HTTP response body into a gen.AlertingRuleSyncResponse
// by JSON round-tripping, since the schemas are structurally identical.
func decodeAdapterSyncResponse(resp *http.Response) (*gen.AlertingRuleSyncResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read adapter response: %w", err)
	}
	var result gen.AlertingRuleSyncResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode adapter sync response: %w", err)
	}
	return &result, nil
}

// decodeAdapterAlertRuleResponse decodes the adapter's HTTP response body into a gen.AlertRuleResponse
// by JSON round-tripping, since the schemas are structurally identical.
func decodeAdapterAlertRuleResponse(resp *http.Response) (*gen.AlertRuleResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read adapter response: %w", err)
	}
	var result gen.AlertRuleResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode adapter alert rule response: %w", err)
	}
	return &result, nil
}
