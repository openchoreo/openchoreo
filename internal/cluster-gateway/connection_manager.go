// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"encoding/json"
	"fmt"
	"log/slog"
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
	mu              sync.Mutex
}

// ConnectionManager manages active agent connections
// Supports multiple concurrent connections per plane identifier for HA
// One agent per physical plane (planeID) handles multiple CRs
type ConnectionManager struct {
	// Primary index: planeIdentifier -> connections (for HA support)
	// Key format: "planeType/planeID", Value: slice of agent connections
	connections map[string][]*AgentConnection

	mu         sync.RWMutex
	roundRobin map[string]int // Track round-robin index per planeIdentifier
	logger     *slog.Logger
}

// NewConnectionManager creates a new ConnectionManager
func NewConnectionManager(logger *slog.Logger) *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[string][]*AgentConnection),
		roundRobin:  make(map[string]int),
		logger:      logger.With("component", "connection-manager"),
	}
}

// Register registers a new agent connection
// planeIdentifier format: {planeType}/{planeID}
// Multiple agent replicas (for HA) for the same plane share the same planeIdentifier
// Returns the connection ID which should be used for unregistration
func (cm *ConnectionManager) Register(planeType, planeID string, conn *websocket.Conn) (string, error) {
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
			conn.Conn.Close()
			cm.connections[planeIdentifier] = append(conns[:i], conns[i+1:]...)

			// Clean up index if no connections remain for this plane
			if len(cm.connections[planeIdentifier]) == 0 {
				delete(cm.connections, planeIdentifier)
				delete(cm.roundRobin, planeIdentifier)
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
