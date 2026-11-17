// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/notifications"
	"github.com/openchoreo/openchoreo/internal/observer/store/alertentry"
)

// defaultRetryPolicy matches the observer's configuration defaults.
var defaultRetryPolicy = RetryPolicy{MaxAttempts: 3, AttemptTimeout: 10 * time.Second, Backoff: time.Second}

const (
	// recordTimeout bounds writing delivery results, which also runs after dispatches are stopped.
	recordTimeout = 10 * time.Second
	// drainRecordGrace is how long the drain waits for stopped dispatches to record their results.
	drainRecordGrace = 5 * time.Second
	// dispatchOverhead covers the work of a dispatch outside its attempts, such as resolving the
	// alert's release line.
	dispatchOverhead = 30 * time.Second
)

// effectiveRetryPolicy returns the configured policy, or the default for a service built without one.
func (s *AlertService) effectiveRetryPolicy() RetryPolicy {
	if s.retryPolicy.MaxAttempts < 1 || s.retryPolicy.AttemptTimeout <= 0 {
		return defaultRetryPolicy
	}
	return s.retryPolicy
}

// maxDispatchDuration returns the longest one alert's dispatch can take. Channels are sent
// concurrently, so this is one channel's worst case: every attempt timing out, plus the config
// lookup, the backoff with its jitter, and the dispatch's other work.
func (s *AlertService) maxDispatchDuration() time.Duration {
	p := s.effectiveRetryPolicy()
	total := time.Duration(p.MaxAttempts+1)*p.AttemptTimeout + dispatchOverhead
	backoff := p.Backoff
	for i := 1; i < p.MaxAttempts; i++ {
		total += backoff + backoff/2
		backoff *= 2
	}
	return total
}

// dispatchContext returns the context that notification dispatches run under. DrainNotifications
// cancels it.
func (s *AlertService) dispatchContext() context.Context {
	s.dispatchOnce.Do(func() {
		s.dispatchCtx, s.stopDispatches = context.WithCancel(context.Background())
	})
	return s.dispatchCtx
}

// recordPendingDeliveries records every channel of a new alert as pending, so a notification that
// never finishes still shows up.
func (s *AlertService) recordPendingDeliveries(ctx context.Context, alertID string, channels []string) {
	if s.alertEntryStore == nil {
		return
	}
	now := time.Now().UTC()
	for _, channel := range channels {
		if err := s.alertEntryStore.RecordNotificationDelivery(ctx, alertentry.NotificationDelivery{
			AlertID: alertID, Channel: channel, Status: alertentry.DeliveryStatusPending, CreatedAt: now,
		}); err != nil {
			s.logger.Warn("Failed to record pending alert notification", "alertID", alertID, "channel", channel, "error", err)
		}
	}
}

// recordDeliveryResults stores each channel's delivery result in store and logs it. It runs on its
// own context, so results are recorded even when the dispatch was stopped. The logs carry the
// classified reason, never the error text, which can include a webhook URL that is itself a
// credential.
func (s *AlertService) recordDeliveryResults(store alertentry.AlertEntryStore, alertID, alertName string, results []ChannelDeliveryResult) {
	ctx, cancel := context.WithTimeout(context.Background(), recordTimeout)
	defer cancel()

	for _, r := range results {
		status := alertentry.DeliveryStatusSent
		if r.Sent {
			s.logger.Info("Alert notification sent", "alertID", alertID, "alertName", alertName,
				"channel", r.Channel, "channelType", r.ChannelType, "attempts", r.Attempts)
		} else {
			status = alertentry.DeliveryStatusFailed
			s.logger.Warn("Alert notification failed", "alertID", alertID, "alertName", alertName,
				"channel", r.Channel, "channelType", r.ChannelType, "reason", r.Reason, "detail", r.Detail,
				"attempts", r.Attempts)
		}
		if store == nil {
			continue
		}
		if err := store.RecordNotificationDelivery(ctx, alertentry.NotificationDelivery{
			AlertID:       alertID,
			Channel:       r.Channel,
			Status:        status,
			Reason:        string(r.Reason),
			Attempts:      r.Attempts,
			LastAttemptAt: r.LastAttempt,
		}); err != nil {
			s.logger.Warn("Failed to record alert notification result", "alertID", alertID, "channel", r.Channel, "error", err)
		}
	}
}

// DrainNotifications waits for in-flight notification dispatches until ctx ends. It then stops
// the rest, which record their unfinished channels as interrupted. Call it after the servers have
// stopped accepting alerts.
func (s *AlertService) DrainNotifications(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		s.dispatches.Wait()
		close(done)
	}()

	select {
	case <-done:
		return
	case <-ctx.Done():
	}

	s.logger.Warn("Stopping alert notification dispatches that did not finish in time")
	s.dispatchContext()
	s.stopDispatches()
	select {
	case <-done:
	case <-time.After(drainRecordGrace):
		s.logger.Warn("Alert notification dispatches did not record their results before shutdown")
	}
}

// RunStaleDeliverySweep marks deliveries still pending well after they could have finished as
// failed with reason interrupted, at start and then every interval until ctx ends. Their dispatch
// stopped without recording a result, as when the observer crashed. The cutoff is twice the
// longest dispatch, so no dispatch in flight on any replica is marked.
func (s *AlertService) RunStaleDeliverySweep(ctx context.Context, interval time.Duration) {
	if s.alertEntryStore == nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		s.sweepStaleDeliveries(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *AlertService) sweepStaleDeliveries(ctx context.Context) {
	cutoff := time.Now().Add(-2 * s.maxDispatchDuration())
	n, err := s.alertEntryStore.FailStaleNotificationDeliveries(ctx, cutoff, string(notifications.ReasonInterrupted))
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("Failed to mark stale alert notifications as interrupted", "error", err)
		}
		return
	}
	if n > 0 {
		s.logger.Warn("Marked stale alert notifications as interrupted", "count", n)
	}
}
