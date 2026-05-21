// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// httptest.ResponseRecorder does not implement http.Flusher; writeSSEEvent
// requires one, so wrap it.
type flushingRecorder struct {
	*httptest.ResponseRecorder
	flushed int
}

func (f *flushingRecorder) Flush() { f.flushed++ }

func newFlushingRecorder() *flushingRecorder {
	return &flushingRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func TestWriteSSEHeaders(t *testing.T) {
	rec := newFlushingRecorder()
	writeSSEHeaders(rec)

	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache, no-transform", rec.Header().Get("Cache-Control"))
	assert.Equal(t, "keep-alive", rec.Header().Get("Connection"))
	assert.Equal(t, "no", rec.Header().Get("X-Accel-Buffering"))
}

func TestWriteSSEEvent_SingleLineJSON(t *testing.T) {
	rec := newFlushingRecorder()
	ok := writeSSEEvent(rec, rec, []byte(`{"flow":1}`))
	assert.True(t, ok)
	assert.Equal(t, "data: {\"flow\":1}\n\n", rec.Body.String())
	assert.Equal(t, 1, rec.flushed, "should flush exactly once per event")
}

func TestWriteSSEEvent_MultiLineSplitsIntoDataLines(t *testing.T) {
	// Defensive: a payload containing a newline must become multiple `data:`
	// lines so the SSE framing stays valid.
	rec := newFlushingRecorder()
	ok := writeSSEEvent(rec, rec, []byte("alpha\nbeta\ngamma"))
	assert.True(t, ok)
	assert.Equal(t, "data: alpha\ndata: beta\ndata: gamma\n\n", rec.Body.String())
}
