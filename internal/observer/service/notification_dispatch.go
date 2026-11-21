// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
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
// retried with backoff. onResult, when set, is called with each channel's result as soon as that
// channel finishes. It returns one result per channel, in channel order. The channels are resolved
// in the alert's namespace.
func DispatchAlertNotifications(
	ctx context.Context,
	alertDetails *observertypes.AlertDetails,
	channels []string,
	getConfig NotificationChannelConfigGetter,
	send NotificationSender,
	policy RetryPolicy,
	logger *slog.Logger,
	onResult func(ChannelDeliveryResult),
) []ChannelDeliveryResult {
	results := make([]ChannelDeliveryResult, len(channels))
	var wg sync.WaitGroup
	for i, channel := range channels {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = deliverToChannel(ctx, alertDetails, channel, getConfig, send, policy, logger)
			if onResult != nil {
				onResult(results[i])
			}
		}()
	}
	wg.Wait()
	return results
}

// deliverToChannel sends the alert to one channel, retrying temporary failures. Each attempt reads
// the channel config, until a read succeeds, and sends within the attempt's timeout.
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
	var channelConfig *notifications.NotificationChannelConfig

	backoff := policy.Backoff
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, policy.AttemptTimeout)
		err := attemptDelivery(attemptCtx, alertDetails, channel, &channelConfig, getConfig, send, logger)
		cancel()
		result.Attempts = attempt
		result.LastAttempt = time.Now().UTC()
		if channelConfig != nil {
			result.ChannelType = channelConfig.Type
		}
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

// attemptDelivery reads the channel config into *channelConfig unless an earlier attempt did, then
// sends. A failed read that is not classified is a channel config error, which is not retried.
func attemptDelivery(
	ctx context.Context,
	alertDetails *observertypes.AlertDetails,
	channel string,
	channelConfig **notifications.NotificationChannelConfig,
	getConfig NotificationChannelConfigGetter,
	send NotificationSender,
	logger *slog.Logger,
) error {
	if *channelConfig == nil {
		cfg, err := getConfig(ctx, alertDetails.Namespace, channel)
		if err != nil {
			var de *notifications.DeliveryError
			if errors.As(err, &de) {
				return err
			}
			// Lookup errors name the ConfigMap or Secret and why it could not be used, never their
			// contents, so they are safe to log.
			return &notifications.DeliveryError{Reason: notifications.ReasonChannelConfig, Detail: err.Error(), Err: err}
		}
		*channelConfig = cfg
	}
	return send(ctx, *channelConfig, alertDetails, logger)
}

// MaxDispatchDuration returns the longest one alert's dispatch can take under policy. Channels are
// sent concurrently, so this is one channel's worst case: the release line lookup and every attempt
// timing out, plus the backoff with its full jitter and some slack.
func MaxDispatchDuration(policy RetryPolicy) time.Duration {
	total := time.Duration(policy.MaxAttempts+1)*policy.AttemptTimeout + dispatchSlack
	backoff := policy.Backoff
	for i := 1; i < policy.MaxAttempts; i++ {
		total += backoff + backoff/2
		backoff *= 2
	}
	return total
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
