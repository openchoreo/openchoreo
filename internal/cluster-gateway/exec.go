// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

// streamSession tracks a bidirectional streaming exec session through the agent tunnel.
type streamSession struct {
	requestID string
	// fromAgent receives stream chunks from the agent (stdout/stderr)
	fromAgent chan *messaging.HTTPTunnelStreamChunk
	done      chan struct{}
	once      sync.Once
}

func (s *streamSession) close() {
	s.once.Do(func() { close(s.done) })
}

// handleExec handles the exec WebSocket endpoint.
// URL: /api/exec/{planeType}/{planeID}/{crNamespace}/{crName}?podNamespace=...&podName=...&container=...&command=...&tty=...&stdin=...
func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	requestID := getOrGenerateRequestID(r)
	logger := s.logger.With("requestId", requestID)

	// Parse URL: /api/exec/{planeType}/{planeID}/{crNamespace}/{crName}
	path := strings.TrimPrefix(r.URL.Path, "/api/exec/")
	parts := strings.SplitN(path, "/", 4)
	if len(parts) < 4 {
		http.Error(w, "invalid exec URL: expected /api/exec/{planeType}/{planeID}/{crNamespace}/{crName}", http.StatusBadRequest)
		return
	}
	planeType := parts[0]
	planeID := parts[1]
	crNamespace := parts[2]
	crName := parts[3]

	query := r.URL.Query()
	podNamespace := query.Get("podNamespace")
	podName := query.Get("podName")

	if podNamespace == "" || podName == "" {
		http.Error(w, "podNamespace and podName query parameters are required", http.StatusBadRequest)
		return
	}

	planeIdentifier := fmt.Sprintf("%s/%s", planeType, planeID)
	if crNamespace == crNamespaceClusterPlaceholder {
		crNamespace = ""
	}
	crKey := fmt.Sprintf("%s/%s", crNamespace, crName)

	logger.Info("Exec request received",
		"plane", planeIdentifier,
		"cr", crKey,
		"podNamespace", podNamespace,
		"podName", podName,
	)

	// Verify agent connection exists for the plane/CR
	conn, err := s.connMgr.GetForCR(planeIdentifier, crKey)
	if err != nil {
		logger.Warn("No agent available for exec", "error", err)
		http.Error(w, fmt.Sprintf("no agent available: %v", err), http.StatusServiceUnavailable)
		return
	}

	// Upgrade the API server connection to WebSocket
	apiConn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("Failed to upgrade exec to WebSocket", "error", err)
		return
	}
	defer apiConn.Close()

	// Build the exec path for the K8s API
	container := query.Get("container")
	commands := query["command"]
	tty := query.Get("tty") == "true"
	stdin := query.Get("stdin") == "true"

	k8sExecPath := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/exec", podNamespace, podName)
	k8sQuery := buildK8sExecQuery(container, commands, tty, stdin)

	// Create stream session
	session := &streamSession{
		requestID: requestID,
		fromAgent: make(chan *messaging.HTTPTunnelStreamChunk, 256),
		done:      make(chan struct{}),
	}

	// Register the session for receiving stream responses from the agent
	s.registerStreamSession(requestID, session)
	defer s.unregisterStreamSession(requestID)

	// Send HTTPTunnelStreamInit to the agent
	streamInit := &messaging.HTTPTunnelStreamInit{
		RequestID:    requestID,
		Target:       "k8s",
		Method:       "POST",
		Path:         k8sExecPath,
		Query:        k8sQuery,
		IsUpgrade:    true,
		UpgradeProto: "SPDY/3.1",
	}

	initData, err := json.Marshal(streamInit)
	if err != nil {
		logger.Error("Failed to marshal stream init", "error", err)
		return
	}

	if err := conn.SendRawMessage(initData); err != nil {
		logger.Error("Failed to send stream init to agent", "error", err)
		return
	}

	logger.Info("Exec stream init sent to agent")

	// Wait for stream response (the agent acknowledges the exec started)
	select {
	case chunk := <-session.fromAgent:
		if chunk == nil {
			logger.Error("Stream session closed before exec started")
			return
		}
		// First chunk is the stream response - check if it's an error
		if chunk.IsClose {
			logger.Warn("Agent closed stream immediately", "data", string(chunk.Data))
			return
		}
		// Forward initial data if any
		if len(chunk.Data) > 0 {
			if err := apiConn.WriteMessage(websocket.BinaryMessage, chunk.Data); err != nil {
				return
			}
		}
	case <-time.After(30 * time.Second):
		logger.Error("Timeout waiting for agent to start exec")
		return
	case <-session.done:
		return
	}

	// Bidirectional streaming: apiConn ↔ agent (via session channels)

	// API server → agent (stdin, resize)
	go func() {
		defer session.close()
		for {
			_, msg, err := apiConn.ReadMessage()
			if err != nil {
				// Client disconnected — notify the agent so it can stop the exec.
				closeChunk, _ := json.Marshal(&messaging.HTTPTunnelStreamChunk{
					RequestID: requestID,
					IsClose:   true,
				})
				_ = conn.SendRawMessage(closeChunk)
				return
			}
			// Forward as a stream chunk to the agent
			chunk := &messaging.HTTPTunnelStreamChunk{
				RequestID: requestID,
				Data:      msg,
				StreamID:  0, // stdin direction
			}
			chunkData, err := json.Marshal(chunk)
			if err != nil {
				return
			}
			if err := conn.SendRawMessage(chunkData); err != nil {
				return
			}
		}
	}()

	// Agent → API server (stdout, stderr)
	for {
		select {
		case chunk, ok := <-session.fromAgent:
			if !ok || chunk == nil {
				return
			}
			if chunk.IsClose {
				_ = apiConn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
			if len(chunk.Data) > 0 {
				if err := apiConn.WriteMessage(websocket.BinaryMessage, chunk.Data); err != nil {
					return
				}
			}
		case <-session.done:
			return
		}
	}
}

func buildK8sExecQuery(container string, commands []string, tty, stdin bool) string {
	params := url.Values{}
	params.Set("stdout", "true")
	params.Set("stderr", "true")
	if stdin {
		params.Set("stdin", "true")
	}
	if tty {
		params.Set("tty", "true")
	}
	if container != "" {
		params.Set("container", container)
	}
	for _, cmd := range commands {
		params.Add("command", cmd)
	}
	return params.Encode()
}

// registerStreamSession registers a stream session for receiving agent responses.
func (s *Server) registerStreamSession(requestID string, session *streamSession) {
	s.streamSessionsMu.Lock()
	defer s.streamSessionsMu.Unlock()
	s.pendingStreamSessions[requestID] = session
}

// unregisterStreamSession removes a stream session.
func (s *Server) unregisterStreamSession(requestID string) {
	s.streamSessionsMu.Lock()
	defer s.streamSessionsMu.Unlock()
	delete(s.pendingStreamSessions, requestID)
}

// handleStreamChunk routes an incoming stream chunk from an agent to the correct session.
func (s *Server) handleStreamChunk(chunk *messaging.HTTPTunnelStreamChunk) {
	s.streamSessionsMu.RLock()
	session, ok := s.pendingStreamSessions[chunk.RequestID]
	s.streamSessionsMu.RUnlock()

	if !ok {
		s.logger.Warn("Received stream chunk for unknown session", "requestID", chunk.RequestID)
		return
	}

	select {
	case session.fromAgent <- chunk:
	case <-session.done:
	case <-time.After(5 * time.Second):
		s.logger.Warn("Stream session backpressure timeout, closing session", "requestID", chunk.RequestID)
		session.close()
	}
}
