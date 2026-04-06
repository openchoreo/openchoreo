// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

// mockConnection implements the Connection interface for testing.
type mockConnection struct {
	mu           sync.Mutex
	readMessages [][]byte // pre-loaded messages for ReadMessage()
	readIndex    int
	writtenMsgs  [][]byte // captured messages from WriteMessage()
	writeErr     error    // error to return from WriteMessage()
	closed       bool
}

func (m *mockConnection) ReadMessage() (int, []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.readIndex >= len(m.readMessages) {
		// No more messages — simulate connection close
		return 0, nil, fmt.Errorf("mock connection closed")
	}
	msg := m.readMessages[m.readIndex]
	m.readIndex++
	return websocket.TextMessage, msg, nil
}

func (m *mockConnection) WriteMessage(_ int, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.writeErr != nil {
		return m.writeErr
	}
	m.writtenMsgs = append(m.writtenMsgs, data)
	return nil
}

func (m *mockConnection) WriteControl(_ int, _ []byte, _ time.Time) error {
	return nil
}

func (m *mockConnection) SetPingHandler(_ func(string) error) {}

func (m *mockConnection) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockConnection) getWrittenMessages() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([][]byte, len(m.writtenMsgs))
	copy(result, m.writtenMsgs)
	return result
}

func (m *mockConnection) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// newTestWSServer creates an httptest WebSocket server with a custom handler.
// Used only for TestAgent_Connect which tests the real connect() method.
func newTestWSServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(handler))
}

// toWSURL converts an http:// URL to ws:// for WebSocket dialing.
func toWSURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http")
}

// newTestAgent creates an Agent configured for testing with a given server URL and router.
func newTestAgent(t *testing.T, serverURL string, router *Router) *Agent {
	t.Helper()
	return &Agent{
		config: &Config{
			ServerURL:      serverURL,
			PlaneType:      "dataplane",
			PlaneID:        "test-plane",
			TLSEnabled:     false,
			ReconnectDelay: 100 * time.Millisecond,
		},
		router:   router,
		logger:   testLogger(),
		stopChan: make(chan struct{}),
	}
}

// --- Tests using mockConnection (no real WebSocket) ---

func TestAgent_CloseConnection(t *testing.T) {
	mock := &mockConnection{}
	agent := newTestAgent(t, "ws://unused", nil)
	agent.conn = mock

	assert.NotNil(t, agent.conn)

	agent.closeConnection()
	assert.Nil(t, agent.conn)
	assert.True(t, mock.isClosed())

	// Safe to call again
	agent.closeConnection()
	assert.Nil(t, agent.conn)
}

func TestAgent_SendHTTPTunnelResponse_NotConnected(t *testing.T) {
	agent := newTestAgent(t, "ws://localhost:0", nil)

	resp := &messaging.HTTPTunnelResponse{
		RequestID:  "req-1",
		StatusCode: 200,
	}

	err := agent.sendHTTPTunnelResponse(resp)
	assert.ErrorIs(t, err, messaging.ErrNotConnected)
}

func TestAgent_SendHTTPTunnelResponse_Success(t *testing.T) {
	mock := &mockConnection{}
	agent := newTestAgent(t, "ws://unused", nil)
	agent.conn = mock

	resp := &messaging.HTTPTunnelResponse{
		RequestID:  "req-123",
		StatusCode: 200,
		Body:       []byte(`{"ok":true}`),
	}

	err := agent.sendHTTPTunnelResponse(resp)
	require.NoError(t, err)

	written := mock.getWrittenMessages()
	require.Len(t, written, 1)

	var got messaging.HTTPTunnelResponse
	require.NoError(t, json.Unmarshal(written[0], &got))
	assert.Equal(t, "req-123", got.RequestID)
	assert.Equal(t, 200, got.StatusCode)
}

func TestAgent_SendHTTPTunnelResponse_WriteError(t *testing.T) {
	mock := &mockConnection{writeErr: fmt.Errorf("write failed")}
	agent := newTestAgent(t, "ws://unused", nil)
	agent.conn = mock

	resp := &messaging.HTTPTunnelResponse{
		RequestID:  "req-1",
		StatusCode: 200,
	}

	err := agent.sendHTTPTunnelResponse(resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write message")
}

func TestAgent_HandleHTTPTunnelRequest(t *testing.T) {
	mock := &mockConnection{}

	mockRoute := newMockRoute("k8s", "https://kubernetes.svc", func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"items":[]}`)),
		}, nil
	})
	router := newTestRouter(t, map[string]*Route{"k8s": mockRoute})

	agent := newTestAgent(t, "ws://unused", router)
	agent.conn = mock

	tunnelReq := &messaging.HTTPTunnelRequest{
		RequestID: "req-handle-1",
		Target:    "k8s",
		Method:    "GET",
		Path:      "/api/v1/pods",
	}

	agent.handleHTTPTunnelRequest(tunnelReq)

	written := mock.getWrittenMessages()
	require.Len(t, written, 1)

	var got messaging.HTTPTunnelResponse
	require.NoError(t, json.Unmarshal(written[0], &got))
	assert.Equal(t, "req-handle-1", got.RequestID)
	assert.Equal(t, http.StatusOK, got.StatusCode)
}

func TestAgent_HandleConnection(t *testing.T) {
	tunnelReq := &messaging.HTTPTunnelRequest{
		RequestID: "req-conn-1",
		Target:    "k8s",
		Method:    "GET",
		Path:      "/api/v1/pods",
	}
	reqData, _ := json.Marshal(tunnelReq)

	mock := &mockConnection{
		readMessages: [][]byte{reqData},
	}

	mockRoute := newMockRoute("k8s", "https://kubernetes.svc", func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		}, nil
	})
	router := newTestRouter(t, map[string]*Route{"k8s": mockRoute})

	agent := newTestAgent(t, "ws://unused", router)
	agent.conn = mock

	// handleConnection blocks until ReadMessage returns error (no more messages)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agent.handleConnection(ctx)

	// Give goroutine time to write response
	time.Sleep(50 * time.Millisecond)

	written := mock.getWrittenMessages()
	require.Len(t, written, 1)

	var got messaging.HTTPTunnelResponse
	require.NoError(t, json.Unmarshal(written[0], &got))
	assert.Equal(t, "req-conn-1", got.RequestID)
	assert.Equal(t, http.StatusOK, got.StatusCode)
}

func TestAgent_HandleConnection_InvalidMessage(t *testing.T) {
	mock := &mockConnection{
		readMessages: [][]byte{[]byte("not json")},
	}

	router := newTestRouter(t, map[string]*Route{})
	agent := newTestAgent(t, "ws://unused", router)
	agent.conn = mock

	// Should not panic on invalid message — just skips it and exits when no more messages
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agent.handleConnection(ctx)
}

func TestAgent_HandleConnection_MissingRequestID(t *testing.T) {
	req := &messaging.HTTPTunnelRequest{
		Target: "k8s",
		Method: "GET",
		Path:   "/api/v1/pods",
		// No RequestID
	}
	reqData, _ := json.Marshal(req)

	mock := &mockConnection{
		readMessages: [][]byte{reqData},
	}

	router := newTestRouter(t, map[string]*Route{})
	agent := newTestAgent(t, "ws://unused", router)
	agent.conn = mock

	// Should skip message without requestID and exit when no more messages
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agent.handleConnection(ctx)

	// No response should have been written
	assert.Empty(t, mock.getWrittenMessages())
}

// --- Tests that use real WebSocket (testing connect() itself) ---

func TestAgent_Connect(t *testing.T) {
	var capturedPlaneType, capturedPlaneID string

	srv := newTestWSServer(t, func(w http.ResponseWriter, r *http.Request) {
		capturedPlaneType = r.URL.Query().Get("planeType")
		capturedPlaneID = r.URL.Query().Get("planeID")
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	agent := newTestAgent(t, toWSURL(srv.URL), nil)
	err := agent.connect()
	require.NoError(t, err)
	defer agent.closeConnection()

	assert.Equal(t, "dataplane", capturedPlaneType)
	assert.Equal(t, "test-plane", capturedPlaneID)
}

func TestAgent_Connect_InvalidURL(t *testing.T) {
	agent := newTestAgent(t, "://invalid-url", nil)

	err := agent.connect()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid server URL")
}

func TestAgent_Connect_ServerUnavailable(t *testing.T) {
	agent := newTestAgent(t, "ws://localhost:1", nil)

	err := agent.connect()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dial failed")
}

// --- Tests for Start/Stop lifecycle ---

func TestAgent_HandleHTTPTunnelRequest_SendError(t *testing.T) {
	mock := &mockConnection{writeErr: fmt.Errorf("connection lost")}

	mockRoute := newMockRoute("k8s", "https://kubernetes.svc", func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	})
	router := newTestRouter(t, map[string]*Route{"k8s": mockRoute})

	agent := newTestAgent(t, "ws://unused", router)
	agent.conn = mock

	// Should not panic even when sending fails
	agent.handleHTTPTunnelRequest(&messaging.HTTPTunnelRequest{
		RequestID: "req-err",
		Target:    "k8s",
		Method:    "GET",
		Path:      "/api/v1/pods",
	})
}

func TestAgent_HandleConnection_ContextCancellation(t *testing.T) {
	// Mock that blocks on ReadMessage until context is canceled
	blockingMock := &blockingConnection{
		readCh: make(chan struct{}),
	}

	router := newTestRouter(t, map[string]*Route{})
	agent := newTestAgent(t, "ws://unused", router)
	agent.conn = blockingMock

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		agent.handleConnection(ctx)
		close(done)
	}()

	// Give handleConnection time to start the ctx watcher goroutine
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handleConnection did not exit after context cancellation")
	}
}

func TestAgent_Start_ConnectAndReconnect(t *testing.T) {
	// Server that accepts connection then immediately closes it
	var connectCount int32
	srv := newTestWSServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&connectCount, 1)
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		// Close immediately to trigger reconnection
		conn.Close()
	})
	defer srv.Close()

	router := newTestRouter(t, map[string]*Route{})
	agent := newTestAgent(t, toWSURL(srv.URL), router)
	agent.config.ReconnectDelay = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- agent.Start(ctx)
	}()

	// Wait deterministically for at least 2 connections (reconnect happened)
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&connectCount) > 1
	}, 5*time.Second, 20*time.Millisecond, "expected at least 2 connections")

	cancel()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return")
	}
}

func TestAgent_Stop(t *testing.T) {
	router := newTestRouter(t, map[string]*Route{})

	// Use an unreachable server so Start blocks on reconnect
	agent := newTestAgent(t, "ws://localhost:1", router)

	ctx := context.Background()
	done := make(chan error, 1)
	go func() {
		done <- agent.Start(ctx)
	}()

	// Give it a moment to start, then stop
	time.Sleep(50 * time.Millisecond)
	agent.Stop()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
}

func TestAgent_Start_ContextCancellation(t *testing.T) {
	router := newTestRouter(t, map[string]*Route{})

	agent := newTestAgent(t, "ws://localhost:1", router)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- agent.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

func TestAgent_Start_StopDuringReconnectWait(t *testing.T) {
	// Server that accepts then closes, triggering the reconnect wait path
	srv := newTestWSServer(t, func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.Close()
	})
	defer srv.Close()

	router := newTestRouter(t, map[string]*Route{})
	agent := newTestAgent(t, toWSURL(srv.URL), router)
	agent.config.ReconnectDelay = 10 * time.Second // Long delay

	done := make(chan error, 1)
	go func() {
		done <- agent.Start(context.Background())
	}()

	// Wait for it to connect, lose connection, and enter reconnect wait
	time.Sleep(200 * time.Millisecond)
	agent.Stop()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Stop during reconnect wait")
	}
}

func TestAgent_New_TLSDisabled(t *testing.T) {
	cfg := &Config{
		ServerURL:  "ws://localhost:8443",
		PlaneType:  "dataplane",
		PlaneID:    "test",
		TLSEnabled: false,
	}

	// NewRouter needs a rest.Config — provide a minimal one
	k8sConfig := &rest.Config{Host: "https://kubernetes.default.svc"}

	agent, err := New(cfg, nil, k8sConfig, testLogger())
	require.NoError(t, err)
	assert.NotNil(t, agent)
	assert.NotNil(t, agent.router)
	assert.NotNil(t, agent.stopChan)
	assert.Equal(t, "dataplane", agent.config.PlaneType)
}

func TestAgent_New_TLSEnabled_BadCert(t *testing.T) {
	cfg := &Config{
		ServerURL:      "ws://localhost:8443",
		PlaneType:      "dataplane",
		PlaneID:        "test",
		TLSEnabled:     true,
		ClientCertPath: "/nonexistent/cert.pem",
		ClientKeyPath:  "/nonexistent/key.pem",
	}

	k8sConfig := &rest.Config{Host: "https://kubernetes.default.svc"}

	_, err := New(cfg, nil, k8sConfig, testLogger())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load client certificate")
}

func TestAgent_HandleConnection_UnexpectedCloseError(t *testing.T) {
	// Return a CloseError with a code NOT in the expected list (GoingAway, AbnormalClosure)
	// so websocket.IsUnexpectedCloseError returns true
	mock := &closeErrorConnection{
		closeErr: &websocket.CloseError{
			Code: websocket.CloseInternalServerErr,
			Text: "internal server error",
		},
	}

	router := newTestRouter(t, map[string]*Route{})
	agent := newTestAgent(t, "ws://unused", router)
	agent.conn = mock

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should log "websocket error" (unexpected close) and return
	agent.handleConnection(ctx)
}

// closeErrorConnection returns a specific websocket close error from ReadMessage.
type closeErrorConnection struct {
	closeErr error
}

func (c *closeErrorConnection) ReadMessage() (int, []byte, error) {
	return 0, nil, c.closeErr
}
func (c *closeErrorConnection) WriteMessage(_ int, _ []byte) error              { return nil }
func (c *closeErrorConnection) WriteControl(_ int, _ []byte, _ time.Time) error { return nil }
func (c *closeErrorConnection) SetPingHandler(_ func(string) error)             {}
func (c *closeErrorConnection) Close() error                                    { return nil }

// blockingConnection blocks on ReadMessage until Close is called.
type blockingConnection struct {
	readCh chan struct{}
	closed bool
	mu     sync.Mutex
}

func (b *blockingConnection) ReadMessage() (int, []byte, error) {
	<-b.readCh
	return 0, nil, fmt.Errorf("connection closed")
}

func (b *blockingConnection) WriteMessage(_ int, _ []byte) error { return nil }
func (b *blockingConnection) WriteControl(_ int, _ []byte, _ time.Time) error {
	return nil
}
func (b *blockingConnection) SetPingHandler(_ func(string) error) {}
func (b *blockingConnection) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.closed = true
		close(b.readCh)
	}
	return nil
}
