// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

// mockRoundTripper implements http.RoundTripper for testing.
type mockRoundTripper struct {
	handler func(*http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.handler(req)
}

// newMockRoute creates a Route with a mock transport for testing.
func newMockRoute(name, endpoint string, handler func(*http.Request) (*http.Response, error)) *Route {
	return &Route{
		Name:      name,
		Backend:   "http",
		Endpoint:  endpoint,
		Transport: &mockRoundTripper{handler: handler},
	}
}

// newTestRouter creates a Router with pre-configured routes, bypassing NewRouter's k8s dependency.
func newTestRouter(t *testing.T, routes map[string]*Route) *Router {
	t.Helper()
	return &Router{
		routes: routes,
		logger: testLogger(),
	}
}

func TestRoute_Success(t *testing.T) {
	route := newMockRoute("k8s", "https://kubernetes.svc", func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"items":[]}`)),
		}, nil
	})

	router := newTestRouter(t, map[string]*Route{"k8s": route})

	req := &messaging.HTTPTunnelRequest{
		RequestID: "req-1",
		Target:    "k8s",
		Method:    "GET",
		Path:      "/api/v1/pods",
	}

	resp := router.Route(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "req-1", resp.RequestID)
	assert.Equal(t, `{"items":[]}`, string(resp.Body))
	assert.Nil(t, resp.Error)
}

func TestRoute_UnknownTarget(t *testing.T) {
	router := newTestRouter(t, map[string]*Route{
		"k8s": newMockRoute("k8s", "https://k8s.svc", nil),
	})

	req := &messaging.HTTPTunnelRequest{
		RequestID: "req-2",
		Target:    "unknown",
		Method:    "GET",
		Path:      "/api/v1/pods",
	}

	resp := router.Route(req)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NotNil(t, resp.Error)
	assert.Contains(t, resp.Error.Message, "unknown target")
}

func TestRoute_BackendError(t *testing.T) {
	route := newMockRoute("k8s", "https://kubernetes.svc", func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("connection refused")
	})

	router := newTestRouter(t, map[string]*Route{"k8s": route})

	req := &messaging.HTTPTunnelRequest{
		RequestID: "req-3",
		Target:    "k8s",
		Method:    "GET",
		Path:      "/api/v1/pods",
	}

	resp := router.Route(req)

	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	require.NotNil(t, resp.Error)
	assert.Contains(t, resp.Error.Message, "backend request failed")
}

func TestRoute_WithQueryParams(t *testing.T) {
	var capturedURL string
	route := newMockRoute("k8s", "https://kubernetes.svc", func(req *http.Request) (*http.Response, error) {
		capturedURL = req.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	router := newTestRouter(t, map[string]*Route{"k8s": route})

	req := &messaging.HTTPTunnelRequest{
		RequestID: "req-4",
		Target:    "k8s",
		Method:    "GET",
		Path:      "/api/v1/pods",
		Query:     "namespace=default&limit=10",
	}

	resp := router.Route(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, capturedURL, "namespace=default&limit=10")
}

func TestRoute_WithRequestBody(t *testing.T) {
	var capturedBody []byte
	route := newMockRoute("k8s", "https://kubernetes.svc", func(req *http.Request) (*http.Response, error) {
		var err error
		capturedBody, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusCreated,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	router := newTestRouter(t, map[string]*Route{"k8s": route})

	body := []byte(`{"name":"test-pod"}`)
	req := &messaging.HTTPTunnelRequest{
		RequestID: "req-5",
		Target:    "k8s",
		Method:    "POST",
		Path:      "/api/v1/namespaces/default/pods",
		Body:      body,
	}

	resp := router.Route(req)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, body, capturedBody)
}

func TestRoute_WithHeaders(t *testing.T) {
	var capturedHeaders http.Header
	route := newMockRoute("k8s", "https://kubernetes.svc", func(req *http.Request) (*http.Response, error) {
		capturedHeaders = req.Header
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	router := newTestRouter(t, map[string]*Route{"k8s": route})

	headers := map[string][]string{
		"Content-Type": {"application/json"},
		"X-Custom":     {"custom-value"},
	}
	req := &messaging.HTTPTunnelRequest{
		RequestID: "req-6",
		Target:    "k8s",
		Method:    "GET",
		Path:      "/api/v1/pods",
		Headers:   headers,
	}

	_ = router.Route(req)

	assert.Equal(t, "application/json", capturedHeaders.Get("Content-Type"))
	assert.Equal(t, "custom-value", capturedHeaders.Get("X-Custom"))
}

func TestRoute_ResponseHeaders(t *testing.T) {
	route := newMockRoute("k8s", "https://kubernetes.svc", func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":    {"application/json"},
				"X-Response-Info": {"test-value"},
			},
			Body: io.NopCloser(strings.NewReader("")),
		}, nil
	})

	router := newTestRouter(t, map[string]*Route{"k8s": route})

	req := &messaging.HTTPTunnelRequest{
		RequestID: "req-7",
		Target:    "k8s",
		Method:    "GET",
		Path:      "/api/v1/pods",
	}

	resp := router.Route(req)

	assert.Equal(t, []string{"application/json"}, resp.Headers["Content-Type"])
	assert.Equal(t, []string{"test-value"}, resp.Headers["X-Response-Info"])
}

func TestRoute_MultipleRoutes(t *testing.T) {
	k8sRoute := newMockRoute("k8s", "https://kubernetes.svc", func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("k8s-response")),
		}, nil
	})

	monitoringRoute := newMockRoute("monitoring", "https://prometheus.svc", func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("monitoring-response")),
		}, nil
	})

	router := newTestRouter(t, map[string]*Route{
		"k8s":        k8sRoute,
		"monitoring": monitoringRoute,
	})

	// Route to monitoring
	req := &messaging.HTTPTunnelRequest{
		RequestID: "req-8",
		Target:    "monitoring",
		Method:    "GET",
		Path:      "/api/v1/query",
	}

	resp := router.Route(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "monitoring-response", string(resp.Body))
}

func TestRoute_WithGatewayRequestID(t *testing.T) {
	route := newMockRoute("k8s", "https://kubernetes.svc", func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	router := newTestRouter(t, map[string]*Route{"k8s": route})

	req := &messaging.HTTPTunnelRequest{
		RequestID:        "req-9",
		GatewayRequestID: "gw-123",
		Target:           "k8s",
		Method:           "GET",
		Path:             "/api/v1/pods",
	}

	resp := router.Route(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestApplyAuth_Bearer(t *testing.T) {
	route := &Route{
		Auth: AuthConfig{Type: "bearer", Token: "my-token"},
	}

	req, _ := http.NewRequest("GET", "https://example.com", nil)
	route.applyAuth(req)

	assert.Equal(t, "Bearer my-token", req.Header.Get("Authorization"))
}

func TestApplyAuth_ServiceAccount(t *testing.T) {
	route := &Route{
		Auth: AuthConfig{Type: "serviceaccount", Token: "sa-token"},
	}

	req, _ := http.NewRequest("GET", "https://example.com", nil)
	route.applyAuth(req)

	assert.Equal(t, "Bearer sa-token", req.Header.Get("Authorization"))
}

func TestApplyAuth_Basic(t *testing.T) {
	route := &Route{
		Auth: AuthConfig{Type: "basic", Username: "admin", Password: "secret"},
	}

	req, _ := http.NewRequest("GET", "https://example.com", nil)
	route.applyAuth(req)

	username, password, ok := req.BasicAuth()
	require.True(t, ok)
	assert.Equal(t, "admin", username)
	assert.Equal(t, "secret", password)
}

func TestApplyAuth_None(t *testing.T) {
	route := &Route{
		Auth: AuthConfig{Type: "none"},
	}

	req, _ := http.NewRequest("GET", "https://example.com", nil)
	route.applyAuth(req)

	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestApplyAuth_EmptyCredentials(t *testing.T) {
	t.Run("bearer with empty token", func(t *testing.T) {
		route := &Route{
			Auth: AuthConfig{Type: "bearer", Token: ""},
		}
		req, _ := http.NewRequest("GET", "https://example.com", nil)
		route.applyAuth(req)
		assert.Empty(t, req.Header.Get("Authorization"))
	})

	t.Run("basic with empty username", func(t *testing.T) {
		route := &Route{
			Auth: AuthConfig{Type: "basic", Username: "", Password: "secret"},
		}
		req, _ := http.NewRequest("GET", "https://example.com", nil)
		route.applyAuth(req)
		_, _, ok := req.BasicAuth()
		assert.False(t, ok)
	})
}

func TestCreateRoute(t *testing.T) {
	cfg := RouteConfig{
		Name:     "monitoring",
		Endpoint: "https://prometheus.svc:9090",
		Auth:     AuthConfig{Type: "bearer", Token: "prom-token"},
	}

	route := createRoute(cfg)

	assert.Equal(t, "monitoring", route.Name)
	assert.Equal(t, "http", route.Backend)
	assert.Equal(t, "https://prometheus.svc:9090", route.Endpoint)
	assert.Equal(t, "bearer", route.Auth.Type)
	assert.Equal(t, "prom-token", route.Auth.Token)
	assert.NotNil(t, route.Transport)
}

func TestCreateRoute_InsecureSkipVerify_DefaultFalse(t *testing.T) {
	cfg := RouteConfig{
		Name:     "internal",
		Endpoint: "https://internal.svc",
		Auth:     AuthConfig{Type: "none"},
	}

	route := createRoute(cfg)

	transport, ok := route.Transport.(*http.Transport)
	require.True(t, ok)
	assert.False(t, transport.TLSClientConfig.InsecureSkipVerify)
}

func TestCreateRoute_InsecureSkipVerify_ExplicitTrue(t *testing.T) {
	cfg := RouteConfig{
		Name:               "internal",
		Endpoint:           "https://internal.svc",
		Auth:               AuthConfig{Type: "none"},
		InsecureSkipVerify: true,
	}

	route := createRoute(cfg)

	transport, ok := route.Transport.(*http.Transport)
	require.True(t, ok)
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
}

func TestGetAvailableTargets(t *testing.T) {
	router := newTestRouter(t, map[string]*Route{
		"k8s":        {},
		"monitoring": {},
		"logs":       {},
	})

	targets := router.getAvailableTargets()

	assert.Len(t, targets, 3)
	assert.Contains(t, targets, "k8s")
	assert.Contains(t, targets, "monitoring")
	assert.Contains(t, targets, "logs")
}

func TestGetAvailableTargets_Empty(t *testing.T) {
	router := newTestRouter(t, map[string]*Route{})

	targets := router.getAvailableTargets()

	assert.Empty(t, targets)
}
