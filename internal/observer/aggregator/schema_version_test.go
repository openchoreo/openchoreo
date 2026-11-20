// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package aggregator

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/store/deliveryinsights"
)

// migratingStore reports the store as migrated past this Observer once newer is
// set, as a newer replica would leave it mid-rollout.
type migratingStore struct {
	deliveryinsights.Store
	newer  atomic.Bool
	checks atomic.Int32
}

func (s *migratingStore) CheckSchemaVersion(ctx context.Context) error {
	s.checks.Add(1)
	if s.newer.Load() {
		return fmt.Errorf("%w: the store is at version 99", deliveryinsights.ErrSchemaNewer)
	}
	return s.Store.CheckSchemaVersion(ctx)
}

// TestARunningAggregatorStopsOnceANewerObserverMigrates pins the rolling-upgrade
// case startup cannot catch: this process opened the store at its own version,
// and a newer replica migrated it while it kept running. It must stop writing,
// and hand the lease over at once so the newer replica is not kept waiting out
// the TTL.
func TestARunningAggregatorStopsOnceANewerObserverMigrates(t *testing.T) {
	store, incidents := newTestStores(t)
	ctx := context.Background()
	capture := &captureHandler{}
	migrating := &migratingStore{Store: store}
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	a := newTestAggregator(migrating, incidents, nil, now)
	a.logger = slog.New(capture)

	a.tick(ctx)
	assert.Equal(t, a.holder, holderOf(ctx, store), "aggregating normally, so holding the lease")
	before, err := store.Watermark(ctx, watermarkSourceIncidents)
	require.NoError(t, err)
	require.Equal(t, now.UnixMilli(), before)

	migrating.newer.Store(true)
	a.now = func() time.Time { return now.Add(5 * time.Minute) }
	a.tick(ctx)
	a.tick(ctx)

	after, err := store.Watermark(ctx, watermarkSourceIncidents)
	require.NoError(t, err)
	assert.Equal(t, before, after, "nothing is written once the store is newer")
	assert.Empty(t, holderOf(ctx, store), "the lease is released for the newer replica")

	errs := capture.messagesAtLeast(slog.LevelError)
	require.Len(t, errs, 1, "the stop is logged once; got %v", errs)
	assert.Contains(t, errs[0], "migrated by a newer Observer")
	assert.Equal(t, int32(2), migrating.checks.Load(), "once stopped, it does not keep asking")
}
