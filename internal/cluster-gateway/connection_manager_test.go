// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestConnectionManager creates a ConnectionManager for testing.
func newTestConnectionManager() *ConnectionManager {
	return NewConnectionManager(testLogger())
}

// newMockWSConn creates a mock WebSocket connection backed by a real HTTP test server.
// Returns the connection and a cleanup function.
func newMockWSConn(t *testing.T) (*websocket.Conn, func()) {
	t.Helper()

	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))

	wsURL := "ws" + server.URL[4:]
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	cleanup := func() {
		conn.Close()
		server.Close()
	}
	return conn, cleanup
}

// generateTestCA creates a self-signed CA certificate and key for testing.
func generateTestCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	caCert, err := x509.ParseCertificate(caBytes)
	require.NoError(t, err)
	return caCert, caKey
}

// generateTestClientCert creates a client certificate signed by the given CA.
func generateTestClientCert(t *testing.T, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) *x509.Certificate {
	t.Helper()
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "test-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, caCert, &clientKey.PublicKey, caKey)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certBytes)
	require.NoError(t, err)
	return cert
}

// encodeCertPEM encodes an x509 certificate to PEM format.
func encodeCertPEM(t *testing.T, cert *x509.Certificate) []byte {
	t.Helper()
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
}

// --- AgentConnection tests ---

func TestAgentConnection_IsValidForCR(t *testing.T) {
	ac := &AgentConnection{
		ValidCRs: []string{"ns-a/dp-1", "ns-b/dp-2"},
	}

	assert.True(t, ac.IsValidForCR("ns-a/dp-1"))
	assert.True(t, ac.IsValidForCR("ns-b/dp-2"))
	assert.False(t, ac.IsValidForCR("ns-c/dp-3"))
	assert.False(t, ac.IsValidForCR(""))
}

func TestAgentConnection_SetGetValidCRs(t *testing.T) {
	ac := &AgentConnection{}

	ac.SetValidCRs([]string{"a/b", "c/d"})
	result := ac.GetValidCRs()
	assert.Equal(t, []string{"a/b", "c/d"}, result)

	// Returned copy should not affect internal state
	result[0] = "modified"
	assert.Equal(t, []string{"a/b", "c/d"}, ac.GetValidCRs())
}

func TestAgentConnection_AddValidCR(t *testing.T) {
	ac := &AgentConnection{
		ValidCRs: []string{"ns/cr1"},
	}

	ac.AddValidCR("ns/cr2")
	assert.Equal(t, []string{"ns/cr1", "ns/cr2"}, ac.GetValidCRs())

	// Adding duplicate should be a no-op
	ac.AddValidCR("ns/cr1")
	assert.Equal(t, []string{"ns/cr1", "ns/cr2"}, ac.GetValidCRs())
}

func TestAgentConnection_RemoveValidCR(t *testing.T) {
	ac := &AgentConnection{
		ValidCRs: []string{"ns/cr1", "ns/cr2", "ns/cr3"},
	}

	ac.RemoveValidCR("ns/cr2")
	assert.Equal(t, []string{"ns/cr1", "ns/cr3"}, ac.GetValidCRs())

	// Removing non-existent CR should be a no-op
	ac.RemoveValidCR("ns/cr-nonexistent")
	assert.Equal(t, []string{"ns/cr1", "ns/cr3"}, ac.GetValidCRs())

	// Remove all
	ac.RemoveValidCR("ns/cr1")
	ac.RemoveValidCR("ns/cr3")
	assert.Empty(t, ac.GetValidCRs())
}

func TestAgentConnection_UpdateCRValidity(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	otherCACert, _ := generateTestCA(t)

	validPool := x509.NewCertPool()
	validPool.AddCert(caCert)

	invalidPool := x509.NewCertPool()
	invalidPool.AddCert(otherCACert)

	t.Run("grant authorization when cert matches new CA", func(t *testing.T) {
		ac := &AgentConnection{
			ValidCRs:   []string{},
			clientCert: clientCert,
		}
		granted, revoked, err := ac.UpdateCRValidity("ns/cr1", validPool)
		assert.True(t, granted)
		assert.False(t, revoked)
		assert.NoError(t, err)
		assert.True(t, ac.IsValidForCR("ns/cr1"))
	})

	t.Run("revoke authorization when cert does not match new CA", func(t *testing.T) {
		ac := &AgentConnection{
			ValidCRs:   []string{"ns/cr1"},
			clientCert: clientCert,
		}
		granted, revoked, err := ac.UpdateCRValidity("ns/cr1", invalidPool)
		assert.False(t, granted)
		assert.True(t, revoked)
		assert.Error(t, err)
		assert.False(t, ac.IsValidForCR("ns/cr1"))
	})

	t.Run("unchanged when still valid", func(t *testing.T) {
		ac := &AgentConnection{
			ValidCRs:   []string{"ns/cr1"},
			clientCert: clientCert,
		}
		granted, revoked, err := ac.UpdateCRValidity("ns/cr1", validPool)
		assert.False(t, granted)
		assert.False(t, revoked)
		assert.NoError(t, err)
		assert.True(t, ac.IsValidForCR("ns/cr1"))
	})

	t.Run("unchanged when still invalid", func(t *testing.T) {
		ac := &AgentConnection{
			ValidCRs:   []string{},
			clientCert: clientCert,
		}
		granted, revoked, err := ac.UpdateCRValidity("ns/cr1", invalidPool)
		assert.False(t, granted)
		assert.False(t, revoked)
		assert.NoError(t, err)
		assert.False(t, ac.IsValidForCR("ns/cr1"))
	})
}

// --- splitPlaneIdentifier tests ---

func TestSplitPlaneIdentifier(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"standard", "dataplane/prod-cluster", []string{"dataplane", "prod-cluster"}},
		{"with slashes in planeID", "buildplane/a/b", []string{"buildplane", "a/b"}},
		{"no slash", "dataplane", []string{"dataplane"}},
		{"empty", "", []string{""}},
		{"leading slash", "/planeID", []string{"", "planeID"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitPlaneIdentifier(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- ConnectionManager tests ---

func TestConnectionManager_RegisterAndCount(t *testing.T) {
	cm := newTestConnectionManager()
	wsConn, cleanup := newMockWSConn(t)
	defer cleanup()

	connID, err := cm.Register("dataplane", "prod", wsConn, []string{"ns/dp1"}, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, connID)
	assert.Equal(t, 1, cm.Count())

	// Register second connection for same plane (HA)
	wsConn2, cleanup2 := newMockWSConn(t)
	defer cleanup2()
	connID2, err := cm.Register("dataplane", "prod", wsConn2, []string{"ns/dp1"}, nil)
	require.NoError(t, err)
	assert.NotEqual(t, connID, connID2)
	assert.Equal(t, 2, cm.Count())

	// Register for a different plane
	wsConn3, cleanup3 := newMockWSConn(t)
	defer cleanup3()
	_, err = cm.Register("buildplane", "build1", wsConn3, []string{"ns/bp1"}, nil)
	require.NoError(t, err)
	assert.Equal(t, 3, cm.Count())
}

func TestConnectionManager_GetRoundRobin(t *testing.T) {
	cm := newTestConnectionManager()

	var connIDs []string
	var cleanups []func()
	for range 3 {
		ws, cleanup := newMockWSConn(t)
		cleanups = append(cleanups, cleanup)
		connID, err := cm.Register("dataplane", "prod", ws, []string{"ns/dp1"}, nil)
		require.NoError(t, err)
		connIDs = append(connIDs, connID)
	}
	defer func() {
		for _, c := range cleanups {
			c()
		}
	}()

	// Should rotate through connections in round-robin
	seen := make(map[string]int)
	for range 6 {
		conn, err := cm.Get("dataplane/prod")
		require.NoError(t, err)
		seen[conn.ID]++
	}

	for _, id := range connIDs {
		assert.Equal(t, 2, seen[id], "connection %s should be selected 2 times", id)
	}
}

func TestConnectionManager_Get_NotFound(t *testing.T) {
	cm := newTestConnectionManager()

	_, err := cm.Get("dataplane/nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no agents found")
}

func TestConnectionManager_GetForCR(t *testing.T) {
	cm := newTestConnectionManager()

	ws1, cleanup1 := newMockWSConn(t)
	defer cleanup1()
	connID1, _ := cm.Register("dataplane", "prod", ws1, []string{"ns/cr1", "ns/cr2"}, nil)

	ws2, cleanup2 := newMockWSConn(t)
	defer cleanup2()
	connID2, _ := cm.Register("dataplane", "prod", ws2, []string{"ns/cr2"}, nil)

	t.Run("cr1 should only return conn1", func(t *testing.T) {
		conn, err := cm.GetForCR("dataplane/prod", "ns/cr1")
		require.NoError(t, err)
		assert.Equal(t, connID1, conn.ID)
	})

	t.Run("cr2 should round-robin between conn1 and conn2", func(t *testing.T) {
		seen := make(map[string]int)
		for range 4 {
			conn, err := cm.GetForCR("dataplane/prod", "ns/cr2")
			require.NoError(t, err)
			seen[conn.ID]++
		}
		assert.Equal(t, 2, seen[connID1])
		assert.Equal(t, 2, seen[connID2])
	})

	t.Run("unknown cr returns ErrNoAuthorizedAgents", func(t *testing.T) {
		_, err := cm.GetForCR("dataplane/prod", "ns/cr-unknown")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrNoAuthorizedAgents))
	})

	t.Run("unknown plane returns error", func(t *testing.T) {
		_, err := cm.GetForCR("dataplane/nonexistent", "ns/cr1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no agents found")
	})
}

func TestConnectionManager_GetAll(t *testing.T) {
	cm := newTestConnectionManager()

	ws1, cleanup1 := newMockWSConn(t)
	defer cleanup1()
	ws2, cleanup2 := newMockWSConn(t)
	defer cleanup2()

	connID1, _ := cm.Register("dataplane", "prod", ws1, []string{"ns/dp1"}, nil)
	connID2, _ := cm.Register("buildplane", "build1", ws2, []string{"ns/bp1"}, nil)

	all := cm.GetAll()
	assert.Len(t, all, 2)

	connIDs := make(map[string]string)
	for _, c := range all {
		connIDs[c.PlaneIdentifier] = c.ID
	}
	assert.Equal(t, connID1, connIDs["dataplane/prod"])
	assert.Equal(t, connID2, connIDs["buildplane/build1"])
}

func TestConnectionManager_Unregister(t *testing.T) {
	cm := newTestConnectionManager()

	ws1, cleanup1 := newMockWSConn(t)
	defer cleanup1()
	ws2, cleanup2 := newMockWSConn(t)
	defer cleanup2()

	connID1, _ := cm.Register("dataplane", "prod", ws1, []string{"ns/cr1"}, nil)
	connID2, _ := cm.Register("dataplane", "prod", ws2, []string{"ns/cr1"}, nil)

	assert.Equal(t, 2, cm.Count())

	cm.Unregister("dataplane/prod", connID1)
	assert.Equal(t, 1, cm.Count())

	conn, err := cm.Get("dataplane/prod")
	require.NoError(t, err)
	assert.Equal(t, connID2, conn.ID)

	cm.Unregister("dataplane/prod", connID2)
	assert.Equal(t, 0, cm.Count())
	_, err = cm.Get("dataplane/prod")
	assert.Error(t, err)
}

func TestConnectionManager_Unregister_NonexistentPlane(t *testing.T) {
	cm := newTestConnectionManager()
	cm.Unregister("dataplane/nonexistent", "fake-id")
}

func TestConnectionManager_Unregister_NonexistentConn(t *testing.T) {
	cm := newTestConnectionManager()
	ws, cleanup := newMockWSConn(t)
	defer cleanup()
	_, _ = cm.Register("dataplane", "prod", ws, nil, nil)

	cm.Unregister("dataplane/prod", "nonexistent-conn-id")
	assert.Equal(t, 1, cm.Count())
}

func TestConnectionManager_Unregister_CleansUpOrphanedRoundRobinKeys(t *testing.T) {
	cm := newTestConnectionManager()

	// Conn1 authorized for cr1 and cr2
	ws1, cleanup1 := newMockWSConn(t)
	defer cleanup1()
	connID1, _ := cm.Register("dataplane", "prod", ws1, []string{"ns/cr1", "ns/cr2"}, nil)

	// Conn2 authorized for cr2 only
	ws2, cleanup2 := newMockWSConn(t)
	defer cleanup2()
	_, _ = cm.Register("dataplane", "prod", ws2, []string{"ns/cr2"}, nil)

	// Trigger per-CR round-robin key creation for both CRs
	_, _ = cm.GetForCR("dataplane/prod", "ns/cr1")
	_, _ = cm.GetForCR("dataplane/prod", "ns/cr2")

	_, cr1ExistsBefore := cm.roundRobin["dataplane/prod/ns/cr1"]
	_, cr2ExistsBefore := cm.roundRobin["dataplane/prod/ns/cr2"]
	assert.True(t, cr1ExistsBefore, "cr1 round-robin key should exist before unregister")
	assert.True(t, cr2ExistsBefore, "cr2 round-robin key should exist before unregister")

	// Unregister conn1: cr1 should be cleaned up (orphaned), cr2 should remain (conn2 has it)
	cm.Unregister("dataplane/prod", connID1)

	_, cr1Exists := cm.roundRobin["dataplane/prod/ns/cr1"]
	_, cr2Exists := cm.roundRobin["dataplane/prod/ns/cr2"]

	assert.False(t, cr1Exists, "orphaned cr1 round-robin key should be cleaned up")
	assert.True(t, cr2Exists, "cr2 round-robin key should remain (conn2 still authorized)")
}

func TestConnectionManager_DisconnectAllForPlane(t *testing.T) {
	cm := newTestConnectionManager()

	ws1, cleanup1 := newMockWSConn(t)
	defer cleanup1()
	ws2, cleanup2 := newMockWSConn(t)
	defer cleanup2()

	_, _ = cm.Register("dataplane", "prod", ws1, []string{"ns/cr1"}, nil)
	_, _ = cm.Register("dataplane", "prod", ws2, []string{"ns/cr1"}, nil)

	// Trigger per-CR round-robin key creation
	_, _ = cm.GetForCR("dataplane/prod", "ns/cr1")

	_, crRRExistsBefore := cm.roundRobin["dataplane/prod/ns/cr1"]
	assert.True(t, crRRExistsBefore, "per-CR round-robin key should exist before disconnect")

	count := cm.DisconnectAllForPlane("dataplane", "prod")
	assert.Equal(t, 2, count)
	assert.Equal(t, 0, cm.Count())

	// Per-CR round-robin key should be cleaned up
	_, crRRExists := cm.roundRobin["dataplane/prod/ns/cr1"]
	assert.False(t, crRRExists, "per-CR round-robin key should be cleaned up")

	count = cm.DisconnectAllForPlane("dataplane", "nonexistent")
	assert.Equal(t, 0, count)
}

func TestConnectionManager_UpdateConnectionLastSeen(t *testing.T) {
	cm := newTestConnectionManager()

	ws, cleanup := newMockWSConn(t)
	defer cleanup()
	connID, _ := cm.Register("dataplane", "prod", ws, nil, nil)

	conn, _ := cm.Get("dataplane/prod")
	initialLastSeen := conn.LastSeen

	time.Sleep(10 * time.Millisecond)
	cm.UpdateConnectionLastSeen("dataplane/prod", connID)

	assert.True(t, conn.LastSeen.After(initialLastSeen))
}

func TestConnectionManager_UpdateConnectionLastSeen_NonexistentPlane(t *testing.T) {
	cm := newTestConnectionManager()
	// Should not panic
	cm.UpdateConnectionLastSeen("dataplane/nonexistent", "fake-id")
}

func TestConnectionManager_GetPlaneStatus(t *testing.T) {
	cm := newTestConnectionManager()

	t.Run("no connections", func(t *testing.T) {
		status := cm.GetPlaneStatus("dataplane", "prod")
		assert.Equal(t, "dataplane", status.PlaneType)
		assert.Equal(t, "prod", status.PlaneID)
		assert.False(t, status.Connected)
		assert.Equal(t, 0, status.ConnectedAgents)
		assert.True(t, status.LastSeen.IsZero())
	})

	t.Run("with connections returns most recent LastSeen", func(t *testing.T) {
		ws1, cleanup1 := newMockWSConn(t)
		defer cleanup1()
		ws2, cleanup2 := newMockWSConn(t)
		defer cleanup2()
		connID1, _ := cm.Register("dataplane", "prod", ws1, nil, nil)
		_, _ = cm.Register("dataplane", "prod", ws2, nil, nil)

		// Advance conn1's LastSeen so it's strictly newer than conn2
		time.Sleep(10 * time.Millisecond)
		beforeUpdate := time.Now()
		cm.UpdateConnectionLastSeen("dataplane/prod", connID1)

		status := cm.GetPlaneStatus("dataplane", "prod")
		assert.True(t, status.Connected)
		assert.Equal(t, 2, status.ConnectedAgents)
		// LastSeen should reflect conn1's updated timestamp, not conn2's older one
		assert.False(t, status.LastSeen.Before(beforeUpdate))
	})
}

func TestConnectionManager_GetCRAuthorizationStatus(t *testing.T) {
	cm := newTestConnectionManager()

	ws1, cleanup1 := newMockWSConn(t)
	defer cleanup1()
	ws2, cleanup2 := newMockWSConn(t)
	defer cleanup2()

	_, _ = cm.Register("dataplane", "prod", ws1, []string{"ns/cr1", "ns/cr2"}, nil)
	_, _ = cm.Register("dataplane", "prod", ws2, []string{"ns/cr2"}, nil)

	t.Run("cr1 authorized by 1 connection", func(t *testing.T) {
		status := cm.GetCRAuthorizationStatus("dataplane", "prod", "ns", "cr1")
		assert.Equal(t, "dataplane", status.PlaneType)
		assert.Equal(t, "prod", status.PlaneID)
		assert.True(t, status.Connected)
		assert.Equal(t, 1, status.ConnectedAgents)
		assert.False(t, status.LastSeen.IsZero())
	})

	t.Run("cr2 authorized by 2 connections", func(t *testing.T) {
		status := cm.GetCRAuthorizationStatus("dataplane", "prod", "ns", "cr2")
		assert.True(t, status.Connected)
		assert.Equal(t, 2, status.ConnectedAgents)
		assert.False(t, status.LastSeen.IsZero())
	})

	t.Run("unknown CR not authorized", func(t *testing.T) {
		status := cm.GetCRAuthorizationStatus("dataplane", "prod", "ns", "cr-unknown")
		assert.False(t, status.Connected)
		assert.Equal(t, 0, status.ConnectedAgents)
		assert.True(t, status.LastSeen.IsZero())
	})

	t.Run("nonexistent plane", func(t *testing.T) {
		status := cm.GetCRAuthorizationStatus("dataplane", "nonexistent", "ns", "cr1")
		assert.False(t, status.Connected)
		assert.Equal(t, 0, status.ConnectedAgents)
	})

	t.Run("cluster-scoped CR with empty namespace", func(t *testing.T) {
		ws3, cleanup3 := newMockWSConn(t)
		defer cleanup3()
		_, _ = cm.Register("dataplane", "prod", ws3, []string{"/cluster-dp"}, nil)

		status := cm.GetCRAuthorizationStatus("dataplane", "prod", "", "cluster-dp")
		assert.True(t, status.Connected)
		assert.Equal(t, 1, status.ConnectedAgents)
	})
}

func TestConnectionManager_GetAllPlaneStatuses(t *testing.T) {
	cm := newTestConnectionManager()

	assert.Empty(t, cm.GetAllPlaneStatuses())

	ws1, cleanup1 := newMockWSConn(t)
	defer cleanup1()
	ws2, cleanup2 := newMockWSConn(t)
	defer cleanup2()

	_, _ = cm.Register("dataplane", "prod", ws1, nil, nil)
	_, _ = cm.Register("buildplane", "build1", ws2, nil, nil)

	statuses := cm.GetAllPlaneStatuses()
	assert.Len(t, statuses, 2)

	statusMap := make(map[string]PlaneConnectionStatus)
	for _, s := range statuses {
		statusMap[s.PlaneType+"/"+s.PlaneID] = s
	}

	assert.True(t, statusMap["dataplane/prod"].Connected)
	assert.Equal(t, 1, statusMap["dataplane/prod"].ConnectedAgents)
	assert.True(t, statusMap["buildplane/build1"].Connected)
	assert.Equal(t, 1, statusMap["buildplane/build1"].ConnectedAgents)
}

func TestConnectionManager_RevalidateCR(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	caPEM := encodeCertPEM(t, caCert)

	t.Run("grant authorization with matching CA", func(t *testing.T) {
		cm := newTestConnectionManager()
		ws, cleanup := newMockWSConn(t)
		defer cleanup()

		_, _ = cm.Register("dataplane", "prod", ws, []string{}, clientCert)

		updated, removed, err := cm.RevalidateCR("dataplane", "prod", "ns", "cr1", caPEM)
		require.NoError(t, err)
		assert.Equal(t, 1, updated)
		assert.Equal(t, 0, removed)
	})

	t.Run("revoke authorization with non-matching CA", func(t *testing.T) {
		cm := newTestConnectionManager()
		ws, cleanup := newMockWSConn(t)
		defer cleanup()

		_, _ = cm.Register("dataplane", "prod", ws, []string{"ns/cr1"}, clientCert)

		otherCACert, _ := generateTestCA(t)
		otherCAPEM := encodeCertPEM(t, otherCACert)

		updated, removed, err := cm.RevalidateCR("dataplane", "prod", "ns", "cr1", otherCAPEM)
		require.NoError(t, err)
		assert.Equal(t, 0, updated)
		assert.Equal(t, 1, removed)
	})

	t.Run("revocation cleans up per-CR round-robin key", func(t *testing.T) {
		cm := newTestConnectionManager()
		ws, cleanup := newMockWSConn(t)
		defer cleanup()

		_, _ = cm.Register("dataplane", "prod", ws, []string{"ns/cr1"}, clientCert)

		// Trigger per-CR round-robin key
		_, _ = cm.GetForCR("dataplane/prod", "ns/cr1")

		otherCACert, _ := generateTestCA(t)
		otherCAPEM := encodeCertPEM(t, otherCACert)

		_, _, _ = cm.RevalidateCR("dataplane", "prod", "ns", "cr1", otherCAPEM)

		_, exists := cm.roundRobin["dataplane/prod/ns/cr1"]
		assert.False(t, exists, "per-CR round-robin key should be cleaned up after revocation")
	})

	t.Run("no connections returns zero", func(t *testing.T) {
		cm := newTestConnectionManager()
		updated, removed, err := cm.RevalidateCR("dataplane", "prod", "ns", "cr1", caPEM)
		require.NoError(t, err)
		assert.Equal(t, 0, updated)
		assert.Equal(t, 0, removed)
	})

	t.Run("invalid CA data returns error", func(t *testing.T) {
		cm := newTestConnectionManager()
		ws, cleanup := newMockWSConn(t)
		defer cleanup()
		_, _ = cm.Register("dataplane", "prod", ws, []string{"ns/cr1"}, clientCert)

		_, _, err := cm.RevalidateCR("dataplane", "prod", "ns", "cr1", []byte("not-pem"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse new CA certificate")
	})

	t.Run("multiple connections mixed grant and revoke", func(t *testing.T) {
		cm := newTestConnectionManager()

		// Client cert signed by caCert
		ws1, cleanup1 := newMockWSConn(t)
		defer cleanup1()
		_, _ = cm.Register("dataplane", "prod", ws1, []string{"ns/cr1"}, clientCert)

		// Client cert signed by a different CA
		otherCACert, otherCAKey := generateTestCA(t)
		otherClientCert := generateTestClientCert(t, otherCACert, otherCAKey)
		ws2, cleanup2 := newMockWSConn(t)
		defer cleanup2()
		_, _ = cm.Register("dataplane", "prod", ws2, []string{}, otherClientCert)

		// Revalidate with the original CA — conn1 stays valid, conn2 stays invalid
		updated, removed, err := cm.RevalidateCR("dataplane", "prod", "ns", "cr1", caPEM)
		require.NoError(t, err)
		assert.Equal(t, 0, updated) // conn1 already had cr1
		assert.Equal(t, 0, removed) // conn2 never had cr1

		// Revalidate with other CA — conn1 loses cr1, conn2 gains cr1
		otherCAPEM := encodeCertPEM(t, otherCACert)
		updated, removed, err = cm.RevalidateCR("dataplane", "prod", "ns", "cr1", otherCAPEM)
		require.NoError(t, err)
		assert.Equal(t, 1, updated) // conn2 gains cr1
		assert.Equal(t, 1, removed) // conn1 loses cr1
	})
}
