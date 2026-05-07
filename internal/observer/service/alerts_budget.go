// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
)

// createBudgetAlertRule is a stub that accepts a budget alert rule and returns a synced response.
// Actual cost-checking logic will be implemented in a future iteration.
func (s *AlertService) createBudgetAlertRule(_ context.Context, req gen.AlertRuleRequest) (*gen.AlertingRuleSyncResponse, error) {
	ruleName := req.Metadata.Name
	backendID := fmt.Sprintf("budget-%s", ruleName)
	s.logger.Debug("Budget alert rule created (stub)", "rule_name", ruleName)
	return buildSyncResponse(alertActionCreated, ruleName, backendID, time.Now().UTC()), nil
}

// getBudgetAlertRule is a stub that always returns not-found since there is no backend store yet.
func (s *AlertService) getBudgetAlertRule(_ context.Context, ruleName string) (*gen.AlertRuleResponse, error) {
	return nil, fmt.Errorf("%w: %s", ErrAlertRuleNotFound, ruleName)
}

// updateBudgetAlertRule is a stub that accepts a budget alert rule update and returns a synced response.
func (s *AlertService) updateBudgetAlertRule(_ context.Context, ruleName string, _ gen.AlertRuleRequest) (*gen.AlertingRuleSyncResponse, error) {
	backendID := fmt.Sprintf("budget-%s", ruleName)
	s.logger.Debug("Budget alert rule updated (stub)", "rule_name", ruleName)
	return buildSyncResponse(alertActionUpdated, ruleName, backendID, time.Now().UTC()), nil
}

// deleteBudgetAlertRule is a stub that accepts a budget alert rule deletion and returns a synced response.
func (s *AlertService) deleteBudgetAlertRule(_ context.Context, ruleName string) (*gen.AlertingRuleSyncResponse, error) {
	backendID := fmt.Sprintf("budget-%s", ruleName)
	s.logger.Debug("Budget alert rule deleted (stub)", "rule_name", ruleName)
	return buildSyncResponse(alertActionDeleted, ruleName, backendID, time.Now().UTC()), nil
}
