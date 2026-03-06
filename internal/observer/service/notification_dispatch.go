// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/openchoreo/openchoreo/internal/observer/notifications"
	observertypes "github.com/openchoreo/openchoreo/internal/observer/types"
)

// NotificationChannelConfigGetter resolves a notification channel config by channel name.
type NotificationChannelConfigGetter func(ctx context.Context, channelName string) (*notifications.NotificationChannelConfig, error)

// DispatchAlertNotifications sends the alert to all channels and returns an aggregated error.
func DispatchAlertNotifications(
	ctx context.Context,
	alertDetails *observertypes.AlertDetails,
	channels []string,
	getConfig NotificationChannelConfigGetter,
	logger *slog.Logger,
) error {
	var errs []error
	for _, channel := range channels {
		channelConfig, err := getConfig(ctx, channel)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get notification channel config for %q: %w", channel, err))
			continue
		}

		if err := notifications.SendAlertNotification(ctx, channelConfig, alertDetails, logger); err != nil {
			errs = append(errs, fmt.Errorf("failed to send notification to channel %q: %w", channel, err))
		}
	}

	return errors.Join(errs...)
}
