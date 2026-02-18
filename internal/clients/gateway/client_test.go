// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
			rawQuery:     "",
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
			rawQuery:     "",
			serverStatus: http.StatusOK,
			serverBody:   `{"kind":"PodList","items":[]}`,
			wantURLPath:  "/api/proxy/dataplane/shared-cluster/_cluster/shared-dp/k8s/api/v1/namespaces/default/pods",
		},
		{
			name:         "buildplane proxy",
			planeType:    "buildplane",
			planeID:      "ci-cluster",
			crNamespace:  "acme",
			crName:       "ci-bp",
			k8sPath:      "api/v1/namespaces/build-ns/pods",
			rawQuery:     "",
			serverStatus: http.StatusOK,
			serverBody:   `{"kind":"PodList","items":[]}`,
			wantURLPath:  "/api/proxy/buildplane/ci-cluster/acme/ci-bp/k8s/api/v1/namespaces/build-ns/pods",
		},
		{
			name:         "observabilityplane proxy",
			planeType:    "observabilityplane",
			planeID:      "obs-cluster",
			crNamespace:  "acme",
			crName:       "obs-op",
			k8sPath:      "api/v1/namespaces/monitoring/pods",
			rawQuery:     "",
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
			rawQuery:     "",
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
			rawQuery:      "",
			serverStatus:  http.StatusOK,
			serverBody:    `{"kind":"PodList","items":[]}`,
			serverHeaders: map[string]string{"X-Custom-Header": "test-value"},
			wantURLPath:   "/api/proxy/dataplane/prod-cluster/acme/prod-dp/k8s/api/v1/namespaces/default/pods",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedPath string
			var receivedQuery string

			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedPath = r.URL.Path
				receivedQuery = r.URL.RawQuery

				// Verify it's a GET request
				if r.Method != http.MethodGet {
					t.Errorf("Expected GET method, got %s", r.Method)
				}

				// Verify Accept header
				if r.Header.Get("Accept") != "application/json" {
					t.Errorf("Expected Accept: application/json, got %s", r.Header.Get("Accept"))
				}

				// Set custom headers if specified
				for k, v := range tt.serverHeaders {
					w.Header().Set(k, v)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.serverStatus)
				_, _ = w.Write([]byte(tt.serverBody))
			}))
			defer server.Close()

			client := &Client{
				baseURL:    server.URL,
				httpClient: server.Client(),
			}

			resp, err := client.ProxyK8sRequest(context.Background(), tt.planeType, tt.planeID, tt.crNamespace, tt.crName, tt.k8sPath, tt.rawQuery)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			defer resp.Body.Close()

			// Verify the URL path
			if receivedPath != tt.wantURLPath {
				t.Errorf("Request URL path = %q, want %q", receivedPath, tt.wantURLPath)
			}

			// Verify query string
			if tt.rawQuery != "" && receivedQuery != tt.rawQuery {
				t.Errorf("Request query = %q, want %q", receivedQuery, tt.rawQuery)
			}

			// Verify response status code
			if resp.StatusCode != tt.serverStatus {
				t.Errorf("Response status = %d, want %d", resp.StatusCode, tt.serverStatus)
			}

			// Verify response body
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}
			if string(body) != tt.serverBody {
				t.Errorf("Response body = %q, want %q", string(body), tt.serverBody)
			}

			// Verify custom response headers
			for k, v := range tt.serverHeaders {
				if resp.Header.Get(k) != v {
					t.Errorf("Response header %s = %q, want %q", k, resp.Header.Get(k), v)
				}
			}
		})
	}
}

func TestProxyK8sRequest_NetworkError(t *testing.T) {
	client := &Client{
		baseURL:    "https://localhost:1", // unreachable port
		httpClient: http.DefaultClient,
	}

	_, err := client.ProxyK8sRequest(context.Background(), "dataplane", "test", "ns", "name", "api/v1/pods", "")
	if err == nil {
		t.Error("Expected error for unreachable server, got nil")
	}

	if !IsTransientError(err) {
		t.Errorf("Expected TransientError, got %T: %v", err, err)
	}
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
			rawQuery:     "",
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

			client := &Client{
				baseURL:    server.URL,
				httpClient: server.Client(),
			}

			resp, err := client.ProxyK8sRequest(context.Background(), tt.planeType, tt.planeID, tt.crNamespace, tt.crName, tt.k8sPath, tt.rawQuery)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			resp.Body.Close()

			if !strings.Contains(receivedURL, tt.wantContains) {
				t.Errorf("Received URL %q does not contain %q", receivedURL, tt.wantContains)
			}
		})
	}
}

func TestProxyK8sRequest_CallerMustCloseBody(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test body"))
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: server.Client(),
	}

	resp, err := client.ProxyK8sRequest(context.Background(), "dataplane", "test", "ns", "name", "api/v1/pods", "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify body is readable (not already closed)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	if string(body) != "test body" {
		t.Errorf("Body = %q, want %q", string(body), "test body")
	}

	// Now close it (caller's responsibility)
	resp.Body.Close()
}
