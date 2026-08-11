// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"log/slog"
)

// Logger handles emitting audit log events using structured logging. It is a
// pure reader of Event — EventID/Timestamp/Service are stamped once by
// buildEvent (emitter.go), not here, so a second sink can't see a different
// identity for the same event (see Emitter's doc comment).
type Logger struct {
	slogger *slog.Logger
}

// NewLogger creates a new audit logger
func NewLogger(slogger *slog.Logger) *Logger {
	return &Logger{slogger: slogger}
}

// LogEvent emits an audit log event using slog
func (l *Logger) LogEvent(event *Event) {
	attrs := []any{
		slog.String("event_id", event.EventID),
		slog.Time("timestamp", event.Timestamp),
	}

	actorAttrs := []any{
		slog.String("type", event.Actor.Type),
		slog.String("id", event.Actor.ID),
	}
	if len(event.Actor.Entitlements) > 0 {
		entitlementAttrs := make([]any, 0, len(event.Actor.Entitlements))
		for k, v := range event.Actor.Entitlements {
			entitlementAttrs = append(entitlementAttrs, slog.Any(k, v))
		}
		actorAttrs = append(actorAttrs, slog.Group("entitlements", entitlementAttrs...))
	}
	attrs = append(attrs, slog.Group("actor", actorAttrs...))

	attrs = append(attrs,
		slog.String("action", event.Action),
		slog.String("category", string(event.Category)),
		slog.String("result", string(event.Result)),
		slog.String("request_id", event.RequestID),
		slog.String("source_ip", event.SourceIP),
		slog.String("service", event.Service),
	)
	if event.Origin != "" {
		attrs = append(attrs, slog.String("origin", string(event.Origin)))
	}
	if event.OperationID != "" {
		attrs = append(attrs, slog.String("operation_id", event.OperationID))
	}

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

	if len(event.Metadata) > 0 {
		metadataAttrs := make([]any, 0, len(event.Metadata))
		for k, v := range event.Metadata {
			metadataAttrs = append(metadataAttrs, slog.Any(k, v))
		}
		attrs = append(attrs, slog.Group("metadata", metadataAttrs...))
	}

	l.slogger.Info("AUDIT-LOG", attrs...)
}
