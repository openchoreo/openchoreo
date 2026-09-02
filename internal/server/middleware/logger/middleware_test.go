// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// newTestLogger returns a JSON-handler logger and the buffer it writes to, so
// a test can assert on individual log lines by decoding each one.
func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, nil)), &buf
}

// logLines decodes every JSON line in buf into a map, skipping blanks.
func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for raw := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if raw == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatalf("failed to decode log line %q: %v", raw, err)
		}
		lines = append(lines, m)
	}
	return lines
}

// TestMiddleware_ValidRequestIDPassedThrough guards the common case: a
// client-supplied UUID must reach both the downstream handler's header and
// the ACCESS-LOG line unchanged, since that's what lets an operator
// correlate an access log entry with the audit event for the same request.
func TestMiddleware_ValidRequestIDPassedThrough(t *testing.T) {
	baseLogger, buf := newTestLogger()

	want := uuid.New().String()
	var gotDownstream string
	handler := Middleware(baseLogger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotDownstream = r.Header.Get("X-Request-ID")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.Header.Set("X-Request-ID", want)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if gotDownstream != want {
		t.Errorf("downstream request ID = %q, want %q (client-supplied UUID must pass through unchanged)", gotDownstream, want)
	}

	lines := logLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d log lines, want 1 (only the ACCESS-LOG line, no rejection warning)", len(lines))
	}
	if got := lines[0]["request_id"]; got != want {
		t.Errorf("ACCESS-LOG request_id = %v, want %q", got, want)
	}
}

// TestMiddleware_MissingRequestIDGenerated guards that an absent header is
// not treated as a rejection (no warning logged) while still producing a
// valid UUID downstream and in the access log.
func TestMiddleware_MissingRequestIDGenerated(t *testing.T) {
	baseLogger, buf := newTestLogger()

	var gotDownstream string
	handler := Middleware(baseLogger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotDownstream = r.Header.Get("X-Request-ID")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if _, err := uuid.Parse(gotDownstream); err != nil {
		t.Errorf("downstream request ID = %q, want a generated UUID", gotDownstream)
	}

	lines := logLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d log lines, want 1 (an absent header must not log a rejection warning)", len(lines))
	}
	if lines[0]["msg"] != "ACCESS-LOG" {
		t.Errorf("log line msg = %v, want ACCESS-LOG", lines[0]["msg"])
	}
}

// TestMiddleware_InvalidRequestIDRejectedAndReplaced is the direct regression
// test for the access-log/audit-log correlation bug: this middleware used to
// accept any non-empty X-Request-ID verbatim, while audit.RequestIDFromHeader
// downstream independently rejected a non-UUID value and generated its own
// replacement — so the access log and the audit event for the same request
// carried two different, unrelated request IDs. Now this middleware is the
// single normalizer: an invalid client-supplied value must be replaced here,
// before either log line is produced, and the same generated value must
// reach both the downstream handler and the access log.
func TestMiddleware_InvalidRequestIDRejectedAndReplaced(t *testing.T) {
	baseLogger, buf := newTestLogger()

	const clientValue = "not-a-uuid"
	var gotDownstream string
	handler := Middleware(baseLogger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotDownstream = r.Header.Get("X-Request-ID")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.Header.Set("X-Request-ID", clientValue)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if gotDownstream == clientValue {
		t.Fatal("downstream request ID was the rejected client value; it must be replaced with a generated UUID")
	}
	if _, err := uuid.Parse(gotDownstream); err != nil {
		t.Errorf("downstream request ID = %q, want a generated UUID", gotDownstream)
	}

	lines := logLines(t, buf)
	if len(lines) != 2 {
		t.Fatalf("got %d log lines, want 2 (a rejection warning, then ACCESS-LOG)", len(lines))
	}

	warn := lines[0]
	if warn["level"] != "WARN" {
		t.Errorf("first log line level = %v, want WARN", warn["level"])
	}
	if warn["value"] != clientValue {
		t.Errorf("rejection warning value = %v, want the rejected client value %q", warn["value"], clientValue)
	}

	access := lines[1]
	if access["msg"] != "ACCESS-LOG" {
		t.Errorf("second log line msg = %v, want ACCESS-LOG", access["msg"])
	}
	if access["request_id"] != gotDownstream {
		t.Errorf("ACCESS-LOG request_id = %v, want the same generated UUID seen downstream (%q) — "+
			"a mismatch here is exactly the correlation bug this test guards", access["request_id"], gotDownstream)
	}
}

// TestMiddleware_RejectionLogTruncatesLongValue guards against a client using
// an oversized X-Request-ID header to inject an arbitrarily large,
// attacker-controlled string into the WARN log: the header carries no length
// limit of its own short of http.Server's MaxHeaderBytes (1MB by default), so
// logging it verbatim on every rejected request would let a client bloat logs
// at will. The rejected value must appear truncated, never in full.
func TestMiddleware_RejectionLogTruncatesLongValue(t *testing.T) {
	baseLogger, buf := newTestLogger()

	clientValue := strings.Repeat("a", 10_000)
	handler := Middleware(baseLogger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.Header.Set("X-Request-ID", clientValue)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	lines := logLines(t, buf)
	if len(lines) != 2 {
		t.Fatalf("got %d log lines, want 2 (a rejection warning, then ACCESS-LOG)", len(lines))
	}

	loggedValue, ok := lines[0]["value"].(string)
	if !ok {
		t.Fatalf("rejection warning has no string \"value\" field: %v", lines[0])
	}
	if loggedValue == clientValue {
		t.Fatal("rejection warning logged the full 10000-byte client value verbatim; it must be truncated")
	}
	if len(loggedValue) > maxLoggedRequestIDLen+len("...(truncated)") {
		t.Errorf("logged value is %d bytes, want at most %d (maxLoggedRequestIDLen plus the truncation marker)",
			len(loggedValue), maxLoggedRequestIDLen+len("...(truncated)"))
	}
}
