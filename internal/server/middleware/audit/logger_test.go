// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

// TestLogEvent_IncludesOriginAndOperationID guards against a regression where
// Origin and OperationID were added to Event (for the MCP adapter) but never
// wired into LogEvent's manually-built slog attrs, so every emitted record
// silently dropped both fields regardless of what Emit passed in.
func TestLogEvent_IncludesOriginAndOperationID(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(slog.New(slog.NewJSONHandler(&buf, nil)))

	logger.LogEvent(&Event{
		Actor:       Actor{Type: "user", ID: "u1"},
		Action:      "create_project",
		Category:    CategoryManagement,
		Origin:      OriginMCP,
		OperationID: testProjectOpID,
		Result:      ResultSuccess,
	})

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("failed to unmarshal log line: %v", err)
	}

	if record["origin"] != "mcp" {
		t.Errorf("origin = %v, want mcp", record["origin"])
	}
	if record["operation_id"] != testProjectOpID {
		t.Errorf("operation_id = %v, want CreateProject", record["operation_id"])
	}
}

// TestLogEvent_OmitsEmptyOriginAndOperationID confirms the REST adapter's
// events (which don't set OperationID today, and always set Origin=api) don't
// grow an empty operation_id field, and that a genuinely empty Origin is
// omitted rather than rendered as an empty string.
func TestLogEvent_OmitsEmptyOriginAndOperationID(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(slog.New(slog.NewJSONHandler(&buf, nil)))

	logger.LogEvent(&Event{
		Actor:    Actor{Type: "user", ID: "u1"},
		Action:   "create_project",
		Category: CategoryManagement,
		Result:   ResultSuccess,
	})

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("failed to unmarshal log line: %v", err)
	}

	if _, ok := record["origin"]; ok {
		t.Errorf("origin = %v, want absent when Origin is unset", record["origin"])
	}
	if _, ok := record["operation_id"]; ok {
		t.Errorf("operation_id = %v, want absent when OperationID is unset", record["operation_id"])
	}
}

// TestLogEvent_ResourceTypeIndependentOfResource guards the resource group's
// two independent sources: type comes from Event.ResourceType (stamped from
// the Operation), id/name come from Event.Resource (set by a handler, or nil
// on a pre-handler denial). The resource group must render with just a type
// when Resource is nil, and must never fall back to a Resource.Type field —
// there isn't one.
func TestLogEvent_ResourceTypeIndependentOfResource(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(slog.New(slog.NewJSONHandler(&buf, nil)))

	logger.LogEvent(&Event{
		Actor:        Actor{Type: "user", ID: "u1"},
		Action:       "update_project",
		Category:     CategoryManagement,
		Result:       ResultDenied,
		ResourceType: "project",
		Resource:     nil,
	})

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("failed to unmarshal log line: %v", err)
	}

	resource, ok := record["resource"].(map[string]any)
	if !ok {
		t.Fatal("resource group must be present when ResourceType is set, even with a nil Resource")
	}
	if resource["type"] != "project" {
		t.Errorf("resource.type = %v, want project", resource["type"])
	}
	if _, ok := resource["id"]; ok {
		t.Errorf("resource.id = %v, want absent", resource["id"])
	}
	if _, ok := resource["name"]; ok {
		t.Errorf("resource.name = %v, want absent", resource["name"])
	}
}

// TestEvent_MarshalJSONMatchesLogEventShape guards against a future sink that
// marshals *Event directly (e.g. a P5 webhook sink) publishing a different
// wire shape than Logger.LogEvent — both must render resource.type nested
// inside "resource", not as a sibling "resource_type" field.
func TestEvent_MarshalJSONMatchesLogEventShape(t *testing.T) {
	event := &Event{
		Actor:        Actor{Type: "user", ID: "u1"},
		Action:       "update_project",
		Category:     CategoryManagement,
		Result:       ResultSuccess,
		ResourceType: "project",
		Resource:     &Resource{ID: "uid-1", Name: "p1"},
	}

	var loggerRecord map[string]any
	var buf bytes.Buffer
	NewLogger(slog.New(slog.NewJSONHandler(&buf, nil))).LogEvent(event)
	if err := json.Unmarshal(buf.Bytes(), &loggerRecord); err != nil {
		t.Fatalf("failed to unmarshal LogEvent output: %v", err)
	}

	var marshalRecord map[string]any
	marshaled, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal(event) failed: %v", err)
	}
	if err := json.Unmarshal(marshaled, &marshalRecord); err != nil {
		t.Fatalf("failed to unmarshal json.Marshal(event) output: %v", err)
	}

	loggerResource, ok := loggerRecord["resource"].(map[string]any)
	if !ok {
		t.Fatal("LogEvent output has no resource group")
	}
	marshalResource, ok := marshalRecord["resource"].(map[string]any)
	if !ok {
		t.Fatal("json.Marshal(event) output has no resource field")
	}

	if marshalResource["type"] != loggerResource["type"] {
		t.Errorf("resource.type = %v, want %v (matching LogEvent)", marshalResource["type"], loggerResource["type"])
	}
	if marshalResource["id"] != loggerResource["id"] {
		t.Errorf("resource.id = %v, want %v (matching LogEvent)", marshalResource["id"], loggerResource["id"])
	}
	if marshalResource["name"] != loggerResource["name"] {
		t.Errorf("resource.name = %v, want %v (matching LogEvent)", marshalResource["name"], loggerResource["name"])
	}
	if _, present := marshalRecord["resource_type"]; present {
		t.Error(`json.Marshal(event) must not emit a sibling "resource_type" field`)
	}
}

// TestForceLevelHandler_WithAttrsAndWithGroupPreserveForce guards against the
// always-enabled override silently disappearing through .WithAttrs/.WithGroup
// — slog.Handler's embedding means those methods, left unimplemented, would
// return the *inner* handler directly, dropping the force. Unreachable via
// LogEvent today (it never derives a logger), but a landmine for the next
// caller that does.
func TestForceLevelHandler_WithAttrsAndWithGroupPreserveForce(t *testing.T) {
	newBase := func(buf *bytes.Buffer) slog.Handler {
		return slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelError})
	}

	t.Run("WithAttrs", func(t *testing.T) {
		var buf bytes.Buffer
		h := &forceLevelHandler{Handler: newBase(&buf)}
		derived := h.WithAttrs([]slog.Attr{slog.String("k", "v")})

		if !derived.Enabled(context.Background(), slog.LevelInfo) {
			t.Fatal("expected the WithAttrs-derived handler to still report Enabled=true below the wrapped handler's minimum level")
		}
		slog.New(derived).Info("test message")
		if buf.Len() == 0 {
			t.Fatal("expected a record via the WithAttrs-derived handler despite LevelError, got none")
		}
	})

	t.Run("WithGroup", func(t *testing.T) {
		var buf bytes.Buffer
		h := &forceLevelHandler{Handler: newBase(&buf)}
		derived := h.WithGroup("g")

		if !derived.Enabled(context.Background(), slog.LevelInfo) {
			t.Fatal("expected the WithGroup-derived handler to still report Enabled=true below the wrapped handler's minimum level")
		}
		slog.New(derived).Info("test message")
		if buf.Len() == 0 {
			t.Fatal("expected a record via the WithGroup-derived handler despite LevelError, got none")
		}
	})
}

// TestLogEvent_NotGatedByAppLogLevel guards against audit events being
// silently dropped when the application logger is configured with a level
// above Info (e.g. logging.level: warn in Helm values). audit.enabled must be
// the only kill switch for audit output.
func TestLogEvent_NotGatedByAppLogLevel(t *testing.T) {
	var buf bytes.Buffer
	appLogger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))
	logger := NewLogger(appLogger)

	logger.LogEvent(&Event{
		Actor:    Actor{Type: "user", ID: "u1"},
		Action:   "create_project",
		Category: CategoryManagement,
		Result:   ResultSuccess,
	})

	if buf.Len() == 0 {
		t.Fatal("expected audit record to be emitted despite app logger's LevelError filter, got no output")
	}

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("failed to unmarshal log line: %v", err)
	}
	if record["action"] != "create_project" {
		t.Errorf("action = %v, want create_project", record["action"])
	}
}
