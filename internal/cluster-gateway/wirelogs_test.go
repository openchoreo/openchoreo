// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
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

// --- handleWirelogs fake-stream tests ------------------------------------

// fakeAgentConn captures the messages handleWirelogs would have written to the
// agent. The onInit hook lets a test inject sentinel/flow/close chunks back
// into the server's stream session, mimicking what the real agent would do.
type fakeAgentConn struct {
	mu       sync.Mutex
	sent     [][]byte
	sendErr  error
	onInit   func(initData []byte)
	initOnce atomic.Bool
}

func (f *fakeAgentConn) SendRawMessage(data []byte) error {
	f.mu.Lock()
	copyOf := make([]byte, len(data))
	copy(copyOf, data)
	f.sent = append(f.sent, copyOf)
	hook := f.onInit
	err := f.sendErr
	f.mu.Unlock()
	if err != nil {
		return err
	}
	if hook != nil && f.initOnce.CompareAndSwap(false, true) {
		hook(copyOf)
	}
	return nil
}

func (f *fakeAgentConn) sentMessages() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, len(f.sent))
	copy(out, f.sent)
	return out
}

// stubGetAgentConnectionForWirelogs replaces the package-level getter and
// restores it on test cleanup.
func stubGetAgentConnectionForWirelogs(t *testing.T, conn wirelogsAgentConn, err error) {
	t.Helper()
	prev := getAgentConnectionForWirelogs
	getAgentConnectionForWirelogs = func(_ *Server, _, _ string) (wirelogsAgentConn, error) {
		if err != nil {
			return nil, err
		}
		return conn, nil
	}
	t.Cleanup(func() { getAgentConnectionForWirelogs = prev })
}

// newWirelogsTestServer returns a Server with only the fields handleWirelogs
// touches initialized. The connMgr is left nil because the seam bypasses it.
func newWirelogsTestServer() *Server {
	return &Server{
		pendingStreamSessions: make(map[string]*streamSession),
		logger:                slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func newWirelogsRequest(t *testing.T, target, query string) (*http.Request, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, target+"?"+query, nil)
	req.Header.Set("X-Request-ID", "test-wirelogs-req")
	return req, cancel
}

func TestHandleWirelogs_InvalidURL(t *testing.T) {
	s := newWirelogsTestServer()
	req, cancel := newWirelogsRequest(t, "/api/wirelogs/dataplane/p1", "environment=dev&namespace=ns")
	defer cancel()
	rec := newFlushingRecorder()

	s.handleWirelogs(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid wirelogs URL")
}

func TestHandleWirelogs_MissingRequiredQueryParams(t *testing.T) {
	s := newWirelogsTestServer()
	req, cancel := newWirelogsRequest(t, "/api/wirelogs/dataplane/p1/ns1/cr1", "environment=dev")
	defer cancel()
	rec := newFlushingRecorder()

	s.handleWirelogs(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "environment and namespace")
}

// noFlushRecorder forwards to httptest.ResponseRecorder but does NOT promote
// its Flush method, so the http.Flusher type assertion in handleWirelogs fails.
// (ResponseRecorder itself implements Flusher since Go 1.10.)
type noFlushRecorder struct {
	rec *httptest.ResponseRecorder
}

func (n *noFlushRecorder) Header() http.Header         { return n.rec.Header() }
func (n *noFlushRecorder) Write(b []byte) (int, error) { return n.rec.Write(b) }
func (n *noFlushRecorder) WriteHeader(code int)        { n.rec.WriteHeader(code) }

func TestHandleWirelogs_NoFlusherSupport(t *testing.T) {
	s := newWirelogsTestServer()
	req, cancel := newWirelogsRequest(t, "/api/wirelogs/dataplane/p1/ns1/cr1", "environment=dev&namespace=ns")
	defer cancel()
	rec := &noFlushRecorder{rec: httptest.NewRecorder()}

	s.handleWirelogs(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.rec.Code)
	assert.Contains(t, rec.rec.Body.String(), "streaming unsupported")
}

func TestHandleWirelogs_NoAgentAvailable(t *testing.T) {
	stubGetAgentConnectionForWirelogs(t, nil, errors.New("no connections registered"))

	s := newWirelogsTestServer()
	req, cancel := newWirelogsRequest(t, "/api/wirelogs/dataplane/p1/ns1/cr1", "environment=dev&namespace=ns")
	defer cancel()
	rec := newFlushingRecorder()

	s.handleWirelogs(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "no connections registered")
}

func TestHandleWirelogs_InitSendFailure(t *testing.T) {
	fake := &fakeAgentConn{sendErr: errors.New("websocket closed")}
	stubGetAgentConnectionForWirelogs(t, fake, nil)

	s := newWirelogsTestServer()
	req, cancel := newWirelogsRequest(t, "/api/wirelogs/dataplane/p1/ns1/cr1", "environment=dev&namespace=ns")
	defer cancel()
	rec := newFlushingRecorder()

	s.handleWirelogs(rec, req)

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "failed to start stream")
	assert.Empty(t, s.pendingStreamSessions, "session must be unregistered on failure")
}

func TestHandleWirelogs_AgentRejectsImmediately(t *testing.T) {
	// Agent sends an IsClose as the very first frame — handler must return 502
	// before committing to SSE headers.
	s := newWirelogsTestServer()
	fake := &fakeAgentConn{
		onInit: func(_ []byte) {
			s.handleStreamChunk(&messaging.HTTPTunnelStreamChunk{
				RequestID: "test-wirelogs-req",
				Data:      []byte("hubble misconfigured"),
				IsClose:   true,
			})
		},
	}
	stubGetAgentConnectionForWirelogs(t, fake, nil)

	req, cancel := newWirelogsRequest(t, "/api/wirelogs/dataplane/p1/ns1/cr1", "environment=dev&namespace=ns")
	defer cancel()
	rec := newFlushingRecorder()

	s.handleWirelogs(rec, req)

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "hubble misconfigured")
}

func TestHandleWirelogs_HappyPath(t *testing.T) {
	s := newWirelogsTestServer()

	// On init, push: empty-Data sentinel → two flow chunks → terminal IsClose.
	// The handler's loop should write 2 SSE events and exit cleanly.
	fake := &fakeAgentConn{
		onInit: func(_ []byte) {
			go func() {
				s.handleStreamChunk(&messaging.HTTPTunnelStreamChunk{
					RequestID: "test-wirelogs-req",
				})
				s.handleStreamChunk(&messaging.HTTPTunnelStreamChunk{
					RequestID: "test-wirelogs-req",
					Data:      []byte(`{"flow":"a"}`),
				})
				s.handleStreamChunk(&messaging.HTTPTunnelStreamChunk{
					RequestID: "test-wirelogs-req",
					Data:      []byte(`{"flow":"b"}`),
				})
				s.handleStreamChunk(&messaging.HTTPTunnelStreamChunk{
					RequestID: "test-wirelogs-req",
					IsClose:   true,
				})
			}()
		},
	}
	stubGetAgentConnectionForWirelogs(t, fake, nil)

	req, cancel := newWirelogsRequest(t,
		"/api/wirelogs/dataplane/p1/ns1/cr1",
		"environment=dev&namespace=ns&project=shopfront&component=checkout")
	// Cancel the request after the handler returns so the watchdog goroutine
	// inside handleWirelogs unblocks instead of leaking past the test.
	defer cancel()
	rec := newFlushingRecorder()

	done := make(chan struct{})
	go func() {
		s.handleWirelogs(rec, req)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not exit after IsClose chunk")
	}

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	body := rec.Body.String()
	assert.Equal(t, "data: {\"flow\":\"a\"}\n\ndata: {\"flow\":\"b\"}\n\n", body,
		"two flow chunks should become two `data:` SSE frames; sentinel carries no payload")
	assert.GreaterOrEqual(t, rec.flushed, 3,
		"flush at least once for headers commit + once per event")

	// Verify the init payload sent to the agent encodes the query params correctly.
	sent := fake.sentMessages()
	require.NotEmpty(t, sent)
	var init messaging.HTTPTunnelStreamInit
	require.NoError(t, json.Unmarshal(sent[0], &init))
	assert.Equal(t, "hubble", init.Target)
	assert.True(t, init.IsUpgrade)
	assert.Equal(t, "test-wirelogs-req", init.RequestID)
	assert.Contains(t, init.Query, "environment=dev")
	assert.Contains(t, init.Query, "namespace=ns")
	assert.Contains(t, init.Query, "project=shopfront")
	assert.Contains(t, init.Query, "component=checkout")

	assert.Empty(t, s.pendingStreamSessions, "session must be deregistered after stream ends")
}
