// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

func TestGenerateRequestID(t *testing.T) {
	id := generateRequestID()
	assert.True(t, strings.HasPrefix(id, "gw-"), "request ID should start with 'gw-'")
	assert.Greater(t, len(id), 3, "request ID should have content after prefix")

	// IDs should be unique
	id2 := generateRequestID()
	assert.NotEqual(t, id, id2)
}

func TestGetOrGenerateRequestID(t *testing.T) {
	t.Run("uses existing header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Request-ID", "custom-id-123")
		id := getOrGenerateRequestID(req)
		assert.Equal(t, "custom-id-123", id)
	})

	t.Run("generates when header missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		id := getOrGenerateRequestID(req)
		assert.True(t, strings.HasPrefix(id, "gw-"))
	})

	t.Run("generates when header empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Request-ID", "")
		id := getOrGenerateRequestID(req)
		assert.True(t, strings.HasPrefix(id, "gw-"))
	})
}

func TestHandleHealth(t *testing.T) {
	s := &Server{logger: testLogger()}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OK", w.Body.String())
}

func TestIsStreamingRequest(t *testing.T) {
	s := &Server{logger: testLogger()}

	tests := []struct {
		name     string
		url      string
		path     string
		headers  map[string]string
		expected bool
	}{
		{
			name:     "watch query param",
			url:      "/test?watch=true",
			path:     "/api/v1/pods",
			expected: true,
		},
		{
			name:     "log follow",
			url:      "/test?follow=true",
			path:     "/api/v1/namespaces/default/pods/mypod/log",
			expected: true,
		},
		{
			name:     "connection upgrade",
			url:      "/test",
			path:     "/api/v1/pods",
			headers:  map[string]string{"Connection": "Upgrade"},
			expected: true,
		},
		{
			name:     "upgrade header",
			url:      "/test",
			path:     "/api/v1/pods",
			headers:  map[string]string{"Upgrade": "SPDY/3.1"},
			expected: true,
		},
		{
			name:     "normal request",
			url:      "/test",
			path:     "/api/v1/pods",
			expected: false,
		},
		{
			name:     "follow without log path",
			url:      "/test?follow=true",
			path:     "/api/v1/pods",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			result := s.isStreamingRequest(req, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHandleStreamingProxy(t *testing.T) {
	s := &Server{logger: testLogger()}

	req := httptest.NewRequest(http.MethodGet, "/api/proxy/dataplane/prod/ns/dp1/k8s/api/v1/pods?watch=true", nil)
	w := httptest.NewRecorder()
	s.handleStreamingProxy(w, req, "dataplane/prod", "ns/dp1", "k8s", "/api/v1/pods")

	assert.Equal(t, http.StatusNotImplemented, w.Code)
	assert.Contains(t, w.Body.String(), "Streaming operations")
}

func TestHandleHTTPProxy_InvalidURL(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	s := New(&Config{}, fakeClient, testLogger())

	// URL with too few parts (need at least 6: planeType/planeID/ns/crName/target/path)
	req := httptest.NewRequest(http.MethodGet, "/api/proxy/dataplane/prod/ns", nil)
	w := httptest.NewRecorder()
	s.handleHTTPProxy(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid proxy URL format")
}

func TestHandleHTTPProxy_ValidationFailed(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	s := New(&Config{}, fakeClient, testLogger())

	// Use an invalid target to trigger validation error
	req := httptest.NewRequest(http.MethodGet, "/api/proxy/dataplane/prod/ns/dp1/invalid-target/api/v1/pods", nil)
	w := httptest.NewRecorder()
	s.handleHTTPProxy(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Target not allowed")
}

func TestHandleHTTPProxy_BlockedPath(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	s := New(&Config{}, fakeClient, testLogger())

	// Access kube-system secrets - should be blocked
	req := httptest.NewRequest(http.MethodGet, "/api/proxy/dataplane/prod/ns/dp1/k8s/api/v1/namespaces/kube-system/secrets", nil)
	w := httptest.NewRecorder()
	s.handleHTTPProxy(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHandleHTTPTunnelResponse(t *testing.T) {
	s := &Server{
		pendingHTTPRequests: make(map[string]chan *messaging.HTTPTunnelResponse),
		logger:              testLogger(),
	}

	t.Run("delivers response to waiting channel", func(t *testing.T) {
		ch := make(chan *messaging.HTTPTunnelResponse, 1)
		s.requestsMu.Lock()
		s.pendingHTTPRequests["req-123"] = ch
		s.requestsMu.Unlock()

		resp := &messaging.HTTPTunnelResponse{
			RequestID:  "req-123",
			StatusCode: 200,
		}

		s.handleHTTPTunnelResponse("dataplane/prod", resp)

		// Channel should receive the response
		select {
		case received := <-ch:
			assert.Equal(t, 200, received.StatusCode)
			assert.Equal(t, "req-123", received.RequestID)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for response")
		}

		// Request should be cleaned up
		s.requestsMu.Lock()
		_, exists := s.pendingHTTPRequests["req-123"]
		s.requestsMu.Unlock()
		assert.False(t, exists)
	})

	t.Run("unknown request does not panic", func(t *testing.T) {
		resp := &messaging.HTTPTunnelResponse{
			RequestID:  "unknown-req",
			StatusCode: 200,
		}

		// Should not panic
		s.handleHTTPTunnelResponse("dataplane/prod", resp)
	})
}

func TestNew(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	config := &Config{
		Port: 8443,
	}

	s := New(config, fakeClient, testLogger())

	assert.NotNil(t, s)
	assert.NotNil(t, s.connMgr)
	assert.NotNil(t, s.validator)
	assert.NotNil(t, s.pendingHTTPRequests)
	assert.Equal(t, config, s.config)
}

func TestSendHTTPTunnelRequest_Timeout(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	s := New(&Config{}, fakeClient, testLogger())

	// Register a connection
	conn, cleanup := newTestWSConn(t)
	defer cleanup()
	_, _ = s.connMgr.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil)

	req := &messaging.HTTPTunnelRequest{
		Target: "k8s",
		Method: "GET",
		Path:   "/api/v1/pods",
	}

	// Use a very short timeout to trigger timeout
	_, err := s.SendHTTPTunnelRequest("dataplane/prod", req, 10*time.Millisecond)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")

	// Pending request should be cleaned up after timeout
	s.requestsMu.Lock()
	assert.Empty(t, s.pendingHTTPRequests)
	s.requestsMu.Unlock()
}

func TestSendHTTPTunnelRequest_NoAgent(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	s := New(&Config{}, fakeClient, testLogger())

	req := &messaging.HTTPTunnelRequest{
		Target: "k8s",
		Method: "GET",
		Path:   "/api/v1/pods",
	}

	_, err := s.SendHTTPTunnelRequest("dataplane/nonexistent", req, time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no agents found")
}

func TestSendHTTPTunnelRequestForCR_NoAuthorizedAgent(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	s := New(&Config{}, fakeClient, testLogger())

	conn, cleanup := newTestWSConn(t)
	defer cleanup()
	_, _ = s.connMgr.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil)

	req := &messaging.HTTPTunnelRequest{
		Target: "k8s",
		Method: "GET",
		Path:   "/api/v1/pods",
	}

	_, err := s.SendHTTPTunnelRequestForCR("dataplane/prod", "ns/dp-other", req, time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no agents authorized for CR")
}

func TestSendHTTPTunnelRequestForCR_Success(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	s := New(&Config{}, fakeClient, testLogger())

	conn, cleanup := newTestWSConn(t)
	defer cleanup()
	_, _ = s.connMgr.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil)

	req := &messaging.HTTPTunnelRequest{
		Target: "k8s",
		Method: "GET",
		Path:   "/api/v1/pods",
	}

	// Send the request in a goroutine since it will block waiting for response
	var wg sync.WaitGroup
	var sendErr error
	var resp *messaging.HTTPTunnelResponse

	wg.Add(1)
	go func() {
		defer wg.Done()
		resp, sendErr = s.SendHTTPTunnelRequestForCR("dataplane/prod", "ns/dp1", req, 2*time.Second)
	}()

	// Wait a bit for the request to be registered, then deliver the response
	time.Sleep(50 * time.Millisecond)

	s.requestsMu.Lock()
	var requestID string
	for id := range s.pendingHTTPRequests {
		requestID = id
		break
	}
	s.requestsMu.Unlock()

	require.NotEmpty(t, requestID)

	s.handleHTTPTunnelResponse("dataplane/prod", &messaging.HTTPTunnelResponse{
		RequestID:  requestID,
		StatusCode: 200,
		Body:       []byte(`{"items":[]}`),
	})

	wg.Wait()
	require.NoError(t, sendErr)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetConnectionManager(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	s := New(&Config{}, fakeClient, testLogger())
	cm := s.GetConnectionManager()
	assert.NotNil(t, cm)
	assert.Equal(t, s.connMgr, cm)
}
