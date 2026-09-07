// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// newAuditSink returns an Emitter writing every published event as JSON into
// the returned buffer, plus the buffer itself. The policy set is the
// publish-everything default, so a test asserting an event was *not* emitted
// is asserting about the middleware chain rather than about policy.
func newAuditSink(t *testing.T) (*audit.Emitter, *bytes.Buffer) {
	t.Helper()

	var buf bytes.Buffer
	sinkLogger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	policies, errs := audit.NewPolicySet(coreconfig.NewPath("audit"), audit.Settings{Publish: true}, nil)
	require.Empty(t, errs, "default policy set must build cleanly")

	emitter, err := audit.NewEmitter("observer-test", policies, audit.NewLogger(sinkLogger))
	require.NoError(t, err)

	return emitter, &buf
}

// auditEvents decodes every audit event the sink buffer captured — one per
// line. The event fields sit among the record's own attributes, so the whole
// line is returned and callers reach into it.
func auditEvents(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var events []map[string]any
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	for dec.More() {
		var line map[string]any
		require.NoError(t, dec.Decode(&line))
		events = append(events, line)
	}
	return events
}

// noopAuditEmitter returns an Emitter whose events go nowhere, for tests that
// need the composers to build but assert nothing about audit. Test-only:
// production shares one real Emitter across all three composers.
func noopAuditEmitter(t *testing.T) *audit.Emitter {
	t.Helper()
	emitter, _ := newAuditSink(t)
	return emitter
}
