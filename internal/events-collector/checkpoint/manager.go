// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package checkpoint

import (
	"context"
	"log/slog"
	"time"
)

// Manager runs periodic cleanup of expired checkpoint records.
type Manager struct {
	store    *Store
	interval time.Duration
	ttl      time.Duration
	logger   *slog.Logger
}

// NewManager creates a new checkpoint cleanup manager.
// interval controls how often the cleanup runs (e.g., 10 minutes).
// ttl controls how long records are kept (e.g., 1 hour).
func NewManager(store *Store, interval, ttl time.Duration, logger *slog.Logger) *Manager {
	return &Manager{
		store:    store,
		interval: interval,
		ttl:      ttl,
		logger:   logger,
	}
}

// Start begins the periodic cleanup loop. It blocks until the context is cancelled.
func (m *Manager) Start(ctx context.Context) {
	if m.interval <= 0 || m.ttl <= 0 {
		m.logger.Error("invalid checkpoint cleanup configuration",
			"interval", m.interval,
			"ttl", m.ttl,
		)
		return
	}

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	m.logger.Info("checkpoint manager started",
		"cleanup_interval", m.interval.String(),
		"ttl", m.ttl.String(),
	)

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("checkpoint manager stopped")
			return
		case <-ticker.C:
			m.cleanup()
		}
	}
}

func (m *Manager) cleanup() {
	cutoff := time.Now().Add(-m.ttl)
	deleted, err := m.store.CleanupBefore(cutoff)
	if err != nil {
		m.logger.Error("failed to cleanup checkpoint records", "error", err)
		return
	}
	if deleted > 0 {
		m.logger.Info("cleaned up expired checkpoint records", "deleted", deleted)
	}
}
