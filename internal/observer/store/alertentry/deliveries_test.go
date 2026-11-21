// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package alertentry

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deliveryStores returns an initialized SQLite store, plus a PostgreSQL store when
// OBSERVER_TEST_POSTGRES_DSN is set. Point it at a throwaway database: the tests write to it and
// share it, so they do not assume an empty table.
func deliveryStores(t *testing.T) map[string]AlertEntryStore {
	t.Helper()
	stores := map[string]AlertEntryStore{}

	sqlite, err := New(BackendSQLite, "file:"+filepath.Join(t.TempDir(), "alerts.db"), slog.Default())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlite.Close() })
	require.NoError(t, sqlite.Initialize(context.Background()))
	stores[BackendSQLite] = sqlite

	if dsn := os.Getenv("OBSERVER_TEST_POSTGRES_DSN"); dsn != "" {
		pg, err := New(BackendPostgreSQL, dsn, slog.Default())
		require.NoError(t, err)
		t.Cleanup(func() { _ = pg.Close() })
		require.NoError(t, pg.Initialize(context.Background()))
		stores[BackendPostgreSQL] = pg
	}
	return stores
}

func TestNotificationDeliveries_RecordAndQuery(t *testing.T) {
	for backend, store := range deliveryStores(t) {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			alertID, other := uuid.NewString(), uuid.NewString()
			created := time.Now().Add(-time.Minute).UTC()

			for _, ch := range []string{"slack", "email"} {
				require.NoError(t, store.RecordNotificationDelivery(ctx, NotificationDelivery{
					AlertID: alertID, Channel: ch, Status: DeliveryStatusPending, CreatedAt: created,
				}))
			}
			require.NoError(t, store.RecordNotificationDelivery(ctx, NotificationDelivery{
				AlertID: other, Channel: "slack", Status: DeliveryStatusPending, CreatedAt: created,
			}))

			attempted := time.Now().UTC()
			require.NoError(t, store.RecordNotificationDelivery(ctx, NotificationDelivery{
				AlertID: alertID, Channel: "slack", Status: DeliveryStatusFailed, Reason: "http_5xx",
				Attempts: 3, LastAttemptAt: attempted,
			}))

			got, err := store.QueryNotificationDeliveries(ctx, []string{alertID})
			require.NoError(t, err)
			require.Len(t, got, 1, "only the requested alert's deliveries")
			require.Len(t, got[alertID], 2)

			byChannel := map[string]NotificationDelivery{}
			for _, d := range got[alertID] {
				byChannel[d.Channel] = d
			}
			slack := byChannel["slack"]
			assert.Equal(t, DeliveryStatusFailed, slack.Status)
			assert.Equal(t, "http_5xx", slack.Reason)
			assert.Equal(t, 3, slack.Attempts)
			assert.WithinDuration(t, attempted, slack.LastAttemptAt, time.Microsecond)
			assert.WithinDuration(t, created, slack.CreatedAt, time.Microsecond, "an update keeps the creation time")

			email := byChannel["email"]
			assert.Equal(t, DeliveryStatusPending, email.Status)
			assert.True(t, email.LastAttemptAt.IsZero())
		})
	}
}

func TestNotificationDeliveries_QueryEmptyAndUnknown(t *testing.T) {
	for backend, store := range deliveryStores(t) {
		t.Run(backend, func(t *testing.T) {
			got, err := store.QueryNotificationDeliveries(context.Background(), nil)
			require.NoError(t, err)
			assert.Empty(t, got)

			got, err = store.QueryNotificationDeliveries(context.Background(), []string{uuid.NewString()})
			require.NoError(t, err)
			assert.Empty(t, got)
		})
	}
}

func TestNotificationDeliveries_QuerySpansChunks(t *testing.T) {
	for backend, store := range deliveryStores(t) {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			ids := make([]string, deliveriesQueryChunk+3)
			for i := range ids {
				ids[i] = uuid.NewString()
				require.NoError(t, store.RecordNotificationDelivery(ctx, NotificationDelivery{
					AlertID: ids[i], Channel: "slack", Status: DeliveryStatusSent, Attempts: 1,
				}))
			}
			got, err := store.QueryNotificationDeliveries(ctx, ids)
			require.NoError(t, err)
			assert.Len(t, got, len(ids))
		})
	}
}

func TestFailStaleNotificationDeliveries(t *testing.T) {
	for backend, store := range deliveryStores(t) {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			now := time.Now().UTC()
			stale, fresh, done := uuid.NewString(), uuid.NewString(), uuid.NewString()
			require.NoError(t, store.RecordNotificationDelivery(ctx, NotificationDelivery{
				AlertID: stale, Channel: "slack", Status: DeliveryStatusPending, CreatedAt: now.Add(-time.Hour),
			}))
			require.NoError(t, store.RecordNotificationDelivery(ctx, NotificationDelivery{
				AlertID: fresh, Channel: "slack", Status: DeliveryStatusPending, CreatedAt: now,
			}))
			require.NoError(t, store.RecordNotificationDelivery(ctx, NotificationDelivery{
				AlertID: done, Channel: "slack", Status: DeliveryStatusSent, Attempts: 1, CreatedAt: now.Add(-time.Hour),
			}))

			cutoff := now.Add(-time.Minute)
			n, err := store.FailStaleNotificationDeliveries(ctx, cutoff, "interrupted")
			require.NoError(t, err)
			assert.GreaterOrEqual(t, n, int64(1))

			n, err = store.FailStaleNotificationDeliveries(ctx, cutoff, "interrupted")
			require.NoError(t, err)
			assert.Equal(t, int64(0), n, "running the sweep again changes nothing")

			got, err := store.QueryNotificationDeliveries(ctx, []string{stale, fresh, done})
			require.NoError(t, err)
			assert.Equal(t, DeliveryStatusFailed, got[stale][0].Status)
			assert.Equal(t, "interrupted", got[stale][0].Reason)
			assert.Equal(t, DeliveryStatusPending, got[fresh][0].Status)
			assert.Equal(t, DeliveryStatusSent, got[done][0].Status)
		})
	}
}

func TestRecordNotificationDelivery_RequiresIdentity(t *testing.T) {
	for backend, store := range deliveryStores(t) {
		t.Run(backend, func(t *testing.T) {
			assert.Error(t, store.RecordNotificationDelivery(context.Background(), NotificationDelivery{Channel: "slack"}))
			assert.Error(t, store.RecordNotificationDelivery(context.Background(), NotificationDelivery{AlertID: "x"}))
		})
	}
}
