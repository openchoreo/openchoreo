// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"bytes"
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
