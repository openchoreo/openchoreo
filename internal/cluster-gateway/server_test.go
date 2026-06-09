// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
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

// --- verifyClientCertificatePerCR tests ---

func TestVerifyClientCertificatePerCR_ValidCert(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	caPEM := encodeCertToPEM(t, caCert)

	scheme := testScheme()
	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp1", Namespace: "ns"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "prod",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: string(caPEM)},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dp).Build()
	s := &Server{k8sClient: fakeClient, logger: testLogger()}

	validCRs, err := s.verifyClientCertificatePerCR(clientCert, nil, "dataplane", "prod")
	require.NoError(t, err)
	assert.Contains(t, validCRs, "ns/dp1")
}

func TestVerifyClientCertificatePerCR_MultipleCRs(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	caPEM := encodeCertToPEM(t, caCert)

	otherCA, _ := generateTestCA(t)
	otherPEM := encodeCertToPEM(t, otherCA)

	scheme := testScheme()
	dp1 := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp1", Namespace: "ns-a"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "shared",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: string(caPEM)},
			},
		},
	}
	dp2 := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp2", Namespace: "ns-b"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "shared",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: string(otherPEM)}, // Different CA
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dp1, dp2).Build()
	s := &Server{k8sClient: fakeClient, logger: testLogger()}

	validCRs, err := s.verifyClientCertificatePerCR(clientCert, nil, "dataplane", "shared")
	require.NoError(t, err)
	assert.Contains(t, validCRs, "ns-a/dp1")
	assert.NotContains(t, validCRs, "ns-b/dp2")
}

func TestVerifyClientCertificatePerCR_NoCRsFound(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)

	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	s := &Server{k8sClient: fakeClient, logger: testLogger()}

	_, err := s.verifyClientCertificatePerCR(clientCert, nil, "dataplane", "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no dataplane CRs found")
}

func TestVerifyClientCertificatePerCR_CertInvalidForAll(t *testing.T) {
	// Client cert signed by one CA, but CR has a different CA
	clientCA, clientCAKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, clientCA, clientCAKey)

	differentCA, _ := generateTestCA(t)
	differentPEM := encodeCertToPEM(t, differentCA)

	scheme := testScheme()
	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp1", Namespace: "ns"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "prod",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: string(differentPEM)},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dp).Build()
	s := &Server{k8sClient: fakeClient, logger: testLogger()}

	_, err := s.verifyClientCertificatePerCR(clientCert, nil, "dataplane", "prod")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "certificate not valid for any CR")
}

func TestVerifyClientCertificatePerCR_NilCAData(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)

	scheme := testScheme()
	// DataPlane exists but has inline empty/nil CA value (nil CA data in the map)
	// Use inline Value so extractPlaneClientCAs includes it, but with content
	// that won't parse as a valid cert pool
	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp1", Namespace: "ns"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "prod",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: "not-a-valid-pem"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dp).Build()
	s := &Server{k8sClient: fakeClient, logger: testLogger()}

	_, err := s.verifyClientCertificatePerCR(clientCert, nil, "dataplane", "prod")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "certificate not valid for any CR")
}

func TestVerifyClientCertificatePerCR_WithIntermediates(t *testing.T) {
	// Create root CA
	rootCA, rootKey := generateTestCA(t)

	// Create intermediate CA signed by root
	intermediateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	intermediateTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(10),
		Subject:               pkix.Name{CommonName: "Intermediate CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	intermediateDER, err := x509.CreateCertificate(rand.Reader, intermediateTemplate, rootCA, &intermediateKey.PublicKey, rootKey)
	require.NoError(t, err)
	intermediateCert, err := x509.ParseCertificate(intermediateDER)
	require.NoError(t, err)

	// Create client cert signed by intermediate
	clientCert := generateTestClientCert(t, intermediateCert, intermediateKey)

	// The CR's CA is the root CA
	rootPEM := encodeCertToPEM(t, rootCA)

	scheme := testScheme()
	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp1", Namespace: "ns"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "prod",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: string(rootPEM)},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dp).Build()
	s := &Server{k8sClient: fakeClient, logger: testLogger()}

	// Pass intermediate cert chain
	validCRs, err := s.verifyClientCertificatePerCR(clientCert, []*x509.Certificate{intermediateCert}, "dataplane", "prod")
	require.NoError(t, err)
	assert.Contains(t, validCRs, "ns/dp1")
}

// --- handleHTTPProxy expanded tests ---

func TestHandleHTTPProxy_StreamingRedirect(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	s := New(&Config{}, fakeClient, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/proxy/dataplane/prod/ns/dp1/k8s/api/v1/pods?watch=true", nil)
	w := httptest.NewRecorder()
	s.handleHTTPProxy(w, req)

	assert.Equal(t, http.StatusNotImplemented, w.Code)
	assert.Contains(t, w.Body.String(), "Streaming operations")
}

func TestHandleHTTPProxy_CRAuthorizationFailed(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	s := New(&Config{}, fakeClient, testLogger())

	// Register agent but only for ns/dp1
	conn, cleanup := newTestWSConn(t)
	defer cleanup()
	_, _ = s.connMgr.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil)

	// Request for a different CR
	req := httptest.NewRequest(http.MethodGet, "/api/proxy/dataplane/prod/ns/dp-other/k8s/api/v1/pods", nil)
	w := httptest.NewRecorder()
	s.handleHTTPProxy(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Forbidden")
}

func TestHandleHTTPProxy_ClusterScopedCRNamespace(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	s := New(&Config{}, fakeClient, testLogger())

	// Register agent for cluster-scoped CR only (key format: "/crName")
	conn, cleanup := newTestWSConn(t)
	defer cleanup()
	_, _ = s.connMgr.Register("dataplane", "prod", conn, []string{"/global-dp"}, nil)

	// Request with _cluster namespace but a different CR name → should get 403 (not found)
	// This verifies _cluster is mapped to empty namespace forming key "/wrong-dp"
	req := httptest.NewRequest(http.MethodGet, "/api/proxy/dataplane/prod/_cluster/wrong-dp/k8s/api/v1/pods", nil)
	w := httptest.NewRecorder()
	s.handleHTTPProxy(w, req)

	// Agent is only authorized for "/global-dp", not "/wrong-dp" → 403
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Forbidden")
}

func TestHandleHTTPProxy_Success(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	s := New(&Config{}, fakeClient, testLogger())

	conn, cleanup := newTestWSConn(t)
	defer cleanup()
	_, _ = s.connMgr.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil)

	// Run proxy request in goroutine (it blocks waiting for tunnel response)
	req := httptest.NewRequest(http.MethodGet, "/api/proxy/dataplane/prod/ns/dp1/k8s/api/v1/pods", nil)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		s.handleHTTPProxy(w, req)
		close(done)
	}()

	// Wait for the pending request to be registered, then deliver response
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
		Headers:    map[string][]string{"Content-Type": {"application/json"}},
		Body:       []byte(`{"items":[]}`),
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handleHTTPProxy did not return")
	}

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"items":[]}`, w.Body.String())
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
}

func TestSendHTTPTunnelRequestForCR_Timeout(t *testing.T) {
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

	_, err := s.SendHTTPTunnelRequestForCR("dataplane/prod", "ns/dp1", req, 50*time.Millisecond)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")

	// Pending request should be cleaned up
	s.requestsMu.Lock()
	assert.Empty(t, s.pendingHTTPRequests)
	s.requestsMu.Unlock()
}

func TestHandleHTTPProxy_NoAgentsRegistered(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	s := New(&Config{}, fakeClient, testLogger())

	// No agents registered → should get 502
	req := httptest.NewRequest(http.MethodGet, "/api/proxy/dataplane/prod/ns/dp1/k8s/api/v1/pods", nil)
	w := httptest.NewRecorder()
	s.handleHTTPProxy(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

// --- handleConnection tests (using Connection interface) ---

// mockGatewayConn implements Connection for testing handleConnection.
type mockGatewayConn struct {
	mu           sync.Mutex
	readMessages [][]byte
	readIndex    int
	writtenMsgs  [][]byte
	closed       bool
}

func (m *mockGatewayConn) ReadMessage() (int, []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.readIndex >= len(m.readMessages) {
		return 0, nil, fmt.Errorf("connection closed")
	}
	msg := m.readMessages[m.readIndex]
	m.readIndex++
	return websocket.TextMessage, msg, nil
}

func (m *mockGatewayConn) WriteMessage(_ int, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writtenMsgs = append(m.writtenMsgs, data)
	return nil
}

func (m *mockGatewayConn) WriteControl(_ int, _ []byte, _ time.Time) error { return nil }
func (m *mockGatewayConn) SetReadDeadline(_ time.Time) error               { return nil }
func (m *mockGatewayConn) SetPongHandler(_ func(string) error)             {}
func (m *mockGatewayConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// closeErrorGatewayConn returns a specific error from ReadMessage.
type closeErrorGatewayConn struct {
	err error
}

func (c *closeErrorGatewayConn) ReadMessage() (int, []byte, error)               { return 0, nil, c.err }
func (c *closeErrorGatewayConn) WriteMessage(_ int, _ []byte) error              { return nil }
func (c *closeErrorGatewayConn) WriteControl(_ int, _ []byte, _ time.Time) error { return nil }
func (c *closeErrorGatewayConn) SetReadDeadline(_ time.Time) error               { return nil }
func (c *closeErrorGatewayConn) SetPongHandler(_ func(string) error)             {}
func (c *closeErrorGatewayConn) Close() error                                    { return nil }

func TestHandleConnection_ProcessesMessages(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	s := New(&Config{HeartbeatInterval: time.Hour, HeartbeatTimeout: time.Hour}, fakeClient, testLogger())

	// Prepare a tunnel response message that handleConnection will receive
	tunnelResp := &messaging.HTTPTunnelResponse{
		RequestID:  "req-1",
		StatusCode: 200,
		Body:       []byte(`{"ok":true}`),
	}
	respData, err := json.Marshal(tunnelResp)
	require.NoError(t, err)

	mock := &mockGatewayConn{
		readMessages: [][]byte{respData},
	}

	// Register a pending request so handleHTTPTunnelResponse has somewhere to deliver
	replyChan := make(chan *messaging.HTTPTunnelResponse, 1)
	s.requestsMu.Lock()
	s.pendingHTTPRequests["req-1"] = replyChan
	s.requestsMu.Unlock()

	// Register the mock connection in connMgr
	connID, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil)
	require.NoError(t, err)

	s.handleConnection("dataplane/prod", connID, mock)

	// Verify the response was delivered
	select {
	case got := <-replyChan:
		assert.Equal(t, "req-1", got.RequestID)
		assert.Equal(t, 200, got.StatusCode)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for tunnel response")
	}
}

func TestHandleConnection_InvalidMessage(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	s := New(&Config{HeartbeatInterval: time.Hour, HeartbeatTimeout: time.Hour}, fakeClient, testLogger())

	mock := &mockGatewayConn{
		readMessages: [][]byte{[]byte("not json")},
	}

	connID, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil)
	require.NoError(t, err)

	// Should not panic — skips invalid message and exits when no more messages
	s.handleConnection("dataplane/prod", connID, mock)
}

func TestHandleConnection_MissingRequestID(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	s := New(&Config{HeartbeatInterval: time.Hour, HeartbeatTimeout: time.Hour}, fakeClient, testLogger())

	resp := &messaging.HTTPTunnelResponse{
		StatusCode: 200,
		// No RequestID
	}
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	mock := &mockGatewayConn{
		readMessages: [][]byte{data},
	}

	connID, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil)
	require.NoError(t, err)

	// Should skip message and exit
	s.handleConnection("dataplane/prod", connID, mock)
}

func TestHandleConnection_UnexpectedClose(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	s := New(&Config{HeartbeatInterval: time.Hour, HeartbeatTimeout: time.Hour}, fakeClient, testLogger())

	// CloseError with code NOT in expected list → triggers unexpected close log
	mock := &closeErrorGatewayConn{
		err: &websocket.CloseError{
			Code: websocket.CloseInternalServerErr,
			Text: "internal error",
		},
	}

	connID, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil)
	require.NoError(t, err)

	// Should log "websocket error" and return
	s.handleConnection("dataplane/prod", connID, mock)
}

func TestHandleConnection_NormalClose(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	s := New(&Config{HeartbeatInterval: time.Hour, HeartbeatTimeout: time.Hour}, fakeClient, testLogger())

	// CloseGoingAway is in the expected list → normal disconnect
	mock := &closeErrorGatewayConn{
		err: &websocket.CloseError{
			Code: websocket.CloseGoingAway,
			Text: "going away",
		},
	}

	connID, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil)
	require.NoError(t, err)

	// Should log "agent disconnected" and return
	s.handleConnection("dataplane/prod", connID, mock)
}
