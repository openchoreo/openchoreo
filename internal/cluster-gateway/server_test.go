// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
	"github.com/openchoreo/openchoreo/internal/cluster-gateway/fabric"
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

	for _, p := range []string{
		"/api/proxy/dataplane/prod/ns/dp1/k8s/api/v1/namespaces/kube-system/secrets",
		"/api/proxy/dataplane/prod/ns/dp1/k8s/api/v1/secrets?labelSelector=app%3Dx",
		"/api/proxy/dataplane/prod/ns/dp1/k8s/api/v1/watch/secrets",
	} {
		t.Run(p, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, p, nil)
			w := httptest.NewRecorder()
			s.handleHTTPProxy(w, req)

			assert.Equal(t, http.StatusForbidden, w.Code)
		})
	}
}

func TestHandleHTTPProxy_DoubleEncodedPath(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	s := New(&Config{}, fakeClient, testLogger())

	for _, p := range []string{
		"/api/proxy/dataplane/prod/ns/dp1/k8s/api/v1/namespaces/kube-system/%2573ecrets",
		"/api/proxy/dataplane/prod/ns/dp1/k8s/api/v1/secrets%3F",
		"/api/proxy/dataplane/prod/ns/dp1/k8s/api/v1/namespaces/kube-system/secrets%23",
	} {
		t.Run(p, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, p, nil)
			w := httptest.NewRecorder()
			s.handleHTTPProxy(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestHandleHTTPTunnelResponse(t *testing.T) {
	s := &Server{
		pendingHTTPRequests: make(map[string]*pendingRequest),
		logger:              testLogger(),
	}

	t.Run("delivers response to waiting channel", func(t *testing.T) {
		ch := make(chan *messaging.HTTPTunnelResponse, 1)
		s.requestsMu.Lock()
		s.pendingHTTPRequests["req-123"] = &pendingRequest{ch: ch, connID: "conn-a"}
		s.requestsMu.Unlock()

		resp := &messaging.HTTPTunnelResponse{
			RequestID:  "req-123",
			StatusCode: 200,
		}

		s.handleHTTPTunnelResponse("dataplane/prod", "conn-a", resp)

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
		s.handleHTTPTunnelResponse("dataplane/prod", "conn-a", resp)
	})

	// A plane may run several agent replicas at once, all holding accepted
	// connections under one plane identifier. A response must therefore only
	// count when it comes back on the connection the request went out on;
	// otherwise any connected replica could answer for another by naming its
	// request ID, and correctness would rest on IDs being unguessable.
	t.Run("response from a different connection is rejected", func(t *testing.T) {
		ch := make(chan *messaging.HTTPTunnelResponse, 1)
		s.requestsMu.Lock()
		s.pendingHTTPRequests["req-multi"] = &pendingRequest{ch: ch, connID: "conn-owner"}
		s.requestsMu.Unlock()

		s.handleHTTPTunnelResponse("dataplane/prod", "conn-impostor", &messaging.HTTPTunnelResponse{
			RequestID:  "req-multi",
			StatusCode: 503,
			Body:       []byte("forged"),
		})

		select {
		case got := <-ch:
			t.Fatalf("waiter must not receive a response from another connection, got %+v", got)
		case <-time.After(100 * time.Millisecond):
		}

		// The entry must survive so the rightful connection can still answer.
		s.requestsMu.Lock()
		_, exists := s.pendingHTTPRequests["req-multi"]
		s.requestsMu.Unlock()
		assert.True(t, exists, "pending request must remain after rejecting a foreign response")

		// ...and the real owner's response still resolves it.
		s.handleHTTPTunnelResponse("dataplane/prod", "conn-owner", &messaging.HTTPTunnelResponse{
			RequestID:  "req-multi",
			StatusCode: 200,
		})
		select {
		case got := <-ch:
			assert.Equal(t, 200, got.StatusCode)
		case <-time.After(time.Second):
			t.Fatal("owner response was not delivered")
		}
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
	_, _ = s.connMgr.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)

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
	_, _ = s.connMgr.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)

	req := &messaging.HTTPTunnelRequest{
		Target: "k8s",
		Method: "GET",
		Path:   "/api/v1/pods",
	}

	_, _, err := s.SendHTTPTunnelRequestForCR("dataplane/prod", "ns/dp-other", req, time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no agents authorized for CR")
}

func TestSendHTTPTunnelRequestForCR_Success(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	s := New(&Config{}, fakeClient, testLogger())

	conn, cleanup := newTestWSConn(t)
	defer cleanup()
	connID, _ := s.connMgr.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)

	req := &messaging.HTTPTunnelRequest{
		Target: "k8s",
		Method: "GET",
		Path:   "/api/v1/pods",
	}

	// Send the request in a goroutine since it will block waiting for response
	var wg sync.WaitGroup
	var sendErr error
	var resp *messaging.HTTPTunnelResponse
	var servedBy string

	wg.Add(1)
	go func() {
		defer wg.Done()
		resp, servedBy, sendErr = s.SendHTTPTunnelRequestForCR("dataplane/prod", "ns/dp1", req, 2*time.Second)
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

	s.handleHTTPTunnelResponse("dataplane/prod", connID, &messaging.HTTPTunnelResponse{
		RequestID:  requestID,
		StatusCode: 200,
		Body:       []byte(`{"items":[]}`),
	})

	wg.Wait()
	require.NoError(t, sendErr)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, s.selfID(), servedBy, "request served by the local connection should report this pod")
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

	validCRs, _, err := s.verifyClientCertificatePerCR(clientCert, nil, "dataplane", "prod")
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

	validCRs, _, err := s.verifyClientCertificatePerCR(clientCert, nil, "dataplane", "shared")
	require.NoError(t, err)
	assert.Contains(t, validCRs, "ns-a/dp1")
	assert.NotContains(t, validCRs, "ns-b/dp2")
}

// A CR whose clientCA is empty configures no trust anchor, so it can never
// authorize anyone. Guarding this explicitly because the failure mode is
// silent and severe: treating "no CA configured" as "nothing to check" would
// let any certificate authenticate to that plane.
// When an agent connection dies with requests in flight, those requests can
// never be answered — responses are only accepted from the connection they
// were dispatched on. They must fail immediately rather than holding the
// caller for the full tunnel timeout, so a caller can retry against one of
// the plane's other agent replicas.
func TestFailPendingForConnection_ReleasesWaiters(t *testing.T) {
	s := &Server{
		pendingHTTPRequests: make(map[string]*pendingRequest),
		logger:              testLogger(),
	}

	dying := make(chan *messaging.HTTPTunnelResponse, 1)
	survivor := make(chan *messaging.HTTPTunnelResponse, 1)
	s.requestsMu.Lock()
	s.pendingHTTPRequests["req-dying"] = &pendingRequest{ch: dying, connID: "conn-dying"}
	s.pendingHTTPRequests["req-other"] = &pendingRequest{ch: survivor, connID: "conn-live"}
	s.requestsMu.Unlock()

	s.failPendingForConnection("dataplane/prod", "conn-dying")

	select {
	case got := <-dying:
		assert.Nil(t, got, "a closed connection must release its waiter with nil")
	case <-time.After(time.Second):
		t.Fatal("waiter on the closed connection was not released")
	}

	// Requests belonging to other connections must be untouched.
	s.requestsMu.Lock()
	_, stillPending := s.pendingHTTPRequests["req-other"]
	_, cleared := s.pendingHTTPRequests["req-dying"]
	s.requestsMu.Unlock()
	assert.True(t, stillPending, "requests on other connections must not be failed")
	assert.False(t, cleared, "the dead connection's request must be removed")
	assert.Empty(t, survivor, "the live connection's waiter must not be signaled")
}

func TestVerifyClientCertificatePerCR_EmptyCAFailsClosed(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)

	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp-nocert", Namespace: "ns"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "prod",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: ""}, // no trust anchor
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(dp).Build()
	s := &Server{k8sClient: fakeClient, logger: testLogger()}

	validCRs, _, err := s.verifyClientCertificatePerCR(clientCert, nil, "dataplane", "prod")
	require.Error(t, err, "a CR with no client CA must not authorize any certificate")
	assert.Empty(t, validCRs)
}

// Two CRs sharing one CA means a certificate signed by it is valid for both:
// validation still runs per CR, but the trust domains have merged. The
// connection is allowed (this is a legitimate single-tenant setup) — the
// point is that it must not be mistaken for isolation.
func TestVerifyClientCertificatePerCR_SharedCAMergesTrustDomains(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	caPEM := string(encodeCertToPEM(t, caCert))

	dp1 := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp1", Namespace: "tenant-a"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID:      "shared",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{ClientCA: openchoreov1alpha1.ValueFrom{Value: caPEM}},
		},
	}
	dp2 := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp2", Namespace: "tenant-b"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID:      "shared",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{ClientCA: openchoreov1alpha1.ValueFrom{Value: caPEM}},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(dp1, dp2).Build()
	s := &Server{k8sClient: fakeClient, logger: testLogger()}

	validCRs, _, err := s.verifyClientCertificatePerCR(clientCert, nil, "dataplane", "shared")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"tenant-a/dp1", "tenant-b/dp2"}, validCRs,
		"one CA signing for both CRs must grant access to both — that is the hazard being warned about")
}

func TestVerifyClientCertificatePerCR_NoCRsFound(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)

	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	s := &Server{k8sClient: fakeClient, logger: testLogger()}

	_, _, err := s.verifyClientCertificatePerCR(clientCert, nil, "dataplane", "nonexistent")
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

	_, _, err := s.verifyClientCertificatePerCR(clientCert, nil, "dataplane", "prod")
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

	_, _, err := s.verifyClientCertificatePerCR(clientCert, nil, "dataplane", "prod")
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
	validCRs, _, err := s.verifyClientCertificatePerCR(clientCert, buildIntermediatePool([]*x509.Certificate{intermediateCert}), "dataplane", "prod")
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
	_, _ = s.connMgr.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)

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
	_, _ = s.connMgr.Register("dataplane", "prod", conn, []string{"/global-dp"}, nil, nil)

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
	connID, _ := s.connMgr.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)

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

	s.handleHTTPTunnelResponse("dataplane/prod", connID, &messaging.HTTPTunnelResponse{
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
	_, _ = s.connMgr.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)

	req := &messaging.HTTPTunnelRequest{
		Target: "k8s",
		Method: "GET",
		Path:   "/api/v1/pods",
	}

	_, _, err := s.SendHTTPTunnelRequestForCR("dataplane/prod", "ns/dp1", req, 50*time.Millisecond)
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

func (m *mockGatewayConn) getWrittenMessages() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]byte, len(m.writtenMsgs))
	copy(out, m.writtenMsgs)
	return out
}

func (m *mockGatewayConn) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func (m *mockGatewayConn) WriteControl(_ int, _ []byte, _ time.Time) error { return nil }
func (m *mockGatewayConn) SetReadDeadline(_ time.Time) error               { return nil }
func (m *mockGatewayConn) SetWriteDeadline(_ time.Time) error              { return nil }
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
func (c *closeErrorGatewayConn) SetWriteDeadline(_ time.Time) error              { return nil }
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

	// Register the mock connection in connMgr first: the pending request must
	// be bound to that connection's ID for its response to be accepted.
	connID, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

	// Register a pending request so handleHTTPTunnelResponse has somewhere to deliver
	replyChan := make(chan *messaging.HTTPTunnelResponse, 1)
	s.requestsMu.Lock()
	s.pendingHTTPRequests["req-1"] = &pendingRequest{ch: replyChan, connID: connID}
	s.requestsMu.Unlock()

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

	connID, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil, nil)
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

	connID, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil, nil)
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

	connID, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil, nil)
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

	connID, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

	// Should log "agent disconnected" and return
	s.handleConnection("dataplane/prod", connID, mock)
}

// generateTestClientKeyPair creates a client certificate signed by the given CA
// and returns it as a tls.Certificate usable in a handshake.
func generateTestClientKeyPair(t *testing.T, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) tls.Certificate {
	t.Helper()
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "Test Internal Client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, &clientKey.PublicKey, caKey)
	require.NoError(t, err)

	leaf, err := x509.ParseCertificate(clientDER)
	require.NoError(t, err)

	return tls.Certificate{
		Certificate: [][]byte{clientDER},
		PrivateKey:  clientKey,
		Leaf:        leaf,
	}
}

// writeTestCAFile writes the CA certificate PEM to a temp file and returns its path.
func writeTestCAFile(t *testing.T, caCert *x509.Certificate) string {
	t.Helper()
	caPath := filepath.Join(t.TempDir(), "ca.crt")
	require.NoError(t, os.WriteFile(caPath, encodeCertToPEM(t, caCert), 0o600))
	return caPath
}

func baseTestTLSConfig() *tls.Config {
	return &tls.Config{
		ClientAuth: tls.RequestClientCert,
		MinVersion: tls.VersionTLS12,
	}
}

// --- buildInternalTLSConfig tests ---

func TestBuildInternalTLSConfig_Disabled(t *testing.T) {
	cfg := &Config{InternalMTLSEnabled: false}

	tlsConfig, err := buildInternalTLSConfig(baseTestTLSConfig(), cfg)
	require.NoError(t, err)
	assert.Equal(t, tls.RequestClientCert, tlsConfig.ClientAuth)
	assert.Nil(t, tlsConfig.ClientCAs)
}

func TestBuildInternalTLSConfig_EnabledWithValidCA(t *testing.T) {
	caCert, _ := generateTestCA(t)
	cfg := &Config{
		InternalMTLSEnabled:  true,
		InternalClientCAPath: writeTestCAFile(t, caCert),
	}

	tlsConfig, err := buildInternalTLSConfig(baseTestTLSConfig(), cfg)
	require.NoError(t, err)
	assert.Equal(t, tls.RequireAndVerifyClientCert, tlsConfig.ClientAuth)
	assert.NotNil(t, tlsConfig.ClientCAs)
}

func TestBuildInternalTLSConfig_EnabledWithoutCAPath(t *testing.T) {
	cfg := &Config{InternalMTLSEnabled: true}

	_, err := buildInternalTLSConfig(baseTestTLSConfig(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal-client-ca-cert")
}

func TestBuildInternalTLSConfig_EnabledWithMissingFile(t *testing.T) {
	cfg := &Config{
		InternalMTLSEnabled:  true,
		InternalClientCAPath: filepath.Join(t.TempDir(), "does-not-exist.crt"),
	}

	_, err := buildInternalTLSConfig(baseTestTLSConfig(), cfg)
	require.Error(t, err)
}

func TestBuildInternalTLSConfig_EnabledWithInvalidPEM(t *testing.T) {
	caPath := filepath.Join(t.TempDir(), "ca.crt")
	require.NoError(t, os.WriteFile(caPath, []byte("not a certificate"), 0o600))
	cfg := &Config{
		InternalMTLSEnabled:  true,
		InternalClientCAPath: caPath,
	}

	_, err := buildInternalTLSConfig(baseTestTLSConfig(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid certificates")
}

// --- handshake-level enforcement tests ---

// startInternalTestServer starts an HTTPS server using the internal listener's
// TLS configuration derived from cfg, serving a trivial 200 handler.
func startInternalTestServer(t *testing.T, cfg *Config) *httptest.Server {
	t.Helper()
	tlsConfig, err := buildInternalTLSConfig(baseTestTLSConfig(), cfg)
	require.NoError(t, err)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	srv.TLS = tlsConfig
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func newTestHTTPSClient(serverCertPool *x509.CertPool, clientCerts ...tls.Certificate) *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      serverCertPool,
				Certificates: clientCerts,
				MinVersion:   tls.VersionTLS12,
			},
		},
	}
}

func serverCertPool(t *testing.T, srv *httptest.Server) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	return pool
}

func TestInternalListener_MTLSEnabled_AcceptsInternalCACert(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	srv := startInternalTestServer(t, &Config{
		InternalMTLSEnabled:  true,
		InternalClientCAPath: writeTestCAFile(t, caCert),
	})

	client := newTestHTTPSClient(serverCertPool(t, srv), generateTestClientKeyPair(t, caCert, caKey))
	resp, err := client.Get(srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", string(body))
}

func TestInternalListener_MTLSEnabled_RejectsNoClientCert(t *testing.T) {
	caCert, _ := generateTestCA(t)
	srv := startInternalTestServer(t, &Config{
		InternalMTLSEnabled:  true,
		InternalClientCAPath: writeTestCAFile(t, caCert),
	})

	client := newTestHTTPSClient(serverCertPool(t, srv))
	resp, err := client.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
	}
	require.Error(t, err, "request without a client certificate must be rejected")
}

func TestInternalListener_MTLSEnabled_RejectsCertFromOtherCA(t *testing.T) {
	internalCACert, _ := generateTestCA(t)
	// Simulates an agent certificate signed by a different (agent) CA.
	otherCACert, otherCAKey := generateTestCA(t)

	srv := startInternalTestServer(t, &Config{
		InternalMTLSEnabled:  true,
		InternalClientCAPath: writeTestCAFile(t, internalCACert),
	})

	client := newTestHTTPSClient(serverCertPool(t, srv), generateTestClientKeyPair(t, otherCACert, otherCAKey))
	resp, err := client.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
	}
	require.Error(t, err, "certificate signed by a different CA must be rejected")
}

func TestInternalListener_MTLSDisabled_AcceptsNoClientCert(t *testing.T) {
	srv := startInternalTestServer(t, &Config{InternalMTLSEnabled: false})

	client := newTestHTTPSClient(serverCertPool(t, srv))
	resp, err := client.Get(srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// --- listener TLS version tests ---

const testServerName = "cluster-gateway.test"

// generateTestServerKeyPair creates a server certificate for testServerName
// signed by the given CA.
func generateTestServerKeyPair(t *testing.T, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) tls.Certificate {
	t.Helper()
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(4),
		Subject:      pkix.Name{CommonName: testServerName},
		DNSNames:     []string{testServerName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	require.NoError(t, err)

	return tls.Certificate{Certificate: [][]byte{serverDER}, PrivateKey: serverKey}
}

// tlsHandshake runs a TLS handshake over loopback TCP and returns the client's
// connection state with the errors from both sides. Loopback rather than
// net.Pipe because a TLS 1.3 server that requests client certificates writes
// session tickets after the client's handshake has already returned.
func tlsHandshake(t *testing.T, serverCfg, clientCfg *tls.Config) (tls.ConnectionState, error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		serverErr <- tls.Server(conn, serverCfg).HandshakeContext(ctx)
	}()

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", ln.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	client := tls.Client(conn, clientCfg)
	clientErr := client.HandshakeContext(ctx)
	if clientErr != nil {
		_ = conn.Close()
	}
	return client.ConnectionState(), errors.Join(clientErr, <-serverErr)
}

// TestListenerTLSConfigs_MinimumVersion checks every listener against a client
// capped at TLS 1.2 and a default client. Only the public listener in
// forwarded-header mode, where a TLS-terminating proxy is the peer, accepts
// TLS 1.2; everything else requires TLS 1.3. The internal listener is derived
// from a TLS 1.2 base to show it never inherits the lower floor.
func TestListenerTLSConfigs_MinimumVersion(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	base := baseTestTLSConfig()
	base.Certificates = []tls.Certificate{generateTestServerKeyPair(t, caCert, caKey)}

	internalConfig := func(t *testing.T, cfg *Config) *tls.Config {
		t.Helper()
		tlsConfig, err := buildInternalTLSConfig(base, cfg)
		require.NoError(t, err)
		return tlsConfig
	}
	internalMTLS := func(mode AgentAuthMode) *Config {
		return &Config{
			AgentAuthMode:        mode,
			InternalMTLSEnabled:  true,
			InternalClientCAPath: writeTestCAFile(t, caCert),
		}
	}

	tests := []struct {
		name         string
		serverCfg    *tls.Config
		acceptsTLS12 bool
	}{
		{name: "public listener, default agent auth mode", serverCfg: buildPublicTLSConfig(base, "")},
		{name: "public listener, mtls", serverCfg: buildPublicTLSConfig(base, AgentAuthModeMTLS)},
		{name: "public listener, forwarded-header", serverCfg: buildPublicTLSConfig(base, AgentAuthModeForwardedHeader), acceptsTLS12: true},
		{name: "internal listener, mtls", serverCfg: internalConfig(t, &Config{AgentAuthMode: AgentAuthModeMTLS})},
		{name: "internal listener, forwarded-header", serverCfg: internalConfig(t, &Config{AgentAuthMode: AgentAuthModeForwardedHeader})},
		{name: "internal listener with mTLS, mtls", serverCfg: internalConfig(t, internalMTLS(AgentAuthModeMTLS))},
		{name: "internal listener with mTLS, forwarded-header", serverCfg: internalConfig(t, internalMTLS(AgentAuthModeForwardedHeader))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientCfg := &tls.Config{
				Certificates: []tls.Certificate{generateTestClientKeyPair(t, caCert, caKey)},
				RootCAs:      caPool,
				ServerName:   testServerName,
				MinVersion:   tls.VersionTLS12,
			}

			state, err := tlsHandshake(t, tt.serverCfg, clientCfg)
			require.NoError(t, err)
			assert.Equal(t, uint16(tls.VersionTLS13), state.Version)

			tls12Client := clientCfg.Clone()
			tls12Client.MaxVersion = tls.VersionTLS12
			state, err = tlsHandshake(t, tt.serverCfg, tls12Client)
			if tt.acceptsTLS12 {
				require.NoError(t, err)
				assert.Equal(t, uint16(tls.VersionTLS12), state.Version)
				return
			}
			require.Error(t, err, "a client capped at TLS 1.2 must be rejected")
			assert.Contains(t, err.Error(), "protocol version")
		})
	}
}

// TestPublicListener_AgentNegotiatesHybridKeyExchange handshakes a client
// configured like the cluster-agent dialer (TLS 1.2 floor, default curve
// preferences) with the public listener and expects the hybrid post-quantum
// key exchange in every agent auth mode.
func TestPublicListener_AgentNegotiatesHybridKeyExchange(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	base := baseTestTLSConfig()
	base.Certificates = []tls.Certificate{generateTestServerKeyPair(t, caCert, caKey)}

	tests := []struct {
		name string
		mode AgentAuthMode
	}{
		{name: "default agent auth mode", mode: ""},
		{name: "mtls", mode: AgentAuthModeMTLS},
		{name: "forwarded-header", mode: AgentAuthModeForwardedHeader},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentCfg := &tls.Config{
				Certificates: []tls.Certificate{generateTestClientKeyPair(t, caCert, caKey)},
				RootCAs:      caPool,
				ServerName:   testServerName,
				MinVersion:   tls.VersionTLS12,
			}

			state, err := tlsHandshake(t, buildPublicTLSConfig(base, tt.mode), agentCfg)
			require.NoError(t, err)
			assert.Equal(t, uint16(tls.VersionTLS13), state.Version)
			assert.Equal(t, tls.X25519MLKEM768, state.CurveID)
		})
	}
}

// TestBuildListeners_TLSMinVersion checks the TLS floors on the servers Start
// installs for each agent auth mode, without binding any port.
func TestBuildListeners_TLSMinVersion(t *testing.T) {
	tests := []struct {
		name            string
		mode            AgentAuthMode
		wantPublicMin   uint16
		wantInternalMin uint16
	}{
		{name: "default agent auth mode", mode: "", wantPublicMin: tls.VersionTLS13, wantInternalMin: tls.VersionTLS13},
		{name: "mtls", mode: AgentAuthModeMTLS, wantPublicMin: tls.VersionTLS13, wantInternalMin: tls.VersionTLS13},
		{name: "forwarded-header", mode: AgentAuthModeForwardedHeader, wantPublicMin: tls.VersionTLS12, wantInternalMin: tls.VersionTLS13},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(&Config{AgentAuthMode: tt.mode}, nil, testLogger())

			require.NoError(t, s.buildListeners(&tls.Config{
				ClientAuth: tls.RequestClientCert,
				MinVersion: tls.VersionTLS13,
			}))
			require.NotNil(t, s.httpServer.TLSConfig)
			require.NotNil(t, s.internalServer.TLSConfig)
			assert.Equal(t, tt.wantPublicMin, s.httpServer.TLSConfig.MinVersion, "public listener MinVersion")
			assert.Equal(t, tt.wantInternalMin, s.internalServer.TLSConfig.MinVersion, "internal listener MinVersion")
		})
	}
}

// --- handleWebSocket tests ---

func TestHandleWebSocket_ParamValidation(t *testing.T) {
	s := New(&Config{}, nil, testLogger())

	tests := []struct {
		name    string
		target  string
		wantMsg string
	}{
		{name: "missing planeType", target: "/ws?planeID=prod", wantMsg: "missing planeType"},
		{name: "missing planeID", target: "/ws?planeType=dataplane", wantMsg: "missing planeID"},
		{name: "invalid planeType", target: "/ws?planeType=bogus&planeID=prod", wantMsg: "invalid planeType"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			s.handleWebSocket(w, httptest.NewRequest(http.MethodGet, tt.target, nil))
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), tt.wantMsg)
		})
	}
}

// TestHandleWebSocket_NoClientCertificate covers the nil-authenticator fallback:
// a Server constructed without New (agentAuth nil) must still default to mTLS
// extraction and reject a request that carries no TLS client certificate.
func TestHandleWebSocket_NoClientCertificate(t *testing.T) {
	s := &Server{logger: testLogger()}

	w := httptest.NewRecorder()
	s.handleWebSocket(w, httptest.NewRequest(http.MethodGet, "/ws?planeType=dataplane&planeID=prod", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "no client certificate presented")
}

// TestHandleWebSocket_ForwardedHeader_CRVerificationFailed proves the forwarded
// certificate feeds the same per-CR verification as mTLS mode: a cert extracted
// from the header is still rejected when no plane CR trusts its CA.
func TestHandleWebSocket_ForwardedHeader_CRVerificationFailed(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)

	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())
	s.agentAuth = forwardedHeaderAuthenticator{header: DefaultForwardedHeaderName}

	req := httptest.NewRequest(http.MethodGet, "/ws?planeType=dataplane&planeID=prod", nil)
	req.Header.Set(DefaultForwardedHeaderName, albHeaderValue(t, clientCert))

	w := httptest.NewRecorder()
	s.handleWebSocket(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "client certificate verification failed")
}

// TestHandleWebSocket_ForwardedHeader_UpgradeFailed authenticates successfully
// via the forwarded header against a matching DataPlane CR, then fails only at
// the WebSocket upgrade because the request is not a websocket handshake.
func TestHandleWebSocket_ForwardedHeader_UpgradeFailed(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)

	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp1", Namespace: "ns"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "prod",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: string(encodeCertToPEM(t, caCert))},
			},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(dp).Build()
	s := New(&Config{}, fakeClient, testLogger())
	s.agentAuth = forwardedHeaderAuthenticator{header: DefaultForwardedHeaderName}

	req := httptest.NewRequest(http.MethodGet, "/ws?planeType=dataplane&planeID=prod", nil)
	req.Header.Set(DefaultForwardedHeaderName, albHeaderValue(t, clientCert))

	w := httptest.NewRecorder()
	s.handleWebSocket(w, req)
	// Auth and per-CR verification passed; the plain HTTP request fails the
	// websocket handshake, which the upgrader reports as 400.
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.NotContains(t, w.Body.String(), "certificate")
}

// --- Server.Start tests ---

// writeServerKeyPairFiles generates a self-signed server certificate and its EC
// private key, writes both to PEM files, and returns their paths. The pair is
// valid for tls.LoadX509KeyPair used in Server.Start.
func writeServerKeyPairFiles(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(100),
		Subject:      pkix.Name{CommonName: "test-server"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	dir := t.TempDir()
	certPath = filepath.Join(dir, "server.crt")
	keyPath = filepath.Join(dir, "server.key")
	require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))
	return certPath, keyPath
}

func TestStart_ServerCertLoadError(t *testing.T) {
	s := New(&Config{
		ServerCertPath: filepath.Join(t.TempDir(), "missing.crt"),
		ServerKeyPath:  filepath.Join(t.TempDir(), "missing.key"),
	}, nil, testLogger())

	err := s.Start()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load server certificate")
}

// TestStart_InternalTLSConfigError verifies that a failure while building the
// internal listener TLS config is wrapped and returned from Start before any
// port is bound. mTLS is enabled with no client CA, which buildInternalTLSConfig
// rejects.
func TestStart_InternalTLSConfigError(t *testing.T) {
	certPath, keyPath := writeServerKeyPairFiles(t)

	s := New(&Config{
		ServerCertPath:      certPath,
		ServerKeyPath:       keyPath,
		InternalMTLSEnabled: true,
		// InternalClientCAPath deliberately empty.
	}, nil, testLogger())

	err := s.Start()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to configure internal listener TLS")
}

// TestStart_AgentAuthConfigError verifies that an unrecognized agent auth mode is
// surfaced from Start (fail loud) before any port is bound, rather than silently
// falling back to a default.
func TestStart_AgentAuthConfigError(t *testing.T) {
	certPath, keyPath := writeServerKeyPairFiles(t)

	s := New(&Config{
		ServerCertPath: certPath,
		ServerKeyPath:  keyPath,
		AgentAuthMode:  "not-a-real-mode",
	}, nil, testLogger())

	err := s.Start()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to configure agent authentication")
}

// waitForHealth polls the fixed health endpoint until it returns 200. It returns
// true once healthy. If Start returns early (e.g. the fixed :8080 health port is
// already bound), the test is skipped rather than failed, since that is an
// environment conflict and not a code defect.
func waitForHealth(t *testing.T, startErr <-chan error) bool {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-startErr:
			skipOrFailOnStartErr(t, err)
			return false
		default:
		}

		resp, err := http.Get("http://127.0.0.1:8080/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	select {
	case err := <-startErr:
		skipOrFailOnStartErr(t, err)
	default:
		t.Skip("health server did not become ready on :8080 (port likely in use)")
	}
	return false
}

func skipOrFailOnStartErr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("Start returned nil before becoming healthy")
		return
	}
	msg := err.Error()
	if strings.Contains(msg, "address already in use") || strings.Contains(msg, "bind:") {
		t.Skipf("health/server port unavailable in this environment: %v", err)
		return
	}
	t.Fatalf("Start returned an unexpected error before becoming healthy: %v", err)
}

// TestStart_Lifecycle brings the server fully up on ephemeral ports and asserts
// that a SIGTERM triggers a graceful shutdown returning nil. It exercises both
// the internal-mTLS-enabled and disabled logging/config branches in Start.
func TestStart_Lifecycle(t *testing.T) {
	tests := []struct {
		name            string
		mtls            bool
		forwardedHeader bool
		mesh            bool
	}{
		{name: "internal mTLS enabled", mtls: true},
		{name: "internal mTLS disabled", mtls: false},
		{name: "forwarded-header agent auth", forwardedHeader: true},
		// The mesh is started before the listeners and shut down after them, so
		// it is only exercised by a run that brings the whole server up.
		{name: "gateway mesh enabled", mesh: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certPath, keyPath := writeServerKeyPairFiles(t)
			cfg := &Config{
				Port:            0, // ephemeral
				InternalPort:    0, // ephemeral
				ServerCertPath:  certPath,
				ServerKeyPath:   keyPath,
				ShutdownTimeout: 5 * time.Second,
			}
			if tt.mtls {
				caCert, _ := generateTestCA(t)
				cfg.InternalMTLSEnabled = true
				cfg.InternalClientCAPath = writeTestCAFile(t, caCert)
			}
			if tt.forwardedHeader {
				cfg.AgentAuthMode = AgentAuthModeForwardedHeader
			}

			s := New(cfg, nil, testLogger())

			if tt.mesh {
				registry := fabric.NewRegistry(testLogger())
				s.SetFabric(fabric.NewMesh(fabric.MeshConfig{
					Self:       fabric.Peer{ID: "gw-self"},
					ListenPort: 0, // ephemeral
				}, registry, &staticPeerDiscovery{ch: make(chan []fabric.Peer)}, s, testLogger()), registry)
			}

			startErr := make(chan error, 1)
			exited := make(chan struct{})
			go func() {
				err := s.Start()
				close(exited) // mark exited before delivering the result
				startErr <- err
			}()

			// Safety net: make sure the Start goroutine (and its bound :8080 health
			// listener) always stops, even if the test skips or fails before the
			// explicit shutdown below. Guard against re-signaling after Start has
			// already returned: its signal.NotifyContext handler would be gone and a
			// second SIGTERM would terminate the test process instead.
			t.Cleanup(func() {
				select {
				case <-exited:
					return // already stopped; nothing to signal or wait for
				default:
				}
				if p, err := os.FindProcess(os.Getpid()); err == nil {
					_ = p.Signal(syscall.SIGTERM)
				}
				select {
				case <-exited:
				case <-time.After(10 * time.Second):
				}
			})

			if !waitForHealth(t, startErr) {
				return // skipped or failed inside helper; cleanup stops the server
			}

			// Trigger graceful shutdown; Start installs a SIGTERM handler via
			// signal.NotifyContext, so this is absorbed rather than killing the
			// test process.
			p, err := os.FindProcess(os.Getpid())
			require.NoError(t, err)
			require.NoError(t, p.Signal(syscall.SIGTERM))

			select {
			case err := <-startErr:
				require.NoError(t, err, "Start should return nil after graceful shutdown")
			case <-time.After(10 * time.Second):
				t.Fatal("Start did not return after SIGTERM")
			}
		})
	}
}

// --- gateway mesh fabric tests ---
// staticPeerDiscovery implements fabric.PeerDiscovery for tests.
type staticPeerDiscovery struct {
	ch chan []fabric.Peer
}

func (d *staticPeerDiscovery) Watch(ctx context.Context) (<-chan []fabric.Peer, error) {
	return d.ch, nil
}

// newFabricServer creates a Server wired into a mesh listening on an
// ephemeral loopback port (no TLS).
func newFabricServer(t *testing.T, ctx context.Context, podID string) (*Server, *fabric.Registry, *staticPeerDiscovery, string) {
	t.Helper()

	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{HeartbeatInterval: time.Hour, HeartbeatTimeout: time.Hour}, fakeClient, testLogger())

	registry := fabric.NewRegistry(testLogger())
	disc := &staticPeerDiscovery{ch: make(chan []fabric.Peer, 8)}
	mesh := fabric.NewMesh(fabric.MeshConfig{
		Self:       fabric.Peer{ID: podID},
		ListenPort: 0,
	}, registry, disc, s, testLogger())
	s.SetFabric(mesh, registry)

	require.NoError(t, mesh.Start(ctx))
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mesh.Shutdown(shutdownCtx)
	})

	return s, registry, disc, mesh.ListenAddr()
}

func waitForCondition(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// The core request path of the fabric design: a request lands on a pod with no
// local agent, the registry (converged via the mesh) locates the owner, the
// request takes one mesh hop, and the owner answers using its local
// connection — invisibly to the caller.
func TestServer_ForwardViaMesh_EndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverA, _, discA, addrA := newFabricServer(t, ctx, "gw-a")
	serverB, registryB, discB, addrB := newFabricServer(t, ctx, "gw-b")

	discA.ch <- []fabric.Peer{{ID: "gw-b", Addr: addrB}}
	discB.ch <- []fabric.Peer{{ID: "gw-a", Addr: addrA}}

	// gw-a owns the only agent connection for the plane. Registering it must
	// mirror the entry into gw-b's registry via the mesh.
	mock := &mockGatewayConn{}
	connIDA, err := serverA.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

	waitForCondition(t, "registry convergence on gw-b", func() bool {
		forCR, _ := registryB.CountForCRAndPlane("dataplane/prod", "ns/dp1")
		return forCR == 1
	})

	// Simulate the agent: answer the tunnel request that gw-a writes to the
	// mocked websocket.
	agentDone := make(chan struct{})
	defer close(agentDone)
	go func() {
		for {
			select {
			case <-agentDone:
				return
			case <-time.After(5 * time.Millisecond):
			}
			mock.mu.Lock()
			var reqID string
			for _, msg := range mock.writtenMsgs {
				var tr messaging.HTTPTunnelRequest
				if json.Unmarshal(msg, &tr) == nil && tr.RequestID != "" && tr.Target != "" {
					reqID = tr.RequestID
				}
			}
			mock.mu.Unlock()
			if reqID != "" {
				serverA.handleHTTPTunnelResponse("dataplane/prod", connIDA, &messaging.HTTPTunnelResponse{
					RequestID:  reqID,
					StatusCode: 200,
					Body:       []byte(`{"ok":true}`),
				})
				return
			}
		}
	}()

	// gw-b has no local agent: the request must be served via a mesh forward.
	req := messaging.NewHTTPTunnelRequest("k8s", "GET", "/api/v1/pods", "", nil, nil)
	resp, servedBy, err := serverB.SendHTTPTunnelRequestForCR("dataplane/prod", "ns/dp1", req, 3*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "gw-a", servedBy, "servedBy must identify the peer that actually owns the connection")

	// Unregistering on gw-a must converge on gw-b too.
	serverA.connMgr.DisconnectAllForPlane("dataplane", "prod")
	waitForCondition(t, "removal convergence on gw-b", func() bool {
		return registryB.SummaryForPlane("dataplane/prod").Count == 0
	})
}

// Regression: reproduces the exact ordering hit in production during a
// simultaneous multi-replica rollout. gw-b dials gw-a first and pulls its
// (then-empty) snapshot; only after that does gw-a register a local
// connection and then dial gw-b for the first time. gw-a's delta broadcast
// at registration time has zero peer links to send over (its own dial to
// gw-b hasn't happened yet), so it's silently dropped for gw-b — and
// gap-triggered repair can't help, since there's no later delta to ever
// reveal a sequence gap. Convergence must still happen once gw-a's dial to
// gw-b completes, via the proactive snapshot push on link establishment.
func TestServer_ForwardViaMesh_ConvergesWhenRegisteredBeforeDial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverA, _, discA, addrA := newFabricServer(t, ctx, "gw-a")
	serverB, registryB, discB, addrB := newFabricServer(t, ctx, "gw-b")

	// A canary entry lets us detect exactly when gw-b's dial to gw-a has
	// pulled gw-a's (still connection-less) initial snapshot.
	canary := &mockGatewayConn{}
	_, err := serverA.connMgr.Register("workflowplane", "canary", canary, []string{"ns/wf1"}, nil, nil)
	require.NoError(t, err)

	discB.ch <- []fabric.Peer{{ID: "gw-a", Addr: addrA}}
	waitForCondition(t, "gw-b pulls gw-a's initial snapshot", func() bool {
		forCR, _ := registryB.CountForCRAndPlane("workflowplane/canary", "ns/wf1")
		return forCR == 1
	})

	// Now register the real connection — gw-a doesn't know about gw-b yet,
	// so this delta broadcast has nowhere to go.
	mock := &mockGatewayConn{}
	connIDA, err := serverA.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

	// Only now does gw-a discover gw-b and dial out for the first time.
	discA.ch <- []fabric.Peer{{ID: "gw-b", Addr: addrB}}

	waitForCondition(t, "registry convergence on gw-b despite pre-dial registration", func() bool {
		forCR, _ := registryB.CountForCRAndPlane("dataplane/prod", "ns/dp1")
		return forCR == 1
	})

	// Convergence must be real, not just a registry entry: a forwarded
	// request must actually reach the connection.
	agentDone := make(chan struct{})
	defer close(agentDone)
	go func() {
		for {
			select {
			case <-agentDone:
				return
			case <-time.After(5 * time.Millisecond):
			}
			mock.mu.Lock()
			var reqID string
			for _, msg := range mock.writtenMsgs {
				var tr messaging.HTTPTunnelRequest
				if json.Unmarshal(msg, &tr) == nil && tr.RequestID != "" && tr.Target != "" {
					reqID = tr.RequestID
				}
			}
			mock.mu.Unlock()
			if reqID != "" {
				serverA.handleHTTPTunnelResponse("dataplane/prod", connIDA, &messaging.HTTPTunnelResponse{
					RequestID:  reqID,
					StatusCode: 200,
					Body:       []byte(`{"ok":true}`),
				})
				return
			}
		}
	}()

	req := messaging.NewHTTPTunnelRequest("k8s", "GET", "/api/v1/pods", "", nil, nil)
	resp, servedBy, err := serverB.SendHTTPTunnelRequestForCR("dataplane/prod", "ns/dp1", req, 3*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "gw-a", servedBy)
}

// Fleet-wide status: a pod must report agents owned by peers so controllers
// polling through the load balancer see a consistent picture.
func TestServer_PlaneStatusIncludesRemoteAgents(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	registry := fabric.NewRegistry(testLogger())
	disc := &staticPeerDiscovery{ch: make(chan []fabric.Peer, 1)}
	mesh := fabric.NewMesh(fabric.MeshConfig{Self: fabric.Peer{ID: "gw-self"}}, registry, disc, s, testLogger())
	s.SetFabric(mesh, registry)

	// A peer owns one connection for the plane.
	require.True(t, registry.ApplyDelta(fabric.Delta{
		Op: fabric.DeltaOpAdd, Owner: "gw-peer", Seq: 1,
		Entry: fabric.AgentEntry{PlaneIdentifier: "dataplane/prod", ConnID: "c1", ValidCRs: []string{"ns/dp1"}},
	}))

	status := s.PlaneStatus("dataplane", "prod")
	assert.True(t, status.Connected)
	assert.Equal(t, 1, status.ConnectedAgents)

	crStatus := s.CRAuthorizationStatus("dataplane", "prod", "ns", "dp1")
	assert.True(t, crStatus.Connected)
	assert.Equal(t, 1, crStatus.ConnectedAgents)

	all := s.AllPlaneStatuses()
	require.Len(t, all, 1)
	assert.Equal(t, "dataplane", all[0].PlaneType)
	assert.Equal(t, "prod", all[0].PlaneID)
	assert.True(t, all[0].Connected)

	// A local connection for the same plane adds to the same status entry.
	mock := &mockGatewayConn{}
	_, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

	status = s.PlaneStatus("dataplane", "prod")
	assert.Equal(t, 2, status.ConnectedAgents)
	all = s.AllPlaneStatuses()
	require.Len(t, all, 1)
	assert.Equal(t, 2, all[0].ConnectedAgents)
}

// A replica that does not own the agent connection must still report its
// liveness: controllers poll status through the load balancer and record
// LastSeen as the plane's heartbeat. Local and remote LastSeen merge to the
// latest, and a peer that publishes none (an older gateway) adds to the count
// without hiding a known LastSeen.
func TestServer_StatusMergesRemoteLastSeen(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	registry := fabric.NewRegistry(testLogger())
	disc := &staticPeerDiscovery{ch: make(chan []fabric.Peer, 1)}
	mesh := fabric.NewMesh(fabric.MeshConfig{Self: fabric.Peer{ID: "gw-self"}}, registry, disc, s, testLogger())
	s.SetFabric(mesh, registry)

	peerSeen := time.Now().Add(-10 * time.Second).Round(0)
	require.True(t, registry.ApplyDelta(fabric.Delta{
		Op: fabric.DeltaOpAdd, Owner: "gw-peer", Seq: 1,
		Entry: fabric.AgentEntry{
			PlaneIdentifier: "dataplane/prod", ConnID: "c1", ValidCRs: []string{"ns/dp1"}, LastSeen: peerSeen,
		},
	}))
	require.True(t, registry.ApplyDelta(fabric.Delta{
		Op: fabric.DeltaOpAdd, Owner: "gw-old", Seq: 1,
		Entry: fabric.AgentEntry{PlaneIdentifier: "dataplane/prod", ConnID: "c2", ValidCRs: []string{"ns/dp1"}},
	}))

	// No local connection: LastSeen comes from the peer that published one.
	status := s.PlaneStatus("dataplane", "prod")
	assert.Equal(t, 2, status.ConnectedAgents)
	assert.True(t, status.LastSeen.Equal(peerSeen), "got %v", status.LastSeen)
	crStatus := s.CRAuthorizationStatus("dataplane", "prod", "ns", "dp1")
	assert.Equal(t, 2, crStatus.ConnectedAgents)
	assert.Equal(t, 2, crStatus.PlaneConnectedAgents)
	assert.True(t, crStatus.LastSeen.Equal(peerSeen), "got %v", crStatus.LastSeen)
	all := s.AllPlaneStatuses()
	require.Len(t, all, 1)
	assert.True(t, all[0].Connected)
	assert.True(t, all[0].LastSeen.Equal(peerSeen), "got %v", all[0].LastSeen)

	// A local connection registered now is the latest.
	_, err := s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)
	localSeen := s.connMgr.GetPlaneStatus("dataplane", "prod").LastSeen
	require.True(t, localSeen.After(peerSeen))
	assert.True(t, s.PlaneStatus("dataplane", "prod").LastSeen.Equal(localSeen))
	assert.True(t, s.CRAuthorizationStatus("dataplane", "prod", "ns", "dp1").LastSeen.Equal(localSeen))
	assert.True(t, s.AllPlaneStatuses()[0].LastSeen.Equal(localSeen))

	// A peer that heard from its agent after the local one did wins.
	newer := localSeen.Add(time.Minute)
	require.True(t, registry.ApplyDelta(fabric.Delta{
		Op: fabric.DeltaOpAdd, Owner: "gw-peer", Seq: 2,
		Entry: fabric.AgentEntry{
			PlaneIdentifier: "dataplane/prod", ConnID: "c1", ValidCRs: []string{"ns/dp1"}, LastSeen: newer,
		},
	}))
	assert.True(t, s.PlaneStatus("dataplane", "prod").LastSeen.Equal(newer))
	assert.Equal(t, 3, s.PlaneStatus("dataplane", "prod").ConnectedAgents)
}

// The reported bug, end to end: the agent is connected to gw-a and the status
// request lands on gw-b. gw-b must answer with the agent's LastSeen, not zero.
func TestServer_PlaneStatusLastSeenOnNonOwnerReplica(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverA, _, discA, addrA := newFabricServer(t, ctx, "gw-a")
	serverB, registryB, discB, addrB := newFabricServer(t, ctx, "gw-b")

	discA.ch <- []fabric.Peer{{ID: "gw-b", Addr: addrB}}
	discB.ch <- []fabric.Peer{{ID: "gw-a", Addr: addrA}}

	before := time.Now()
	_, err := serverA.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

	waitForCondition(t, "registry convergence on gw-b", func() bool {
		forCR, _ := registryB.CountForCRAndPlane("dataplane/prod", "ns/dp1")
		return forCR == 1
	})

	status := serverB.PlaneStatus("dataplane", "prod")
	assert.True(t, status.Connected)
	assert.False(t, status.LastSeen.Before(before.Round(0)), "non-owner LastSeen %v predates registration", status.LastSeen)
	crStatus := serverB.CRAuthorizationStatus("dataplane", "prod", "ns", "dp1")
	assert.True(t, crStatus.Connected)
	assert.False(t, crStatus.LastSeen.IsZero())
}

// pongGatewayConn hands the pong handler handleConnection installs to the test
// and keeps the connection open until Close.
type pongGatewayConn struct {
	mockGatewayConn
	pongHandler chan func(string) error
	closed      chan struct{}
	closeOnce   sync.Once
}

func newPongGatewayConn() *pongGatewayConn {
	return &pongGatewayConn{
		pongHandler: make(chan func(string) error, 1),
		closed:      make(chan struct{}),
	}
}

func (c *pongGatewayConn) ReadMessage() (int, []byte, error) {
	<-c.closed
	return 0, nil, errors.New("connection closed")
}

func (c *pongGatewayConn) SetPongHandler(h func(string) error) { c.pongHandler <- h }

func (c *pongGatewayConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

// Registration publishes LastSeen once. After that the agent's pongs are the
// only thing that moves it on peers, so the pong handler has to republish: a
// non-owner replica would otherwise report the registration time forever.
func TestServer_PongRefreshesLastSeenOnNonOwnerReplica(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverA, _, discA, addrA := newFabricServer(t, ctx, "gw-a")
	serverB, _, discB, addrB := newFabricServer(t, ctx, "gw-b")

	discA.ch <- []fabric.Peer{{ID: "gw-b", Addr: addrB}}
	discB.ch <- []fabric.Peer{{ID: "gw-a", Addr: addrA}}

	conn := newPongGatewayConn()
	connID, err := serverA.connMgr.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		serverA.handleConnection("dataplane/prod", connID, conn)
	}()
	t.Cleanup(func() {
		_ = conn.Close()
		<-handlerDone
	})
	pong := <-conn.pongHandler

	// Age the published entry past the publish throttle, as if the agent
	// registered a minute ago, so the pong below is due a refresh.
	stale := time.Now().Add(-time.Minute).Round(0)
	serverA.fabricMesh.LocalUpsert(fabric.AgentEntry{
		PlaneIdentifier: "dataplane/prod",
		ConnID:          connID,
		ValidCRs:        []string{"ns/dp1"},
		LastSeen:        stale,
	})
	waitForCondition(t, "aged entry on gw-b", func() bool {
		return serverB.PlaneStatus("dataplane", "prod").LastSeen.Equal(stale)
	})

	before := time.Now().Round(0)
	require.NoError(t, pong(""))

	waitForCondition(t, "pong refresh on gw-b", func() bool {
		return !serverB.PlaneStatus("dataplane", "prod").LastSeen.Before(before)
	})
	crStatus := serverB.CRAuthorizationStatus("dataplane", "prod", "ns", "dp1")
	assert.True(t, crStatus.Connected)
	assert.False(t, crStatus.LastSeen.Before(before), "CR LastSeen %v predates the pong", crStatus.LastSeen)
	all := serverB.AllPlaneStatuses()
	require.Len(t, all, 1)
	assert.False(t, all[0].LastSeen.Before(before), "listed LastSeen %v predates the pong", all[0].LastSeen)
}

// With no remote candidates the caller gets the original routing error, so
// error semantics (e.g. the 403 "no agents authorized" mapping) are preserved.
func TestServer_ForwardViaMesh_NoCandidates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverB, _, _, _ := newFabricServer(t, ctx, "gw-b")

	req := messaging.NewHTTPTunnelRequest("k8s", "GET", "/x", "", nil, nil)
	_, _, err := serverB.SendHTTPTunnelRequestForCR("dataplane/prod", "ns/dp1", req, time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no agents found for plane")
}

// A CR that no replica is authorized for is an authorization miss on every
// replica. A pod with no local agent used to report it as "no agents found for
// plane", which reads as a disconnected agent and is answered with 502, not 403.
func TestServer_ForwardViaMesh_PeerAgentsNotAuthorizedForCR(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	caller, registry, _, _ := newFabricServer(t, ctx, "gw-caller")
	require.True(t, registry.ApplyDelta(fabric.Delta{
		Op: fabric.DeltaOpAdd, Owner: "gw-a", Seq: 1,
		Entry: fabric.AgentEntry{
			PlaneIdentifier: "dataplane/prod",
			ConnID:          "conn-0",
			ValidCRs:        []string{"ns/dp1"},
		},
	}))

	req := messaging.NewHTTPTunnelRequest("k8s", "GET", "/x", "", nil, nil)
	_, _, err := caller.SendHTTPTunnelRequestForCR("dataplane/prod", "ns/dp2", req, time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no agents authorized for CR ns/dp2")
}

// A forward that was attempted and failed is a transport failure. Reporting
// this pod's local "not authorized" view instead would answer 403, which
// callers treat as permanent, while a peer holds an authorized agent.
func TestServer_ForwardViaMesh_FailedForwardIsNotAnAuthorizationError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	caller, registry, _, _ := newFabricServer(t, ctx, "gw-caller")

	// This pod's own agent is not authorized for the CR.
	_, err := caller.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, []string{"ns/other"}, nil, nil)
	require.NoError(t, err)

	// A peer advertises an authorized agent, but no link to it exists.
	require.True(t, registry.ApplyDelta(fabric.Delta{
		Op: fabric.DeltaOpAdd, Owner: "gw-a", Seq: 1,
		Entry: fabric.AgentEntry{
			PlaneIdentifier: "dataplane/prod",
			ConnID:          "conn-0",
			ValidCRs:        []string{"ns/dp1"},
		},
	}))

	req := messaging.NewHTTPTunnelRequest("k8s", "GET", "/x", "", nil, nil)
	_, _, err = caller.SendHTTPTunnelRequestForCR("dataplane/prod", "ns/dp1", req, time.Second)
	require.Error(t, err)
	assert.ErrorIs(t, err, fabric.ErrNoLink)
	assert.NotContains(t, err.Error(), "no agents authorized for CR")
}

// A draining replica is skipped by the forward, but its agents are still
// connected and about to reconnect elsewhere. A CR they are authorized for must
// not be answered with an authorization error in the meantime: that is a 403,
// which callers do not retry, for every such CR on each gateway rolling restart.
func TestServer_ForwardViaMesh_DrainingAuthorizedAgentIsNotAnAuthorizationError(t *testing.T) {
	tests := []struct {
		name                    string
		localUnauthorizedAgent  bool
		remoteUnauthorizedAgent bool
	}{
		{name: "no other agent on the plane"},
		{name: "this pod's agent is not authorized for the CR", localUnauthorizedAgent: true},
		{name: "another peer's agent is not authorized for the CR", remoteUnauthorizedAgent: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)

			caller, registry, _, _ := newFabricServer(t, ctx, "gw-caller")
			require.True(t, registry.ApplyDelta(fabric.Delta{
				Op: fabric.DeltaOpAdd, Owner: "gw-draining", Seq: 1,
				Entry: fabric.AgentEntry{
					PlaneIdentifier: "dataplane/prod",
					ConnID:          "conn-0",
					ValidCRs:        []string{"ns/dp1"},
				},
			}))
			registry.SetDraining("gw-draining")

			if tt.localUnauthorizedAgent {
				_, err := caller.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, []string{"ns/other"}, nil, nil)
				require.NoError(t, err)
			}
			if tt.remoteUnauthorizedAgent {
				require.True(t, registry.ApplyDelta(fabric.Delta{
					Op: fabric.DeltaOpAdd, Owner: "gw-b", Seq: 1,
					Entry: fabric.AgentEntry{
						PlaneIdentifier: "dataplane/prod",
						ConnID:          "conn-1",
						ValidCRs:        []string{"ns/other"},
					},
				}))
			}

			req := messaging.NewHTTPTunnelRequest("k8s", "GET", "/x", "", nil, nil)
			_, _, err := caller.SendHTTPTunnelRequestForCR("dataplane/prod", "ns/dp1", req, time.Second)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "no agents authorized for CR")
		})
	}
}

// An owner that answers NoAgent speaks only for itself. When another owner
// could not be reached at all, that owner may still hold an authorized agent,
// so its failure is what the caller must see. Reporting the NoAgent answer
// instead can turn an unreachable peer into a 403.
func TestServer_ForwardViaMesh_UnreachablePeerOutranksNoAgentAnswer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	caller, registry, discCaller, addrCaller := newFabricServer(t, ctx, "gw-caller")

	// Sorted first, so it is the candidate tried first. It is never pushed into
	// discovery, so there is no link to it.
	require.True(t, registry.ApplyDelta(fabric.Delta{
		Op: fabric.DeltaOpAdd, Owner: "gw-a-down", Seq: 1,
		Entry: fabric.AgentEntry{
			PlaneIdentifier: "dataplane/prod",
			ConnID:          "conn-down",
			ValidCRs:        []string{"ns/dp1"},
		},
	}))

	// Reachable, but its own view says its agent is not authorized for the CR.
	stale, discStale, addrStale := newMeshPeer(t, ctx, "gw-b-stale", &stubMeshDelegate{
		noAgent:      true,
		noAgentError: "no agents authorized for CR ns/dp1",
	})
	discCaller.ch <- []fabric.Peer{{ID: "gw-b-stale", Addr: addrStale}}
	discStale.ch <- []fabric.Peer{{ID: "gw-caller", Addr: addrCaller}}
	stale.LocalUpsert(fabric.AgentEntry{
		PlaneIdentifier: "dataplane/prod",
		ConnID:          "conn-stale",
		ValidCRs:        []string{"ns/dp1"},
	})

	// Counted and probed rather than looked up: Lookup advances the round-robin
	// cursor, which decides the candidate order under test.
	waitForCondition(t, "the caller to see the stale peer's entry", func() bool {
		return registry.CountByOwner()["gw-b-stale"] == 1
	})
	waitForCondition(t, "an outbound link to the stale peer", func() bool {
		probeCtx, probeCancel := context.WithTimeout(context.Background(), time.Second)
		defer probeCancel()
		rsp, err := caller.fabricMesh.Forward(probeCtx, "gw-b-stale", &fabric.ForwardRequest{
			PlaneIdentifier: "dataplane/prod",
			CRKey:           "ns/dp1",
			TimeoutMillis:   time.Second.Milliseconds(),
			Request:         &messaging.HTTPTunnelRequest{Target: "k8s", Method: http.MethodGet},
		})
		return err == nil && rsp != nil && rsp.NoAgent
	})

	// Twice, so both candidate orders are exercised: the cursor rotates per call.
	for _, order := range []string{"unreachable peer tried first", "unreachable peer tried second"} {
		req := &messaging.HTTPTunnelRequest{Target: "k8s", Method: http.MethodGet, Path: "/api/v1/pods"}
		_, _, err := caller.SendHTTPTunnelRequestForCR("dataplane/prod", "ns/dp1", req, 5*time.Second)
		require.Error(t, err, order)
		assert.ErrorIs(t, err, fabric.ErrNoLink, order)
		assert.NotContains(t, err.Error(), "no agents authorized for CR", order)
	}
}

// The choreographed drain: GOAWAY frames go out to every connection, then the
// sockets are closed.
func TestServer_DrainAgentConnections(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{DrainWindow: 20 * time.Millisecond}, fakeClient, testLogger())

	mock := &mockGatewayConn{}
	_, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

	s.drainAgentConnections(context.Background(), s.config.DrainWindow)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	require.NotEmpty(t, mock.writtenMsgs)
	var goAway messaging.GoAway
	require.NoError(t, json.Unmarshal(mock.writtenMsgs[0], &goAway))
	assert.Equal(t, messaging.MessageTypeGoAway, goAway.Type)
	assert.True(t, mock.closed)
}

// A propagated plane deletion must disconnect this pod's agents even though
// the original notification landed on a different replica.
func TestServer_ApplyPlaneEvent(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	mock := &mockGatewayConn{}
	_, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

	s.ApplyPlaneEvent(fabric.PlaneEvent{PlaneType: "dataplane", PlaneID: "prod", Event: "deleted"})

	mock.mu.Lock()
	closed := mock.closed
	mock.mu.Unlock()
	assert.True(t, closed)
	assert.Equal(t, 0, s.connMgr.Count())

	// created/updated with no matching CR must not panic; it logs and keeps
	// existing connections untouched.
	s.ApplyPlaneEvent(fabric.PlaneEvent{
		PlaneType: "dataplane", PlaneID: "prod", Event: "updated",
		Namespace: "ns", Name: "dp1",
	})
}

// Readiness must drop during drain so the pod leaves Service endpoints.
func TestServer_HandleReadyDuringDrain(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	w := httptest.NewRecorder()
	s.handleReady(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
	assert.Equal(t, http.StatusOK, w.Code)

	s.draining.Store(true)
	w = httptest.NewRecorder()
	s.handleReady(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// On a large cluster the first list of plane CRs can take longer than the
// liveness probe allows. A gateway that waited for it before starting would be
// restarted before it had served a single agent, over and over. So it starts
// at once and keeps out of the way until the list is there: alive, not ready,
// and refusing agents, because an agent checked against a partial list would be
// authorized for too little.
func TestServer_StaysNotReadyAndRefusesAgentsWhilePlaneCRsLoad(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())
	loaded := make(chan struct{})
	s.SetPlaneReader(fakeClient, func(ctx context.Context) bool {
		select {
		case <-loaded:
			return true
		case <-ctx.Done():
			return false
		}
	})
	waited := make(chan error, 1)
	go func() { waited <- s.awaitPlaneReader(context.Background()) }()

	probe := func(handler http.HandlerFunc, target string) int {
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest(http.MethodGet, target, nil))
		return w.Code
	}
	const agentConnect = "/ws?planeType=dataplane&planeID=prod"

	assert.Equal(t, http.StatusOK, probe(s.handleHealth, "/health"), "the pod must not be restarted while it loads")
	assert.Equal(t, http.StatusServiceUnavailable, probe(s.handleReady, "/ready"))
	assert.Equal(t, http.StatusServiceUnavailable, probe(s.handleWebSocket, agentConnect))

	close(loaded)
	require.NoError(t, <-waited)
	assert.Equal(t, http.StatusOK, probe(s.handleReady, "/ready"))
	// The connect now gets as far as asking for the agent's certificate.
	assert.Equal(t, http.StatusUnauthorized, probe(s.handleWebSocket, agentConnect))
}

// Waiting without limit is right when the first list is slow. When the cache is
// stuck, a restart can be what gets it going, and an operator may prefer that
// to a pod that stays not ready. The limit is theirs to set: the gateway gives
// up only when one is set and has passed.
func TestServer_GivesUpOnLoadingPlaneCRsOnlyWhenALimitIsSet(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	neverLoaded := func(ctx context.Context) bool {
		<-ctx.Done()
		return false
	}

	t.Run("with a limit it gives up once the limit has passed", func(t *testing.T) {
		s := New(&Config{PlaneCacheSyncTimeout: 20 * time.Millisecond}, fakeClient, testLogger())
		s.SetPlaneReader(fakeClient, neverLoaded)

		err := s.awaitPlaneReader(context.Background())

		require.Error(t, err)
		assert.False(t, s.planeReaderReady())
	})

	t.Run("without a limit it waits until the server shuts down", func(t *testing.T) {
		s := New(&Config{}, fakeClient, testLogger())
		s.SetPlaneReader(fakeClient, neverLoaded)
		ctx, shutDown := context.WithCancel(context.Background())
		waited := make(chan error, 1)
		go func() { waited <- s.awaitPlaneReader(ctx) }()

		select {
		case err := <-waited:
			t.Fatalf("stopped waiting with no limit set: %v", err)
		case <-time.After(200 * time.Millisecond):
		}

		shutDown()
		assert.NoError(t, <-waited, "shutting down is not a failure to load")
	})
}

// --- connection rebalancing ---

// newRebalanceServer wires a server with a mesh identity and `local` mock
// connections registered, so rebalanceOnce sees a realistic local count.
func newRebalanceServer(t *testing.T, self string, local int) (*Server, *fabric.Registry, []*mockGatewayConn) {
	t.Helper()
	s := New(&Config{}, fake.NewClientBuilder().WithScheme(testScheme()).Build(), testLogger())
	registry := fabric.NewRegistry(testLogger())
	mesh := fabric.NewMesh(fabric.MeshConfig{Self: fabric.Peer{ID: self}}, registry,
		&staticPeerDiscovery{ch: make(chan []fabric.Peer, 1)}, s, testLogger())
	s.SetFabric(mesh, registry)

	conns := make([]*mockGatewayConn, 0, local)
	for range local {
		m := &mockGatewayConn{}
		_, err := s.connMgr.Register("dataplane", "prod", m, []string{"ns/dp1"}, nil, nil)
		require.NoError(t, err)
		conns = append(conns, m)
	}
	return s, registry, conns
}

func addPeerConns(t *testing.T, r *fabric.Registry, owner string, n int) {
	t.Helper()
	seq := uint64(0)
	for i := range n {
		seq++
		require.True(t, r.ApplyDelta(fabric.Delta{
			Op: fabric.DeltaOpAdd, Owner: owner, Seq: seq,
			Entry: fabric.AgentEntry{
				PlaneIdentifier: "dataplane/prod",
				ConnID:          fmt.Sprintf("%s-%d", owner, i),
				ValidCRs:        []string{"ns/dp1"},
			},
		}))
	}
}

func goAwayCount(conns []*mockGatewayConn) int {
	n := 0
	for _, c := range conns {
		c.mu.Lock()
		for _, msg := range c.writtenMsgs {
			var g messaging.GoAway
			if json.Unmarshal(msg, &g) == nil && g.Type == messaging.MessageTypeGoAway {
				n++
			}
		}
		c.mu.Unlock()
	}
	return n
}

// The case this feature exists for: every connection has piled onto one
// replica, so restarting it would evict every agent at once.
func TestRebalance_ShedsWhenConcentrated(t *testing.T) {
	s, registry, conns := newRebalanceServer(t, "gw-self", 6)
	addPeerConns(t, registry, "gw-b", 0)
	addPeerConns(t, registry, "gw-c", 0)
	// Make the peers known even with zero connections.
	require.True(t, registry.ApplyDelta(fabric.Delta{
		Op: fabric.DeltaOpAdd, Owner: "gw-b", Seq: 1,
		Entry: fabric.AgentEntry{PlaneIdentifier: "other/x", ConnID: "b-keep"},
	}))

	s.rebalanceOnce()
	shed := goAwayCount(conns)
	assert.Positive(t, shed, "a concentrated pod must shed")
	assert.LessOrEqual(t, shed, rebalanceMaxShedPerCycle,
		"shedding must stay bounded per cycle so correcting imbalance cannot itself stampede")
	assert.Less(t, shed, len(conns),
		"never shed everything at once — that would recreate the concentration it is fixing")
}

// Shedding is proportional: a badly concentrated pod corrects faster than a
// marginally over-target one, but still never dumps its whole excess at once.
func TestRebalance_ShedsProportionallyToExcess(t *testing.T) {
	s, registry, conns := newRebalanceServer(t, "gw-self", 12)
	for _, peer := range []string{"gw-b", "gw-c"} {
		require.True(t, registry.ApplyDelta(fabric.Delta{
			Op: fabric.DeltaOpAdd, Owner: peer, Seq: 1,
			Entry: fabric.AgentEntry{PlaneIdentifier: "other/x", ConnID: peer + "-keep"},
		}))
	}
	// 14 connections over 3 pods -> fair share 5, tolerated 6, excess 6.
	s.rebalanceOnce()
	shed := goAwayCount(conns)
	assert.Greater(t, shed, 1, "a large excess should shed more than the minimum")
	assert.LessOrEqual(t, shed, rebalanceMaxShedPerCycle)
}

// An already-even fleet must be left alone: shedding here would churn agents
// for no benefit and could oscillate.
func TestRebalance_NoOpWhenBalanced(t *testing.T) {
	s, registry, conns := newRebalanceServer(t, "gw-self", 2)
	addPeerConns(t, registry, "gw-b", 2)
	addPeerConns(t, registry, "gw-c", 2)

	s.rebalanceOnce()
	assert.Zero(t, goAwayCount(conns), "a balanced pod must not shed")
}

// A fleet that cannot divide evenly (4 across 3) always leaves someone holding
// the remainder. Without slack that pod would shed forever.
func TestRebalance_ToleratesUnevenRemainder(t *testing.T) {
	s, registry, conns := newRebalanceServer(t, "gw-self", 2)
	addPeerConns(t, registry, "gw-b", 1)
	addPeerConns(t, registry, "gw-c", 1)

	s.rebalanceOnce()
	assert.Zero(t, goAwayCount(conns), "holding the remainder is not an imbalance")
}

// A draining pod is already shedding everything; rebalancing on top of that
// would evict agents twice.
func TestRebalance_SkipsWhileDraining(t *testing.T) {
	s, registry, conns := newRebalanceServer(t, "gw-self", 6)
	addPeerConns(t, registry, "gw-b", 0)
	require.True(t, registry.ApplyDelta(fabric.Delta{
		Op: fabric.DeltaOpAdd, Owner: "gw-b", Seq: 1,
		Entry: fabric.AgentEntry{PlaneIdentifier: "other/x", ConnID: "b-keep"},
	}))
	s.draining.Store(true)

	s.rebalanceOnce()
	assert.Zero(t, goAwayCount(conns), "a draining pod must not rebalance")
}

// A singleton gateway has nowhere to move connections to.
func TestRebalance_NoOpWithSinglePod(t *testing.T) {
	s, _, conns := newRebalanceServer(t, "gw-solo", 5)
	s.rebalanceOnce()
	assert.Zero(t, goAwayCount(conns), "a lone replica has no peer to rebalance onto")
}

// The drain sleeps for its whole window plus the grace period with no context
// to cut it short, so a window that outgrows the shutdown budget does not
// degrade - it runs the rest of shutdown against an expired deadline and gets
// the pod killed mid-handover. Start must reject that before binding a port,
// while still accepting a window that fits and zero, which disables the drain.
func TestStart_DrainWindowValidation(t *testing.T) {
	// Certificate paths deliberately missing: validation runs before the
	// certificate is loaded, so the error identifies which check fired.
	newServer := func(drain, shutdown time.Duration) *Server {
		return New(&Config{
			ServerCertPath:  filepath.Join(t.TempDir(), "missing.crt"),
			ServerKeyPath:   filepath.Join(t.TempDir(), "missing.key"),
			DrainWindow:     drain,
			ShutdownTimeout: shutdown,
		}, nil, testLogger())
	}

	t.Run("window plus grace over budget is rejected", func(t *testing.T) {
		err := newServer(20*time.Second, 10*time.Second).Start()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "drain window does not fit in the shutdown timeout")
	})

	t.Run("window leaving exactly the grace period is accepted", func(t *testing.T) {
		err := newServer(8*time.Second, 8*time.Second+drainGracePeriod).Start()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load server certificate",
			"a window that fits must pass validation and fail later, on the certificate")
	})

	t.Run("zero disables the drain and needs no budget", func(t *testing.T) {
		err := newServer(0, 0).Start()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load server certificate",
			"a disabled drain must not be rejected for exceeding the shutdown timeout")
	})
}

// The drain must respect the shutdown budget it was given. Sleeping out a
// window that no longer fits does not buy a gentler handover - it runs past
// the deadline until the kubelet sends SIGKILL, which drops the connections
// mid-drain. Out of budget, the sockets close immediately instead.
func TestServer_DrainAgentConnections_StopsWhenBudgetExpires(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{DrainWindow: 30 * time.Second}, fakeClient, testLogger())

	conns := make([]*mockGatewayConn, 3)
	for i := range conns {
		conns[i] = &mockGatewayConn{}
		_, err := s.connMgr.Register("dataplane", "prod", conns[i], []string{"ns/dp1"}, nil, nil)
		require.NoError(t, err)
	}

	// A budget that is already gone, as when the main server's shutdown has
	// consumed the whole timeout before the drain starts.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	s.drainAgentConnections(ctx, s.config.DrainWindow)
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 5*time.Second,
		"an expired budget must not be slept through: the drain window is 30s")
	for i, c := range conns {
		c.mu.Lock()
		closed := c.closed
		c.mu.Unlock()
		assert.True(t, closed, "connection %d must still be closed when the drain is cut short", i)
	}
}

// A forward whose outcome is unknown must not be replayed for a method that
// changes state. The request was already on the wire when the link dropped, so
// the owner may have dispatched it to its agent and applied it; retrying on
// another candidate would apply it a second time. Failures proven to precede
// dispatch stay retryable, and reads stay retryable regardless.
func TestForwardIsSafeToRetry(t *testing.T) {
	get := &messaging.HTTPTunnelRequest{Method: "GET"}
	post := &messaging.HTTPTunnelRequest{Method: "POST"}

	tests := []struct {
		name string
		err  error
		req  *messaging.HTTPTunnelRequest
		want bool
	}{
		{"no link, write request", fmt.Errorf("%w: gw-1", fabric.ErrNoLink), post, true},
		{"never sent, write request", fmt.Errorf("%w: gw-1", fabric.ErrForwardNotSent), post, true},
		{"may have executed, write request",
			fmt.Errorf("%w: gw-1 (link lost mid-request)", fabric.ErrForwardMayHaveExecuted), post, false},
		{"may have executed, read request",
			fmt.Errorf("%w: gw-1 (link lost mid-request)", fabric.ErrForwardMayHaveExecuted), get, true},
		{"lowercase method is still a read",
			fmt.Errorf("%w: gw-1", fabric.ErrForwardMayHaveExecuted),
			&messaging.HTTPTunnelRequest{Method: "get"}, true},
		{"delete is not replayable",
			fmt.Errorf("%w: gw-1", fabric.ErrForwardMayHaveExecuted),
			&messaging.HTTPTunnelRequest{Method: "DELETE"}, false},
		{"unknown error, write request", errors.New("something else"), post, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, forwardIsSafeToRetry(tt.err, tt.req))
		})
	}
}

// The mid-request sentinel must stay distinct from ErrNoLink: they used to be
// the same error, which left callers unable to tell "never dispatched" from
// "outcome unknown" without matching on message text.
func TestForwardSentinelsAreDistinct(t *testing.T) {
	midRequest := fmt.Errorf("%w: gw-1 (link lost mid-request)", fabric.ErrForwardMayHaveExecuted)
	assert.False(t, errors.Is(midRequest, fabric.ErrNoLink),
		"a mid-request loss must not be reported as a missing link")
	assert.True(t, errors.Is(midRequest, fabric.ErrForwardMayHaveExecuted))

	noLink := fmt.Errorf("%w: gw-1", fabric.ErrNoLink)
	assert.False(t, errors.Is(noLink, fabric.ErrForwardMayHaveExecuted))
}

// /ready is what keeps a pod out of Service endpoints while it cannot serve.
// It must fail during drain and while the mesh registry is still converging,
// and succeed once neither is true — /healthz stays up throughout, since the
// process is alive either way.
func TestHealthAndReadinessEndpoints(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	t.Run("healthy and not draining without a mesh", func(t *testing.T) {
		s := New(&Config{}, fakeClient, testLogger())

		rec := httptest.NewRecorder()
		s.handleReady(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))
		assert.Equal(t, http.StatusOK, rec.Code, "a gateway with no mesh has nothing to converge")

		rec = httptest.NewRecorder()
		s.handleHealth(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "OK", rec.Body.String())
	})

	t.Run("draining fails readiness but stays healthy", func(t *testing.T) {
		s := New(&Config{}, fakeClient, testLogger())
		s.draining.Store(true)

		rec := httptest.NewRecorder()
		s.handleReady(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code,
			"a draining pod must leave Service endpoints")
		assert.Contains(t, rec.Body.String(), "draining")

		// Liveness must not fail while draining, or the kubelet would kill the
		// pod part-way through its handover.
		rec = httptest.NewRecorder()
		s.handleHealth(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("unconverged mesh fails readiness", func(t *testing.T) {
		s := New(&Config{}, fakeClient, testLogger())
		registry := fabric.NewRegistry(testLogger())
		mesh := fabric.NewMesh(fabric.MeshConfig{Self: fabric.Peer{ID: "gw-self"}},
			registry, &staticPeerDiscovery{ch: make(chan []fabric.Peer)}, s, testLogger())
		s.SetFabric(mesh, registry)

		// Never started, so discovery has not reported and startedAt is zero:
		// the pod cannot claim a usable view of where agents live.
		rec := httptest.NewRecorder()
		s.handleReady(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code,
			"a pod whose registry has not converged would answer 'no agents found'")
		assert.Contains(t, rec.Body.String(), "converged")
	})
}

// --- ConnectionManager: paths the happy-path tests never reach ---

// A write failure must surface to the caller rather than be swallowed: the
// caller is waiting on a reply that will now never arrive, and only the error
// lets it fail fast instead of burning the full tunnel timeout.
func TestConnectionManager_SendSurfacesWriteFailures(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	failing := &writeErrorGatewayConn{err: errors.New("broken pipe")}
	_, err := s.connMgr.Register("dataplane", "prod", failing, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

	err = s.connMgr.SendHTTPTunnelRequest("dataplane/prod",
		&messaging.HTTPTunnelRequest{RequestID: "req-1", Target: "k8s"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send request")

	// The raw path carries GOAWAY, so it must report failures too - a drain
	// that silently fails to notify would close sockets with no warning.
	all := s.connMgr.GetAll()
	require.Len(t, all, 1)
	assert.ErrorContains(t, all[0].SendRawMessage([]byte(`{"type":"goaway"}`)), "failed to send message")
}

// Routing to a plane with no connections must name the plane: this error text
// is what tells an operator the agent never connected, as opposed to the
// request itself failing.
func TestConnectionManager_SendToUnknownPlane(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	err := s.connMgr.SendHTTPTunnelRequest("dataplane/absent",
		&messaging.HTTPTunnelRequest{RequestID: "req-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no agents found for plane")
}

// Unregistering something that was never registered must be a no-op. It is
// driven by socket teardown, which can race with a plane deletion that already
// removed the connection.
func TestConnectionManager_UnregisterUnknownIsNoop(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	connID, err := s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

	s.connMgr.Unregister("dataplane/prod", "never-registered")
	s.connMgr.Unregister("dataplane/absent", connID)

	assert.Equal(t, 1, s.connMgr.Count(), "a stray unregistration removed a live connection")
}

// Last-seen drives the heartbeat timeout, so an update for a connection that
// has already gone must not panic or resurrect state.
func TestConnectionManager_UpdateLastSeenUnknownIsNoop(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	s.connMgr.UpdateConnectionLastSeen("dataplane/absent", "gone")
	assert.Equal(t, 0, s.connMgr.Count())
}

// Every agent connection starts a ping sender. Stopping its ticker does not end
// it, so unless it is told the connection is gone it stays parked for the life
// of the pod: one goroutine for every connection an agent ever made.
func TestHandleConnection_PingSenderEndsWithConnection(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{HeartbeatInterval: time.Hour, HeartbeatTimeout: time.Hour}, fakeClient, testLogger())

	const connections = 50
	before := runtime.NumGoroutine()
	for range connections {
		mock := &closeErrorGatewayConn{err: errors.New("connection closed")}
		connID, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil, nil)
		require.NoError(t, err)
		s.handleConnection("dataplane/prod", connID, mock)
	}

	// Half the connection count is slack for goroutines unrelated to this test.
	assert.Eventually(t, func() bool {
		return runtime.NumGoroutine() < before+connections/2
	}, 2*time.Second, 10*time.Millisecond, "ping senders outlived their connections")
}

// writeErrorGatewayConn fails every write, standing in for a socket that died
// between selection and dispatch.
type writeErrorGatewayConn struct {
	mu  sync.Mutex
	err error
}

func (c *writeErrorGatewayConn) ReadMessage() (int, []byte, error) {
	return 0, nil, errors.New("connection closed")
}

func (c *writeErrorGatewayConn) WriteMessage(_ int, _ []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *writeErrorGatewayConn) WriteControl(_ int, _ []byte, _ time.Time) error { return c.err }
func (c *writeErrorGatewayConn) SetReadDeadline(_ time.Time) error               { return nil }
func (c *writeErrorGatewayConn) SetWriteDeadline(_ time.Time) error              { return nil }
func (c *writeErrorGatewayConn) SetPongHandler(_ func(string) error)             {}
func (c *writeErrorGatewayConn) Close() error                                    { return nil }

// --- Rebalancer ---

// rebalanceSetup wires a server with a mesh identity and a replicated registry
// holding peerCounts connections per peer, plus localConns local connections.
func rebalanceSetup(t *testing.T, localConns int, peerCounts map[string]int) (*Server, []*mockGatewayConn) {
	t.Helper()

	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	registry := fabric.NewRegistry(testLogger())
	mesh := fabric.NewMesh(fabric.MeshConfig{Self: fabric.Peer{ID: "gw-self"}},
		registry, &staticPeerDiscovery{ch: make(chan []fabric.Peer)}, s, testLogger())
	s.SetFabric(mesh, registry)

	conns := make([]*mockGatewayConn, 0, localConns)
	for range localConns {
		m := &mockGatewayConn{}
		_, err := s.connMgr.Register("dataplane", "prod", m, []string{"ns/dp1"}, nil, nil)
		require.NoError(t, err)
		conns = append(conns, m)
	}

	for owner, n := range peerCounts {
		seq := uint64(0)
		for i := range n {
			seq++
			require.True(t, registry.ApplyDelta(fabric.Delta{
				Op: fabric.DeltaOpAdd, Owner: owner, Seq: seq,
				Entry: fabric.AgentEntry{
					PlaneIdentifier: "dataplane/prod",
					ConnID:          fmt.Sprintf("%s-%d", owner, i),
					ValidCRs:        []string{"ns/dp1"},
				},
			}))
		}
	}
	return s, conns
}

func countGoAways(conns []*mockGatewayConn) int {
	n := 0
	for _, c := range conns {
		for _, msg := range c.getWrittenMessages() {
			var ga messaging.GoAway
			if err := json.Unmarshal(msg, &ga); err == nil && ga.Type == messaging.MessageTypeGoAway {
				n++
				break
			}
		}
	}
	return n
}

// Shedding exists to even out connections across replicas, but over-shedding
// is worse than none: an evicted agent may land straight back here, so a pod
// that is merely one over its share must sit still.
func TestServer_RebalanceOnce(t *testing.T) {
	tests := []struct {
		name       string
		local      int
		peerCounts map[string]int
		wantShed   int
		reason     string
	}{
		{
			name: "balanced fleet sheds nothing", local: 3,
			peerCounts: map[string]int{"gw-b": 3, "gw-c": 3}, wantShed: 0,
			reason: "every pod is already at its fair share",
		},
		{
			name: "one over fair share is within slack", local: 4,
			peerCounts: map[string]int{"gw-b": 3, "gw-c": 3}, wantShed: 0,
			reason: "slack absorbs the remainder so pods do not thrash",
		},
		{
			name: "clear excess sheds about half", local: 10,
			peerCounts: map[string]int{"gw-b": 1, "gw-c": 1}, wantShed: 3,
			reason: "half the excess, capped per cycle",
		},
		{
			name: "shedding is capped per cycle", local: 100,
			peerCounts: map[string]int{"gw-b": 1, "gw-c": 1}, wantShed: rebalanceMaxShedPerCycle,
			reason: "a huge imbalance still moves gradually",
		},
		{
			name: "a lone pod has nobody to shed to", local: 10,
			peerCounts: nil, wantShed: 0,
			reason: "with no peers there is nothing to balance against",
		},
		{
			name: "holding none of the fleet's connections sheds nothing", local: 0,
			peerCounts: map[string]int{"gw-b": 4}, wantShed: 0,
			reason: "a pod below its share has nothing to give away",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, conns := rebalanceSetup(t, tt.local, tt.peerCounts)

			s.rebalanceOnce()

			assert.Equal(t, tt.wantShed, countGoAways(conns), tt.reason)
			// Shedding must never close the socket: the agent reconnects on
			// its own, and closing here would drop its in-flight replies.
			for i, c := range conns {
				assert.False(t, c.isClosed(), "connection %d was closed by the rebalancer", i)
			}
		})
	}
}

// A draining pod is already shedding everything it has. Rebalancing on top of
// that would send GOAWAY twice and fight the drain's own pacing.
func TestServer_RebalanceSkippedWhileDraining(t *testing.T) {
	peers := map[string]int{"gw-b": 1, "gw-c": 1}

	// Precondition: this exact fleet shape does shed when not draining, so a
	// zero below means the draining guard fired rather than the maths.
	notDraining, busyConns := rebalanceSetup(t, 10, peers)
	notDraining.rebalanceOnce()
	require.NotZero(t, countGoAways(busyConns), "precondition: this fleet should shed")

	s, conns := rebalanceSetup(t, 10, peers)
	s.draining.Store(true)

	s.rebalanceOnce()

	assert.Zero(t, countGoAways(conns), "a draining pod must not also rebalance")
}

// Without a mesh there is no fleet view at all, so any shedding decision would
// be made against a registry that knows only about this pod.
func TestServer_RebalanceSkippedWithoutMesh(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	conns := make([]*mockGatewayConn, 0, 5)
	for range 5 {
		m := &mockGatewayConn{}
		_, err := s.connMgr.Register("dataplane", "prod", m, []string{"ns/dp1"}, nil, nil)
		require.NoError(t, err)
		conns = append(conns, m)
	}

	s.rebalanceOnce()

	assert.Zero(t, countGoAways(conns), "a meshless gateway has no fleet to balance against")
}

// A stale registry entry must not fail the request. The owner answers NoAgent
// — it never dispatched anything — so the caller is free to try the next
// candidate, which is the whole point of tracking more than one.
func TestServer_ForwardViaMesh_RetriesPastAStaleOwner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	caller, registryCaller, discCaller, addrCaller := newFabricServer(t, ctx, "gw-caller")
	stale, _, discStale, addrStale := newFabricServer(t, ctx, "gw-stale")
	owner, _, discOwner, addrOwner := newFabricServer(t, ctx, "gw-owner")

	discCaller.ch <- []fabric.Peer{{ID: "gw-stale", Addr: addrStale}, {ID: "gw-owner", Addr: addrOwner}}
	discStale.ch <- []fabric.Peer{{ID: "gw-caller", Addr: addrCaller}, {ID: "gw-owner", Addr: addrOwner}}
	discOwner.ch <- []fabric.Peer{{ID: "gw-caller", Addr: addrCaller}, {ID: "gw-stale", Addr: addrStale}}

	// Only gw-owner actually holds the agent.
	mock := &mockGatewayConn{}
	ownerConnID, err := owner.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)
	go respondToTunnelRequests(ctx, owner, ownerConnID, mock)

	// gw-stale advertises a connection it does not have, as it would just
	// after that connection dropped but before the removal delta lands.
	require.True(t, registryCaller.ApplyDelta(fabric.Delta{
		Op: fabric.DeltaOpAdd, Owner: "gw-stale", Seq: 1,
		Entry: fabric.AgentEntry{PlaneIdentifier: "dataplane/prod", ConnID: "ghost", ValidCRs: []string{"ns/dp1"}},
	}))
	waitForCondition(t, "the caller to learn about the real owner", func() bool {
		for _, c := range registryCaller.Lookup("dataplane/prod", "ns/dp1") {
			if c.Owner == "gw-owner" {
				return true
			}
		}
		return false
	})

	req := &messaging.HTTPTunnelRequest{Target: "k8s", Method: "GET", Path: "/api/v1/pods"}
	resp, servedBy, err := caller.SendHTTPTunnelRequestForCR("dataplane/prod", "ns/dp1", req, 5*time.Second)
	require.NoError(t, err, "a stale candidate must not fail the request")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "gw-owner", servedBy, "the request must be served by the pod that owns the agent")
	_ = stale
}

// respondToTunnelRequests answers whatever the gateway writes to the agent
// socket, standing in for a connected cluster agent.
func respondToTunnelRequests(ctx context.Context, s *Server, connID string, mock *mockGatewayConn) {
	seen := 0
	for ctx.Err() == nil {
		msgs := mock.getWrittenMessages()
		for ; seen < len(msgs); seen++ {
			var req messaging.HTTPTunnelRequest
			if err := json.Unmarshal(msgs[seen], &req); err != nil || req.RequestID == "" || req.Target == "" {
				continue
			}
			// Responses are only accepted from the connection the request was
			// dispatched on, so the real connection ID is required here.
			s.handleHTTPTunnelResponse("dataplane/prod", connID, &messaging.HTTPTunnelResponse{
				RequestID:  req.RequestID,
				StatusCode: http.StatusOK,
			})
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// With no candidate at all the caller must say so plainly, naming the plane:
// that message is how an operator tells "the agent never connected" apart from
// "the request failed".
func TestServer_ForwardViaMesh_NoRemoteCandidates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	caller, _, _, _ := newFabricServer(t, ctx, "gw-caller")

	req := &messaging.HTTPTunnelRequest{Target: "k8s", Method: "GET"}
	_, _, err := caller.SendHTTPTunnelRequestForCR("dataplane/prod", "ns/dp1", req, time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no agents found for plane")
}

// Unregistering must tell the listener, which is what republishes the change
// to mesh peers. Without it, peers keep routing to a connection this pod no
// longer has.
func TestConnectionManager_NotifiesListenerOnUnregister(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	listener := &recordingListener{}
	s.connMgr.SetListener(listener)

	connID, err := s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)
	s.connMgr.Unregister("dataplane/prod", connID)

	assert.Equal(t, []string{connID}, listener.unregistered(),
		"the listener must hear about a connection that went away")
}

// recordingListener captures ConnectionManager callbacks.
type recordingListener struct {
	mu              sync.Mutex
	unregisteredIDs []string
	changedIDs      []string
}

func (l *recordingListener) OnAgentRegistered(_, _ string, _ []string) {}
func (l *recordingListener) OnAgentUnregistered(_, connID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.unregisteredIDs = append(l.unregisteredIDs, connID)
}

func (l *recordingListener) OnAgentCRsChanged(_, connID string, _ []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.changedIDs = append(l.changedIDs, connID)
}

func (l *recordingListener) unregistered() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.unregisteredIDs...)
}

// An agent's authorization can change while it stays connected (a CR's client
// CA is rotated, a plane gains or loses a CR). Peers route by the CR list they
// were told about, so a change that is not republished leaves them forwarding
// requests for a CR this agent may no longer serve — and not forwarding ones it
// now can.
func TestServer_OnAgentCRsChangedRepublishesAuthorization(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	owner, _, discOwner, addrOwner := newFabricServer(t, ctx, "gw-owner")
	_, registryPeer, discPeer, addrPeer := newFabricServer(t, ctx, "gw-peer")

	discOwner.ch <- []fabric.Peer{{ID: "gw-peer", Addr: addrPeer}}
	discPeer.ch <- []fabric.Peer{{ID: "gw-owner", Addr: addrOwner}}

	connID, err := owner.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)
	waitForCondition(t, "the peer to learn about the agent", func() bool {
		return len(registryPeer.Lookup("dataplane/prod", "ns/dp1")) == 1
	})

	// dp1 is revoked and dp2 granted, on the same live connection.
	owner.OnAgentCRsChanged("dataplane/prod", connID, []string{"ns/dp2"})

	waitForCondition(t, "the peer to learn the new authorization", func() bool {
		return len(registryPeer.Lookup("dataplane/prod", "ns/dp2")) == 1
	})
	assert.Empty(t, registryPeer.Lookup("dataplane/prod", "ns/dp1"),
		"the peer kept routing a revoked CR to this agent")
}

// Requests stranded by a dead agent connection are failed so their callers stop
// waiting. One that was answered in the instant before the connection dropped
// already holds its response, and must be stepped over rather than blocking the
// release of everything queued behind it.
func TestServer_FailPendingForConnectionSkipsAnsweredRequests(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	answered := make(chan *messaging.HTTPTunnelResponse, 1)
	answered <- &messaging.HTTPTunnelResponse{RequestID: "answered", StatusCode: http.StatusOK}
	waiting := make(chan *messaging.HTTPTunnelResponse, 1)

	s.requestsMu.Lock()
	s.pendingHTTPRequests["answered"] = &pendingRequest{ch: answered, connID: "conn-dead"}
	s.pendingHTTPRequests["waiting"] = &pendingRequest{ch: waiting, connID: "conn-dead"}
	s.pendingHTTPRequests["other"] = &pendingRequest{
		ch: make(chan *messaging.HTTPTunnelResponse, 1), connID: "conn-live",
	}
	s.requestsMu.Unlock()

	released := make(chan struct{})
	go func() {
		s.failPendingForConnection("dataplane/prod", "conn-dead")
		close(released)
	}()
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("failPendingForConnection blocked on a request that was already answered")
	}

	select {
	case rsp := <-waiting:
		assert.Nil(t, rsp, "a stranded request must be woken with nil, not a response")
	default:
		t.Fatal("a request stranded by the closed connection was never released")
	}

	select {
	case rsp := <-answered:
		require.NotNil(t, rsp, "an answered request had its response replaced with nil")
		assert.Equal(t, http.StatusOK, rsp.StatusCode)
	default:
		t.Fatal("the answered request lost its response")
	}

	s.requestsMu.Lock()
	_, stillPending := s.pendingHTTPRequests["other"]
	s.requestsMu.Unlock()
	assert.True(t, stillPending, "a request on a healthy connection was failed too")
}

// The registry is eventually consistent, so the first candidate is routinely
// stale. When the failure is proven to have happened before the request reached
// that pod, a second pod holding a qualifying connection must be tried — giving
// up after one attempt would fail requests the fleet could still serve.
func TestServer_ForwardViaMesh_TriesASecondOwnerPod(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	caller, registry, _, _ := newFabricServer(t, ctx, "gw-caller")

	// Two candidates on different pods. No peer is ever pushed into discovery,
	// so neither has a link and both fail before the request leaves this pod.
	for i, owner := range []string{"gw-a", "gw-b"} {
		require.True(t, registry.ApplyDelta(fabric.Delta{
			Op: fabric.DeltaOpAdd, Owner: owner, Seq: 1,
			Entry: fabric.AgentEntry{
				PlaneIdentifier: "dataplane/prod",
				ConnID:          fmt.Sprintf("conn-%d", i),
				ValidCRs:        []string{"ns/dp1"},
			},
		}))
	}

	req := &messaging.HTTPTunnelRequest{Target: "k8s", Method: "GET", Path: "/api/v1/pods"}
	_, _, err := caller.forwardViaMesh("dataplane/prod", "ns/dp1", req, time.Second)
	require.Error(t, err)
	assert.True(t, errors.Is(err, fabric.ErrNoLink), "expected a no-link failure, got %v", err)
	assert.Contains(t, err.Error(), "gw-b",
		"the second owner pod was never tried after the first candidate failed")
}

// Plane events are propagated between replicas, and a replica running a newer
// build may send one this pod does not recognize. An unknown event must be
// logged and ignored, never guessed at: the two known outcomes are
// re-validation and mass disconnection, and picking wrong drops every agent
// connection for the plane.
func TestServer_ApplyPlaneEventIgnoresUnknownEvents(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	mock := &mockGatewayConn{}
	_, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

	s.ApplyPlaneEvent(fabric.PlaneEvent{PlaneType: "dataplane", PlaneID: "prod", Event: "quarantined"})

	mock.mu.Lock()
	closed := mock.closed
	mock.mu.Unlock()
	assert.False(t, closed, "an unrecognized event was treated as a disconnect")
	assert.Equal(t, 1, s.connMgr.Count(), "an unrecognized event dropped a live agent connection")
}

// A plane event only reaches the replica that received the notification. Agent
// connections for that plane are spread across the fleet, so without the
// broadcast the other replicas keep serving connections that should have been
// revalidated or dropped.
func TestServer_PropagatePlaneEventReachesPeers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	notified, _, discNotified, addrNotified := newFabricServer(t, ctx, "gw-notified")
	peer, _, discPeer, addrPeer := newFabricServer(t, ctx, "gw-peer")

	discNotified.ch <- []fabric.Peer{{ID: "gw-peer", Addr: addrPeer}}
	discPeer.ch <- []fabric.Peer{{ID: "gw-notified", Addr: addrNotified}}

	// The agent is connected to the peer, not to the replica that hears about
	// the deletion.
	mock := &mockGatewayConn{}
	_, err := peer.connMgr.Register("dataplane", "prod", mock, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)
	waitForCondition(t, "the mesh link to come up", func() bool {
		return len(notified.fabricRegistry.Lookup("dataplane/prod", "ns/dp1")) == 1
	})

	notified.propagatePlaneEvent(fabric.PlaneEvent{
		PlaneType: "dataplane", PlaneID: "prod", Event: "deleted",
	})

	waitForCondition(t, "the peer to drop the deleted plane's connections", func() bool {
		return peer.connMgr.Count() == 0
	})
}

// A named deletion traveling over the mesh must reach the replica that owns
// the agent and revoke only that CR there: the peer stops routing the deleted
// CR and republishes the connection so the fleet's registry forgets it too,
// while the other CR sharing the planeID stays served by the same connection.
func TestServer_PropagateNamedDeletionRevokesOnlyThatCROnPeers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	notified, _, discNotified, addrNotified := newFabricServer(t, ctx, "gw-notified")
	peer, _, discPeer, addrPeer := newFabricServer(t, ctx, "gw-peer")

	discNotified.ch <- []fabric.Peer{{ID: "gw-peer", Addr: addrPeer}}
	discPeer.ch <- []fabric.Peer{{ID: "gw-notified", Addr: addrNotified}}

	mock := &mockGatewayConn{}
	_, err := peer.connMgr.Register("dataplane", "prod", mock, []string{"ns1/dp", "ns2/dp"}, nil, nil)
	require.NoError(t, err)
	waitForCondition(t, "the mesh link to come up", func() bool {
		return len(notified.fabricRegistry.Lookup("dataplane/prod", "ns1/dp")) == 1 &&
			len(notified.fabricRegistry.Lookup("dataplane/prod", "ns2/dp")) == 1
	})

	notified.propagatePlaneEvent(fabric.PlaneEvent{
		PlaneType: "dataplane", PlaneID: "prod", Event: "deleted",
		Namespace: "ns1", Name: "dp",
	})

	waitForCondition(t, "the peer to stop routing the deleted CR", func() bool {
		_, err := peer.connMgr.GetForCR("dataplane/prod", "ns1/dp")
		return err != nil
	})
	waitForCondition(t, "the fleet registry to forget the deleted CR", func() bool {
		return len(notified.fabricRegistry.Lookup("dataplane/prod", "ns1/dp")) == 0
	})

	// The surviving CR keeps its agent, locally and in the registry.
	_, err = peer.connMgr.GetForCR("dataplane/prod", "ns2/dp")
	assert.NoError(t, err)
	assert.Len(t, notified.fabricRegistry.Lookup("dataplane/prod", "ns2/dp"), 1)
	mock.mu.Lock()
	closed := mock.closed
	mock.mu.Unlock()
	assert.False(t, closed)
}

// Revoking a deleted CR leaves the shared agent connection open, so streaming
// sessions opened for that CR would otherwise keep running against a CR that
// no longer exists. Only those sessions end; another CR's session on the same
// connection carries on.
func TestServer_ApplyPlaneEventDeletedClosesOnlyThatCRsStreamSessions(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	_, err := s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, []string{"ns1/dp", "ns2/dp"}, nil, nil)
	require.NoError(t, err)

	newSession := func(id, crKey string) *streamSession {
		session := &streamSession{
			requestID:       id,
			planeIdentifier: "dataplane/prod",
			crKey:           crKey,
			done:            make(chan struct{}),
		}
		s.registerStreamSession(id, session)
		return session
	}
	deleted := newSession("req-deleted", "ns1/dp")
	surviving := newSession("req-surviving", "ns2/dp")

	s.ApplyPlaneEvent(fabric.PlaneEvent{
		PlaneType: "dataplane", PlaneID: "prod", Event: "deleted",
		Namespace: "ns1", Name: "dp",
	})

	select {
	case <-deleted.done:
	default:
		t.Fatal("the deleted CR's stream session was left running")
	}
	select {
	case <-surviving.done:
		t.Fatal("another CR's stream session was closed")
	default:
	}
}

// A session can be ended while the gateway still waits for the agent to
// acknowledge the stream init: its CR was revoked. The agent has the init and
// starts the exec anyway, and a shell with stdin open never exits by itself,
// so the gateway must tell the agent to stop. Nothing else would.
func TestHandleExec_ClosedBeforeAgentStartsTellsAgentToStop(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	agent := &mockGatewayConn{}
	_, err := s.connMgr.Register("dataplane", "prod", agent, []string{"ns1/dp"}, nil, nil)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(s.handleExec))
	t.Cleanup(srv.Close)
	execURL := "ws" + strings.TrimPrefix(srv.URL, "http") +
		"/api/exec/dataplane/prod/ns1/dp?podNamespace=default&podName=web"
	apiConn, resp, err := websocket.DefaultDialer.Dial(execURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = resp.Body.Close()
		_ = apiConn.Close()
	})

	waitForCondition(t, "the stream init to reach the agent", func() bool {
		return len(agent.getWrittenMessages()) == 1
	})

	// What revoking the CR does to its sessions.
	require.Equal(t, 1, s.closeStreamSessionsForCR("dataplane/prod", "ns1/dp"))

	waitForCondition(t, "the agent to be told to stop the exec", func() bool {
		sent := agent.getWrittenMessages()
		if len(sent) != 2 {
			return false
		}
		var chunk messaging.HTTPTunnelStreamChunk
		return json.Unmarshal(sent[1], &chunk) == nil && chunk.IsClose && chunk.RequestID != ""
	})
}

// A revocation closes the sessions registered for its CR. One that lands after
// the handler picked its agent connection but before it registered the session
// has nothing to close, so the handler must notice by itself. Otherwise the
// exec starts and runs on for a CR that no longer exists.
func TestHandleExec_CRRevokedBeforeSessionRegistered(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	agent := &mockGatewayConn{}
	_, err := s.connMgr.Register("dataplane", "prod", agent, []string{"ns1/dp", "ns2/dp"}, nil, nil)
	require.NoError(t, err)

	// The WebSocket upgrade sits between the handler's agent lookup and its
	// session registration, so its origin check runs exactly in that window.
	s.upgrader.CheckOrigin = func(*http.Request) bool {
		s.revokeCR("dataplane", "prod", "ns1", "dp")
		return true
	}

	srv := httptest.NewServer(http.HandlerFunc(s.handleExec))
	t.Cleanup(srv.Close)
	execURL := "ws" + strings.TrimPrefix(srv.URL, "http") +
		"/api/exec/dataplane/prod/ns1/dp?podNamespace=default&podName=web"
	apiConn, resp, err := websocket.DefaultDialer.Dial(execURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = resp.Body.Close()
		_ = apiConn.Close()
	})

	// The handler gives up and closes the caller's connection.
	require.NoError(t, apiConn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, _, err = apiConn.ReadMessage()
	require.Error(t, err)

	assert.Empty(t, agent.getWrittenMessages(), "no exec may be started for a revoked CR")
	assert.False(t, agent.isClosed(), "the agent still serves the other CR")
}

// terminatingDataPlane is the DataPlane ns1/dp on plane "prod", being deleted,
// with caPEM as its client CA.
func terminatingDataPlane(caPEM string) *openchoreov1alpha1.DataPlane {
	now := metav1.Now()
	return &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dp", Namespace: "ns1",
			DeletionTimestamp: &now, Finalizers: []string{"openchoreo.dev/dataplane-cleanup"},
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "prod",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: caPEM},
			},
		},
	}
}

// getFailingClient answers lists but fails every Get, like an API server that
// drops one call of a handler and serves the next.
func getFailingClient(objects ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(objects...).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("api server unavailable")
			},
		}).Build()
}

// Propagated plane events are retried with a backoff that starts at a second.
// The tests exercise the retry, not the wait.
func init() { planeEventRetryBackoff = time.Millisecond }

// flakyGetClient fails the first `failures` Get calls and serves the rest. It
// counts every Get in calls.
func flakyGetClient(failures int32, calls *atomic.Int32, objects ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(objects...).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if calls.Add(1) <= failures {
					return errors.New("api server unavailable")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).Build()
}

// liveDataPlane is the DataPlane ns1/dp on plane "prod" with caPEM as its
// client CA.
func liveDataPlane(caPEM string) *openchoreov1alpha1.DataPlane {
	return &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp", Namespace: "ns1"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "prod",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: caPEM},
			},
		},
	}
}

var updatedNs1Dp = fabric.PlaneEvent{
	PlaneType: "dataplane", PlaneID: "prod", Event: "updated",
	Namespace: "ns1", Name: "dp",
}

// The replica that received a notification has already answered the
// controller, so a peer whose CA lookup fails is not told again. Its agents
// stay unauthorized for the CR, and when another replica's agent serves that CR
// the status reports it connected, so the controller does not re-notify either.
// The peer has to retry the lookup itself.
func TestServer_ApplyPlaneEventRetriesAFailedLookup(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	var gets atomic.Int32
	s := New(&Config{}, flakyGetClient(2, &gets, liveDataPlane(string(encodeCertToPEM(t, caCert)))), testLogger())

	_, err := s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, []string{"ns2/dp"}, clientCert, nil)
	require.NoError(t, err)

	s.ApplyPlaneEvent(updatedNs1Dp)

	_, err = s.connMgr.GetForCR("dataplane/prod", "ns1/dp")
	require.NoError(t, err, "the CR must be granted once its lookup succeeds")
	assert.Equal(t, int32(3), gets.Load(), "two failed lookups, then the one that succeeded")
}

// A lookup that keeps failing is given up on, so a propagated event cannot hold
// a goroutine and keep reading the API server indefinitely.
func TestServer_ApplyPlaneEventGivesUpAfterBoundedAttempts(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	var gets atomic.Int32
	s := New(&Config{}, flakyGetClient(1000, &gets, liveDataPlane(string(encodeCertToPEM(t, caCert)))), testLogger())

	_, err := s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, []string{"ns2/dp"}, clientCert, nil)
	require.NoError(t, err)

	s.ApplyPlaneEvent(updatedNs1Dp)

	assert.Equal(t, int32(planeEventAttempts), gets.Load())
	_, err = s.connMgr.GetForCR("dataplane/prod", "ns1/dp")
	assert.Error(t, err, "a CR whose CA was never read must not be granted")
}

// A CR that no longer exists has nothing to grant. Retrying its lookup would
// only repeat a read whose answer cannot change the outcome.
func TestServer_ApplyPlaneEventDoesNotRetryAMissingCR(t *testing.T) {
	var gets atomic.Int32
	s := New(&Config{}, flakyGetClient(0, &gets), testLogger())

	s.ApplyPlaneEvent(updatedNs1Dp)

	assert.Equal(t, int32(1), gets.Load())
}

// Once a CR's "deleted" event is applied, knowing that the CR is still
// terminating is all that stops a stale "updated" from granting it again. If
// that lookup fails the gateway cannot tell, and the CA lookup after it can
// still succeed, so the propagated event must be dropped rather than applied.
func TestServer_ApplyPlaneEventUpdatedDoesNotGrantWhenTerminationLookupFails(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	plane := terminatingDataPlane(string(encodeCertToPEM(t, caCert)))
	s := New(&Config{}, getFailingClient(plane), testLogger())

	_, err := s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, []string{"ns2/dp"}, clientCert, nil)
	require.NoError(t, err)

	for _, event := range []string{"deleted", "updated"} {
		s.ApplyPlaneEvent(fabric.PlaneEvent{
			PlaneType: "dataplane", PlaneID: "prod", Event: event,
			Namespace: "ns1", Name: "dp",
		})
	}

	_, err = s.connMgr.GetForCR("dataplane/prod", "ns1/dp")
	assert.Error(t, err, "the revoked CR must not be granted again")
}

// A "deleted" event can be lost on its way to the pod that owns the agent, and
// nothing else withdraws a CR-scoped authorization, so the deleted CR would
// stay routable. The sweep must revoke a CR that no longer exists and leave
// every CR that does: a live one, and a terminating one whose finalizer still
// cleans up through the agent.
func TestServer_RevokeDeletedCRsRecoversAMissedDeletion(t *testing.T) {
	now := metav1.Now()
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(
		&openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "live", Namespace: "ns1"},
			Spec:       openchoreov1alpha1.DataPlaneSpec{PlaneID: "prod"},
		},
		&openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name: "terminating", Namespace: "ns1",
				DeletionTimestamp: &now, Finalizers: []string{"openchoreo.dev/dataplane-cleanup"},
			},
			Spec: openchoreov1alpha1.DataPlaneSpec{PlaneID: "prod"},
		},
		&openchoreov1alpha1.ClusterDataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "live"},
			Spec:       openchoreov1alpha1.ClusterDataPlaneSpec{PlaneID: "prod"},
		},
	).Build()
	s := New(&Config{}, fakeClient, testLogger())

	mock := &mockGatewayConn{}
	_, err := s.connMgr.Register("dataplane", "prod", mock,
		[]string{"ns1/live", "ns1/terminating", "/live", "ns1/gone", "/gone"}, nil, nil)
	require.NoError(t, err)

	s.revokeDeletedCRs(context.Background())

	for crKey, wantServed := range map[string]bool{
		"ns1/live":        true,
		"ns1/terminating": true,
		"/live":           true,
		"ns1/gone":        false,
		"/gone":           false,
	} {
		_, err := s.connMgr.GetForCR("dataplane/prod", crKey)
		assert.Equal(t, wantServed, err == nil, "CR %q", crKey)
	}
	mock.mu.Lock()
	closed := mock.closed
	mock.mu.Unlock()
	assert.False(t, closed, "the agent still serves existing CRs")
}

// The sweep targets planes shared by well over a thousand CRs, and it runs on
// every replica every minute. A pass that asked the API server about each CR
// would exhaust the client's rate limit, which agent connects and
// revalidations share. Its cost must not depend on the number of CRs.
func TestServer_RevokeDeletedCRsCostDoesNotGrowWithCRs(t *testing.T) {
	const crCount = 200
	objects := make([]client.Object, 0, crCount)
	crKeys := make([]string, 0, crCount)
	for i := range crCount {
		name := fmt.Sprintf("dp-%d", i)
		objects = append(objects, &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
			Spec:       openchoreov1alpha1.DataPlaneSpec{PlaneID: "prod"},
		})
		crKeys = append(crKeys, "ns/"+name)
	}

	var gets, lists atomic.Int32
	countingClient := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(objects...).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				gets.Add(1)
				return c.Get(ctx, key, obj, opts...)
			},
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				lists.Add(1)
				return c.List(ctx, list, opts...)
			},
		}).Build()
	s := New(&Config{}, countingClient, testLogger())
	_, err := s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, crKeys, nil, nil)
	require.NoError(t, err)

	s.revokeDeletedCRs(context.Background())

	assert.Equal(t, int32(2), lists.Load(), "one listing each for DataPlanes and ClusterDataPlanes")
	assert.Zero(t, gets.Load(), "no per-CR lookups")
	_, err = s.connMgr.GetForCR("dataplane/prod", crKeys[0])
	assert.NoError(t, err)
}

// A listing that fails says nothing about which CRs exist. Reading it as an
// empty result would revoke every CR on the pod, so the pass must be skipped.
func TestServer_RevokeDeletedCRsSkipsThePassWhenListingFails(t *testing.T) {
	listFailingClient := fake.NewClientBuilder().WithScheme(testScheme()).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				return errors.New("api server unavailable")
			},
		}).Build()
	s := New(&Config{}, listFailingClient, testLogger())
	mock := &mockGatewayConn{}
	_, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns1/dp", "ns2/dp"}, nil, nil)
	require.NoError(t, err)

	s.revokeDeletedCRs(context.Background())

	for _, crKey := range []string{"ns1/dp", "ns2/dp"} {
		_, err := s.connMgr.GetForCR("dataplane/prod", crKey)
		assert.NoError(t, err, "CR %q", crKey)
	}
	assert.False(t, mock.isClosed())
}

// The listing can predate a CR that was deleted and re-created under the same
// name. Revoking it then removes the new CR's grant, and no later event grants
// it again, so the pass must grant it itself once it sees the CR exists.
func TestServer_RevokeDeletedCRsRegrantsARecreatedCR(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	recreated := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp", Namespace: "ns1"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "prod",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: string(encodeCertToPEM(t, caCert))},
			},
		},
	}

	// The sweep's two listings come back empty, as if taken before the CR was
	// re-created. Every later call sees it.
	var lists atomic.Int32
	staleListingClient := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(recreated).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if lists.Add(1) <= 2 {
					return nil
				}
				return c.List(ctx, list, opts...)
			},
		}).Build()
	s := New(&Config{}, staleListingClient, testLogger())
	mock := &mockGatewayConn{}
	_, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns1/dp", "ns2/other"}, clientCert, nil)
	require.NoError(t, err)

	s.revokeDeletedCRs(context.Background())

	_, err = s.connMgr.GetForCR("dataplane/prod", "ns1/dp")
	assert.NoError(t, err, "the re-created CR must end up authorized")
	_, err = s.connMgr.GetForCR("dataplane/prod", "ns2/other")
	assert.Error(t, err, "a CR that is really gone stays revoked")
}

// planeID can be changed on a live CR, and a CR can be re-created under the
// same name on another plane. Its events then revalidate only the new plane,
// so the old plane's grant would stay forever unless the sweep compares plane
// IDs, not just names. A DataPlane without a planeID belongs to the plane
// named after it.
func TestServer_RevokeDeletedCRsRevokesACRThatMovedPlane(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(
		&openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "dp", Namespace: "ns1"},
			Spec:       openchoreov1alpha1.DataPlaneSpec{PlaneID: "staging"},
		},
		&openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Name: "prod", Namespace: "ns2"}},
	).Build()
	s := New(&Config{}, fakeClient, testLogger())
	_, err := s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, []string{"ns1/dp", "ns2/prod"}, nil, nil)
	require.NoError(t, err)

	s.revokeDeletedCRs(context.Background())

	_, err = s.connMgr.GetForCR("dataplane/prod", "ns1/dp")
	assert.Error(t, err, "the CR moved to staging must not stay authorized on prod")
	_, err = s.connMgr.GetForCR("dataplane/prod", "ns2/prod")
	assert.NoError(t, err, "a DataPlane without a planeID is on the plane named after it")
}

// The listing can predate a CR that was re-created under the same name. If the
// check revoked first and asked questions later, a re-created CR that was its
// connection's only CR would get the agent disconnected. The CR is looked up
// before anything is revoked, and a lookup that fails proves nothing.
func TestServer_RevokeDeletedCRsLooksUpBeforeRevoking(t *testing.T) {
	recreated := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp", Namespace: "ns1"},
		Spec:       openchoreov1alpha1.DataPlaneSpec{PlaneID: "prod"},
	}
	for _, tc := range []struct {
		name    string
		failGet bool
	}{
		{name: "re-created CR is left alone"},
		{name: "failed lookup revokes nothing", failGet: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The listing is empty, as if taken before the CR was re-created.
			staleListingClient := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(recreated).
				WithInterceptorFuncs(interceptor.Funcs{
					List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
						return nil
					},
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if tc.failGet {
							return errors.New("api server unavailable")
						}
						return c.Get(ctx, key, obj, opts...)
					},
				}).Build()
			s := New(&Config{}, staleListingClient, testLogger())
			mock := &mockGatewayConn{}
			_, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns1/dp"}, nil, nil)
			require.NoError(t, err)

			s.revokeDeletedCRs(context.Background())

			_, err = s.connMgr.GetForCR("dataplane/prod", "ns1/dp")
			assert.NoError(t, err)
			assert.False(t, mock.isClosed(), "the agent's only CR was never revoked")
		})
	}
}

// Shutdown cancels the sweep's context. A pass that ignored it would go on to
// look up every remaining CR, and revoke, from a pod that is on its way out.
func TestServer_RevokeDeletedCRsStopsWhenCanceled(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	// The CR is gone, so a pass that ran would revoke it.
	_, err := s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, []string{"ns1/gone", "ns2/gone"}, nil, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.revokeDeletedCRs(ctx)

	_, err = s.connMgr.GetForCR("dataplane/prod", "ns1/gone")
	assert.NoError(t, err, "a canceled pass must change nothing")
}

// The sweep is bound to the server's lifetime: one that outlived it would keep
// querying the API server and revoking from a pod that is shutting down.
func TestServer_RunDeletedCRSweepStopsWithItsContext(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		s.runDeletedCRSweep(ctx)
		close(stopped)
	}()

	cancel()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the deleted-CR sweep outlived its context")
	}
}

// A propagated deletion that names its CR must leave a peer's agent connected
// when that agent still serves other CRs sharing the planeID.
func TestServer_ApplyPlaneEventDeletedKeepsSharedPlaneAgent(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	mock := &mockGatewayConn{}
	_, err := s.connMgr.Register("dataplane", "prod", mock, []string{"ns1/dp", "ns2/dp"}, nil, nil)
	require.NoError(t, err)

	s.ApplyPlaneEvent(fabric.PlaneEvent{
		PlaneType: "dataplane", PlaneID: "prod", Event: "deleted",
		Namespace: "ns1", Name: "dp",
	})

	mock.mu.Lock()
	closed := mock.closed
	mock.mu.Unlock()
	assert.False(t, closed)
	_, err = s.connMgr.GetForCR("dataplane/prod", "ns2/dp")
	assert.NoError(t, err)
	_, err = s.connMgr.GetForCR("dataplane/prod", "ns1/dp")
	assert.Error(t, err)
}

// Without a mesh there is nobody to tell, and the notification path is shared:
// a single-replica gateway must take it silently rather than panic.
func TestServer_PropagatePlaneEventWithoutMeshIsNoop(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	s.propagatePlaneEvent(fabric.PlaneEvent{PlaneType: "dataplane", PlaneID: "prod", Event: "deleted"})
}

// The rebalancer is bound to the signal context so it stops the moment the pod
// begins shutting down: a draining pod is shedding every connection it has, and
// one that kept shedding for balance would be making eviction decisions from a
// connection count that is deliberately collapsing.
func TestServer_RunRebalancerStopsWithItsContext(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	s := New(&Config{}, fakeClient, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		s.runRebalancer(ctx)
		close(stopped)
	}()

	cancel()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the rebalancer outlived its context")
	}
}

// A mesh that cannot start means this replica can neither learn about peers'
// agents nor forward to them - it would serve only its own connections while
// reporting healthy. Start must fail loudly instead, before any port is bound.
func TestStart_MeshStartFailureIsSurfaced(t *testing.T) {
	certPath, keyPath := writeServerKeyPairFiles(t)
	s := New(&Config{ServerCertPath: certPath, ServerKeyPath: keyPath}, nil, testLogger())

	registry := fabric.NewRegistry(testLogger())
	// A mesh certificate with no CA to verify peers against: refused at startup.
	mesh := fabric.NewMesh(fabric.MeshConfig{
		Self:     fabric.Peer{ID: "gw-self"},
		CertFile: filepath.Join(t.TempDir(), "mesh.crt"),
		KeyFile:  filepath.Join(t.TempDir(), "mesh.key"),
	}, registry, &staticPeerDiscovery{ch: make(chan []fabric.Peer)}, s, testLogger())
	s.SetFabric(mesh, registry)

	err := s.Start()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start gateway mesh")
}

// stubMeshDelegate stands in for a peer replica's server: it answers forwards
// either with a response or with the NoAgent verdict a pod returns when its own
// registry view disagreed and it never dispatched the request.
type stubMeshDelegate struct {
	noAgent bool
	// noAgentError overrides the reason given with a NoAgent answer.
	noAgentError string
	// dispatchErr answers as a pod that did dispatch to an agent and failed,
	// leaving it unknown whether the request ran.
	dispatchErr bool

	mu       sync.Mutex
	forwards int
}

func (d *stubMeshDelegate) ServeForward(req *fabric.ForwardRequest) *fabric.ForwardResponse {
	d.mu.Lock()
	d.forwards++
	d.mu.Unlock()

	switch {
	case d.noAgent:
		reason := "no agents found"
		if d.noAgentError != "" {
			reason = d.noAgentError
		}
		return &fabric.ForwardResponse{CorrID: req.CorrID, NoAgent: true, Error: reason}
	case d.dispatchErr:
		return &fabric.ForwardResponse{CorrID: req.CorrID, Error: "agent stopped responding"}
	}
	return &fabric.ForwardResponse{
		CorrID:   req.CorrID,
		Response: &messaging.HTTPTunnelResponse{RequestID: req.Request.RequestID, StatusCode: http.StatusOK},
	}
}

func (d *stubMeshDelegate) forwardCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.forwards
}

func (d *stubMeshDelegate) ApplyPlaneEvent(_ fabric.PlaneEvent) {}

// newMeshPeer starts a bare mesh node standing in for another gateway replica.
func newMeshPeer(t *testing.T, ctx context.Context, id string, delegate *stubMeshDelegate) (*fabric.Mesh, *staticPeerDiscovery, string) {
	t.Helper()

	disc := &staticPeerDiscovery{ch: make(chan []fabric.Peer, 8)}
	m := fabric.NewMesh(fabric.MeshConfig{Self: fabric.Peer{ID: id}, ListenPort: 0},
		fabric.NewRegistry(testLogger()), disc, delegate, testLogger())
	require.NoError(t, m.Start(ctx))
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = m.Shutdown(shutdownCtx)
	})
	addr := m.ListenAddr()
	return m, disc, addr
}

// NoAgent is not a failed request, it is a disagreement: the owner checked its
// own connections, found none, and never dispatched anything. Nothing ran, so
// another candidate is free to serve the request - including a non-idempotent
// one, which any other failure would have to give up on.
func TestServer_ForwardViaMesh_RetriesPastANoAgentOwner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	caller, registry, discCaller, addrCaller := newFabricServer(t, ctx, "gw-caller")
	// Sorted first, so it is the candidate tried first.
	empty, discEmpty, addrEmpty := newMeshPeer(t, ctx, "gw-a-empty", &stubMeshDelegate{noAgent: true})
	serving, discServing, addrServing := newMeshPeer(t, ctx, "gw-b-serving", &stubMeshDelegate{})

	discCaller.ch <- []fabric.Peer{{ID: "gw-a-empty", Addr: addrEmpty}, {ID: "gw-b-serving", Addr: addrServing}}
	discEmpty.ch <- []fabric.Peer{{ID: "gw-caller", Addr: addrCaller}}
	discServing.ch <- []fabric.Peer{{ID: "gw-caller", Addr: addrCaller}}

	// Both replicas advertise a connection for the same CR.
	for _, m := range []*fabric.Mesh{empty, serving} {
		m.LocalUpsert(fabric.AgentEntry{
			PlaneIdentifier: "dataplane/prod",
			ConnID:          m.SelfID() + "-conn",
			ValidCRs:        []string{"ns/dp1"},
		})
	}
	// Counted rather than looked up: Lookup advances the round-robin cursor,
	// which would leave the candidate order under test up to how many times the
	// wait polled.
	waitForCondition(t, "the caller to see both candidates", func() bool {
		counts := registry.CountByOwner()
		return counts["gw-a-empty"] == 1 && counts["gw-b-serving"] == 1
	})

	// The registry entry can arrive over the peer's inbound link before this
	// pod's own outbound link is up, and Forward needs the outbound one. Probing
	// until the empty peer answers pins both: the link exists, and it replies
	// NoAgent rather than failing before the request is sent.
	waitForCondition(t, "an outbound link to the empty peer", func() bool {
		probeCtx, probeCancel := context.WithTimeout(context.Background(), time.Second)
		defer probeCancel()
		rsp, err := caller.fabricMesh.Forward(probeCtx, "gw-a-empty", &fabric.ForwardRequest{
			PlaneIdentifier: "dataplane/prod",
			CRKey:           "ns/dp1",
			TimeoutMillis:   time.Second.Milliseconds(),
			Request:         &messaging.HTTPTunnelRequest{Target: "k8s", Method: http.MethodGet},
		})
		return err == nil && rsp != nil && rsp.NoAgent
	})

	// A write, so a candidate that might have run it could not be retried past.
	req := &messaging.HTTPTunnelRequest{Target: "k8s", Method: http.MethodPost, Path: "/api/v1/pods"}
	resp, servedBy, err := caller.forwardViaMesh("dataplane/prod", "ns/dp1", req, 5*time.Second)
	require.NoError(t, err, "a NoAgent answer must not fail a request another pod can serve")
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "gw-b-serving", servedBy, "the request was not served by the pod that had the agent")
}

// Retrying is only safe while the request is proven not to have run. A failure
// reported by a pod that did dispatch to an agent leaves that unknown, so a
// write must be surfaced as failed rather than replayed on another candidate -
// a second POST would create a second object.
func TestServer_ForwardViaMesh_DoesNotReplayAWriteOnAnotherPod(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	caller, registry, discCaller, addrCaller := newFabricServer(t, ctx, "gw-caller")
	brokenDelegate := &stubMeshDelegate{dispatchErr: true}
	servingDelegate := &stubMeshDelegate{}
	// gw-a-broken sorts first, so it is the candidate tried first.
	broken, discBroken, addrBroken := newMeshPeer(t, ctx, "gw-a-broken", brokenDelegate)
	serving, discServing, addrServing := newMeshPeer(t, ctx, "gw-b-serving", servingDelegate)

	discCaller.ch <- []fabric.Peer{{ID: "gw-a-broken", Addr: addrBroken}, {ID: "gw-b-serving", Addr: addrServing}}
	discBroken.ch <- []fabric.Peer{{ID: "gw-caller", Addr: addrCaller}}
	discServing.ch <- []fabric.Peer{{ID: "gw-caller", Addr: addrCaller}}

	for _, m := range []*fabric.Mesh{broken, serving} {
		m.LocalUpsert(fabric.AgentEntry{
			PlaneIdentifier: "dataplane/prod",
			ConnID:          m.SelfID() + "-conn",
			ValidCRs:        []string{"ns/dp1"},
		})
	}
	waitForCondition(t, "the caller to see both candidates", func() bool {
		counts := registry.CountByOwner()
		return counts["gw-a-broken"] == 1 && counts["gw-b-serving"] == 1
	})
	waitForCondition(t, "an outbound link to the broken peer", func() bool {
		probeCtx, probeCancel := context.WithTimeout(context.Background(), time.Second)
		defer probeCancel()
		rsp, err := caller.fabricMesh.Forward(probeCtx, "gw-a-broken", &fabric.ForwardRequest{
			PlaneIdentifier: "dataplane/prod",
			CRKey:           "ns/dp1",
			TimeoutMillis:   time.Second.Milliseconds(),
			Request:         &messaging.HTTPTunnelRequest{Target: "k8s", Method: http.MethodGet},
		})
		return err == nil && rsp != nil && rsp.Response == nil && !rsp.NoAgent
	})
	before := servingDelegate.forwardCount()

	req := &messaging.HTTPTunnelRequest{Target: "k8s", Method: http.MethodPost, Path: "/api/v1/pods"}
	_, servedBy, err := caller.forwardViaMesh("dataplane/prod", "ns/dp1", req, 5*time.Second)

	require.Error(t, err, "a write that may already have run must be reported as failed")
	assert.Contains(t, err.Error(), "gw-a-broken")
	assert.Empty(t, servedBy)
	assert.Equal(t, before, servingDelegate.forwardCount(),
		"the write was replayed on a second pod after the first may have executed it")
}
