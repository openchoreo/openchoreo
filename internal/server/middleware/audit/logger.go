// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Logger handles emitting audit log events using structured logging
type Logger struct {
	slogger     *slog.Logger
	serviceName string
}

// NewLogger creates a new audit logger
func NewLogger(slogger *slog.Logger, serviceName string) *Logger {
	return &Logger{
		slogger:     slogger,
		serviceName: serviceName,
	}
}

// LogEvent emits an audit log event using slog
func (l *Logger) LogEvent(event *Event) {
	// Generate UUID v7 for event ID if not set
	if event.EventID == "" {
		if id, err := uuid.NewV7(); err == nil {
			event.EventID = id.String()
		} else {
			// Fallback to v4 if v7 generation fails
			event.EventID = uuid.New().String()
		}
	}

	// Set timestamp if not already set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Set service name if not already set
	if event.Service == "" {
		event.Service = l.serviceName
	}

	// Build slog attributes
	attrs := []any{
		slog.String("event_id", event.EventID),
		slog.Time("timestamp", event.Timestamp),
	}

	// Build actor attributes
	actorAttrs := []any{
		slog.String("type", event.Actor.Type),
		slog.String("id", event.Actor.ID),
	}
	// Add entitlements if present
	if len(event.Actor.Entitlements) > 0 {
		entitlementAttrs := make([]any, 0, len(event.Actor.Entitlements))
		for k, v := range event.Actor.Entitlements {
			entitlementAttrs = append(entitlementAttrs, slog.Any(k, v))
		}
		actorAttrs = append(actorAttrs, slog.Group("entitlements", entitlementAttrs...))
	}
	attrs = append(attrs, slog.Group("actor", actorAttrs...))

	// Add remaining attributes
	attrs = append(attrs,
		slog.String("action", event.Action),
		slog.String("category", string(event.Category)),
		slog.String("result", string(event.Result)),
		slog.String("request_id", event.RequestID),
		slog.String("source_ip", event.SourceIP),
		slog.String("service", event.Service),
	)

	// Add resource if present
	if event.Resource != nil {
		resourceAttrs := []any{
			slog.String("type", event.Resource.Type),
		}
		if event.Resource.ID != "" {
			resourceAttrs = append(resourceAttrs, slog.String("id", event.Resource.ID))
		}
		if event.Resource.Name != "" {
			resourceAttrs = append(resourceAttrs, slog.String("name", event.Resource.Name))
		}
		attrs = append(attrs, slog.Group("resource", resourceAttrs...))
	}

	// Add metadata if present
	if len(event.Metadata) > 0 {
		metadataAttrs := make([]any, 0, len(event.Metadata))
		for k, v := range event.Metadata {
			metadataAttrs = append(metadataAttrs, slog.Any(k, v))
		}
		attrs = append(attrs, slog.Group("metadata", metadataAttrs...))
	}

	// Emit the audit log
	l.slogger.Info("AUDIT-LOG", attrs...)
}
