// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestWSConn creates a real WebSocket connection for testing.
// Returns the client-side connection and a cleanup function.
func newTestWSConn(t *testing.T) (*websocket.Conn, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	return conn, func() { conn.Close(); srv.Close() }
}

// generateTestCA creates a self-signed CA certificate and key for testing.
func generateTestCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	return caCert, caKey
}

// generateTestClientCert creates a client certificate signed by the given CA.
func generateTestClientCert(t *testing.T, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) *x509.Certificate {
	t.Helper()
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "Test Client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, &clientKey.PublicKey, caKey)
	require.NoError(t, err)

	clientCert, err := x509.ParseCertificate(clientDER)
	require.NoError(t, err)

	return clientCert
}

// encodeCertToPEM encodes a certificate to PEM format for testing.
func encodeCertToPEM(t *testing.T, cert *x509.Certificate) []byte {
	t.Helper()
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
}

// --- AgentConnection Tests ---

func TestAgentConnection_IsValidForCR(t *testing.T) {
	ac := &AgentConnection{
		ValidCRs: []string{"ns-a/dp1", "ns-b/dp2"},
	}

	assert.True(t, ac.IsValidForCR("ns-a/dp1"))
	assert.True(t, ac.IsValidForCR("ns-b/dp2"))
	assert.False(t, ac.IsValidForCR("ns-c/dp3"))
	assert.False(t, ac.IsValidForCR(""))
}

func TestAgentConnection_SetValidCRs(t *testing.T) {
	ac := &AgentConnection{
		ValidCRs: []string{"old-cr"},
	}

	ac.SetValidCRs([]string{"new-cr-1", "new-cr-2"})
	assert.Equal(t, []string{"new-cr-1", "new-cr-2"}, ac.GetValidCRs())
}

func TestAgentConnection_GetValidCRs(t *testing.T) {
	ac := &AgentConnection{
		ValidCRs: []string{"ns/dp1", "ns/dp2"},
	}

	result := ac.GetValidCRs()
	assert.Equal(t, []string{"ns/dp1", "ns/dp2"}, result)

	// Mutating the copy should not affect the original
	result[0] = "mutated"
	assert.Equal(t, "ns/dp1", ac.GetValidCRs()[0])
}

func TestAgentConnection_AddValidCR(t *testing.T) {
	ac := &AgentConnection{
		ValidCRs: []string{"ns/dp1"},
	}

	ac.AddValidCR("ns/dp2")
	assert.Equal(t, []string{"ns/dp1", "ns/dp2"}, ac.GetValidCRs())

	// Adding duplicate is a no-op
	ac.AddValidCR("ns/dp1")
	assert.Len(t, ac.GetValidCRs(), 2)
}

func TestAgentConnection_RemoveValidCR(t *testing.T) {
	ac := &AgentConnection{
		ValidCRs: []string{"ns/dp1", "ns/dp2", "ns/dp3"},
	}

	ac.RemoveValidCR("ns/dp2")
	assert.Equal(t, []string{"ns/dp1", "ns/dp3"}, ac.GetValidCRs())

	// Removing absent CR is a no-op
	ac.RemoveValidCR("ns/dp99")
	assert.Len(t, ac.GetValidCRs(), 2)
}

func TestAgentConnection_UpdateCRValidity(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)

	// A different CA that did NOT sign the client cert
	otherCACert, _ := generateTestCA(t)

	t.Run("grant - cert valid, CR not in list", func(t *testing.T) {
		ac := &AgentConnection{
			ValidCRs:   []string{},
			clientCert: clientCert,
		}

		pool := x509.NewCertPool()
		pool.AddCert(caCert)

		granted, revoked, err := ac.UpdateCRValidity("ns/dp1", pool)
		assert.True(t, granted)
		assert.False(t, revoked)
		assert.NoError(t, err)
		assert.Contains(t, ac.GetValidCRs(), "ns/dp1")
	})

	t.Run("revoke - cert invalid, CR in list", func(t *testing.T) {
		ac := &AgentConnection{
			ValidCRs:   []string{"ns/dp1"},
			clientCert: clientCert,
		}

		pool := x509.NewCertPool()
		pool.AddCert(otherCACert) // Different CA

		granted, revoked, err := ac.UpdateCRValidity("ns/dp1", pool)
		assert.False(t, granted)
		assert.True(t, revoked)
		assert.Error(t, err)
		assert.NotContains(t, ac.GetValidCRs(), "ns/dp1")
	})

	t.Run("unchanged - cert valid, CR already in list", func(t *testing.T) {
		ac := &AgentConnection{
			ValidCRs:   []string{"ns/dp1"},
			clientCert: clientCert,
		}

		pool := x509.NewCertPool()
		pool.AddCert(caCert)

		granted, revoked, err := ac.UpdateCRValidity("ns/dp1", pool)
		assert.False(t, granted)
		assert.False(t, revoked)
		assert.NoError(t, err)
	})

	t.Run("unchanged - cert invalid, CR not in list", func(t *testing.T) {
		ac := &AgentConnection{
			ValidCRs:   []string{},
			clientCert: clientCert,
		}

		pool := x509.NewCertPool()
		pool.AddCert(otherCACert)

		granted, revoked, err := ac.UpdateCRValidity("ns/dp1", pool)
		assert.False(t, granted)
		assert.False(t, revoked)
		assert.NoError(t, err)
	})
}

// --- ConnectionManager Tests ---

func TestConnectionManager_Register(t *testing.T) {
	cm := NewConnectionManager(testLogger())
	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	connID, err := cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, connID)
	assert.Equal(t, 1, cm.Count())
}

func TestConnectionManager_Register_HA(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()

	id1, err := cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil)
	require.NoError(t, err)
	id2, err := cm.Register("dataplane", "prod", conn2, []string{"ns/dp1"}, nil)
	require.NoError(t, err)

	assert.NotEqual(t, id1, id2)
	assert.Equal(t, 2, cm.Count())
}

func TestConnectionManager_Unregister(t *testing.T) {
	cm := NewConnectionManager(testLogger())
	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	connID, err := cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, cm.Count())

	cm.Unregister("dataplane/prod", connID)
	assert.Equal(t, 0, cm.Count())
}

func TestConnectionManager_Unregister_NotFound(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	// Should not panic
	cm.Unregister("dataplane/prod", "nonexistent-id")
	cm.Unregister("nonexistent/plane", "some-id")
}

func TestConnectionManager_Get_RoundRobin(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()

	id1, _ := cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil)
	id2, _ := cm.Register("dataplane", "prod", conn2, []string{"ns/dp1"}, nil)

	// Round-robin should alternate between connections
	got1, err := cm.Get("dataplane/prod")
	require.NoError(t, err)
	got2, err := cm.Get("dataplane/prod")
	require.NoError(t, err)

	// The two returned connections should be different
	assert.NotEqual(t, got1.ID, got2.ID)
	ids := []string{got1.ID, got2.ID}
	assert.Contains(t, ids, id1)
	assert.Contains(t, ids, id2)
}

func TestConnectionManager_Get_NotFound(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	_, err := cm.Get("dataplane/nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no agents found")
}

func TestConnectionManager_GetForCR(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()

	// conn1 is authorized for ns/dp1, conn2 for ns/dp2
	_, _ = cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil)
	id2, _ := cm.Register("dataplane", "prod", conn2, []string{"ns/dp2"}, nil)

	got, err := cm.GetForCR("dataplane/prod", "ns/dp2")
	require.NoError(t, err)
	assert.Equal(t, id2, got.ID)
}

func TestConnectionManager_GetForCR_NoneAuthorized(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	_, _ = cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil)

	_, err := cm.GetForCR("dataplane/prod", "ns/dp99")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no agents authorized for CR")
}

func TestConnectionManager_GetAll(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()

	_, _ = cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil)
	_, _ = cm.Register("workflowplane", "ci", conn2, []string{"ns/wp1"}, nil)

	all := cm.GetAll()
	assert.Len(t, all, 2)
}

func TestConnectionManager_Count(t *testing.T) {
	cm := NewConnectionManager(testLogger())
	assert.Equal(t, 0, cm.Count())

	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()

	_, _ = cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil)
	assert.Equal(t, 1, cm.Count())

	_, _ = cm.Register("workflowplane", "ci", conn2, []string{"ns/wp1"}, nil)
	assert.Equal(t, 2, cm.Count())
}

func TestConnectionManager_UpdateConnectionLastSeen(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	connID, _ := cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil)

	beforeUpdate, err := cm.Get("dataplane/prod")
	require.NoError(t, err)
	initialLastSeen := beforeUpdate.LastSeen

	// Small delay to ensure time difference
	time.Sleep(10 * time.Millisecond)

	cm.UpdateConnectionLastSeen("dataplane/prod", connID)

	afterUpdate, err := cm.Get("dataplane/prod")
	require.NoError(t, err)
	assert.True(t, afterUpdate.LastSeen.After(initialLastSeen))
}

func TestConnectionManager_DisconnectAllForPlane(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()
	conn3, cleanup3 := newTestWSConn(t)
	defer cleanup3()

	_, _ = cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil)
	_, _ = cm.Register("dataplane", "prod", conn2, []string{"ns/dp2"}, nil)
	_, _ = cm.Register("workflowplane", "ci", conn3, []string{"ns/wp1"}, nil)

	disconnected := cm.DisconnectAllForPlane("dataplane", "prod")
	assert.Equal(t, 2, disconnected)
	assert.Equal(t, 1, cm.Count()) // Only workflowplane remains

	// Disconnecting non-existent plane
	disconnected = cm.DisconnectAllForPlane("dataplane", "nonexistent")
	assert.Equal(t, 0, disconnected)
}

func TestConnectionManager_GetPlaneStatus(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	_, _ = cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil)

	status := cm.GetPlaneStatus("dataplane", "prod")
	assert.True(t, status.Connected)
	assert.Equal(t, 1, status.ConnectedAgents)
	assert.Equal(t, "dataplane", status.PlaneType)
	assert.Equal(t, "prod", status.PlaneID)
	assert.False(t, status.LastSeen.IsZero())
}

func TestConnectionManager_GetPlaneStatus_NoConnections(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	status := cm.GetPlaneStatus("dataplane", "nonexistent")
	assert.False(t, status.Connected)
	assert.Equal(t, 0, status.ConnectedAgents)
}

func TestConnectionManager_GetCRAuthorizationStatus(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()

	// conn1 authorized for ns/dp1, conn2 authorized for ns/dp2
	_, _ = cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil)
	_, _ = cm.Register("dataplane", "prod", conn2, []string{"ns/dp2"}, nil)

	// Check authorization for ns/dp1 - only 1 agent authorized
	status := cm.GetCRAuthorizationStatus("dataplane", "prod", "ns", "dp1")
	assert.True(t, status.Connected)
	assert.Equal(t, 1, status.ConnectedAgents)

	// Check authorization for unknown CR
	status = cm.GetCRAuthorizationStatus("dataplane", "prod", "ns", "dp99")
	assert.False(t, status.Connected)
	assert.Equal(t, 0, status.ConnectedAgents)
}

func TestConnectionManager_GetAllPlaneStatuses(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()

	_, _ = cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil)
	_, _ = cm.Register("workflowplane", "ci", conn2, []string{"ns/wp1"}, nil)

	statuses := cm.GetAllPlaneStatuses()
	assert.Len(t, statuses, 2)

	for _, s := range statuses {
		assert.True(t, s.Connected)
		assert.Equal(t, 1, s.ConnectedAgents)
	}
}

func TestSplitPlaneIdentifier(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"normal", "dataplane/prod", []string{"dataplane", "prod"}},
		{"with slashes in planeID", "dataplane/prod/cluster", []string{"dataplane", "prod/cluster"}},
		{"no slash", "dataplane", []string{"dataplane"}},
		{"empty", "", []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitPlaneIdentifier(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConnectionManager_RevalidateCR(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)

	// Encode CA cert to PEM for RevalidateCR
	caPEM := encodeCertToPEM(t, caCert)

	t.Run("grant authorization to new CR", func(t *testing.T) {
		cm := NewConnectionManager(testLogger())
		conn, cleanup := newTestWSConn(t)
		defer cleanup()

		// Register with no valid CRs, but with a client cert signed by the CA
		_, _ = cm.Register("dataplane", "prod", conn, []string{}, clientCert)

		updated, removed, err := cm.RevalidateCR("dataplane", "prod", "ns", "dp1", caPEM)
		require.NoError(t, err)
		assert.Equal(t, 1, updated)
		assert.Equal(t, 0, removed)
	})

	t.Run("revoke authorization with wrong CA", func(t *testing.T) {
		cm := NewConnectionManager(testLogger())
		conn, cleanup := newTestWSConn(t)
		defer cleanup()

		// Register with valid CR
		_, _ = cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, clientCert)

		// Generate a different CA
		otherCACert, _ := generateTestCA(t)
		otherPEM := encodeCertToPEM(t, otherCACert)

		updated, removed, err := cm.RevalidateCR("dataplane", "prod", "ns", "dp1", otherPEM)
		require.NoError(t, err)
		assert.Equal(t, 0, updated)
		assert.Equal(t, 1, removed)
	})

	t.Run("no connections returns zero counts", func(t *testing.T) {
		cm := NewConnectionManager(testLogger())

		updated, removed, err := cm.RevalidateCR("dataplane", "prod", "ns", "dp1", caPEM)
		require.NoError(t, err)
		assert.Equal(t, 0, updated)
		assert.Equal(t, 0, removed)
	})

	t.Run("invalid PEM returns error", func(t *testing.T) {
		cm := NewConnectionManager(testLogger())
		conn, cleanup := newTestWSConn(t)
		defer cleanup()

		_, _ = cm.Register("dataplane", "prod", conn, []string{}, clientCert)

		_, _, err := cm.RevalidateCR("dataplane", "prod", "ns", "dp1", []byte("not-valid-pem"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse")
	})
}
