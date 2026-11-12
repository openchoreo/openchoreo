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
		connMgr:   cm,
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
	_, err := cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

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
	assert.Equal(t, "revoke", resp.Action)
	require.NotNil(t, resp.AuthorizationsRevoked)
	assert.Equal(t, 1, *resp.AuthorizationsRevoked)
	// ns/dp1 was the agent's only CR, so nothing is left for it to serve.
	require.NotNil(t, resp.DisconnectedAgents)
	assert.Equal(t, 1, *resp.DisconnectedAgents)
	assert.Equal(t, 0, cm.Count())
}

// Many namespaces can point DataPlane CRs at one physical plane through a
// shared planeID. Deleting one of those CRs must not drop the agent: every
// other namespace would lose its plane until the agent reconnects.
func TestHandlePlaneNotification_DeletedKeepsAgentForSharedPlaneID(t *testing.T) {
	mux, cm := newTestPlaneAPI(t)

	conn, cleanup := newTestWSConn(t)
	defer cleanup()
	_, err := cm.Register("dataplane", "prod", conn, []string{"ns1/dp", "ns2/dp"}, nil, nil)
	require.NoError(t, err)

	body, _ := json.Marshal(PlaneNotification{
		PlaneType: "dataplane",
		PlaneID:   "prod",
		Event:     "deleted",
		Namespace: "ns1",
		Name:      "dp",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/planes/notify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp PlaneNotificationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "revoke", resp.Action)
	require.NotNil(t, resp.AuthorizationsRevoked)
	assert.Equal(t, 1, *resp.AuthorizationsRevoked)
	require.NotNil(t, resp.DisconnectedAgents)
	assert.Equal(t, 0, *resp.DisconnectedAgents)

	// The surviving CR is still routable; the deleted one is not.
	_, err = cm.GetForCR("dataplane/prod", "ns2/dp")
	assert.NoError(t, err)
	_, err = cm.GetForCR("dataplane/prod", "ns1/dp")
	assert.Error(t, err)
}

// A deletion that does not name the CR cannot be scoped, so the whole plane
// is dropped as before rather than leaving a stale authorization behind.
func TestHandlePlaneNotification_DeletedWithoutNameDisconnectsPlane(t *testing.T) {
	mux, cm := newTestPlaneAPI(t)

	conn, cleanup := newTestWSConn(t)
	defer cleanup()
	_, err := cm.Register("dataplane", "prod", conn, []string{"ns1/dp", "ns2/dp"}, nil, nil)
	require.NoError(t, err)

	body, _ := json.Marshal(PlaneNotification{PlaneType: "dataplane", PlaneID: "prod", Event: "deleted"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/planes/notify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp PlaneNotificationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "disconnect", resp.Action)
	assert.Equal(t, 0, cm.Count())
}

// An "updated" that arrives after the CR's "deleted" event is a stale one:
// applying it would hand the agent back an authorization nothing will revoke
// again. Until that event the CR is still in service, terminating or not,
// because its finalizer cleans up through the agent, so it must be granted.
func TestHandlePlaneNotification_UpdatedDoesNotGrantDeletedCR(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	caPEM := string(encodeCertToPEM(t, caCert))

	newPlane := func(terminating bool) *openchoreov1alpha1.DataPlane {
		dp := &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "dp", Namespace: "ns1"},
			Spec: openchoreov1alpha1.DataPlaneSpec{
				PlaneID: "prod",
				ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
					ClientCA: openchoreov1alpha1.ValueFrom{Value: caPEM},
				},
			},
		}
		if terminating {
			now := metav1.Now()
			dp.DeletionTimestamp = &now
			dp.Finalizers = []string{"openchoreo.dev/dataplane-cleanup"}
		}
		return dp
	}

	for _, tc := range []struct {
		name        string
		terminating bool
		deleted     bool // the CR's "deleted" event has already been applied
		wantGranted int
	}{
		{name: "live CR is granted", wantGranted: 1},
		{name: "terminating CR is granted until its deleted event", terminating: true, wantGranted: 1},
		{name: "terminating CR is not granted after its deleted event", terminating: true, deleted: true, wantGranted: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux, cm := newTestPlaneAPI(t, newPlane(tc.terminating))
			_, err := cm.Register("dataplane", "prod", &mockGatewayConn{}, []string{"ns2/dp"}, clientCert, nil)
			require.NoError(t, err)

			notify := func(event string) *httptest.ResponseRecorder {
				body, _ := json.Marshal(PlaneNotification{
					PlaneType: "dataplane", PlaneID: "prod", Event: event,
					Namespace: "ns1", Name: "dp",
				})
				req := httptest.NewRequest(http.MethodPost, "/api/v1/planes/notify", bytes.NewReader(body))
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)
				return w
			}
			if tc.deleted {
				notify("deleted")
			}
			w := notify("updated")

			var resp PlaneNotificationResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.NotNil(t, resp.AuthorizationsGranted)
			assert.Equal(t, tc.wantGranted, *resp.AuthorizationsGranted)

			_, err = cm.GetForCR("dataplane/prod", "ns1/dp")
			assert.Equal(t, tc.wantGranted == 1, err == nil)
		})
	}
}

// Once a CR's "deleted" event is applied, knowing that the CR is still
// terminating is all that stops a stale "updated" from granting it again. If
// that lookup fails the gateway cannot tell, and the CA lookup after it can
// still succeed, so it must refuse instead of granting. The 503 makes the
// controller retry.
func TestHandlePlaneNotification_UpdatedIsRefusedWhenTerminationLookupFails(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	plane := terminatingDataPlane(string(encodeCertToPEM(t, caCert)))

	cm := NewConnectionManager(testLogger())
	server := &Server{connMgr: cm, k8sClient: getFailingClient(plane), logger: testLogger()}
	mux := http.NewServeMux()
	NewPlaneAPI(cm, server, testLogger()).RegisterRoutes(mux)

	_, err := cm.Register("dataplane", "prod", &mockGatewayConn{}, []string{"ns2/dp"}, clientCert, nil)
	require.NoError(t, err)

	notify := func(event string) int {
		body, _ := json.Marshal(PlaneNotification{
			PlaneType: "dataplane", PlaneID: "prod", Event: event,
			Namespace: "ns1", Name: "dp",
		})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/planes/notify", bytes.NewReader(body)))
		return w.Code
	}
	require.Equal(t, http.StatusOK, notify("deleted"))

	assert.Equal(t, http.StatusServiceUnavailable, notify("updated"))
	_, err = cm.GetForCR("dataplane/prod", "ns1/dp")
	assert.Error(t, err, "the revoked CR must not be granted again")
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
	_, err := cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

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
	_, err := cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

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
	_, err := cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

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
	_, err := cm.Register("dataplane", "prod", conn, []string{"/global-dp"}, nil, nil)
	require.NoError(t, err)

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

	_, err := cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)
	_, err = cm.Register("workflowplane", "ci", conn2, []string{"ns/wp1"}, nil, nil)
	require.NoError(t, err)

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

func TestHandlePlaneNotification_NilCAData(t *testing.T) {
	// DataPlane exists but has no CA configured (empty ClusterAgent)
	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp1", Namespace: "ns"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID:      "prod",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
		},
	}

	mux, _ := newTestPlaneAPI(t, dp)

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

	// CR exists but has no CA → fetchCRClientCA returns error
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleReconnect_NoConnections(t *testing.T) {
	mux, _ := newTestPlaneAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/planes/dataplane/prod/reconnect", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp PlaneReconnectResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, 0, resp.DisconnectedAgents)
}

func TestHandleGetPlaneStatus_NotFound(t *testing.T) {
	mux, _ := newTestPlaneAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/planes/dataplane/nonexistent/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var status PlaneConnectionStatus
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
	assert.False(t, status.Connected)
	assert.Equal(t, 0, status.ConnectedAgents)
}

func TestHandleGetAllPlaneStatus_Empty(t *testing.T) {
	mux, _ := newTestPlaneAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/planes/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp AllPlaneStatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Total)
}

func TestHandlePlaneNotification_RevalidationError(t *testing.T) {
	// DataPlane with invalid inline CA → revalidation will fail to parse PEM
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ca", Namespace: "ns"},
		Data:       map[string][]byte{"ca.crt": []byte("valid-data")},
	}
	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp1", Namespace: "ns"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "prod",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					SecretKeyRef: &openchoreov1alpha1.SecretKeyReference{Name: "ca", Key: "ca.crt"},
				},
			},
		},
	}

	mux, cm := newTestPlaneAPI(t, caSecret, dp)

	// Register a connection so RevalidateCR is actually called (with no client cert → will fail)
	conn, cleanup := newTestWSConn(t)
	defer cleanup()
	_, err := cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

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

	// RevalidateCR will fail because "valid-data" is not valid PEM
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
