// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

type Server struct {
	config          *Config
	httpServer      *http.Server
	healthServer    *http.Server
	upgrader        websocket.Upgrader
	connMgr         *ConnectionManager
	pendingRequests map[string]chan *messaging.ClusterAgentResponse
	requestsMu      sync.Mutex
	logger          *slog.Logger
}

func New(config *Config, logger *slog.Logger) *Server {
	return &Server{
		config: config,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		connMgr:         NewConnectionManager(logger),
		pendingRequests: make(map[string]chan *messaging.ClusterAgentResponse),
		logger:          logger.With("component", "agent-server"),
	}
}

func (s *Server) Start() error {
	cert, err := tls.LoadX509KeyPair(s.config.ServerCertPath, s.config.ServerKeyPath)
	if err != nil {
		return fmt.Errorf("failed to load server certificate: %w", err)
	}

	// Configure TLS - client certificates are not verified at TLS level
	// Client certificate verification will be implemented at the application level based on DataPlane CR configuration
	// TODO: Implement certificate verification using DataPlane CR's clientCA configuration
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.NoClientCert, // Certificate verification will be done at application level
		MinVersion:   tls.VersionTLS12,
	}

	s.logger.Info("TLS configured",
		"clientAuth", "NoClientCert",
		"note", "Client certificate verification will be implemented via DataPlane CR configuration",
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/api/k8s-resources/", s.handleK8sResourceRequest) // Kubernetes resource operations via agent

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Port),
		Handler:      mux,
		TLSConfig:    tlsConfig,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	// Setup health server (separate, no TLS, no client cert verification)
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/health", s.handleHealth)
	healthMux.HandleFunc("/ready", s.handleHealth)

	s.healthServer = &http.Server{
		Addr:         ":8080",
		Handler:      healthMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErrors := make(chan error, 2)

	go func() {
		s.logger.Info("agent server starting",
			"port", s.config.Port,
			"tls", "enabled",
		)
		serverErrors <- s.httpServer.ListenAndServeTLS("", "")
	}()

	go func() {
		s.logger.Info("health server starting",
			"port", 8080,
			"tls", "disabled",
		)
		if err := s.healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- fmt.Errorf("health server error: %w", err)
		}
	}()

	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		s.logger.Info("shutdown signal received")

		// Graceful shutdown with timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
		defer cancel()

		// Shutdown both servers
		var shutdownErr error
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("main server shutdown error", "error", err)
			shutdownErr = fmt.Errorf("main server shutdown failed: %w", err)
		}

		if err := s.healthServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("health server shutdown error", "error", err)
			if shutdownErr != nil {
				shutdownErr = fmt.Errorf("%w; health server shutdown failed: %w", shutdownErr, err)
			} else {
				shutdownErr = fmt.Errorf("health server shutdown failed: %w", err)
			}
		}

		if shutdownErr != nil {
			return shutdownErr
		}

		s.logger.Info("server shutdown completed")
		return nil
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		s.logger.Warn("failed to write health response", "error", err)
	}
}

// handleK8sResourceRequest handles Kubernetes resource operations via agent
// URL format: /api/k8s-resources/{planeName}
// Request body must include: {"action": "...", ...other fields...}
func (s *Server) handleK8sResourceRequest(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	planeName := path[len("/api/k8s-resources/"):]
	if planeName == "" {
		http.Error(w, "plane name is required in path: /api/k8s-resources/{planeName}", http.StatusBadRequest)
		return
	}

	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "only POST and PUT methods are supported", http.StatusMethodNotAllowed)
		return
	}

	var requestBody map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		s.logger.Warn("failed to parse request body", "error", err)
		http.Error(w, "invalid request body: must be valid JSON", http.StatusBadRequest)
		return
	}

	actionValue, ok := requestBody["action"]
	if !ok {
		http.Error(w, "missing 'action' field in request body", http.StatusBadRequest)
		return
	}

	action, ok := actionValue.(string)
	if !ok {
		http.Error(w, "'action' field must be a string", http.StatusBadRequest)
		return
	}

	payload := make(map[string]interface{})
	for key, value := range requestBody {
		if key != "action" {
			payload[key] = value
		}
	}

	s.logger.Info("k8s resource request received",
		"plane", planeName,
		"action", action,
		"method", r.Method,
	)

	// Determine request type based on action (CQRS pattern)
	var requestType messaging.RequestType
	switch messaging.Action(action) {
	case messaging.ActionApplyResource, messaging.ActionDeleteResource,
		messaging.ActionPatchResource, messaging.ActionCreateNamespace:
		requestType = messaging.TypeCommand
	case messaging.ActionListResources, messaging.ActionGetResource, messaging.ActionWatchResources:
		requestType = messaging.TypeQuery
	default:
		requestType = messaging.TypeCommand
	}

	// Construct composite plane identifier for agent lookup
	// Check if planeName already includes the plane type prefix
	planeIdentifier := planeName
	if !strings.Contains(planeName, "/") {
		// If no "/" found, this is just the plane name, prepend "dataplane/"
		planeIdentifier = fmt.Sprintf("dataplane/%s", planeName)
	}

	// Send request to agent and wait for response
	response, err := s.SendClusterAgentRequest(planeIdentifier, requestType, action, payload, 30*time.Second)
	if err != nil {
		s.logger.Error("k8s resource request failed",
			"plane", planeName,
			"action", action,
			"error", err,
		)
		http.Error(w, fmt.Sprintf("request failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if response.IsSuccess() {
		w.WriteHeader(http.StatusOK)
	} else {
		statusCode := http.StatusInternalServerError
		if response.Error != nil {
			switch response.Error.Code {
			case 400:
				statusCode = http.StatusBadRequest
			case 404:
				statusCode = http.StatusNotFound
			case 409:
				statusCode = http.StatusConflict
			case 422:
				statusCode = http.StatusUnprocessableEntity
			default:
				statusCode = http.StatusInternalServerError
			}
		}
		w.WriteHeader(statusCode)
	}

	responseData := map[string]interface{}{
		"success":  response.IsSuccess(),
		"plane":    planeName,
		"action":   action,
		"request":  payload,
		"response": response,
	}

	if err := json.NewEncoder(w).Encode(responseData); err != nil {
		s.logger.Warn("failed to encode response", "error", err)
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	var clientCN string
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		clientCN = r.TLS.PeerCertificates[0].Subject.CommonName
		s.logger.Info("client connecting with certificate", "CN", clientCN)
	}

	query := r.URL.Query()
	planeType := query.Get("planeType")
	planeName := query.Get("planeName")

	if planeType == "" {
		s.logger.Warn("connection rejected: missing planeType parameter")
		http.Error(w, "missing planeType parameter", http.StatusBadRequest)
		return
	}

	if planeName == "" {
		s.logger.Warn("connection rejected: missing planeName parameter")
		http.Error(w, "missing planeName parameter", http.StatusBadRequest)
		return
	}

	// Validate planeType
	if planeType != "dataplane" && planeType != "buildplane" {
		s.logger.Warn("connection rejected: invalid planeType",
			"planeType", planeType,
		)
		http.Error(w, "invalid planeType: must be 'dataplane' or 'buildplane'", http.StatusBadRequest)
		return
	}

	// Construct composite plane identifier: {planeType}/{planeName}
	planeIdentifier := fmt.Sprintf("%s/%s", planeType, planeName)

	if clientCN != "" && clientCN != planeName {
		s.logger.Warn("certificate CN does not match plane name",
			"certificateCN", clientCN,
			"planeName", planeName,
		)
		// Note: We continue anyway for now, but this could be enforced in production
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("failed to upgrade connection", "error", err)
		return
	}

	// Register the connection with composite identifier
	// Multiple agent replicas for the same plane will share the same identifier for HA
	connID, err := s.connMgr.Register(planeIdentifier, conn)
	if err != nil {
		s.logger.Error("failed to register connection", "error", err)
		conn.Close()
		return
	}

	welcomeMsg := messaging.NewBroadcastMessage(map[string]interface{}{
		"message":         fmt.Sprintf("Welcome! Connected as %s/%s", planeType, planeName),
		"planeType":       planeType,
		"planeName":       planeName,
		"planeIdentifier": planeIdentifier,
	})
	if err := s.connMgr.SendMessage(planeIdentifier, welcomeMsg); err != nil {
		s.logger.Error("failed to send welcome message", "error", err)
	}

	go s.handleConnection(planeIdentifier, connID, conn)
}

func (s *Server) handleConnection(planeName, connID string, conn *websocket.Conn) {
	defer s.connMgr.Unregister(planeName, connID)

	if err := conn.SetReadDeadline(time.Now().Add(s.config.HeartbeatTimeout)); err != nil {
		s.logger.Warn("failed to set initial read deadline", "plane", planeName, "error", err)
	}
	conn.SetPongHandler(func(string) error {
		if err := conn.SetReadDeadline(time.Now().Add(s.config.HeartbeatTimeout)); err != nil {
			s.logger.Warn("failed to set read deadline", "plane", planeName, "error", err)
		}
		s.connMgr.UpdateConnectionLastSeen(planeName, connID)
		return nil
	})

	// Start periodic ping sender
	pingTicker := time.NewTicker(s.config.HeartbeatInterval)
	defer pingTicker.Stop()

	go func() {
		for range pingTicker.C {
			if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
				s.logger.Debug("failed to send ping", "plane", planeName, "error", err)
				return
			}
		}
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				s.logger.Error("websocket error", "plane", planeName, "error", err)
			} else {
				s.logger.Info("agent disconnected", "plane", planeName)
			}
			return
		}

		s.connMgr.UpdateConnectionLastSeen(planeName, connID)

		var agentResp messaging.ClusterAgentResponse
		if err := json.Unmarshal(data, &agentResp); err != nil {
			s.logger.Warn("failed to parse response", "plane", planeName, "error", err)
			continue
		}

		if agentResp.RequestID == "" {
			s.logger.Warn("received response without requestID", "plane", planeName)
			continue
		}

		s.handleClusterAgentResponse(planeName, &agentResp)
	}
}

func (s *Server) handleClusterAgentResponse(planeName string, resp *messaging.ClusterAgentResponse) {
	s.logger.Debug("received dispatched response",
		"plane", planeName,
		"type", resp.Type,
		"requestID", resp.RequestID,
		"status", resp.Status,
	)

	s.requestsMu.Lock()
	ch, ok := s.pendingRequests[resp.RequestID]
	if ok {
		delete(s.pendingRequests, resp.RequestID)
	}
	s.requestsMu.Unlock()

	if !ok {
		s.logger.Warn("received response for unknown request", "requestID", resp.RequestID)
		return
	}

	select {
	case ch <- resp:
	default:
		s.logger.Warn("reply channel full", "requestID", resp.RequestID)
	}
}

func (s *Server) SendClusterAgentRequest(planeName string, requestType messaging.RequestType, identifier string, payload map[string]interface{}, timeout time.Duration) (*messaging.ClusterAgentResponse, error) {
	var req *messaging.ClusterAgentRequest
	if requestType == messaging.TypeCommand {
		req = messaging.NewCommand(identifier, "", planeName, payload)
	} else {
		req = messaging.NewQuery(identifier, "", planeName, payload)
	}

	req.RequestID = messaging.GenerateMessageID()

	replyChan := make(chan *messaging.ClusterAgentResponse, 1)
	s.requestsMu.Lock()
	s.pendingRequests[req.RequestID] = replyChan
	s.requestsMu.Unlock()

	s.logger.Debug("sending dispatchable request",
		"requestID", req.RequestID,
		"type", req.Type,
		"identifier", req.Identifier,
		"plane", planeName,
	)

	if err := s.connMgr.SendClusterAgentRequest(planeName, req); err != nil {
		s.requestsMu.Lock()
		delete(s.pendingRequests, req.RequestID)
		s.requestsMu.Unlock()
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	select {
	case response := <-replyChan:
		return response, nil
	case <-time.After(timeout):
		s.requestsMu.Lock()
		delete(s.pendingRequests, req.RequestID)
		s.requestsMu.Unlock()
		return nil, messaging.ErrRequestTimeout
	}
}

func (s *Server) GetConnectionManager() *ConnectionManager {
	return s.connMgr
}
