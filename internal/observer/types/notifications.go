// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package types

// AlertNotificationResponse represents the outcome of processing an alert notification.
type AlertNotificationResponse struct {
	Status          string   `json:"status"`
	AlertID         string   `json:"alertId,omitempty"`
	EmailRecipients []string `json:"emailRecipients,omitempty"`
}
