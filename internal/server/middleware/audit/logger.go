// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"log/slog"
)

// Logger handles emitting audit log events using structured logging. It is a
// pure reader of Event — EventID/Timestamp/Service are stamped once by
// buildEvent (emitter.go), not here, so a second sink can't see a different
// identity for the same event (see Emitter's doc comment).
type Logger struct {
	slogger *slog.Logger
}

// forceLevelHandler wraps a slog.Handler so every record is treated as
// enabled, regardless of the minimum level the wrapped handler was
// constructed with. Audit events must not be silently dropped by the
// application's log-level configuration (e.g. logging.level: warn), since
// audit.enabled is meant to be the only kill switch for audit output.
type forceLevelHandler struct {
	slog.Handler
}

// Enabled always returns true so the wrapped handler's level filter never
// suppresses an audit record.
func (h *forceLevelHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

// WithAttrs and WithGroup re-wrap the result in a forceLevelHandler. Without
// these, the embedded slog.Handler's own WithAttrs/WithGroup would return the
// *inner* handler directly — silently dropping the always-enabled override on
// any derived logger, the same silent-drop failure mode this handler exists
// to close. Unreachable today (LogEvent never derives a logger via .With),
// but left un-implemented it is a landmine for the next caller that does.
func (h *forceLevelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &forceLevelHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *forceLevelHandler) WithGroup(name string) slog.Handler {
	return &forceLevelHandler{Handler: h.Handler.WithGroup(name)}
}

// NewLogger creates a new audit logger. It reuses the given logger's handler
// (output stream, format, and any attrs already attached, e.g. "component")
// but forces every record through regardless of the handler's configured
// minimum level.
func NewLogger(slogger *slog.Logger) *Logger {
	return &Logger{slogger: slog.New(&forceLevelHandler{Handler: slogger.Handler()})}
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

	if event.ResourceType != "" || event.Resource != nil || event.Hierarchy != (Hierarchy{}) {
		var resourceAttrs []any
		if event.ResourceType != "" {
			resourceAttrs = append(resourceAttrs, slog.String("type", event.ResourceType))
		}
		// Same precedence as Event.MarshalJSON and buildEvent's
		// withHierarchyNamespaceFallback: the hierarchy's namespace unless
		// the Resource carries one of its own. Keeping the two render paths
		// identical here is what TestEvent_MarshalJSONMatchesLogEventShape
		// guards.
		namespace := event.Hierarchy.Namespace
		if event.Resource != nil && event.Resource.Namespace != "" {
			namespace = event.Resource.Namespace
		}
		if namespace != "" {
			resourceAttrs = append(resourceAttrs, slog.String("namespace", namespace))
		}
		if event.Hierarchy.Project != "" {
			resourceAttrs = append(resourceAttrs, slog.String("project", event.Hierarchy.Project))
		}
		if event.Hierarchy.Component != "" {
			resourceAttrs = append(resourceAttrs, slog.String("component", event.Hierarchy.Component))
		}
		if event.Hierarchy.Resource != "" {
			resourceAttrs = append(resourceAttrs, slog.String("resource", event.Hierarchy.Resource))
		}
		if event.Resource != nil {
			if event.Resource.ID != "" {
				resourceAttrs = append(resourceAttrs, slog.String("id", event.Resource.ID))
			}
			if event.Resource.Name != "" {
				resourceAttrs = append(resourceAttrs, slog.String("name", event.Resource.Name))
			}
			if len(event.Resource.Metadata) > 0 {
				metadataAttrs := make([]any, 0, len(event.Resource.Metadata))
				for k, v := range event.Resource.Metadata {
					metadataAttrs = append(metadataAttrs, slog.Any(k, v))
				}
				resourceAttrs = append(resourceAttrs, slog.Group("metadata", metadataAttrs...))
			}
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
