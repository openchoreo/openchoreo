// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gateway

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyK8sRequest(t *testing.T) {
	tests := []struct {
		name          string
		planeType     string
		planeID       string
		crNamespace   string
		crName        string
		k8sPath       string
		rawQuery      string
		serverStatus  int
		serverBody    string
		serverHeaders map[string]string
		wantURLPath   string
		wantErr       bool
	}{
		{
			name:         "namespace-scoped pod list",
			planeType:    "dataplane",
			planeID:      "prod-cluster",
			crNamespace:  "acme",
			crName:       "prod-dp",
			k8sPath:      "api/v1/namespaces/default/pods",
			serverStatus: http.StatusOK,
			serverBody:   `{"kind":"PodList","items":[]}`,
			wantURLPath:  "/api/proxy/dataplane/prod-cluster/acme/prod-dp/k8s/api/v1/namespaces/default/pods",
		},
		{
			name:         "namespace-scoped with query params",
			planeType:    "dataplane",
			planeID:      "prod-cluster",
			crNamespace:  "acme",
			crName:       "prod-dp",
			k8sPath:      "api/v1/namespaces/default/events",
			rawQuery:     "fieldSelector=involvedObject.name%3Dmy-pod",
			serverStatus: http.StatusOK,
			serverBody:   `{"kind":"EventList","items":[]}`,
			wantURLPath:  "/api/proxy/dataplane/prod-cluster/acme/prod-dp/k8s/api/v1/namespaces/default/events",
		},
		{
			name:         "cluster-scoped with _cluster namespace",
			planeType:    "dataplane",
			planeID:      "shared-cluster",
			crNamespace:  "_cluster",
			crName:       "shared-dp",
			k8sPath:      "api/v1/namespaces/default/pods",
			serverStatus: http.StatusOK,
			serverBody:   `{"kind":"PodList","items":[]}`,
			wantURLPath:  "/api/proxy/dataplane/shared-cluster/_cluster/shared-dp/k8s/api/v1/namespaces/default/pods",
		},
		{
			name:         "workflowplane proxy",
			planeType:    "workflowplane",
			planeID:      "ci-cluster",
			crNamespace:  "acme",
			crName:       "ci-wp",
			k8sPath:      "api/v1/namespaces/workflow-ns/pods",
			serverStatus: http.StatusOK,
			serverBody:   `{"kind":"PodList","items":[]}`,
			wantURLPath:  "/api/proxy/workflowplane/ci-cluster/acme/ci-wp/k8s/api/v1/namespaces/workflow-ns/pods",
		},
		{
			name:         "observabilityplane proxy",
			planeType:    "observabilityplane",
			planeID:      "obs-cluster",
			crNamespace:  "acme",
			crName:       "obs-op",
			k8sPath:      "api/v1/namespaces/monitoring/pods",
			serverStatus: http.StatusOK,
			serverBody:   `{"kind":"PodList","items":[]}`,
			wantURLPath:  "/api/proxy/observabilityplane/obs-cluster/acme/obs-op/k8s/api/v1/namespaces/monitoring/pods",
		},
		{
			name:         "server returns 404",
			planeType:    "dataplane",
			planeID:      "prod-cluster",
			crNamespace:  "acme",
			crName:       "prod-dp",
			k8sPath:      "api/v1/namespaces/default/pods/nonexistent",
			serverStatus: http.StatusNotFound,
			serverBody:   `{"kind":"Status","status":"Failure","message":"pods \"nonexistent\" not found"}`,
			wantURLPath:  "/api/proxy/dataplane/prod-cluster/acme/prod-dp/k8s/api/v1/namespaces/default/pods/nonexistent",
		},
		{
			name:          "response headers are preserved",
			planeType:     "dataplane",
			planeID:       "prod-cluster",
			crNamespace:   "acme",
			crName:        "prod-dp",
			k8sPath:       "api/v1/namespaces/default/pods",
			serverStatus:  http.StatusOK,
			serverBody:    `{"kind":"PodList","items":[]}`,
			serverHeaders: map[string]string{"X-Custom-Header": "test-value"},
			wantURLPath:   "/api/proxy/dataplane/prod-cluster/acme/prod-dp/k8s/api/v1/namespaces/default/pods",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedPath, receivedQuery string

			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedPath = r.URL.Path
				receivedQuery = r.URL.RawQuery
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Accept"))
				for k, v := range tt.serverHeaders {
					w.Header().Set(k, v)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.serverStatus)
				_, _ = w.Write([]byte(tt.serverBody))
			}))
			defer server.Close()

			c := &Client{baseURL: server.URL, httpClient: server.Client()}
			resp, err := c.ProxyK8sRequest(context.Background(), tt.planeType, tt.planeID,
				tt.crNamespace, tt.crName, tt.k8sPath, tt.rawQuery)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.wantURLPath, receivedPath)
			if tt.rawQuery != "" {
				assert.Equal(t, tt.rawQuery, receivedQuery)
			}
			assert.Equal(t, tt.serverStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, tt.serverBody, string(body))

			for k, v := range tt.serverHeaders {
				assert.Equal(t, v, resp.Header.Get(k))
			}
		})
	}
}

func TestProxyK8sRequest_NetworkError(t *testing.T) {
	c := &Client{baseURL: "https://localhost:1", httpClient: http.DefaultClient}

	_, err := c.ProxyK8sRequest(context.Background(), "dataplane", "test", "ns", "name", "api/v1/pods", "")

	require.Error(t, err)
	assert.True(t, IsTransientError(err), "expected TransientError, got %T: %v", err, err)
}

func TestProxyK8sRequest_URLConstruction(t *testing.T) {
	tests := []struct {
		name         string
		planeType    string
		planeID      string
		crNamespace  string
		crName       string
		k8sPath      string
		rawQuery     string
		wantContains string
	}{
		{
			name:         "URL without query",
			planeType:    "dataplane",
			planeID:      "prod",
			crNamespace:  "ns",
			crName:       "dp",
			k8sPath:      "api/v1/pods",
			wantContains: "/api/proxy/dataplane/prod/ns/dp/k8s/api/v1/pods",
		},
		{
			name:         "URL with query",
			planeType:    "dataplane",
			planeID:      "prod",
			crNamespace:  "ns",
			crName:       "dp",
			k8sPath:      "api/v1/events",
			rawQuery:     "fieldSelector=key%3Dvalue",
			wantContains: "?fieldSelector=key%3Dvalue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedURL string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedURL = r.URL.String()
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			c := &Client{baseURL: server.URL, httpClient: server.Client()}
			resp, err := c.ProxyK8sRequest(context.Background(), tt.planeType, tt.planeID,
				tt.crNamespace, tt.crName, tt.k8sPath, tt.rawQuery)
			require.NoError(t, err)
			resp.Body.Close()

			assert.Contains(t, receivedURL, tt.wantContains)
		})
	}
}

func TestProxyK8sRequest_CallerMustCloseBody(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test body"))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, httpClient: server.Client()}
	resp, err := c.ProxyK8sRequest(context.Background(), "dataplane", "test", "ns", "name", "api/v1/pods", "")
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "test body", string(body))
	resp.Body.Close()
}

// ─── Pure-logic tests ─────────────────────────────────────────────────────────

// mockLogger captures calls to Error for use in HandleGatewayError tests.
type mockLogger struct {
	called bool
	err    error
	msg    string
}

func (m *mockLogger) Error(err error, msg string, keysAndValues ...any) {
	m.called = true
	m.err = err
	m.msg = msg
}

func TestTransientError_Error(t *testing.T) {
	t.Run("with status code", func(t *testing.T) {
		e := &TransientError{StatusCode: 503, Message: "service unavailable"}
		assert.Equal(t, "transient gateway error (status 503): service unavailable", e.Error())
	})

	t.Run("without status code", func(t *testing.T) {
		e := &TransientError{Message: "network fail"}
		assert.Equal(t, "transient gateway error: network fail", e.Error())
	})
}

func TestTransientError_Unwrap(t *testing.T) {
	inner := errors.New("underlying cause")
	e := &TransientError{Err: inner}
	assert.Equal(t, inner, e.Unwrap())

	eNoInner := &TransientError{}
	assert.Nil(t, eNoInner.Unwrap())
}

func TestPermanentError_Error(t *testing.T) {
	t.Run("with status code", func(t *testing.T) {
		e := &PermanentError{StatusCode: 404, Message: "not found"}
		assert.Equal(t, "permanent gateway error (status 404): not found", e.Error())
	})

	t.Run("without status code", func(t *testing.T) {
		e := &PermanentError{Message: "invalid"}
		assert.Equal(t, "permanent gateway error: invalid", e.Error())
	})
}

func TestIsPermanentError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"PermanentError", &PermanentError{StatusCode: 404}, true},
		{"wrapped PermanentError", fmt.Errorf("wrapped: %w", &PermanentError{StatusCode: 400}), true},
		{"TransientError", &TransientError{StatusCode: 500}, false},
		{"plain error", errors.New("random"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsPermanentError(tt.err))
		})
	}
}

func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"connection refused", errors.New("dial tcp: connection refused"), true},
		{"connection reset", errors.New("read: connection reset by peer"), true},
		{"no such host", errors.New("dial tcp: no such host"), true},
		{"network unreachable", errors.New("network is unreachable"), true},
		{"i/o timeout", errors.New("i/o timeout"), true},
		{"context.DeadlineExceeded", context.DeadlineExceeded, true},
		{"wrapped DeadlineExceeded", fmt.Errorf("operation: %w", context.DeadlineExceeded), true},
		{"unrelated error", errors.New("something else"), false},
		{"PermanentError (not network)", &PermanentError{StatusCode: 404}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isNetworkError(tt.err))
		})
	}
}

func TestClassifyHTTPError(t *testing.T) {
	tests := []struct {
		code          int
		wantTransient bool
		wantMsg       string
	}{
		{500, true, "gateway server error"},
		{502, true, "gateway server error"},
		{503, true, "gateway server error"},
		{599, true, "gateway server error"},
		{429, true, "gateway rate limited"},
		{400, false, "gateway client error"},
		{401, false, "gateway client error"},
		{404, false, "gateway client error"},
		{499, false, "gateway client error"},
		{301, true, "unexpected status code"},
		{200, true, "unexpected status code"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("HTTP %d", tt.code), func(t *testing.T) {
			err := classifyHTTPError(tt.code)
			require.Error(t, err)
			if tt.wantTransient {
				assert.True(t, IsTransientError(err), "code %d: expected TransientError, got %T", tt.code, err)
			} else {
				assert.True(t, IsPermanentError(err), "code %d: expected PermanentError, got %T", tt.code, err)
			}
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestHandleGatewayError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantRetry  bool
		wantNilErr bool
	}{
		{
			name:       "TransientError → retry",
			err:        &TransientError{Message: "timeout"},
			wantRetry:  true,
			wantNilErr: false,
		},
		{
			name:       "network error is transient → retry",
			err:        errors.New("connection refused"),
			wantRetry:  true,
			wantNilErr: false,
		},
		{
			name:       "PermanentError → no retry, nil returned error",
			err:        &PermanentError{Message: "bad request"},
			wantRetry:  false,
			wantNilErr: true,
		},
		{
			name:       "generic error → no retry, nil returned error",
			err:        errors.New("unknown"),
			wantRetry:  false,
			wantNilErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &mockLogger{}
			shouldRetry, _, retryErr := HandleGatewayError(logger, tt.err, "test-operation")

			assert.Equal(t, tt.wantRetry, shouldRetry)
			assert.Equal(t, tt.wantNilErr, retryErr == nil)
			assert.True(t, logger.called, "expected logger.Error to be called")
		})
	}
}

// ─── Gateway HTTP endpoint tests ─────────────────────────────────────────────

func newTestGatewayClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	return &Client{baseURL: server.URL, httpClient: server.Client()}
}

func TestNotifyPlaneLifecycle(t *testing.T) {
	t.Run("success returns response", func(t *testing.T) {
		var capturedMethod, capturedPath, capturedContentType string
		var capturedBody []byte

		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedMethod = r.Method
			capturedPath = r.URL.Path
			capturedContentType = r.Header.Get("Content-Type")
			capturedBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true,"disconnectedAgents":2}`))
		})

		notification := &PlaneNotification{
			PlaneType: "dataplane", PlaneID: "prod",
			Event: "created", Namespace: "default", Name: "my-dp",
		}
		resp, err := c.NotifyPlaneLifecycle(context.Background(), notification)

		require.NoError(t, err)
		assert.Equal(t, http.MethodPost, capturedMethod)
		assert.Equal(t, "/api/v1/planes/notify", capturedPath)
		assert.Equal(t, "application/json", capturedContentType)
		assert.Contains(t, string(capturedBody), "dataplane")
		assert.True(t, resp.Success)
		assert.Equal(t, 2, resp.DisconnectedAgents)
	})

	t.Run("500 returns TransientError", func(t *testing.T) {
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		_, err := c.NotifyPlaneLifecycle(context.Background(), &PlaneNotification{})
		require.Error(t, err)
		assert.True(t, IsTransientError(err), "expected TransientError, got %T: %v", err, err)
	})

	t.Run("429 returns TransientError", func(t *testing.T) {
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		})
		_, err := c.NotifyPlaneLifecycle(context.Background(), &PlaneNotification{})
		require.Error(t, err)
		assert.True(t, IsTransientError(err), "expected TransientError, got %T: %v", err, err)
	})

	t.Run("400 returns PermanentError", func(t *testing.T) {
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		})
		_, err := c.NotifyPlaneLifecycle(context.Background(), &PlaneNotification{})
		require.Error(t, err)
		assert.True(t, IsPermanentError(err), "expected PermanentError, got %T: %v", err, err)
	})

	t.Run("network error returns TransientError", func(t *testing.T) {
		c := &Client{baseURL: "https://localhost:1", httpClient: http.DefaultClient}
		_, err := c.NotifyPlaneLifecycle(context.Background(), &PlaneNotification{})
		require.Error(t, err)
		assert.True(t, IsTransientError(err), "expected TransientError for network error, got %T", err)
	})

	t.Run("invalid JSON response returns decode error", func(t *testing.T) {
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not json"))
		})
		_, err := c.NotifyPlaneLifecycle(context.Background(), &PlaneNotification{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode")
	})
}

func TestForceReconnect(t *testing.T) {
	t.Run("success with correct URL and POST", func(t *testing.T) {
		var capturedMethod, capturedPath string
		var capturedBodyLen int

		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedMethod = r.Method
			capturedPath = r.URL.Path
			body, _ := io.ReadAll(r.Body)
			capturedBodyLen = len(body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true,"disconnectedAgents":0}`))
		})

		resp, err := c.ForceReconnect(context.Background(), "dataplane", "prod-cluster")

		require.NoError(t, err)
		assert.Equal(t, http.MethodPost, capturedMethod)
		assert.Equal(t, "/api/v1/planes/dataplane/prod-cluster/reconnect", capturedPath)
		assert.Equal(t, 0, capturedBodyLen, "request body should be empty")
		assert.True(t, resp.Success)
	})

	t.Run("500 returns TransientError", func(t *testing.T) {
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		_, err := c.ForceReconnect(context.Background(), "dataplane", "prod")
		require.Error(t, err)
		assert.True(t, IsTransientError(err))
	})

	t.Run("404 returns PermanentError", func(t *testing.T) {
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		_, err := c.ForceReconnect(context.Background(), "dataplane", "prod")
		require.Error(t, err)
		assert.True(t, IsPermanentError(err))
	})

	t.Run("network error returns TransientError", func(t *testing.T) {
		c := &Client{baseURL: "https://localhost:1", httpClient: http.DefaultClient}
		_, err := c.ForceReconnect(context.Background(), "dataplane", "prod")
		require.Error(t, err)
		assert.True(t, IsTransientError(err))
	})
}

func TestGetPlaneStatus(t *testing.T) {
	t.Run("success without namespace and name", func(t *testing.T) {
		var capturedMethod, capturedQuery string

		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedMethod = r.Method
			capturedQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"planeType":"dataplane","planeID":"prod","connected":true,"connectedAgents":1}`))
		})

		status, err := c.GetPlaneStatus(context.Background(), "dataplane", "prod", "", "")

		require.NoError(t, err)
		assert.Equal(t, http.MethodGet, capturedMethod)
		assert.Empty(t, capturedQuery)
		assert.True(t, status.Connected)
		assert.Equal(t, 1, status.ConnectedAgents)
	})

	t.Run("success with namespace and name adds query params", func(t *testing.T) {
		var capturedQuery string

		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"planeType":"dataplane","planeID":"prod","connected":true}`))
		})

		_, err := c.GetPlaneStatus(context.Background(), "dataplane", "prod", "acme", "my-dp")

		require.NoError(t, err)
		assert.Contains(t, capturedQuery, "namespace=acme")
		assert.Contains(t, capturedQuery, "name=my-dp")
	})

	t.Run("500 returns TransientError", func(t *testing.T) {
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		_, err := c.GetPlaneStatus(context.Background(), "dataplane", "prod", "", "")
		require.Error(t, err)
		assert.True(t, IsTransientError(err))
	})

	t.Run("invalid JSON response returns decode error", func(t *testing.T) {
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not valid json"))
		})
		_, err := c.GetPlaneStatus(context.Background(), "dataplane", "prod", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode")
	})
}

func TestGetPodLogsFromPlane(t *testing.T) {
	podRef := &PodReference{Namespace: "default", Name: "my-pod"}

	t.Run("success with no options", func(t *testing.T) {
		var capturedPath string

		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.RequestURI()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("log line 1\nlog line 2\n"))
		})

		logs, err := c.GetPodLogsFromPlane(context.Background(), "dataplane", "prod", "acme", "my-dp", podRef, nil)

		require.NoError(t, err)
		assert.Equal(t, "log line 1\nlog line 2\n", logs)
		assert.Contains(t, capturedPath, "/pods/my-pod/log")
		assert.Contains(t, capturedPath, "namespaces/default")
	})

	t.Run("with container name adds query param", func(t *testing.T) {
		var capturedURI string
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedURI = r.URL.RequestURI()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("container logs"))
		})

		_, err := c.GetPodLogsFromPlane(context.Background(), "dataplane", "prod", "acme", "my-dp",
			podRef, &PodLogsOptions{ContainerName: "mycontainer"})
		require.NoError(t, err)
		assert.Equal(t, "/api/proxy/dataplane/prod/acme/my-dp/k8s/api/v1/namespaces/default/pods/my-pod/log?container=mycontainer", capturedURI)
	})

	t.Run("with timestamps adds query param", func(t *testing.T) {
		var capturedURI string
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedURI = r.URL.RequestURI()
			w.WriteHeader(http.StatusOK)
		})
		_, err := c.GetPodLogsFromPlane(context.Background(), "dataplane", "prod", "acme", "my-dp",
			podRef, &PodLogsOptions{IncludeTimestamps: true})
		require.NoError(t, err)
		assert.Equal(t, "/api/proxy/dataplane/prod/acme/my-dp/k8s/api/v1/namespaces/default/pods/my-pod/log?timestamps=true", capturedURI)
	})

	t.Run("with sinceSeconds adds query param", func(t *testing.T) {
		var capturedURI string
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedURI = r.URL.RequestURI()
			w.WriteHeader(http.StatusOK)
		})
		seconds := int64(300)
		_, err := c.GetPodLogsFromPlane(context.Background(), "dataplane", "prod", "acme", "my-dp",
			podRef, &PodLogsOptions{SinceSeconds: &seconds})
		require.NoError(t, err)
		assert.Equal(t, "/api/proxy/dataplane/prod/acme/my-dp/k8s/api/v1/namespaces/default/pods/my-pod/log?sinceSeconds=300", capturedURI)
	})

	t.Run("nil pod reference returns error", func(t *testing.T) {
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {})
		_, err := c.GetPodLogsFromPlane(context.Background(), "dataplane", "prod", "acme", "my-dp", nil, nil)
		require.Error(t, err)
	})

	t.Run("empty pod name returns error", func(t *testing.T) {
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {})
		_, err := c.GetPodLogsFromPlane(context.Background(), "dataplane", "prod", "acme", "my-dp",
			&PodReference{Namespace: "default", Name: ""}, nil)
		require.Error(t, err)
	})

	t.Run("empty pod namespace returns error", func(t *testing.T) {
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {})
		_, err := c.GetPodLogsFromPlane(context.Background(), "dataplane", "prod", "acme", "my-dp",
			&PodReference{Namespace: "", Name: "pod"}, nil)
		require.Error(t, err)
	})

	t.Run("500 returns TransientError", func(t *testing.T) {
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		_, err := c.GetPodLogsFromPlane(context.Background(), "dataplane", "prod", "acme", "my-dp", podRef, nil)
		require.Error(t, err)
		assert.True(t, IsTransientError(err))
	})

	t.Run("network error returns TransientError", func(t *testing.T) {
		c := &Client{baseURL: "https://localhost:1", httpClient: http.DefaultClient}
		_, err := c.GetPodLogsFromPlane(context.Background(), "dataplane", "prod", "acme", "my-dp", podRef, nil)
		require.Error(t, err)
		assert.True(t, IsTransientError(err))
	})
}

func TestGetPodEventsFromPlane(t *testing.T) {
	podRef := &PodReference{Namespace: "default", Name: "my-pod"}

	t.Run("success returns event bytes", func(t *testing.T) {
		var capturedURI, capturedAccept string

		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedURI = r.URL.RequestURI()
			capturedAccept = r.Header.Get("Accept")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"kind":"EventList","items":[]}`))
		})

		body, err := c.GetPodEventsFromPlane(context.Background(), "dataplane", "prod", "acme", "my-dp", podRef)

		require.NoError(t, err)
		assert.NotEmpty(t, body)
		assert.Equal(t, "application/json", capturedAccept)
		assert.Equal(t, "/api/proxy/dataplane/prod/acme/my-dp/k8s/api/v1/namespaces/default/events?fieldSelector=involvedObject.name%3Dmy-pod%2CinvolvedObject.kind%3DPod", capturedURI)
	})

	t.Run("nil pod reference returns error", func(t *testing.T) {
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {})
		_, err := c.GetPodEventsFromPlane(context.Background(), "dataplane", "prod", "acme", "my-dp", nil)
		require.Error(t, err)
	})

	t.Run("empty pod name returns error", func(t *testing.T) {
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {})
		_, err := c.GetPodEventsFromPlane(context.Background(), "dataplane", "prod", "acme", "my-dp",
			&PodReference{Namespace: "default", Name: ""})
		require.Error(t, err)
	})

	t.Run("500 returns TransientError", func(t *testing.T) {
		c := newTestGatewayClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		_, err := c.GetPodEventsFromPlane(context.Background(), "dataplane", "prod", "acme", "my-dp", podRef)
		require.Error(t, err)
		assert.True(t, IsTransientError(err))
	})

	t.Run("network error returns TransientError", func(t *testing.T) {
		c := &Client{baseURL: "https://localhost:1", httpClient: http.DefaultClient}
		_, err := c.GetPodEventsFromPlane(context.Background(), "dataplane", "prod", "acme", "my-dp", podRef)
		require.Error(t, err)
		assert.True(t, IsTransientError(err))
	})
}

// ─── BuildTLSConfig tests ─────────────────────────────────────────────────────

func TestBuildTLSConfig(t *testing.T) {
	t.Run("empty config uses default verification", func(t *testing.T) {
		cfg, err := BuildTLSConfig(&TLSConfig{})
		require.NoError(t, err)
		assert.False(t, cfg.InsecureSkipVerify)
		assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
		assert.Nil(t, cfg.RootCAs)
		assert.Empty(t, cfg.Certificates)
	})

	t.Run("InsecureSkipVerify=true opts into skipping verification", func(t *testing.T) {
		cfg, err := BuildTLSConfig(&TLSConfig{InsecureSkipVerify: true})
		require.NoError(t, err)
		assert.True(t, cfg.InsecureSkipVerify)
	})

	t.Run("InsecureSkipVerify=true still loads client cert for mTLS", func(t *testing.T) {
		certPEM, keyPEM := mustGenerateKeyPairPEM(t)
		dir := t.TempDir()
		certFile := filepath.Join(dir, "client.crt")
		keyFile := filepath.Join(dir, "client.key")
		require.NoError(t, os.WriteFile(certFile, certPEM, 0o600))
		require.NoError(t, os.WriteFile(keyFile, keyPEM, 0o600))

		cfg, err := BuildTLSConfig(&TLSConfig{
			InsecureSkipVerify: true,
			ClientCertFile:     certFile,
			ClientKeyFile:      keyFile,
		})
		require.NoError(t, err)
		assert.True(t, cfg.InsecureSkipVerify)
		assert.Len(t, cfg.Certificates, 1)
	})

	t.Run("ServerName is propagated", func(t *testing.T) {
		cfg, err := BuildTLSConfig(&TLSConfig{ServerName: "gateway.example"})
		require.NoError(t, err)
		assert.Equal(t, "gateway.example", cfg.ServerName)
	})

	t.Run("valid CAFile is loaded into RootCAs", func(t *testing.T) {
		certPEM, _ := mustGenerateKeyPairPEM(t)
		caFile := filepath.Join(t.TempDir(), "ca.crt")
		require.NoError(t, os.WriteFile(caFile, certPEM, 0o600))

		cfg, err := BuildTLSConfig(&TLSConfig{CAFile: caFile})
		require.NoError(t, err)
		assert.NotNil(t, cfg.RootCAs)
	})

	t.Run("missing CAFile returns error", func(t *testing.T) {
		_, err := BuildTLSConfig(&TLSConfig{CAFile: "/path/does/not/exist/ca.crt"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read CA file")
	})

	t.Run("malformed CAData returns parse error", func(t *testing.T) {
		_, err := BuildTLSConfig(&TLSConfig{CAData: []byte("not-a-cert")})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse CA certificate")
	})

	t.Run("valid client cert and key are loaded for mTLS", func(t *testing.T) {
		certPEM, keyPEM := mustGenerateKeyPairPEM(t)
		dir := t.TempDir()
		certFile := filepath.Join(dir, "client.crt")
		keyFile := filepath.Join(dir, "client.key")
		require.NoError(t, os.WriteFile(certFile, certPEM, 0o600))
		require.NoError(t, os.WriteFile(keyFile, keyPEM, 0o600))

		cfg, err := BuildTLSConfig(&TLSConfig{
			ClientCertFile: certFile,
			ClientKeyFile:  keyFile,
		})
		require.NoError(t, err)
		assert.Len(t, cfg.Certificates, 1)
	})

	t.Run("only ClientCertFile set returns asymmetric-config error", func(t *testing.T) {
		_, err := BuildTLSConfig(&TLSConfig{ClientCertFile: "/tmp/client.crt"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "both ClientCertFile and ClientKeyFile must be set")
	})

	t.Run("only ClientKeyFile set returns asymmetric-config error", func(t *testing.T) {
		_, err := BuildTLSConfig(&TLSConfig{ClientKeyFile: "/tmp/client.key"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "both ClientCertFile and ClientKeyFile must be set")
	})

	t.Run("invalid client key pair returns load error", func(t *testing.T) {
		_, err := BuildTLSConfig(&TLSConfig{
			ClientCertFile: "/path/does/not/exist/client.crt",
			ClientKeyFile:  "/path/does/not/exist/client.key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load client key pair")
	})
}

// mustGenerateKeyPairPEM creates a self-signed cert + matching private key in PEM form,
// usable for both CA and client-cert tests.
func mustGenerateKeyPairPEM(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM
}
