// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// newTestPlaneAPI creates a PlaneAPI with a test mux and optional k8s objects.
func newTestPlaneAPI(t *testing.T, objects ...client.Object) (*http.ServeMux, *ConnectionManager) {
	t.Helper()
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		Build()

	cm := NewConnectionManager(testLogger())
	server := &Server{
		k8sClient: fakeClient,
		logger:    testLogger(),
	}
	api := NewPlaneAPI(cm, server, testLogger())
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	return mux, cm
}

func TestHandlePlaneNotification_Deleted(t *testing.T) {
	mux, cm := newTestPlaneAPI(t)

	// Register a connection so there's something to disconnect
	conn, cleanup := newTestWSConn(t)
	defer cleanup()
	_, _ = cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil)

	notification := PlaneNotification{
		PlaneType: "dataplane",
		PlaneID:   "prod",
		Event:     "deleted",
		Namespace: "ns",
		Name:      "dp1",
	}
	body, _ := json.Marshal(notification)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/planes/notify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp PlaneNotificationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, "disconnect", resp.Action)
	require.NotNil(t, resp.DisconnectedAgents)
	assert.Equal(t, 1, *resp.DisconnectedAgents)
}

func TestHandlePlaneNotification_Created(t *testing.T) {
	// Create k8s objects so fetchCRClientCA can find the CA
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ca-secret", Namespace: "ns"},
		Data:       map[string][]byte{"ca.crt": []byte("test-ca-data")},
	}
	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp1", Namespace: "ns"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "prod",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					SecretKeyRef: &openchoreov1alpha1.SecretKeyReference{Name: "ca-secret", Key: "ca.crt"},
				},
			},
		},
	}

	mux, _ := newTestPlaneAPI(t, caSecret, dp)

	notification := PlaneNotification{
		PlaneType: "dataplane",
		PlaneID:   "prod",
		Event:     "created",
		Namespace: "ns",
		Name:      "dp1",
	}
	body, _ := json.Marshal(notification)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/planes/notify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should succeed (revalidate with 0 connections)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp PlaneNotificationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, "revalidate", resp.Action)
}

func TestHandlePlaneNotification_Updated(t *testing.T) {
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ca-secret", Namespace: "ns"},
		Data:       map[string][]byte{"ca.crt": []byte("test-ca-data")},
	}
	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp1", Namespace: "ns"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "prod",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					SecretKeyRef: &openchoreov1alpha1.SecretKeyReference{Name: "ca-secret", Key: "ca.crt"},
				},
			},
		},
	}

	mux, _ := newTestPlaneAPI(t, caSecret, dp)

	notification := PlaneNotification{
		PlaneType: "dataplane",
		PlaneID:   "prod",
		Event:     "updated",
		Namespace: "ns",
		Name:      "dp1",
	}
	body, _ := json.Marshal(notification)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/planes/notify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp PlaneNotificationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, "revalidate", resp.Action)
}

func TestHandlePlaneNotification_InvalidPayload(t *testing.T) {
	mux, _ := newTestPlaneAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/planes/notify", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandlePlaneNotification_MissingFields(t *testing.T) {
	mux, _ := newTestPlaneAPI(t)

	tests := []struct {
		name         string
		notification PlaneNotification
	}{
		{"missing planeType", PlaneNotification{PlaneID: "prod", Event: "created"}},
		{"missing planeID", PlaneNotification{PlaneType: "dataplane", Event: "created"}},
		{"missing event", PlaneNotification{PlaneType: "dataplane", PlaneID: "prod"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.notification)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/planes/notify", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestHandlePlaneNotification_UnknownEvent(t *testing.T) {
	mux, _ := newTestPlaneAPI(t)

	notification := PlaneNotification{
		PlaneType: "dataplane",
		PlaneID:   "prod",
		Event:     "unknown-event",
		Namespace: "ns",
		Name:      "dp1",
	}
	body, _ := json.Marshal(notification)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/planes/notify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleReconnect(t *testing.T) {
	mux, cm := newTestPlaneAPI(t)

	conn, cleanup := newTestWSConn(t)
	defer cleanup()
	_, _ = cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/planes/dataplane/prod/reconnect", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp PlaneReconnectResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, 1, resp.DisconnectedAgents)
}

func TestHandleGetPlaneStatus(t *testing.T) {
	mux, cm := newTestPlaneAPI(t)

	conn, cleanup := newTestWSConn(t)
	defer cleanup()
	_, _ = cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/planes/dataplane/prod/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var status PlaneConnectionStatus
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
	assert.True(t, status.Connected)
	assert.Equal(t, 1, status.ConnectedAgents)
	assert.Equal(t, "dataplane", status.PlaneType)
	assert.Equal(t, "prod", status.PlaneID)
}

func TestHandleGetPlaneStatus_CRSpecific(t *testing.T) {
	mux, cm := newTestPlaneAPI(t)

	conn, cleanup := newTestWSConn(t)
	defer cleanup()
	_, _ = cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/planes/dataplane/prod/status?namespace=ns&name=dp1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var status PlaneConnectionStatus
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
	assert.True(t, status.Connected)
	assert.Equal(t, 1, status.ConnectedAgents)
}

func TestHandleGetPlaneStatus_ClusterScoped(t *testing.T) {
	mux, cm := newTestPlaneAPI(t)

	conn, cleanup := newTestWSConn(t)
	defer cleanup()
	// Cluster-scoped CR key format: "/name"
	_, _ = cm.Register("dataplane", "prod", conn, []string{"/global-dp"}, nil)

	// Cluster-scoped: name only, no namespace
	req := httptest.NewRequest(http.MethodGet, "/api/v1/planes/dataplane/prod/status?name=global-dp", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var status PlaneConnectionStatus
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
	assert.True(t, status.Connected)
	assert.Equal(t, 1, status.ConnectedAgents)
}

func TestHandleGetAllPlaneStatus(t *testing.T) {
	mux, cm := newTestPlaneAPI(t)

	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()

	_, _ = cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil)
	_, _ = cm.Register("workflowplane", "ci", conn2, []string{"ns/wp1"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/planes/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp AllPlaneStatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Total)
	assert.Len(t, resp.Planes, 2)
}

func TestHandlePlaneNotification_FetchCAFailure(t *testing.T) {
	// No k8s objects, so fetching CA will fail
	mux, _ := newTestPlaneAPI(t)

	notification := PlaneNotification{
		PlaneType: "dataplane",
		PlaneID:   "prod",
		Event:     "created",
		Namespace: "ns",
		Name:      "dp1",
	}
	body, _ := json.Marshal(notification)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/planes/notify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should return 503 when CA fetch fails (transient error for retry)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
