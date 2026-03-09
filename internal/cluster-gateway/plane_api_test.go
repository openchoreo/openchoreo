// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestPlaneAPI(t *testing.T) (*PlaneAPI, *ConnectionManager) {
	t.Helper()
	cm := newTestConnectionManager()
	// PlaneAPI needs a Server for fetchCRClientCA, but we can pass nil for tests
	// that don't exercise that code path
	api := NewPlaneAPI(cm, nil, testLogger())
	return api, cm
}

func TestPlaneAPI_HandleGetAllPlaneStatus(t *testing.T) {
	api, cm := newTestPlaneAPI(t)

	t.Run("empty state", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/planes/status", nil)
		w := httptest.NewRecorder()
		api.handleGetAllPlaneStatus(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp AllPlaneStatusResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, 0, resp.Total)
		assert.Empty(t, resp.Planes)
	})

	t.Run("with connections", func(t *testing.T) {
		ws1, cleanup1 := newMockWSConn(t)
		defer cleanup1()
		ws2, cleanup2 := newMockWSConn(t)
		defer cleanup2()
		_, _ = cm.Register("dataplane", "prod", ws1, nil, nil)
		_, _ = cm.Register("buildplane", "build1", ws2, nil, nil)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/planes/status", nil)
		w := httptest.NewRecorder()
		api.handleGetAllPlaneStatus(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp AllPlaneStatusResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, 2, resp.Total)
		assert.Len(t, resp.Planes, 2)
	})
}

func TestPlaneAPI_HandleGetPlaneStatus(t *testing.T) {
	api, cm := newTestPlaneAPI(t)

	ws, cleanup := newMockWSConn(t)
	defer cleanup()
	_, _ = cm.Register("dataplane", "prod", ws, []string{"ns-a/dp1"}, nil)

	t.Run("plane-level status", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/planes/{type}/{id}/status", api.handleGetPlaneStatus)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/planes/dataplane/prod/status", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var status PlaneConnectionStatus
		require.NoError(t, json.NewDecoder(w.Body).Decode(&status))
		assert.True(t, status.Connected)
		assert.Equal(t, 1, status.ConnectedAgents)
		assert.Equal(t, "dataplane", status.PlaneType)
		assert.Equal(t, "prod", status.PlaneID)
	})

	t.Run("CR-specific status with namespace and name", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/planes/{type}/{id}/status", api.handleGetPlaneStatus)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/planes/dataplane/prod/status?namespace=ns-a&name=dp1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var status PlaneConnectionStatus
		require.NoError(t, json.NewDecoder(w.Body).Decode(&status))
		assert.True(t, status.Connected)
		assert.Equal(t, 1, status.ConnectedAgents)
	})

	t.Run("CR-specific status for unknown CR", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/planes/{type}/{id}/status", api.handleGetPlaneStatus)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/planes/dataplane/prod/status?namespace=ns-a&name=unknown", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var status PlaneConnectionStatus
		require.NoError(t, json.NewDecoder(w.Body).Decode(&status))
		assert.False(t, status.Connected)
		assert.Equal(t, 0, status.ConnectedAgents)
	})

	t.Run("cluster-scoped CR status (name only, no namespace)", func(t *testing.T) {
		ws2, cleanup2 := newMockWSConn(t)
		defer cleanup2()
		_, _ = cm.Register("dataplane", "prod", ws2, []string{"/cluster-dp"}, nil)

		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/planes/{type}/{id}/status", api.handleGetPlaneStatus)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/planes/dataplane/prod/status?name=cluster-dp", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var status PlaneConnectionStatus
		require.NoError(t, json.NewDecoder(w.Body).Decode(&status))
		assert.True(t, status.Connected)
		// Only ws2 (with ValidCRs=["/cluster-dp"]) should match, not ws (with ValidCRs=["ns-a/dp1"])
		assert.Equal(t, 1, status.ConnectedAgents)
	})
}

func TestPlaneAPI_HandleReconnect(t *testing.T) {
	t.Run("disconnect existing connections", func(t *testing.T) {
		api, cm := newTestPlaneAPI(t)

		ws1, cleanup1 := newMockWSConn(t)
		defer cleanup1()
		ws2, cleanup2 := newMockWSConn(t)
		defer cleanup2()
		_, _ = cm.Register("dataplane", "prod", ws1, nil, nil)
		_, _ = cm.Register("dataplane", "prod", ws2, nil, nil)

		mux := http.NewServeMux()
		mux.HandleFunc("POST /api/v1/planes/{type}/{id}/reconnect", api.handleReconnect)

		r := httptest.NewRequest(http.MethodPost, "/api/v1/planes/dataplane/prod/reconnect", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp PlaneReconnectResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
		assert.Equal(t, 2, resp.DisconnectedAgents)
		assert.Equal(t, 0, cm.Count())
	})

	t.Run("no connections to disconnect", func(t *testing.T) {
		api, _ := newTestPlaneAPI(t)

		mux := http.NewServeMux()
		mux.HandleFunc("POST /api/v1/planes/{type}/{id}/reconnect", api.handleReconnect)

		r := httptest.NewRequest(http.MethodPost, "/api/v1/planes/dataplane/prod/reconnect", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp PlaneReconnectResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
		assert.Equal(t, 0, resp.DisconnectedAgents)
	})
}

func TestPlaneAPI_HandlePlaneNotification(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		api, _ := newTestPlaneAPI(t)

		r := httptest.NewRequest(http.MethodPost, "/api/v1/planes/notify", strings.NewReader("not-json"))
		w := httptest.NewRecorder()
		api.handlePlaneNotification(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing required fields", func(t *testing.T) {
		api, _ := newTestPlaneAPI(t)

		payload := `{"planeType":"dataplane","planeID":"prod"}`
		r := httptest.NewRequest(http.MethodPost, "/api/v1/planes/notify", strings.NewReader(payload))
		w := httptest.NewRecorder()
		api.handlePlaneNotification(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("unknown event type", func(t *testing.T) {
		api, _ := newTestPlaneAPI(t)

		payload := `{"planeType":"dataplane","planeID":"prod","event":"unknown","namespace":"ns","name":"dp1"}`
		r := httptest.NewRequest(http.MethodPost, "/api/v1/planes/notify", strings.NewReader(payload))
		w := httptest.NewRecorder()
		api.handlePlaneNotification(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("created event disconnects agents", func(t *testing.T) {
		api, cm := newTestPlaneAPI(t)
		ws, cleanup := newMockWSConn(t)
		defer cleanup()
		_, _ = cm.Register("dataplane", "prod", ws, nil, nil)

		payload := `{"planeType":"dataplane","planeID":"prod","event":"created","namespace":"ns","name":"dp1"}`
		r := httptest.NewRequest(http.MethodPost, "/api/v1/planes/notify", strings.NewReader(payload))
		w := httptest.NewRecorder()
		api.handlePlaneNotification(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp PlaneNotificationResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
		assert.Equal(t, "disconnect", resp.Action)
		assert.NotNil(t, resp.DisconnectedAgents)
		assert.Equal(t, 1, *resp.DisconnectedAgents)
	})

	t.Run("deleted event disconnects agents", func(t *testing.T) {
		api, cm := newTestPlaneAPI(t)
		ws, cleanup := newMockWSConn(t)
		defer cleanup()
		_, _ = cm.Register("dataplane", "prod", ws, nil, nil)

		payload := `{"planeType":"dataplane","planeID":"prod","event":"deleted","namespace":"ns","name":"dp1"}`
		r := httptest.NewRequest(http.MethodPost, "/api/v1/planes/notify", strings.NewReader(payload))
		w := httptest.NewRecorder()
		api.handlePlaneNotification(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp PlaneNotificationResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
		assert.Equal(t, "disconnect", resp.Action)
	})

	t.Run("updated event falls back to disconnect when CA fetch fails", func(t *testing.T) {
		cm := newTestConnectionManager()
		// Create a server with a fake k8s client (no plane CRs exist)
		// so fetchCRClientCA returns "CR not found" error
		fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).Build()
		srv := &Server{
			config:    &Config{},
			connMgr:   cm,
			validator: NewRequestValidator(),
			logger:    testLogger(),
			k8sClient: fakeClient,
		}
		api := NewPlaneAPI(cm, srv, testLogger())

		ws, cleanup := newMockWSConn(t)
		defer cleanup()
		_, _ = cm.Register("dataplane", "prod", ws, nil, nil)

		payload := `{"planeType":"dataplane","planeID":"prod","event":"updated","namespace":"ns","name":"dp1"}`
		r := httptest.NewRequest(http.MethodPost, "/api/v1/planes/notify", strings.NewReader(payload))
		w := httptest.NewRecorder()
		api.handlePlaneNotification(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp PlaneNotificationResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
		assert.Equal(t, "disconnect_fallback", resp.Action)
		assert.NotEmpty(t, resp.Error)
	})
}
