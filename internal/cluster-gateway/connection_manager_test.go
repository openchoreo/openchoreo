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
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
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

// generateTestIntermediateCA creates an intermediate CA certificate signed by the
// given parent CA, for building multi-level chains in testing.
func generateTestIntermediateCA(t *testing.T, parentCert *x509.Certificate, parentKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	interKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	interTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(3),
		Subject:               pkix.Name{CommonName: "Test Intermediate CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	interDER, err := x509.CreateCertificate(rand.Reader, interTemplate, parentCert, &interKey.PublicKey, parentKey)
	require.NoError(t, err)

	interCert, err := x509.ParseCertificate(interDER)
	require.NoError(t, err)

	return interCert, interKey
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

// Regression test: when the client cert is issued by an intermediate CA and the CR's CA
// is the root only, incremental re-validation must use the handshake intermediates to
// chain to the root, otherwise CRs created after the agent connects are never authorized.
func TestAgentConnection_UpdateCRValidity_WithIntermediates(t *testing.T) {
	// root CA -> intermediate CA -> client cert
	rootCA, rootKey := generateTestCA(t)

	intermediateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	intermediateTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(20),
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

	clientCert := generateTestClientCert(t, intermediateCert, intermediateKey)

	// The CR's CA pool contains only the root (single cert, no chain).
	rootPool := x509.NewCertPool()
	rootPool.AddCert(rootCA)

	t.Run("without intermediates - cannot chain, not granted (the bug)", func(t *testing.T) {
		ac := &AgentConnection{
			ValidCRs:   []string{},
			clientCert: clientCert,
			// intermediates deliberately nil
		}

		granted, revoked, err := ac.UpdateCRValidity("ns/dp1", rootPool)
		assert.False(t, granted)
		assert.False(t, revoked)
		assert.NoError(t, err)
		assert.NotContains(t, ac.GetValidCRs(), "ns/dp1")
	})

	t.Run("with intermediates - chains to root, authorization granted (the fix)", func(t *testing.T) {
		ac := &AgentConnection{
			ValidCRs:      []string{},
			clientCert:    clientCert,
			intermediates: buildIntermediatePool([]*x509.Certificate{intermediateCert}),
		}

		granted, revoked, err := ac.UpdateCRValidity("ns/dp1", rootPool)
		assert.True(t, granted)
		assert.False(t, revoked)
		assert.NoError(t, err)
		assert.Contains(t, ac.GetValidCRs(), "ns/dp1")
	})

	t.Run("no intermediates - client signed directly by CR CA, still granted", func(t *testing.T) {
		// Common case: client cert issued directly by the CR's CA, no handshake
		// intermediates. Behavior must be unchanged by the fix.
		directCA, directKey := generateTestCA(t)
		directClient := generateTestClientCert(t, directCA, directKey)

		directPool := x509.NewCertPool()
		directPool.AddCert(directCA)

		ac := &AgentConnection{
			ValidCRs:      []string{},
			clientCert:    directClient,
			intermediates: buildIntermediatePool(nil), // nil - no handshake intermediates
		}
		require.Nil(t, ac.intermediates)

		granted, revoked, err := ac.UpdateCRValidity("ns/dp1", directPool)
		assert.True(t, granted)
		assert.False(t, revoked)
		assert.NoError(t, err)
		assert.Contains(t, ac.GetValidCRs(), "ns/dp1")
	})
}

func TestBuildIntermediatePool(t *testing.T) {
	assert.Nil(t, buildIntermediatePool(nil), "nil slice should yield a nil pool")
	assert.Nil(t, buildIntermediatePool([]*x509.Certificate{}), "empty slice should yield a nil pool")

	caCert, _ := generateTestCA(t)
	assert.NotNil(t, buildIntermediatePool([]*x509.Certificate{caCert}), "non-empty slice should yield a non-nil pool")
}

// --- ConnectionManager Tests ---

func TestConnectionManager_Register(t *testing.T) {
	cm := NewConnectionManager(testLogger())
	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	connID, err := cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)
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

	id1, err := cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)
	id2, err := cm.Register("dataplane", "prod", conn2, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

	assert.NotEqual(t, id1, id2)
	assert.Equal(t, 2, cm.Count())
}

func TestConnectionManager_Unregister(t *testing.T) {
	cm := NewConnectionManager(testLogger())
	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	connID, err := cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)
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

	id1, _ := cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil, nil)
	id2, _ := cm.Register("dataplane", "prod", conn2, []string{"ns/dp1"}, nil, nil)

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
	_, _ = cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil, nil)
	id2, _ := cm.Register("dataplane", "prod", conn2, []string{"ns/dp2"}, nil, nil)

	got, err := cm.GetForCR("dataplane/prod", "ns/dp2")
	require.NoError(t, err)
	assert.Equal(t, id2, got.ID)
}

func TestConnectionManager_GetForCR_NoneAuthorized(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	_, _ = cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)

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

	_, _ = cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil, nil)
	_, _ = cm.Register("workflowplane", "ci", conn2, []string{"ns/wp1"}, nil, nil)

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

	_, _ = cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil, nil)
	assert.Equal(t, 1, cm.Count())

	_, _ = cm.Register("workflowplane", "ci", conn2, []string{"ns/wp1"}, nil, nil)
	assert.Equal(t, 2, cm.Count())
}

func TestConnectionManager_UpdateConnectionLastSeen(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	connID, _ := cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)

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

	_, _ = cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil, nil)
	_, _ = cm.Register("dataplane", "prod", conn2, []string{"ns/dp2"}, nil, nil)
	_, _ = cm.Register("workflowplane", "ci", conn3, []string{"ns/wp1"}, nil, nil)

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

	_, _ = cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)

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
	_, _ = cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil, nil)
	_, _ = cm.Register("dataplane", "prod", conn2, []string{"ns/dp2"}, nil, nil)

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

	_, _ = cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil, nil)
	_, _ = cm.Register("workflowplane", "ci", conn2, []string{"ns/wp1"}, nil, nil)

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
		_, _ = cm.Register("dataplane", "prod", conn, []string{}, clientCert, nil)

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
		_, _ = cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, clientCert, nil)

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

		_, _ = cm.Register("dataplane", "prod", conn, []string{}, clientCert, nil)

		_, _, err := cm.RevalidateCR("dataplane", "prod", "ns", "dp1", []byte("not-valid-pem"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse")
	})
}

func TestAgentConnection_Close(t *testing.T) {
	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	ac := &AgentConnection{
		Conn: conn,
	}

	err := ac.Close()
	assert.NoError(t, err)
}

func TestConnectionManager_CleanupOrphanedCRKeys(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()

	// Register two connections: conn1 for dp1+dp2, conn2 for dp2 only
	_, _ = cm.Register("dataplane", "prod", conn1, []string{"ns/dp1", "ns/dp2"}, nil, nil)
	id2, _ := cm.Register("dataplane", "prod", conn2, []string{"ns/dp2"}, nil, nil)

	// Do a GetForCR to create per-CR round-robin keys
	_, _ = cm.GetForCR("dataplane/prod", "ns/dp1")
	_, _ = cm.GetForCR("dataplane/prod", "ns/dp2")

	// Unregister conn2 — ns/dp2 still has conn1, but ns/dp1 round-robin key should persist
	cm.Unregister("dataplane/prod", id2)

	// ns/dp1 should still work (conn1 is authorized)
	got, err := cm.GetForCR("dataplane/prod", "ns/dp1")
	require.NoError(t, err)
	assert.NotNil(t, got)
}

func TestConnectionManager_Unregister_PartialCleanup(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()

	// conn1 authorized for dp1 only, conn2 authorized for dp1 + dp2
	id1, _ := cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil, nil)
	_, _ = cm.Register("dataplane", "prod", conn2, []string{"ns/dp1", "ns/dp2"}, nil, nil)

	// Access dp1 via GetForCR to create round-robin key
	_, _ = cm.GetForCR("dataplane/prod", "ns/dp1")

	// Unregister conn1 — dp1 still has conn2, round-robin key should persist
	cm.Unregister("dataplane/prod", id1)

	// dp1 should still be accessible via conn2
	got, err := cm.GetForCR("dataplane/prod", "ns/dp1")
	require.NoError(t, err)
	assert.NotNil(t, got)

	// dp2 should still be accessible
	got, err = cm.GetForCR("dataplane/prod", "ns/dp2")
	require.NoError(t, err)
	assert.NotNil(t, got)

	assert.Equal(t, 1, cm.Count())
}

func TestConnectionManager_GetPlaneStatus_MultipleConnections(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()

	_, _ = cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil, nil)

	// Small delay so conn2 has a later LastSeen
	time.Sleep(10 * time.Millisecond)
	connID2, _ := cm.Register("dataplane", "prod", conn2, []string{"ns/dp1"}, nil, nil)

	// Update conn2 LastSeen to be newer
	time.Sleep(10 * time.Millisecond)
	cm.UpdateConnectionLastSeen("dataplane/prod", connID2)

	status := cm.GetPlaneStatus("dataplane", "prod")
	assert.True(t, status.Connected)
	assert.Equal(t, 2, status.ConnectedAgents)
	assert.False(t, status.LastSeen.IsZero())
}

func TestConnectionManager_GetAllPlaneStatuses_MultipleConnsPerPlane(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()

	_, _ = cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil, nil)

	time.Sleep(10 * time.Millisecond)
	connID2, _ := cm.Register("dataplane", "prod", conn2, []string{"ns/dp2"}, nil, nil)
	time.Sleep(10 * time.Millisecond)
	cm.UpdateConnectionLastSeen("dataplane/prod", connID2)

	statuses := cm.GetAllPlaneStatuses()
	require.Len(t, statuses, 1)
	assert.Equal(t, 2, statuses[0].ConnectedAgents)
	assert.False(t, statuses[0].LastSeen.IsZero())
}

func TestConnectionManager_SendHTTPTunnelRequest_Error(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	err := cm.SendHTTPTunnelRequest("dataplane/nonexistent", &messaging.HTTPTunnelRequest{
		Target: "k8s",
		Method: "GET",
		Path:   "/api/v1/pods",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no agents found")
}

func TestConnectionManager_Unregister_CleansUpOrphanedCRKeys(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()

	// conn1 has unique CR "ns/dp-unique", conn2 has shared CR "ns/dp-shared"
	id1, _ := cm.Register("dataplane", "prod", conn1, []string{"ns/dp-unique", "ns/dp-shared"}, nil, nil)
	_, _ = cm.Register("dataplane", "prod", conn2, []string{"ns/dp-shared"}, nil, nil)

	// Access both CRs to create round-robin keys
	_, _ = cm.GetForCR("dataplane/prod", "ns/dp-unique")
	_, _ = cm.GetForCR("dataplane/prod", "ns/dp-shared")

	// Unregister conn1 → "ns/dp-unique" becomes orphaned, "ns/dp-shared" still has conn2
	cm.Unregister("dataplane/prod", id1)

	// dp-unique should no longer have authorized connections
	_, err := cm.GetForCR("dataplane/prod", "ns/dp-unique")
	assert.Error(t, err)

	// dp-shared should still work
	got, err := cm.GetForCR("dataplane/prod", "ns/dp-shared")
	require.NoError(t, err)
	assert.NotNil(t, got)
}

func TestConnectionManager_GetForCR_RoundRobin(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()

	// Both connections authorized for the same CR
	id1, _ := cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil, nil)
	id2, _ := cm.Register("dataplane", "prod", conn2, []string{"ns/dp1"}, nil, nil)

	got1, err := cm.GetForCR("dataplane/prod", "ns/dp1")
	require.NoError(t, err)
	got2, err := cm.GetForCR("dataplane/prod", "ns/dp1")
	require.NoError(t, err)

	assert.NotEqual(t, got1.ID, got2.ID)
	ids := []string{got1.ID, got2.ID}
	assert.Contains(t, ids, id1)
	assert.Contains(t, ids, id2)
}

func TestConnectionManager_Unregister_LastConnection_CleansRRKeys(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	connID, _ := cm.Register("dataplane", "prod", conn, []string{"ns/dp1", "ns/dp2"}, nil, nil)

	// Access CRs to create per-CR round-robin keys
	_, _ = cm.GetForCR("dataplane/prod", "ns/dp1")
	_, _ = cm.GetForCR("dataplane/prod", "ns/dp2")

	// Unregister the LAST connection → cleanupPerCRRoundRobinKeys should be called
	cm.Unregister("dataplane/prod", connID)

	assert.Equal(t, 0, cm.Count())

	// Plane is completely gone
	_, err := cm.Get("dataplane/prod")
	assert.Error(t, err)
}

func TestConnectionManager_DisconnectAllForPlane_CleansRRKeys(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	_, _ = cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, nil, nil)

	// Create per-CR round-robin key
	_, _ = cm.GetForCR("dataplane/prod", "ns/dp1")

	// DisconnectAll also calls cleanupPerCRRoundRobinKeys
	disconnected := cm.DisconnectAllForPlane("dataplane", "prod")
	assert.Equal(t, 1, disconnected)
	assert.Equal(t, 0, cm.Count())
}

func TestConnectionManager_RevalidateCR_UnchangedStatus(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	caPEM := encodeCertToPEM(t, caCert)

	cm := NewConnectionManager(testLogger())
	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	// Register with CR already valid
	_, _ = cm.Register("dataplane", "prod", conn, []string{"ns/dp1"}, clientCert, nil)

	// Revalidate with same CA — should be unchanged (still valid)
	updated, removed, err := cm.RevalidateCR("dataplane", "prod", "ns", "dp1", caPEM)
	require.NoError(t, err)
	assert.Equal(t, 0, updated)
	assert.Equal(t, 0, removed)
}

func TestAgentConnection_SendHTTPTunnelRequest_Success(t *testing.T) {
	conn, cleanup := newTestWSConn(t)
	defer cleanup()

	ac := &AgentConnection{
		Conn: conn,
	}

	req := &messaging.HTTPTunnelRequest{
		RequestID: "req-1",
		Target:    "k8s",
		Method:    "GET",
		Path:      "/api/v1/pods",
	}

	err := ac.SendHTTPTunnelRequest(req)
	assert.NoError(t, err)
}

func TestAgentConnection_Draining(t *testing.T) {
	ac := &AgentConnection{}
	assert.False(t, ac.IsDraining(), "new connection must not be draining")

	ac.SetDraining()
	assert.True(t, ac.IsDraining(), "connection must be marked draining after SetDraining")
}

func TestPreferLive(t *testing.T) {
	c1 := &AgentConnection{ID: "c1"}
	c2 := &AgentConnection{ID: "c2"}
	c3 := &AgentConnection{ID: "c3"}
	c2.SetDraining()

	// Mixed: returns only non-draining
	res := preferLive([]*AgentConnection{c1, c2, c3})
	assert.Equal(t, []*AgentConnection{c1, c3}, res)

	// All live: returns all
	res = preferLive([]*AgentConnection{c1, c3})
	assert.Equal(t, []*AgentConnection{c1, c3}, res)

	// All draining: falls back to all
	c1.SetDraining()
	c3.SetDraining()
	res = preferLive([]*AgentConnection{c1, c2, c3})
	assert.Equal(t, []*AgentConnection{c1, c2, c3}, res)

	// Empty
	res = preferLive([]*AgentConnection{})
	assert.Empty(t, res)
}

func TestConnectionManager_SetDraining(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	// SetDraining handles nil safely
	cm.SetDraining(nil)

	ac := &AgentConnection{ID: "c1"}
	assert.False(t, ac.IsDraining())
	cm.SetDraining(ac)
	assert.True(t, ac.IsDraining())
}

func TestConnectionManager_Get_PreferLive(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()

	id1, _ := cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil, nil)
	id2, _ := cm.Register("dataplane", "prod", conn2, []string{"ns/dp1"}, nil, nil)

	// Both live: round robin returns id1 then id2
	got1, err := cm.Get("dataplane/prod")
	require.NoError(t, err)
	assert.Equal(t, id1, got1.ID)

	// Mark conn1 as draining: Get should always pick conn2
	cm.SetDraining(got1)

	for range 3 {
		got, err := cm.Get("dataplane/prod")
		require.NoError(t, err)
		assert.Equal(t, id2, got.ID, "should exclusively select the live connection")
	}

	// Mark conn2 as draining as well: Get falls back to rotating between draining connections
	got2, err := cm.Get("dataplane/prod")
	require.NoError(t, err)
	cm.SetDraining(got2)

	seen := make(map[string]bool)
	for range 2 {
		got, err := cm.Get("dataplane/prod")
		require.NoError(t, err)
		seen[got.ID] = true
	}
	assert.True(t, seen[id1] && seen[id2], "should fall back to both connections when all are draining")
}

func TestConnectionManager_GetForCR_PreferLive(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()
	conn3, cleanup3 := newTestWSConn(t)
	defer cleanup3()

	id1, _ := cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil, nil)
	id2, _ := cm.Register("dataplane", "prod", conn2, []string{"ns/dp1"}, nil, nil)
	id3, _ := cm.Register("dataplane", "prod", conn3, []string{"ns/dp2"}, nil, nil)

	// Both conn1 and conn2 are valid for ns/dp1
	got1, err := cm.GetForCR("dataplane/prod", "ns/dp1")
	require.NoError(t, err)
	assert.Equal(t, id1, got1.ID)

	// Mark conn1 as draining: GetForCR for ns/dp1 should always select conn2
	cm.SetDraining(got1)

	for range 3 {
		got, err := cm.GetForCR("dataplane/prod", "ns/dp1")
		require.NoError(t, err)
		assert.Equal(t, id2, got.ID, "should exclusively select the live connection for CR")
	}

	// For ns/dp2, only conn3 is authorized and live
	got3, err := cm.GetForCR("dataplane/prod", "ns/dp2")
	require.NoError(t, err)
	assert.Equal(t, id3, got3.ID)

	// Mark conn2 as draining: now all authorized connections for ns/dp1 are draining
	// GetForCR should fall back to returning draining connections rather than failing
	got2, err := cm.GetForCR("dataplane/prod", "ns/dp1")
	require.NoError(t, err)
	assert.Equal(t, id2, got2.ID)
	cm.SetDraining(got2)

	fallbackGot, err := cm.GetForCR("dataplane/prod", "ns/dp1")
	require.NoError(t, err)
	assert.True(t, fallbackGot.ID == id1 || fallbackGot.ID == id2, "should fall back to draining authorized connections")
}

func TestConnectionManager_DrainConnection_AtomicWithSelection(t *testing.T) {
	cm := NewConnectionManager(testLogger())

	// c1 will be drained, c2 will remain live
	conn1, cleanup1 := newTestWSConn(t)
	defer cleanup1()
	conn2, cleanup2 := newTestWSConn(t)
	defer cleanup2()

	id1, err := cm.Register("dataplane", "prod", conn1, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)
	id2, err := cm.Register("dataplane", "prod", conn2, []string{"ns/dp1"}, nil, nil)
	require.NoError(t, err)

	goAway, err := json.Marshal(messaging.GoAway{Type: messaging.MessageTypeGoAway})
	require.NoError(t, err)

	conns := cm.GetAll()
	var c1 *AgentConnection
	for _, c := range conns {
		if c.ID == id1 {
			c1 = c
			break
		}
	}
	require.NotNil(t, c1)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var sawDrainingAfterDrain atomic.Bool

	// Concurrently call Get and GetForCR
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					got, err := cm.Get("dataplane/prod")
					if err == nil && c1.IsDraining() && got.ID == id1 {
						sawDrainingAfterDrain.Store(true)
					}
					gotCR, err := cm.GetForCR("dataplane/prod", "ns/dp1")
					if err == nil && c1.IsDraining() && gotCR.ID == id1 {
						sawDrainingAfterDrain.Store(true)
					}
				}
			}
		}()
	}

	time.Sleep(10 * time.Millisecond)
	err = cm.DrainConnection(c1, goAway)
	require.NoError(t, err)
	assert.True(t, c1.IsDraining())

	// Run selections while c1 is drained to verify only c2 is picked
	for range 50 {
		got, err := cm.Get("dataplane/prod")
		require.NoError(t, err)
		assert.Equal(t, id2, got.ID)

		gotCR, err := cm.GetForCR("dataplane/prod", "ns/dp1")
		require.NoError(t, err)
		assert.Equal(t, id2, gotCR.ID)
	}

	close(stop)
	wg.Wait()

	assert.False(t, sawDrainingAfterDrain.Load(), "selection must never return a connection after it has begun draining")
}
