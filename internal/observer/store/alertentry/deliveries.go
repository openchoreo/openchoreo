// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package alertentry

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Notification delivery statuses.
const (
	DeliveryStatusPending = "pending"
	DeliveryStatusSent    = "sent"
	DeliveryStatusFailed  = "failed"
)

// deliveriesQueryChunk bounds the alert IDs bound into one query.
const deliveriesQueryChunk = 500

// NotificationDelivery is the delivery result of an alert's notification to one channel.
type NotificationDelivery struct {
	AlertID string
	Channel string
	Status  string
	// Reason is the classified reason of a failure. It never holds error text.
	Reason        string
	Attempts      int
	CreatedAt     time.Time
	LastAttemptAt time.Time // zero until the first attempt
}

const createDeliveriesTableQuery = `
CREATE TABLE IF NOT EXISTS alert_notification_deliveries (
	alert_id TEXT NOT NULL,
	channel TEXT NOT NULL,
	status TEXT NOT NULL,
	reason TEXT,
	attempts INTEGER NOT NULL,
	created_at_ns BIGINT NOT NULL,
	last_attempt_at_ns BIGINT,
	PRIMARY KEY (alert_id, channel)
);`

const createDeliveriesStatusIndexQuery = `
CREATE INDEX IF NOT EXISTS idx_alert_notification_deliveries_status_created
ON alert_notification_deliveries(status, created_at_ns);`

// initializeDeliveries creates the notification delivery table.
func (s *sqlStore) initializeDeliveries(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, createDeliveriesTableQuery); err != nil {
		return fmt.Errorf("failed to create alert_notification_deliveries table: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, createDeliveriesStatusIndexQuery); err != nil {
		return fmt.Errorf("failed to create alert_notification_deliveries index: %w", err)
	}
	return nil
}

// placeholder returns the bind placeholder for the n-th (1-based) argument.
func (s *sqlStore) placeholder(n int) string {
	if s.backend == BackendPostgreSQL {
		return "$" + strconv.Itoa(n)
	}
	return "?"
}

// RecordNotificationDelivery inserts or updates the delivery result of one channel. An update
// keeps the row's creation time.
func (s *sqlStore) RecordNotificationDelivery(ctx context.Context, d NotificationDelivery) error {
	if d.AlertID == "" || d.Channel == "" {
		return fmt.Errorf("alert ID and channel are required")
	}
	createdAt := d.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	var lastAttempt sql.NullInt64
	if !d.LastAttemptAt.IsZero() {
		lastAttempt = sql.NullInt64{Int64: d.LastAttemptAt.UnixNano(), Valid: true}
	}
	p := s.placeholder
	// #nosec G202 -- only bind placeholders are concatenated
	query := `INSERT INTO alert_notification_deliveries
	(alert_id, channel, status, reason, attempts, created_at_ns, last_attempt_at_ns)
VALUES (` + strings.Join([]string{p(1), p(2), p(3), p(4), p(5), p(6), p(7)}, ", ") + `)
ON CONFLICT (alert_id, channel) DO UPDATE SET
	status = excluded.status,
	reason = excluded.reason,
	attempts = excluded.attempts,
	last_attempt_at_ns = excluded.last_attempt_at_ns;`
	if _, err := s.db.ExecContext(ctx, query,
		d.AlertID, d.Channel, d.Status, d.Reason, d.Attempts, createdAt.UnixNano(), lastAttempt); err != nil {
		return fmt.Errorf("failed to record notification delivery: %w", err)
	}
	return nil
}

// QueryNotificationDeliveries returns the delivery results of the given alerts, by alert ID.
func (s *sqlStore) QueryNotificationDeliveries(ctx context.Context, alertIDs []string) (map[string][]NotificationDelivery, error) {
	result := make(map[string][]NotificationDelivery, len(alertIDs))
	for start := 0; start < len(alertIDs); start += deliveriesQueryChunk {
		end := min(start+deliveriesQueryChunk, len(alertIDs))
		chunk := alertIDs[start:end]
		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, id := range chunk {
			placeholders[i] = s.placeholder(i + 1)
			args[i] = id
		}
		// #nosec G202 -- only bind placeholders are concatenated
		query := `SELECT alert_id, channel, status, reason, attempts, created_at_ns, last_attempt_at_ns
FROM alert_notification_deliveries
WHERE alert_id IN (` + strings.Join(placeholders, ", ") + `)
ORDER BY alert_id, channel;`
		if err := s.scanDeliveries(ctx, query, args, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *sqlStore) scanDeliveries(ctx context.Context, query string, args []any, result map[string][]NotificationDelivery) error {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to query notification deliveries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var d NotificationDelivery
		var reason sql.NullString
		var createdAtNS int64
		var lastAttemptNS sql.NullInt64
		if err := rows.Scan(&d.AlertID, &d.Channel, &d.Status, &reason, &d.Attempts, &createdAtNS, &lastAttemptNS); err != nil {
			return fmt.Errorf("failed to scan notification delivery: %w", err)
		}
		d.Reason = reason.String
		d.CreatedAt = time.Unix(0, createdAtNS).UTC()
		if lastAttemptNS.Valid {
			d.LastAttemptAt = time.Unix(0, lastAttemptNS.Int64).UTC()
		}
		result[d.AlertID] = append(result[d.AlertID], d)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to read notification deliveries: %w", err)
	}
	return nil
}

// FailStaleNotificationDeliveries marks deliveries still pending that were created before cutoff
// as failed with the given reason, and returns how many it marked. Running it again, or from
// several observers at once, is harmless.
func (s *sqlStore) FailStaleNotificationDeliveries(ctx context.Context, cutoff time.Time, reason string) (int64, error) {
	p := s.placeholder
	// #nosec G202 -- only bind placeholders are concatenated
	query := `UPDATE alert_notification_deliveries SET status = ` + p(1) + `, reason = ` + p(2) + `
WHERE status = ` + p(3) + ` AND created_at_ns < ` + p(4) + `;`
	res, err := s.db.ExecContext(ctx, query, DeliveryStatusFailed, reason, DeliveryStatusPending, cutoff.UnixNano())
	if err != nil {
		return 0, fmt.Errorf("failed to fail stale notification deliveries: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to count stale notification deliveries: %w", err)
	}
	return n, nil
}
