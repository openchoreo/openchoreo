// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/notifications"
	observertypes "github.com/openchoreo/openchoreo/internal/observer/types"
)

// NotificationChannelConfigGetter resolves the config of the notification channel with the given name in the
// given namespace.
type NotificationChannelConfigGetter func(ctx context.Context, namespace, channelName string) (*notifications.NotificationChannelConfig, error)

// NotificationSender makes one attempt to send an alert notification to a channel.
type NotificationSender func(ctx context.Context, config *notifications.NotificationChannelConfig,
	alertDetails *observertypes.AlertDetails, logger *slog.Logger) error

// RetryPolicy bounds how a notification is sent to one channel.
type RetryPolicy struct {
	// MaxAttempts is how many times a notification is sent before it counts as failed.
	MaxAttempts int
	// AttemptTimeout bounds one attempt.
	AttemptTimeout time.Duration
	// Backoff is the delay before the first retry. It doubles for each later retry, with jitter.
	Backoff time.Duration
}

// ChannelDeliveryResult is the outcome of sending an alert notification to one channel.
type ChannelDeliveryResult struct {
	Channel string
	// ChannelType is "webhook" or "email", or empty when the channel config could not be read.
	ChannelType string
	Sent        bool
	Reason      notifications.Reason
	// Detail is a short description of the failure that is safe to log, such as "HTTP 503".
	Detail      string
	Attempts    int
	LastAttempt time.Time
}

// DispatchAlertNotifications sends the alert to every channel concurrently, each channel with its
// own attempts and timeouts, so a slow channel does not hold up the others. Temporary failures are
// retried with backoff. It returns one result per channel, in channel order. The channels are
// resolved in the alert's namespace.
func DispatchAlertNotifications(
	ctx context.Context,
	alertDetails *observertypes.AlertDetails,
	channels []string,
	getConfig NotificationChannelConfigGetter,
	send NotificationSender,
	policy RetryPolicy,
	logger *slog.Logger,
) []ChannelDeliveryResult {
	results := make([]ChannelDeliveryResult, len(channels))
	var wg sync.WaitGroup
	for i, channel := range channels {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = deliverToChannel(ctx, alertDetails, channel, getConfig, send, policy, logger)
		}()
	}
	wg.Wait()
	return results
}

// deliverToChannel sends the alert to one channel, retrying temporary failures.
func deliverToChannel(
	ctx context.Context,
	alertDetails *observertypes.AlertDetails,
	channel string,
	getConfig NotificationChannelConfigGetter,
	send NotificationSender,
	policy RetryPolicy,
	logger *slog.Logger,
) ChannelDeliveryResult {
	result := ChannelDeliveryResult{Channel: channel}

	configCtx, cancel := context.WithTimeout(ctx, policy.AttemptTimeout)
	channelConfig, err := getConfig(configCtx, alertDetails.Namespace, channel)
	cancel()
	if err != nil {
		result.Reason = notifications.ReasonChannelConfig
		// Lookup errors name the ConfigMap or Secret and why it could not be used, never their
		// contents, so they are safe to log.
		result.Detail = err.Error()
		if ctx.Err() != nil {
			result.Reason = notifications.ReasonInterrupted
		}
		return result
	}
	result.ChannelType = channelConfig.Type

	backoff := policy.Backoff
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, policy.AttemptTimeout)
		err := send(attemptCtx, channelConfig, alertDetails, logger)
		cancel()
		result.Attempts = attempt
		result.LastAttempt = time.Now().UTC()
		if err == nil {
			result.Sent, result.Reason, result.Detail = true, "", ""
			return result
		}

		result.Reason = notifications.ReasonOf(err)
		result.Detail = notifications.DetailOf(err)
		// The attempt's own timeout is a timeout, but the dispatch being stopped is an interruption.
		if ctx.Err() != nil {
			result.Reason = notifications.ReasonInterrupted
			return result
		}
		if !result.Reason.Retryable() || attempt == policy.MaxAttempts {
			return result
		}

		logger.Debug("Retrying alert notification", "alertName", alertDetails.AlertName, "channel", channel,
			"attempt", attempt, "reason", result.Reason, "detail", result.Detail)
		if !sleepContext(ctx, withJitter(backoff)) {
			result.Reason = notifications.ReasonInterrupted
			return result
		}
		backoff *= 2
	}
	return result
}

// withJitter adds up to half of d, so retries of many alerts do not line up.
func withJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	//nolint:gosec // Spreading retries over time, not a security decision.
	return d + rand.N(d/2+1)
}

// sleepContext waits for d, and reports false if ctx ends first.
func sleepContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
