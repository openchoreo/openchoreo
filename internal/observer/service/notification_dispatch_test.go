// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/observer/notifications"
	"github.com/openchoreo/openchoreo/internal/observer/store/alertentry"
	observertypes "github.com/openchoreo/openchoreo/internal/observer/types"
)

var testPolicy = RetryPolicy{MaxAttempts: 3, AttemptTimeout: 200 * time.Millisecond, Backoff: 10 * time.Millisecond}

// webhookConfigs resolves every channel as a webhook whose URL is the channel name.
func webhookConfigs(_ context.Context, _, channel string) (*notifications.NotificationChannelConfig, error) {
	return &notifications.NotificationChannelConfig{Type: "webhook", Webhook: notifications.WebhookConfig{URL: channel}}, nil
}

func temporaryErr() error {
	return &notifications.DeliveryError{Reason: notifications.ReasonHTTP5xx, Detail: "HTTP 503", Err: errors.New("unavailable")}
}

func TestDispatch_SlowChannelDoesNotDelayOthers(t *testing.T) {
	send := func(ctx context.Context, cfg *notifications.NotificationChannelConfig, _ *observertypes.AlertDetails, _ *slog.Logger) error {
		if cfg.Webhook.URL == "slow" {
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}
	start := time.Now()
	policy := RetryPolicy{MaxAttempts: 1, AttemptTimeout: 300 * time.Millisecond}
	results := DispatchAlertNotifications(context.Background(), &observertypes.AlertDetails{},
		[]string{"slow", "fast"}, webhookConfigs, send, policy, discardLogger())

	require.Len(t, results, 2)
	assert.False(t, results[0].Sent)
	assert.Equal(t, notifications.ReasonTimeout, results[0].Reason)
	assert.True(t, results[1].Sent)
	assert.Less(t, results[1].LastAttempt.Sub(start), 200*time.Millisecond, "the fast channel must not wait for the slow one")
}

func TestDispatch_RetriesTemporaryFailures(t *testing.T) {
	var calls atomic.Int32
	send := func(context.Context, *notifications.NotificationChannelConfig, *observertypes.AlertDetails, *slog.Logger) error {
		if calls.Add(1) < 3 {
			return temporaryErr()
		}
		return nil
	}
	results := DispatchAlertNotifications(context.Background(), &observertypes.AlertDetails{},
		[]string{"hook"}, webhookConfigs, send, testPolicy, discardLogger())

	require.Len(t, results, 1)
	assert.True(t, results[0].Sent)
	assert.Equal(t, 3, results[0].Attempts)
	assert.Empty(t, results[0].Reason)
}

func TestDispatch_DoesNotRetryPermanentFailures(t *testing.T) {
	var calls atomic.Int32
	send := func(context.Context, *notifications.NotificationChannelConfig, *observertypes.AlertDetails, *slog.Logger) error {
		calls.Add(1)
		return &notifications.DeliveryError{Reason: notifications.ReasonHTTP4xx, Detail: "HTTP 400"}
	}
	results := DispatchAlertNotifications(context.Background(), &observertypes.AlertDetails{},
		[]string{"hook"}, webhookConfigs, send, testPolicy, discardLogger())

	assert.Equal(t, int32(1), calls.Load())
	assert.Equal(t, notifications.ReasonHTTP4xx, results[0].Reason)
	assert.Equal(t, "HTTP 400", results[0].Detail)
	assert.Equal(t, 1, results[0].Attempts)
}

func TestDispatch_GivesUpAfterMaxAttempts(t *testing.T) {
	send := func(context.Context, *notifications.NotificationChannelConfig, *observertypes.AlertDetails, *slog.Logger) error {
		return temporaryErr()
	}
	results := DispatchAlertNotifications(context.Background(), &observertypes.AlertDetails{},
		[]string{"hook"}, webhookConfigs, send, testPolicy, discardLogger())

	assert.False(t, results[0].Sent)
	assert.Equal(t, notifications.ReasonHTTP5xx, results[0].Reason)
	assert.Equal(t, testPolicy.MaxAttempts, results[0].Attempts)
}

func TestDispatch_ChannelConfigFailure(t *testing.T) {
	getConfig := func(context.Context, string, string) (*notifications.NotificationChannelConfig, error) {
		return nil, errors.New("configmap not found")
	}
	send := func(context.Context, *notifications.NotificationChannelConfig, *observertypes.AlertDetails, *slog.Logger) error {
		t.Fatal("must not send without a channel config")
		return nil
	}
	results := DispatchAlertNotifications(context.Background(), &observertypes.AlertDetails{},
		[]string{"hook"}, getConfig, send, testPolicy, discardLogger())

	assert.Equal(t, notifications.ReasonChannelConfig, results[0].Reason)
	assert.Equal(t, "configmap not found", results[0].Detail, "the lookup error says what is wrong with the channel")
	assert.Zero(t, results[0].Attempts)
}

func TestDispatch_StopDuringBackoffIsInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	send := func(context.Context, *notifications.NotificationChannelConfig, *observertypes.AlertDetails, *slog.Logger) error {
		cancel()
		return temporaryErr()
	}
	policy := RetryPolicy{MaxAttempts: 3, AttemptTimeout: time.Second, Backoff: time.Minute}
	results := DispatchAlertNotifications(ctx, &observertypes.AlertDetails{},
		[]string{"hook"}, webhookConfigs, send, policy, discardLogger())

	assert.False(t, results[0].Sent)
	assert.Equal(t, notifications.ReasonInterrupted, results[0].Reason)
	assert.Equal(t, 1, results[0].Attempts)
}

// newDeliveryTestService returns a service that resolves webhook channels from the fake cluster
// and sends with send.
func newDeliveryTestService(t *testing.T, store *fakeAlertEntryStore, send NotificationSender, channels ...string) *AlertService {
	t.Helper()
	svc := newNotificationTestService(t, channelObjects(channels)...)
	svc.alertEntryStore = store
	svc.sendNotification = send
	svc.retryPolicy = testPolicy
	return svc
}

// channelObjects returns the plane objects of webhook channels in namespace tenant, each sending to
// http://<channel name>.
func channelObjects(channels []string) []client.Object {
	objs := make([]client.Object, 0, 2*len(channels))
	for _, ch := range channels {
		cm, secret := webhookChannelObjects("tenant", ch, "http://"+ch, "")
		objs = append(objs, cm, secret)
	}
	return objs
}

func TestDispatch_RecordsResults(t *testing.T) {
	store := &fakeAlertEntryStore{}
	send := func(_ context.Context, cfg *notifications.NotificationChannelConfig, _ *observertypes.AlertDetails, _ *slog.Logger) error {
		if cfg.Webhook.URL == "http://down" {
			return temporaryErr()
		}
		return nil
	}
	svc := newDeliveryTestService(t, store, send, "up", "down")
	details := &observertypes.AlertDetails{AlertName: "rule", Namespace: "tenant", NotificationChannels: []string{"up", "down"}}

	svc.recordPendingDeliveries(context.Background(), "alert-1", details.NotificationChannels)
	for _, ch := range details.NotificationChannels {
		d, ok := store.delivery("alert-1", ch)
		require.True(t, ok)
		assert.Equal(t, alertentry.DeliveryStatusPending, d.Status)
	}

	svc.startNotificationDispatch("alert-1", details)
	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	svc.DrainNotifications(drainCtx)

	up, _ := store.delivery("alert-1", "up")
	assert.Equal(t, alertentry.DeliveryStatusSent, up.Status)
	assert.Equal(t, 1, up.Attempts)
	down, _ := store.delivery("alert-1", "down")
	assert.Equal(t, alertentry.DeliveryStatusFailed, down.Status)
	assert.Equal(t, string(notifications.ReasonHTTP5xx), down.Reason)
	assert.Equal(t, testPolicy.MaxAttempts, down.Attempts)
}

func TestDrainNotifications_StopsAndRecordsUnfinishedDispatches(t *testing.T) {
	store := &fakeAlertEntryStore{}
	send := func(ctx context.Context, _ *notifications.NotificationChannelConfig, _ *observertypes.AlertDetails, _ *slog.Logger) error {
		<-ctx.Done()
		return ctx.Err()
	}
	svc := newDeliveryTestService(t, store, send, "stuck")
	svc.retryPolicy = RetryPolicy{MaxAttempts: 1, AttemptTimeout: time.Hour}
	details := &observertypes.AlertDetails{AlertName: "rule", Namespace: "tenant", NotificationChannels: []string{"stuck"}}

	svc.recordPendingDeliveries(context.Background(), "alert-1", details.NotificationChannels)
	svc.startNotificationDispatch("alert-1", details)

	drainCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	svc.DrainNotifications(drainCtx)
	assert.Less(t, time.Since(start), drainRecordGrace)

	d, _ := store.delivery("alert-1", "stuck")
	assert.Equal(t, alertentry.DeliveryStatusFailed, d.Status)
	assert.Equal(t, string(notifications.ReasonInterrupted), d.Reason)
}

func TestDrainNotifications_NoDispatches(t *testing.T) {
	svc := &AlertService{logger: discardLogger()}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	svc.DrainNotifications(ctx)
	assert.NoError(t, ctx.Err(), "the drain returns at once when nothing is in flight")
}

func TestRunStaleDeliverySweep(t *testing.T) {
	store := &fakeAlertEntryStore{}
	svc := &AlertService{logger: discardLogger(), alertEntryStore: store, retryPolicy: testPolicy}
	ctx := context.Background()
	old := time.Now().Add(-2*svc.maxDispatchDuration() - time.Minute)
	require.NoError(t, store.RecordNotificationDelivery(ctx, alertentry.NotificationDelivery{
		AlertID: "old", Channel: "hook", Status: alertentry.DeliveryStatusPending, CreatedAt: old,
	}))
	require.NoError(t, store.RecordNotificationDelivery(ctx, alertentry.NotificationDelivery{
		AlertID: "new", Channel: "hook", Status: alertentry.DeliveryStatusPending, CreatedAt: time.Now(),
	}))

	sweepCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		svc.RunStaleDeliverySweep(sweepCtx, time.Hour)
		close(done)
	}()
	require.Eventually(t, func() bool {
		d, _ := store.delivery("old", "hook")
		return d.Status == alertentry.DeliveryStatusFailed
	}, 2*time.Second, 10*time.Millisecond)
	cancel()
	<-done

	oldDelivery, _ := store.delivery("old", "hook")
	assert.Equal(t, string(notifications.ReasonInterrupted), oldDelivery.Reason)
	newDelivery, _ := store.delivery("new", "hook")
	assert.Equal(t, alertentry.DeliveryStatusPending, newDelivery.Status, "a recent pending delivery may still be in flight")
}
