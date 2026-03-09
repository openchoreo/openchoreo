// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

func TestIsStreamingRequest(t *testing.T) {
	s := &Server{logger: testLogger()}

	tests := []struct {
		name       string
		url        string
		path       string
		headers    map[string]string
		expectBool bool
	}{
		{
			name:       "watch query param",
			url:        "/api/v1/pods?watch=true",
			path:       "/api/v1/pods",
			expectBool: true,
		},
		{
			name:       "watch=false is not streaming",
			url:        "/api/v1/pods?watch=false",
			path:       "/api/v1/pods",
			expectBool: false,
		},
		{
			name:       "log with follow",
			url:        "/api/v1/pods/x/log?follow=true",
			path:       "/api/v1/pods/x/log",
			expectBool: true,
		},
		{
			name:       "log without follow is not streaming",
			url:        "/api/v1/pods/x/log",
			path:       "/api/v1/pods/x/log",
			expectBool: false,
		},
		{
			name:       "follow without log path is not streaming",
			url:        "/api/v1/pods?follow=true",
			path:       "/api/v1/pods",
			expectBool: false,
		},
		{
			name:       "Connection: Upgrade header",
			url:        "/api/v1/pods/x/exec",
			path:       "/api/v1/pods/x/exec",
			headers:    map[string]string{"Connection": "Upgrade"},
			expectBool: true,
		},
		{
			name:       "Upgrade header present",
			url:        "/api/v1/pods/x/exec",
			path:       "/api/v1/pods/x/exec",
			headers:    map[string]string{"Upgrade": "SPDY/3.1"},
			expectBool: true,
		},
		{
			name:       "normal request",
			url:        "/api/v1/pods",
			path:       "/api/v1/pods",
			expectBool: false,
		},
		{
			name:       "watch in path but not query",
			url:        "/api/v1/watch/pods",
			path:       "/api/v1/watch/pods",
			expectBool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := http.NewRequest(http.MethodGet, tt.url, nil)
			require.NoError(t, err)
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			result := s.isStreamingRequest(r, tt.path)
			assert.Equal(t, tt.expectBool, result)
		})
	}
}

func TestGenerateRequestID(t *testing.T) {
	id := generateRequestID()
	assert.NotEmpty(t, id)
	assert.Contains(t, id, "gw-")
}

func TestGetOrGenerateRequestID(t *testing.T) {
	t.Run("uses existing X-Request-ID", func(t *testing.T) {
		r, _ := http.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Request-ID", "custom-id-123")
		assert.Equal(t, "custom-id-123", getOrGenerateRequestID(r))
	})

	t.Run("generates ID when header absent", func(t *testing.T) {
		r, _ := http.NewRequest(http.MethodGet, "/", nil)
		id := getOrGenerateRequestID(r)
		assert.NotEmpty(t, id)
		assert.Contains(t, id, "gw-")
	})
}

// newTestProxyServer creates a minimal Server suitable for testing handleHTTPProxy.
// It has a real ConnectionManager, validator, and initialized pendingHTTPRequests map
// but no WebSocket infrastructure.
func newTestProxyServer() *Server {
	return &Server{
		config:              &Config{},
		connMgr:             NewConnectionManager(testLogger()),
		validator:           NewRequestValidator(),
		pendingHTTPRequests: make(map[string]chan *messaging.HTTPTunnelResponse),
		logger:              testLogger(),
	}
}

func TestHandleHTTPProxy_URLParsing(t *testing.T) {
	s := newTestProxyServer()

	tests := []struct {
		name       string
		path       string
		expectCode int
	}{
		// Valid URLs pass parsing and fail later at tunnel stage (no agents)
		{
			name:       "valid URL with target path",
			path:       "/api/proxy/dataplane/prod/ns/cr1/k8s/api/v1/pods",
			expectCode: http.StatusBadGateway,
		},
		{
			name:       "trailing slash preserved in target path",
			path:       "/api/proxy/dataplane/prod/ns/cr1/k8s/",
			expectCode: http.StatusBadGateway,
		},
		// Too few segments
		{
			name:       "only 5 segments returns 400",
			path:       "/api/proxy/dataplane/prod/ns/cr1/k8s",
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "only 3 segments returns 400",
			path:       "/api/proxy/dataplane/prod/ns",
			expectCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			s.handleHTTPProxy(w, r)
			assert.Equal(t, tt.expectCode, w.Code)
		})
	}
}

func TestHandleHTTPProxy_EmptySegments(t *testing.T) {
	s := newTestProxyServer()

	// Call handleHTTPProxy directly (not via ServeMux) to avoid path cleaning
	tests := []struct {
		name string
		path string
	}{
		{"empty planeType", "/api/proxy//prod/ns/cr1/k8s/api/v1/pods"},
		{"empty planeID", "/api/proxy/dataplane//ns/cr1/k8s/api/v1/pods"},
		{"empty namespace", "/api/proxy/dataplane/prod//cr1/k8s/api/v1/pods"},
		{"empty crName", "/api/proxy/dataplane/prod/ns//k8s/api/v1/pods"},
		{"empty target", "/api/proxy/dataplane/prod/ns/cr1//api/v1/pods"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			s.handleHTTPProxy(w, r)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestHandleHTTPProxy_PlaneTypeValidation(t *testing.T) {
	s := newTestProxyServer()

	tests := []struct {
		name       string
		planeType  string
		expectCode int
	}{
		{"dataplane accepted", "dataplane", http.StatusBadGateway},
		{"workflowplane accepted", "workflowplane", http.StatusBadGateway},
		{"observabilityplane accepted", "observabilityplane", http.StatusBadGateway},
		{"buildplane rejected", "buildplane", http.StatusBadRequest},
		{"unknown rejected", "foobar", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/api/proxy/" + tt.planeType + "/prod/ns/cr1/k8s/api/v1/pods"
			r := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			s.handleHTTPProxy(w, r)
			assert.Equal(t, tt.expectCode, w.Code)
		})
	}
}

func TestHandleHTTPProxy_ErrNoAuthorizedAgents(t *testing.T) {
	s := newTestProxyServer()

	// Register a connection for the plane but with no valid CRs,
	// so GetForCR returns ErrNoAuthorizedAgents
	ws, cleanup := newMockWSConn(t)
	defer cleanup()
	s.pendingHTTPRequests = make(map[string]chan *messaging.HTTPTunnelResponse)
	s.requestsMu = sync.Mutex{}
	_, err := s.connMgr.Register("dataplane", "prod", ws, []string{}, nil)
	require.NoError(t, err)

	r := httptest.NewRequest(http.MethodGet, "/api/proxy/dataplane/prod/ns/cr1/k8s/api/v1/pods", nil)
	w := httptest.NewRecorder()
	s.handleHTTPProxy(w, r)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "no authorized agent")
}

