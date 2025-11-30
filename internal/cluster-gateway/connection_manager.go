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
type AgentConnection struct {
	ID              string // Unique connection identifier
	Conn            *websocket.Conn
	PlaneIdentifier string // Composite identifier: {planeType}/{planeName}
	ConnectedAt     time.Time
	LastSeen        time.Time
	mu              sync.Mutex
}

// ConnectionManager manages active agent connections
// Supports multiple concurrent connections per plane identifier for HA
type ConnectionManager struct {
	connections map[string][]*AgentConnection // key: planeIdentifier, value: slice of connections
	mu          sync.RWMutex
	roundRobin  map[string]int // Track round-robin index per planeIdentifier
	logger      *slog.Logger
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
// Multiple agent replicas (for HA) for the same plane share the same planeIdentifier
// Returns the connection ID which should be used for unregistration
func (cm *ConnectionManager) Register(planeIdentifier string, conn *websocket.Conn) (string, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	connID := uuid.New().String()

	now := time.Now()
	newConn := &AgentConnection{
		ID:              connID,
		Conn:            conn,
		PlaneIdentifier: planeIdentifier,
		ConnectedAt:     now,
		LastSeen:        now,
	}

	cm.connections[planeIdentifier] = append(cm.connections[planeIdentifier], newConn)

	totalForPlane := len(cm.connections[planeIdentifier])
	totalConnections := cm.countAllConnections()

	cm.logger.Info("agent registered",
		"planeIdentifier", planeIdentifier,
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

func (cm *ConnectionManager) SendMessage(planeIdentifier string, msg *messaging.Message) error {
	conn, err := cm.Get(planeIdentifier)
	if err != nil {
		return err
	}

	return conn.SendMessage(msg)
}

func (cm *ConnectionManager) SendClusterAgentRequest(planeIdentifier string, req *messaging.ClusterAgentRequest) error {
	conn, err := cm.Get(planeIdentifier)
	if err != nil {
		return err
	}

	return conn.SendClusterAgentRequest(req)
}

func (cm *ConnectionManager) BroadcastMessage(msg *messaging.Message) error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var errs []error
	for planeIdentifier, conns := range cm.connections {
		for _, conn := range conns {
			if err := conn.SendMessage(msg); err != nil {
				cm.logger.Error("failed to broadcast to agent",
					"planeIdentifier", planeIdentifier,
					"connectionID", conn.ID,
					"error", err,
				)
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to broadcast to %d agents", len(errs))
	}

	return nil
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

// SendMessage sends a message through this connection
func (ac *AgentConnection) SendMessage(msg *messaging.Message) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("SendMessage: failed to marshal message: %w", err)
	}

	if err := ac.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("SendMessage: failed to send message: %w", err)
	}

	return nil
}

// SendClusterAgentRequest sends a ClusterAgentRequest through this connection
func (ac *AgentConnection) SendClusterAgentRequest(req *messaging.ClusterAgentRequest) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("SendClusterAgentRequest: failed to marshal request: %w", err)
	}

	if err := ac.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("SendClusterAgentRequest: failed to send request: %w", err)
	}

	return nil
}

// Close closes the agent connection
func (ac *AgentConnection) Close() error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	return ac.Conn.Close()
}
