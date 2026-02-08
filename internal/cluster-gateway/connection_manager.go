// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

// AgentConnection represents an active agent connection
// Multiple agent replicas for the same plane can share the same PlaneIdentifier for HA
// One agent per physical plane handles multiple CRs with the same planeID
type AgentConnection struct {
	ID              string // Unique connection identifier
	Conn            *websocket.Conn
	PlaneType       string // e.g., "dataplane", "buildplane", "observabilityplane"
	PlaneID         string // Logical plane identifier (shared across CRs with same physical plane)
	PlaneIdentifier string // Simplified identifier: {planeType}/{planeID}
	ConnectedAt     time.Time
	LastSeen        time.Time
	ValidCRs        []string          // List of CRs (namespace/name) this connection is authorized for
	clientCert      *x509.Certificate // Client certificate for re-validation on CR updates
	mu              sync.Mutex
}

// IsValidForCR checks if this connection is authorized for the specified CR
func (ac *AgentConnection) IsValidForCR(crKey string) bool {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	return slices.Contains(ac.ValidCRs, crKey)
}

// SetValidCRs replaces the ValidCRs list (used during re-validation)
func (ac *AgentConnection) SetValidCRs(validCRs []string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.ValidCRs = validCRs
}

// GetValidCRs returns a copy of the ValidCRs list
func (ac *AgentConnection) GetValidCRs() []string {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	result := make([]string, len(ac.ValidCRs))
	copy(result, ac.ValidCRs)
	return result
}

// AddValidCR adds a CR to the ValidCRs list if not already present
func (ac *AgentConnection) AddValidCR(crKey string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if slices.Contains(ac.ValidCRs, crKey) {
		return
	}
	ac.ValidCRs = append(ac.ValidCRs, crKey)
}

// RemoveValidCR removes a CR from the ValidCRs list
func (ac *AgentConnection) RemoveValidCR(crKey string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	newValidCRs := []string{}
	for _, validCR := range ac.ValidCRs {
		if validCR != crKey {
			newValidCRs = append(newValidCRs, validCR)
		}
	}
	ac.ValidCRs = newValidCRs
}

// removeValidCRUnsafe removes a CR from the ValidCRs list
// Must be called with ac.mu lock held
func (ac *AgentConnection) removeValidCRUnsafe(crKey string) {
	newValidCRs := []string{}
	for _, validCR := range ac.ValidCRs {
		if validCR != crKey {
			newValidCRs = append(newValidCRs, validCR)
		}
	}
	ac.ValidCRs = newValidCRs
}

// UpdateCRValidity validates the connection's client certificate against the given CA pool
// and updates the CR's authorization status accordingly.
// Returns (granted=true) if authorization was newly granted, (revoked=true) if authorization was revoked.
// Returns error if certificate verification fails and authorization is revoked.
func (ac *AgentConnection) UpdateCRValidity(crKey string, certPool *x509.CertPool) (granted bool, revoked bool, err error) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	wasValid := slices.Contains(ac.ValidCRs, crKey)

	// Verify connection's client cert against CA pool
	opts := x509.VerifyOptions{
		Roots:     certPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	_, verifyErr := ac.clientCert.Verify(opts)
	isValid := (verifyErr == nil)

	if isValid && !wasValid {
		// Authorization granted
		ac.ValidCRs = append(ac.ValidCRs, crKey)
		return true, false, nil
	} else if !isValid && wasValid {
		// Authorization revoked
		ac.removeValidCRUnsafe(crKey)
		return false, true, verifyErr
	}

	// Status unchanged (still valid or still invalid)
	return false, false, nil
}

// ConnectionManager manages active agent connections
// Supports multiple concurrent connections per plane identifier for HA
// One agent per physical plane (planeID) handles multiple CRs
type ConnectionManager struct {
	// Primary index: planeIdentifier -> connections (for HA support)
	// Key format: "planeType/planeID", Value: slice of agent connections
	connections map[string][]*AgentConnection

	mu                   sync.RWMutex
	roundRobin           map[string]int // Track round-robin index per planeIdentifier
	skipClientCertVerify bool           // When true, all connections are authorized for all CRs
	logger               *slog.Logger
}

// NewConnectionManager creates a new ConnectionManager
func NewConnectionManager(logger *slog.Logger, skipClientCertVerify bool) *ConnectionManager {
	return &ConnectionManager{
		connections:          make(map[string][]*AgentConnection),
		roundRobin:           make(map[string]int),
		skipClientCertVerify: skipClientCertVerify,
		logger:               logger.With("component", "connection-manager"),
	}
}

// Register registers a new agent connection with per-CR authorization
// planeIdentifier format: {planeType}/{planeID}
// Multiple agent replicas (for HA) for the same plane share the same planeIdentifier
// validCRs contains the list of CRs (namespace/name) this connection is authorized for
// clientCert is stored for re-validation when CRs are updated
// Returns the connection ID which should be used for unregistration
func (cm *ConnectionManager) Register(
	planeType, planeID string,
	conn *websocket.Conn,
	validCRs []string,
	clientCert *x509.Certificate,
) (string, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	connID := uuid.New().String()
	planeIdentifier := fmt.Sprintf("%s/%s", planeType, planeID)

	now := time.Now()
	newConn := &AgentConnection{
		ID:              connID,
		Conn:            conn,
		PlaneType:       planeType,
		PlaneID:         planeID,
		PlaneIdentifier: planeIdentifier,
		ConnectedAt:     now,
		LastSeen:        now,
		ValidCRs:        validCRs,
		clientCert:      clientCert,
	}

	// Store by planeIdentifier (supports HA - multiple replicas)
	cm.connections[planeIdentifier] = append(cm.connections[planeIdentifier], newConn)

	totalForPlane := len(cm.connections[planeIdentifier])
	totalConnections := cm.countAllConnections()

	cm.logger.Info("agent registered",
		"planeIdentifier", planeIdentifier,
		"planeType", planeType,
		"planeID", planeID,
		"connectionID", connID,
		"validCRs", validCRs,
		"validCRCount", len(validCRs),
		"connectionsForPlane", totalForPlane,
		"totalConnections", totalConnections,
	)

	return connID, nil
}

// countAllConnections returns total number of connections across all planes
// Must be called with lock held
func (cm *ConnectionManager) countAllConnections() int {
	total := 0
	for _, conns := range cm.connections {
		total += len(conns)
	}
	return total
}

func (cm *ConnectionManager) Unregister(planeIdentifier, connID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conns, exists := cm.connections[planeIdentifier]
	if !exists {
		cm.logger.Debug("plane not found during unregister", "planeIdentifier", planeIdentifier, "connectionID", connID)
		return
	}

	for i, conn := range conns {
		if conn.ID == connID {
			// Capture CRs from the connection being removed for cleanup
			validCRs := conn.GetValidCRs()

			conn.Conn.Close()
			cm.connections[planeIdentifier] = append(conns[:i], conns[i+1:]...)

			// Clean up indices if no connections remain for this plane
			if len(cm.connections[planeIdentifier]) == 0 {
				delete(cm.connections, planeIdentifier)
				delete(cm.roundRobin, planeIdentifier)
				// Clean up all per-CR round-robin keys for this plane
				cm.cleanupPerCRRoundRobinKeys(planeIdentifier)
			} else {
				// Clean up per-CR round-robin keys for CRs that no longer have any authorized connections
				cm.cleanupOrphanedCRKeys(planeIdentifier, validCRs)
			}

			totalAll := cm.countAllConnections()
			cm.logger.Info("agent unregistered",
				"planeIdentifier", planeIdentifier,
				"connectionID", connID,
				"remainingForPlane", len(cm.connections[planeIdentifier]),
				"totalConnections", totalAll,
			)
			return
		}
	}

	cm.logger.Warn("connection not found for unregistration",
		"planeIdentifier", planeIdentifier,
		"connectionID", connID,
	)
}

// Get retrieves an agent connection by plane identifier using round-robin selection
// If multiple connections exist for the plane, it rotates between them
func (cm *ConnectionManager) Get(planeIdentifier string) (*AgentConnection, error) {
	cm.mu.Lock() // Need write lock for roundRobin update
	defer cm.mu.Unlock()

	conns, exists := cm.connections[planeIdentifier]
	if !exists || len(conns) == 0 {
		return nil, fmt.Errorf("no agents found for plane %s", planeIdentifier)
	}

	idx := cm.roundRobin[planeIdentifier] % len(conns)
	cm.roundRobin[planeIdentifier] = (idx + 1) % len(conns)

	return conns[idx], nil
}

// GetForCR retrieves an agent connection authorized for the specified CR using round-robin selection
// Only connections where the agent's certificate is valid for the requested CR are considered
// This enforces per-CR security boundaries in multi-tenant scenarios
// Returns error if no authorized connections are found
func (cm *ConnectionManager) GetForCR(planeIdentifier, crKey string) (*AgentConnection, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conns, exists := cm.connections[planeIdentifier]
	if !exists || len(conns) == 0 {
		return nil, fmt.Errorf("no agents found for plane %s", planeIdentifier)
	}

	var validConns []*AgentConnection
	for _, conn := range conns {
		// When client cert verification is skipped, all connections are considered authorized
		if cm.skipClientCertVerify || conn.IsValidForCR(crKey) {
			validConns = append(validConns, conn)
		}
	}

	if len(validConns) == 0 {
		cm.logger.Warn("no agents authorized for CR",
			"planeIdentifier", planeIdentifier,
			"requestedCR", crKey,
			"totalAgents", len(conns),
		)
		return nil, fmt.Errorf("no agents authorized for CR %s", crKey)
	}

	// Round-robin among valid connections only
	// Use CR-specific round-robin key to ensure fair distribution per CR
	rrKey := fmt.Sprintf("%s/%s", planeIdentifier, crKey)
	idx := cm.roundRobin[rrKey] % len(validConns)
	cm.roundRobin[rrKey] = (idx + 1) % len(validConns)

	selectedConn := validConns[idx]

	cm.logger.Debug("selected agent for CR",
		"planeIdentifier", planeIdentifier,
		"cr", crKey,
		"connectionID", selectedConn.ID,
		"validAgents", len(validConns),
		"totalAgents", len(conns),
	)

	return selectedConn, nil
}

func (cm *ConnectionManager) GetAll() []*AgentConnection {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var connections []*AgentConnection
	for _, conns := range cm.connections {
		connections = append(connections, conns...)
	}

	return connections
}

func (cm *ConnectionManager) SendHTTPTunnelRequest(planeIdentifier string, req *messaging.HTTPTunnelRequest) error {
	conn, err := cm.Get(planeIdentifier)
	if err != nil {
		return err
	}

	return conn.SendHTTPTunnelRequest(req)
}

// UpdateConnectionLastSeen updates the last seen time for a specific connection
func (cm *ConnectionManager) UpdateConnectionLastSeen(planeIdentifier, connID string) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if conns, exists := cm.connections[planeIdentifier]; exists {
		for _, conn := range conns {
			if conn.ID == connID {
				conn.mu.Lock()
				conn.LastSeen = time.Now()
				conn.mu.Unlock()
				return
			}
		}
	}
}

// Count returns the total number of active connections across all planes
func (cm *ConnectionManager) Count() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.countAllConnections()
}

// DisconnectAllForPlane disconnects all agent connections for a specific planeType/planeID
// This is used when a CR is deleted or updated to force agents to reconnect with new configuration
func (cm *ConnectionManager) DisconnectAllForPlane(planeType, planeID string) int {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	planeIdentifier := fmt.Sprintf("%s/%s", planeType, planeID)
	conns, exists := cm.connections[planeIdentifier]
	if !exists || len(conns) == 0 {
		cm.logger.Debug("no connections to disconnect",
			"planeType", planeType,
			"planeID", planeID,
		)
		return 0
	}

	disconnectedCount := len(conns)

	for _, conn := range conns {
		conn.Conn.Close()
		cm.logger.Info("disconnecting agent due to CR change",
			"planeType", planeType,
			"planeID", planeID,
			"connectionID", conn.ID,
		)
	}

	delete(cm.connections, planeIdentifier)
	delete(cm.roundRobin, planeIdentifier)
	// Clean up all per-CR round-robin keys for this plane
	cm.cleanupPerCRRoundRobinKeys(planeIdentifier)

	cm.logger.Info("disconnected all agents for plane",
		"planeType", planeType,
		"planeID", planeID,
		"disconnectedCount", disconnectedCount,
	)

	return disconnectedCount
}

// SendHTTPTunnelRequest sends an HTTPTunnelRequest through this connection
func (ac *AgentConnection) SendHTTPTunnelRequest(req *messaging.HTTPTunnelRequest) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("SendHTTPTunnelRequest: failed to marshal request: %w", err)
	}

	if err := ac.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("SendHTTPTunnelRequest: failed to send request: %w", err)
	}

	return nil
}

// Close closes the agent connection
func (ac *AgentConnection) Close() error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	return ac.Conn.Close()
}

// PlaneConnectionStatus holds connection status for a specific plane
type PlaneConnectionStatus struct {
	PlaneType       string    `json:"planeType"`
	PlaneID         string    `json:"planeID"`
	Connected       bool      `json:"connected"`
	ConnectedAgents int       `json:"connectedAgents"`
	LastSeen        time.Time `json:"lastSeen,omitempty"`
}

// GetPlaneStatus returns connection status for a specific plane
func (cm *ConnectionManager) GetPlaneStatus(planeType, planeID string) *PlaneConnectionStatus {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	planeIdentifier := fmt.Sprintf("%s/%s", planeType, planeID)
	conns, exists := cm.connections[planeIdentifier]

	status := &PlaneConnectionStatus{
		PlaneType:       planeType,
		PlaneID:         planeID,
		Connected:       exists && len(conns) > 0,
		ConnectedAgents: len(conns),
	}

	if len(conns) > 0 {
		// Lock the first connection to read LastSeen safely
		conns[0].mu.Lock()
		mostRecent := conns[0].LastSeen
		conns[0].mu.Unlock()

		for _, conn := range conns[1:] {
			conn.mu.Lock()
			if conn.LastSeen.After(mostRecent) {
				mostRecent = conn.LastSeen
			}
			conn.mu.Unlock()
		}
		status.LastSeen = mostRecent
	}

	return status
}

// GetCRAuthorizationStatus returns authorization status for a specific CR within a plane
// This is different from GetPlaneStatus which only checks if agents are connected to the plane
// This method checks if the connected agents are actually authorized for the specific CR (namespace/name)
func (cm *ConnectionManager) GetCRAuthorizationStatus(planeType, planeID, namespace, name string) *PlaneConnectionStatus {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	planeIdentifier := fmt.Sprintf("%s/%s", planeType, planeID)
	conns, exists := cm.connections[planeIdentifier]

	crIdentifier := fmt.Sprintf("%s/%s", namespace, name)

	status := &PlaneConnectionStatus{
		PlaneType:       planeType,
		PlaneID:         planeID,
		Connected:       false, // Will be set to true only if at least one agent is authorized for this CR
		ConnectedAgents: 0,     // Will count only agents authorized for this CR
	}

	if !exists || len(conns) == 0 {
		return status
	}

	var authorizedConnCount int
	var mostRecentLastSeen time.Time

	for _, conn := range conns {
		conn.mu.Lock()
		// When client cert verification is skipped, all connections are considered authorized
		isAuthorized := cm.skipClientCertVerify || slices.Contains(conn.ValidCRs, crIdentifier)

		if isAuthorized {
			authorizedConnCount++
			if conn.LastSeen.After(mostRecentLastSeen) {
				mostRecentLastSeen = conn.LastSeen
			}
		}
		conn.mu.Unlock()
	}

	if authorizedConnCount > 0 {
		status.Connected = true
		status.ConnectedAgents = authorizedConnCount
		status.LastSeen = mostRecentLastSeen
	}

	return status
}

// GetAllPlaneStatuses returns connection status for all planes
func (cm *ConnectionManager) GetAllPlaneStatuses() []PlaneConnectionStatus {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	statuses := make([]PlaneConnectionStatus, 0, len(cm.connections))

	for planeIdentifier, conns := range cm.connections {
		// Parse planeType and planeID from identifier
		// Format: "{planeType}/{planeID}"
		parts := splitPlaneIdentifier(planeIdentifier)
		if len(parts) != 2 {
			continue
		}

		status := PlaneConnectionStatus{
			PlaneType:       parts[0],
			PlaneID:         parts[1],
			Connected:       len(conns) > 0,
			ConnectedAgents: len(conns),
		}

		if len(conns) > 0 {
			// Lock the first connection to read LastSeen safely
			conns[0].mu.Lock()
			mostRecent := conns[0].LastSeen
			conns[0].mu.Unlock()

			for _, conn := range conns[1:] {
				conn.mu.Lock()
				if conn.LastSeen.After(mostRecent) {
					mostRecent = conn.LastSeen
				}
				conn.mu.Unlock()
			}
			status.LastSeen = mostRecent
		}

		statuses = append(statuses, status)
	}

	return statuses
}

// splitPlaneIdentifier splits "planeType/planeID" into parts
func splitPlaneIdentifier(identifier string) []string {
	// Simple split on first "/"
	for i, ch := range identifier {
		if ch == '/' {
			return []string{identifier[:i], identifier[i+1:]}
		}
	}
	return []string{identifier}
}

// RevalidateCR re-validates all connections for a specific CR without disconnecting
// This is called when a CR is updated (e.g., certificate rotation) to enforce new security policy
// without causing service disruption. Returns counts of authorizations granted and revoked.
func (cm *ConnectionManager) RevalidateCR(
	planeType, planeID, crNamespace, crName string,
	newCAData []byte,
) (updated int, removed int, err error) {
	cm.mu.RLock()
	planeIdentifier := fmt.Sprintf("%s/%s", planeType, planeID)
	conns, exists := cm.connections[planeIdentifier]
	// Make a copy of the connections slice to safely iterate without holding the lock
	// This prevents race conditions if another goroutine modifies the slice during iteration
	connsCopy := make([]*AgentConnection, len(conns))
	copy(connsCopy, conns)
	cm.mu.RUnlock()

	if !exists || len(connsCopy) == 0 {
		cm.logger.Debug("no connections to revalidate",
			"planeType", planeType,
			"planeID", planeID,
			"cr", fmt.Sprintf("%s/%s", crNamespace, crName),
		)
		return 0, 0, nil
	}

	crKey := fmt.Sprintf("%s/%s", crNamespace, crName)

	// Parse new CA certificate
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(newCAData) {
		return 0, 0, fmt.Errorf("failed to parse new CA certificate")
	}

	cm.logger.Info("revalidating connections for CR",
		"planeType", planeType,
		"planeID", planeID,
		"cr", crKey,
		"totalConnections", len(connsCopy),
	)

	// Re-validate each connection using the extracted UpdateCRValidity method
	for _, conn := range connsCopy {
		granted, revoked, verifyErr := conn.UpdateCRValidity(crKey, certPool)

		if granted {
			updated++
			cm.logger.Info("CR authorization granted to connection",
				"connectionID", conn.ID,
				"cr", crKey,
				"clientCN", conn.clientCert.Subject.CommonName,
			)
		} else if revoked {
			removed++
			cm.logger.Warn("CR authorization revoked from connection",
				"connectionID", conn.ID,
				"cr", crKey,
				"clientCN", conn.clientCert.Subject.CommonName,
				"reason", verifyErr,
			)
		}
		// else: status unchanged (still valid or still invalid)
	}

	cm.logger.Info("CR re-validation completed",
		"planeType", planeType,
		"planeID", planeID,
		"cr", crKey,
		"totalConnections", len(connsCopy),
		"authorizationsGranted", updated,
		"authorizationsRevoked", removed,
	)

	// If authorizations were revoked, clean up per-CR round-robin key if no connections remain authorized
	if removed > 0 {
		cm.mu.Lock()
		defer cm.mu.Unlock()

		hasAuthorizedConn := false
		if conns, exists := cm.connections[planeIdentifier]; exists {
			for _, conn := range conns {
				if conn.IsValidForCR(crKey) {
					hasAuthorizedConn = true
					break
				}
			}
		}

		if !hasAuthorizedConn {
			rrKey := fmt.Sprintf("%s/%s", planeIdentifier, crKey)
			if _, exists := cm.roundRobin[rrKey]; exists {
				delete(cm.roundRobin, rrKey)
				cm.logger.Debug("cleaned up per-CR round-robin key after revocation",
					"planeIdentifier", planeIdentifier,
					"cr", crKey,
				)
			}
		}
	}

	return updated, removed, nil
}

// cleanupPerCRRoundRobinKeys removes all per-CR round-robin keys for a given plane identifier
// This should be called when all connections for a plane are removed
// Must be called with cm.mu lock held
func (cm *ConnectionManager) cleanupPerCRRoundRobinKeys(planeIdentifier string) {
	prefix := planeIdentifier + "/"
	keysToDelete := []string{}

	for key := range cm.roundRobin {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(cm.roundRobin, key)
	}

	if len(keysToDelete) > 0 {
		cm.logger.Debug("cleaned up per-CR round-robin keys",
			"planeIdentifier", planeIdentifier,
			"keysRemoved", len(keysToDelete),
		)
	}
}

// cleanupOrphanedCRKeys removes per-CR round-robin keys for CRs that no longer have any authorized connections
// This should be called when a connection is removed but other connections remain for the plane
// Must be called with cm.mu lock held
func (cm *ConnectionManager) cleanupOrphanedCRKeys(planeIdentifier string, removedCRs []string) {
	if len(removedCRs) == 0 {
		return
	}

	remainingConns := cm.connections[planeIdentifier]
	keysToDelete := []string{}

	// For each CR that was on the removed connection, check if any remaining connections have it
	for _, crKey := range removedCRs {
		hasAuthorizedConn := false
		for _, conn := range remainingConns {
			if conn.IsValidForCR(crKey) {
				hasAuthorizedConn = true
				break
			}
		}

		// If no remaining connections are authorized for this CR, clean up its round-robin key
		if !hasAuthorizedConn {
			rrKey := fmt.Sprintf("%s/%s", planeIdentifier, crKey)
			if _, exists := cm.roundRobin[rrKey]; exists {
				keysToDelete = append(keysToDelete, rrKey)
				delete(cm.roundRobin, rrKey)
			}
		}
	}

	if len(keysToDelete) > 0 {
		cm.logger.Debug("cleaned up orphaned per-CR round-robin keys",
			"planeIdentifier", planeIdentifier,
			"keysRemoved", len(keysToDelete),
		)
	}
}
