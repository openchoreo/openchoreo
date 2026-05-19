// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

// handleWirelogs handles the wirelogs (Cilium Hubble flow) Server-Sent Events endpoint.
// URL: /api/wirelogs/{planeType}/{planeID}/{crNamespace}/{crName}?component=...&environment=...&namespace=...
//
// Flow:
//  1. Respond with text/event-stream; each Hubble flow JSON is emitted as a single SSE
//     `data:` frame followed by a blank line.
//  2. Send a HTTPTunnelStreamInit{Target: "hubble"} to the data-plane agent
//     authorized for the CR; the agent opens a gRPC GetFlows stream against
//     hubble-relay and forwards each flow as a HTTPTunnelStreamChunk.
//  3. Forward chunks one-way (agent → API server). When the API server's request
//     context is canceled (client disconnect / timeout) send IsClose to the agent
//     so it cancels the gRPC stream.
//
// Note: the gateway↔agent hop is still the persistent multiplexed WebSocket
// tunnel (`/ws`). Only the api↔gateway leg is SSE.
func (s *Server) handleWirelogs(w http.ResponseWriter, r *http.Request) {
	requestID := getOrGenerateRequestID(r)
	logger := s.logger.With("requestId", requestID)

	// Parse URL: /api/wirelogs/{planeType}/{planeID}/{crNamespace}/{crName}
	path := strings.TrimPrefix(r.URL.Path, "/api/wirelogs/")
	parts := strings.SplitN(path, "/", 4)
	if len(parts) < 4 {
		http.Error(w, "invalid wirelogs URL: expected /api/wirelogs/{planeType}/{planeID}/{crNamespace}/{crName}", http.StatusBadRequest)
		return
	}
	planeType := parts[0]
	planeID := parts[1]
	crNamespace := parts[2]
	crName := parts[3]

	query := r.URL.Query()
	component := query.Get("component")
	project := query.Get("project")
	environment := query.Get("environment")
	namespace := query.Get("namespace")
	if component == "" || project == "" || environment == "" || namespace == "" {
		http.Error(w, "component, project, environment, and namespace query parameters are required", http.StatusBadRequest)
		return
	}

	planeIdentifier := fmt.Sprintf("%s/%s", planeType, planeID)
	if crNamespace == "_cluster" {
		crNamespace = ""
	}
	crKey := fmt.Sprintf("%s/%s", crNamespace, crName)

	logger.Info("Wirelogs request received",
		"plane", planeIdentifier,
		"cr", crKey,
		"component", component,
		"project", project,
		"environment", environment,
	)

	flusher, ok := w.(http.Flusher)
	if !ok {
		logger.Error("ResponseWriter does not support flushing; cannot stream SSE")
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// The http.Server's WriteTimeout is an absolute deadline from when request
	// headers are read; for a long-lived SSE stream it would kill the connection
	// after that deadline regardless of activity (the classic "curl: (18) transfer
	// closed with outstanding read data remaining" once nothing has been sent for
	// a while). Clear the deadline on this connection only — other endpoints keep
	// the server's default protection.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		logger.Warn("Failed to disable write deadline for SSE stream", "error", err)
	}

	conn, err := s.connMgr.GetForCR(planeIdentifier, crKey)
	if err != nil {
		logger.Warn("No agent available for wirelogs", "error", err)
		http.Error(w, fmt.Sprintf("no agent available: %v", err), http.StatusServiceUnavailable)
		return
	}

	session := &streamSession{
		requestID: requestID,
		fromAgent: make(chan *messaging.HTTPTunnelStreamChunk, 256),
		done:      make(chan struct{}),
	}

	s.registerStreamSession(requestID, session)
	defer s.unregisterStreamSession(requestID)

	agentQuery := url.Values{}
	agentQuery.Set("component", component)
	agentQuery.Set("project", project)
	agentQuery.Set("environment", environment)
	agentQuery.Set("namespace", namespace)

	streamInit := &messaging.HTTPTunnelStreamInit{
		RequestID:    requestID,
		Target:       "hubble",
		Method:       "GET",
		Path:         "/wirelogs",
		Query:        agentQuery.Encode(),
		IsUpgrade:    true,
		UpgradeProto: "hubble/v1",
	}

	initData, err := json.Marshal(streamInit)
	if err != nil {
		logger.Error("Failed to marshal stream init", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := conn.SendRawMessage(initData); err != nil {
		logger.Error("Failed to send stream init to agent", "error", err)
		http.Error(w, fmt.Sprintf("failed to start stream: %v", err), http.StatusBadGateway)
		return
	}

	logger.Info("Wirelogs stream init sent to agent")

	// Wait for the agent's first chunk (sentinel) so we know the stream is live
	// before we commit to a 200 SSE response.
	select {
	case chunk := <-session.fromAgent:
		if chunk == nil {
			logger.Error("Stream session closed before wirelogs started")
			http.Error(w, "stream closed before start", http.StatusBadGateway)
			return
		}
		if chunk.IsClose {
			logger.Warn("Agent closed wirelogs stream immediately", "data", string(chunk.Data))
			http.Error(w, fmt.Sprintf("agent rejected stream: %s", string(chunk.Data)), http.StatusBadGateway)
			return
		}
		// Commit to SSE: write headers + flush before any data frames.
		writeSSEHeaders(w)
		flusher.Flush()
		if len(chunk.Data) > 0 {
			if !writeSSEEvent(w, flusher, chunk.Data) {
				return
			}
		}
	case <-time.After(30 * time.Second):
		logger.Error("Timeout waiting for agent to start wirelogs stream")
		http.Error(w, "timeout waiting for agent", http.StatusGatewayTimeout)
		return
	case <-r.Context().Done():
		logger.Info("Client disconnected before stream started")
		return
	case <-session.done:
		http.Error(w, "stream closed before start", http.StatusBadGateway)
		return
	}

	// Notify the agent when the client disconnects (request context canceled),
	// so it can cancel the upstream gRPC stream against hubble-relay.
	go func() {
		<-r.Context().Done()
		closeChunk, _ := json.Marshal(&messaging.HTTPTunnelStreamChunk{
			RequestID: requestID,
			IsClose:   true,
		})
		_ = conn.SendRawMessage(closeChunk)
		session.close()
	}()

	// Agent → API server: forward each flow chunk as one SSE event.
	for {
		select {
		case chunk, ok := <-session.fromAgent:
			if !ok || chunk == nil {
				return
			}
			if chunk.IsClose {
				return
			}
			if len(chunk.Data) > 0 {
				if !writeSSEEvent(w, flusher, chunk.Data) {
					return
				}
			}
		case <-session.done:
			return
		case <-r.Context().Done():
			return
		}
	}
}

// writeSSEHeaders sets the response headers required for a Server-Sent Events
// stream. Must be called before the first response byte is written.
func writeSSEHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("Connection", "keep-alive")
	// Hint to intermediate proxies (e.g. nginx) not to buffer the stream.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
}

// writeSSEEvent serializes a single payload as one `data:` SSE frame and flushes
// it. Each newline in the payload is emitted as a continuation `data:` line so
// the framing remains valid even if Hubble's protojson output ever contains a
// newline (it currently does not — `protojson.MarshalOptions` produces compact
// JSON — but the cost of being defensive is one bytes.Index call per flow).
// Returns false if the write failed and the caller should stop streaming.
func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, data []byte) bool {
	var buf bytes.Buffer
	for len(data) > 0 {
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			buf.WriteString("data: ")
			buf.Write(data)
			buf.WriteByte('\n')
			break
		}
		buf.WriteString("data: ")
		buf.Write(data[:idx])
		buf.WriteByte('\n')
		data = data[idx+1:]
	}
	buf.WriteByte('\n')
	if _, err := w.Write(buf.Bytes()); err != nil {
		return false
	}
	flusher.Flush()
	return true
}
