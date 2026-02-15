// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

const (
	planeTypeDataPlane          = "dataplane"
	planeTypeBuildPlane         = "buildplane"
	planeTypeObservabilityPlane = "observabilityplane"
)

type Server struct {
	config              *Config
	httpServer          *http.Server
	healthServer        *http.Server
	upgrader            websocket.Upgrader
	connMgr             *ConnectionManager
	pendingHTTPRequests map[string]chan *messaging.HTTPTunnelResponse
	requestsMu          sync.Mutex
	validator           *RequestValidator
	logger              *slog.Logger
	k8sClient           client.Client // Kubernetes client for querying DataPlane/BuildPlane CRs
}

func New(config *Config, k8sClient client.Client, logger *slog.Logger) *Server {
	return &Server{
		config: config,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		connMgr:             NewConnectionManager(logger),
		pendingHTTPRequests: make(map[string]chan *messaging.HTTPTunnelResponse),
		validator:           NewRequestValidator(),
		logger:              logger.With("component", "agent-server"),
		k8sClient:           k8sClient,
	}
}

func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails (extremely rare)
		return fmt.Sprintf("gw-%d", time.Now().UnixNano())
	}
	return "gw-" + hex.EncodeToString(b)
}

func getOrGenerateRequestID(r *http.Request) string {
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = generateRequestID()
	}
	return requestID
}

func (s *Server) Start() error {
	cert, err := tls.LoadX509KeyPair(s.config.ServerCertPath, s.config.ServerKeyPath)
	if err != nil {
		return fmt.Errorf("failed to load server certificate: %w", err)
	}

	// Configure TLS - request client certificates but don't verify at TLS level
	// Verification is done at application level based on DataPlane/BuildPlane CR configuration
	// This allows per-plane CA configuration for enhanced security
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequestClientCert, // Request cert but don't verify at TLS level
		MinVersion:   tls.VersionTLS12,
	}

	s.logger.Info("TLS configured",
		"clientAuth", "RequestClientCert",
		"note", "Client certificate verification performed at application level per DataPlane/BuildPlane CR",
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/api/proxy/", s.handleHTTPProxy) // HTTP proxy to data plane services

	// Register plane lifecycle API (for controller notifications and status queries)
	planeAPI := NewPlaneAPI(s.connMgr, s, s.logger)
	planeAPI.RegisterRoutes(mux)
	s.logger.Info("plane API registered",
		"endpoints", []string{
			"/api/v1/planes/notify",
			"/api/v1/planes/{type}/{id}/reconnect",
			"/api/v1/planes/{type}/{id}/status",
			"/api/v1/planes/status",
		},
	)

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

		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
		defer cancel()

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

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	planeType := query.Get("planeType")
	planeID := query.Get("planeID")

	if planeType == "" {
		s.logger.Warn("connection rejected: missing planeType parameter")
		http.Error(w, "missing planeType parameter", http.StatusBadRequest)
		return
	}

	if planeID == "" {
		s.logger.Warn("connection rejected: missing planeID parameter")
		http.Error(w, "missing planeID parameter", http.StatusBadRequest)
		return
	}

	if planeType != planeTypeDataPlane && planeType != planeTypeBuildPlane && planeType != planeTypeObservabilityPlane {
		s.logger.Warn("connection rejected: invalid planeType",
			"planeType", planeType,
		)
		http.Error(w, "invalid planeType: must be 'dataplane', 'buildplane', or 'observabilityplane'", http.StatusBadRequest)
		return
	}

	// Extract client certificate for per-CR validation
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		s.logger.Warn("connection rejected: no client certificate presented",
			"planeType", planeType,
			"planeID", planeID,
		)
		http.Error(w, "no client certificate presented", http.StatusUnauthorized)
		return
	}

	peerCerts := r.TLS.PeerCertificates
	clientCert := peerCerts[0]
	intermediates := peerCerts[1:]

	// Per-CR certificate validation enforces security boundaries
	// Each CR is validated independently to prevent cross-tenant access
	validCRs, err := s.verifyClientCertificatePerCR(clientCert, intermediates, planeType, planeID)
	if err != nil {
		s.logger.Warn("per-CR certificate verification failed",
			"planeType", planeType,
			"planeID", planeID,
			"error", err,
		)
		http.Error(w, fmt.Sprintf("client certificate verification failed: %v", err), http.StatusUnauthorized)
		return
	}

	planeIdentifier := fmt.Sprintf("%s/%s", planeType, planeID)

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("failed to upgrade connection", "error", err)
		return
	}

	// Register the connection with validated CR list and client certificate
	// Multiple agent replicas for the same plane will share the same identifier for HA
	connID, err := s.connMgr.Register(planeType, planeID, conn, validCRs, clientCert)
	if err != nil {
		s.logger.Error("failed to register connection", "error", err)
		conn.Close()
		return
	}

	s.logger.Info("agent connected successfully",
		"planeType", planeType,
		"planeID", planeID,
		"planeIdentifier", planeIdentifier,
		"connectionID", connID,
		"validCRs", validCRs,
		"validCRCount", len(validCRs),
	)

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

		var httpResp messaging.HTTPTunnelResponse
		if err := json.Unmarshal(data, &httpResp); err != nil {
			s.logger.Warn("failed to parse HTTP tunnel response", "plane", planeName, "error", err)
			continue
		}

		if httpResp.RequestID == "" {
			s.logger.Warn("received HTTP tunnel response without requestID", "plane", planeName)
			continue
		}

		s.handleHTTPTunnelResponse(planeName, &httpResp)
	}
}

func (s *Server) handleHTTPTunnelResponse(planeName string, resp *messaging.HTTPTunnelResponse) {
	s.logger.Debug("received HTTP tunnel response",
		"plane", planeName,
		"requestID", resp.RequestID,
		"statusCode", resp.StatusCode,
	)

	s.requestsMu.Lock()
	ch, ok := s.pendingHTTPRequests[resp.RequestID]
	if ok {
		delete(s.pendingHTTPRequests, resp.RequestID)
	}
	s.requestsMu.Unlock()

	if !ok {
		s.logger.Warn("received HTTP tunnel response for unknown request", "requestID", resp.RequestID)
		return
	}

	select {
	case ch <- resp:
	default:
		s.logger.Warn("HTTP tunnel reply channel full", "requestID", resp.RequestID)
	}
}

// handleHTTPProxy handles HTTP proxy requests to data plane services
// URL format: /api/proxy/{planeType}/{planeID}/{namespace}/{crName}/{target}/{path...}
// Examples:
//   - /api/proxy/dataplane/prod-cluster/namespace-a/namespace-a-dataplane/k8s/api/v1/pods
//   - /api/proxy/buildplane/default/default/default/k8s/api/v1/namespaces
//
// Note: crNamespace and crName are metadata only (for logging, future authorization)
// Routing to agent uses only planeType and planeID
func (s *Server) handleHTTPProxy(w http.ResponseWriter, r *http.Request) {
	requestID := getOrGenerateRequestID(r)
	logger := s.logger.With("requestId", requestID)

	// Parse URL
	path := strings.TrimPrefix(r.URL.Path, "/api/proxy/")
	parts := strings.Split(path, "/")

	var planeType, planeID, crNamespace, crName, target, targetPath string

	// Expected format: /api/proxy/{planeType}/{planeID}/{namespace}/{crName}/{target}/{path...}
	// Minimum 6 parts required
	if len(parts) >= 6 {
		planeType = parts[0]
		planeID = parts[1]
		crNamespace = parts[2]
		crName = parts[3]
		target = parts[4]
		targetPath = "/" + strings.Join(parts[5:], "/")
	} else {
		logger.Warn("invalid proxy URL format",
			"path", r.URL.Path,
			"expected", "/api/proxy/{planeType}/{planeID}/{namespace}/{crName}/{target}/{path}",
		)
		http.Error(w, "invalid proxy URL format: /api/proxy/{planeType}/{planeID}/{namespace}/{crName}/{target}/{path}", http.StatusBadRequest)
		return
	}

	if err := s.validator.ValidateRequest(r, target, targetPath); err != nil {
		var valErr *ValidationError
		if errors.As(err, &valErr) {
			logger.Warn("request validation failed",
				"planeType", planeType,
				"planeID", planeID,
				"crNamespace", crNamespace,
				"crName", crName,
				"target", target,
				"path", targetPath,
				"error", valErr.Message,
			)
			http.Error(w, valErr.Message, valErr.Code)
			return
		}
		logger.Error("request validation error", "error", err)
		http.Error(w, "request validation failed", http.StatusInternalServerError)
		return
	}

	// Construct identifiers for CR-aware routing
	planeIdentifier := fmt.Sprintf("%s/%s", planeType, planeID)
	// Handle cluster-scoped CR namespace placeholder: "_cluster" maps to empty namespace
	// to match the key format "/name" used by getAllPlaneClientCAs for cluster-scoped resources
	if crNamespace == "_cluster" {
		crNamespace = ""
	}
	crKey := fmt.Sprintf("%s/%s", crNamespace, crName)

	isStreaming := s.isStreamingRequest(r, targetPath)

	if isStreaming {
		s.handleStreamingProxy(w, r, planeIdentifier, crKey, target, targetPath)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Warn("failed to read request body", "error", err)
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	logger.Info("HTTP proxy request received",
		"planeType", planeType,
		"planeID", planeID,
		"cr", crKey,
		"target", target,
		"path", targetPath,
		"method", r.Method,
	)

	tunnelReq := messaging.NewHTTPTunnelRequest(
		target,
		r.Method,
		targetPath,
		r.URL.RawQuery,
		r.Header,
		body,
	)
	tunnelReq.GatewayRequestID = requestID

	// Route request to agent authorized for this specific CR
	response, err := s.SendHTTPTunnelRequestForCR(planeIdentifier, crKey, tunnelReq, 30*time.Second)
	if err != nil {
		// Check if authorization error (no agents authorized for CR)
		if strings.Contains(err.Error(), "no agents authorized for CR") {
			logger.Warn("CR authorization failed",
				"plane", planeIdentifier,
				"cr", crKey,
				"error", err,
			)
			http.Error(w, fmt.Sprintf("Forbidden: Agent not authorized for CR %s", crKey), http.StatusForbidden)
			return
		}

		logger.Error("HTTP tunnel request failed",
			"plane", planeIdentifier,
			"cr", crKey,
			"target", target,
			"error", err,
		)
		http.Error(w, fmt.Sprintf("proxy request failed: %v", err), http.StatusBadGateway)
		return
	}

	for key, values := range response.Headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(response.StatusCode)
	if len(response.Body) > 0 {
		if _, err := w.Write(response.Body); err != nil {
			logger.Warn("failed to write response body", "error", err)
		}
	}

	logger.Info("HTTP proxy request completed",
		"plane", planeIdentifier,
		"target", target,
		"statusCode", response.StatusCode,
	)
}

// isStreamingRequest detects if the request requires streaming
func (s *Server) isStreamingRequest(r *http.Request, path string) bool {
	if strings.Contains(r.URL.RawQuery, "watch=true") {
		return true
	}

	if strings.Contains(path, "/log") && strings.Contains(r.URL.RawQuery, "follow=true") {
		return true
	}

	// Check for HTTP upgrade headers (SPDY, WebSocket for exec/port-forward)
	if r.Header.Get("Connection") == "Upgrade" || r.Header.Get("Upgrade") != "" {
		return true
	}

	return false
}

// handleStreamingProxy handles streaming HTTP requests (watch, logs, exec, port-forward)
func (s *Server) handleStreamingProxy(w http.ResponseWriter, r *http.Request, planeIdentifier, crKey, target, targetPath string) {
	requestID := getOrGenerateRequestID(r)
	logger := s.logger.With("requestId", requestID)

	logger.Info("HTTP streaming proxy request received",
		"plane", planeIdentifier,
		"cr", crKey,
		"target", target,
		"path", targetPath,
		"method", r.Method,
		"query", r.URL.RawQuery,
	)

	http.Error(w, "Streaming operations (watch, logs -f, exec, port-forward) are not yet supported through the HTTP proxy. "+
		"Use the CQRS API (/api/k8s-resources/) for resource operations, or connect directly to the data plane for streaming operations.",
		http.StatusNotImplemented)

	// TODO: Implement full streaming support with CR authorization
	// 1. Get agent authorized for CR using GetForCR()
	// 2. Send HTTPTunnelStreamInit to agent
	// 3. Set up bidirectional channel for stream chunks
	// 4. Stream data chunks back and forth
	// 5. Handle connection close gracefully
}

// SendHTTPTunnelRequest sends an HTTP tunnel request to an agent and waits for the response
func (s *Server) SendHTTPTunnelRequest(planeName string, req *messaging.HTTPTunnelRequest, timeout time.Duration) (*messaging.HTTPTunnelResponse, error) {
	req.RequestID = messaging.GenerateMessageID()

	replyChan := make(chan *messaging.HTTPTunnelResponse, 1)
	s.requestsMu.Lock()
	s.pendingHTTPRequests[req.RequestID] = replyChan
	s.requestsMu.Unlock()

	s.logger.Debug("sending HTTP tunnel request",
		"requestID", req.RequestID,
		"target", req.Target,
		"method", req.Method,
		"path", req.Path,
		"plane", planeName,
	)

	if err := s.connMgr.SendHTTPTunnelRequest(planeName, req); err != nil {
		s.requestsMu.Lock()
		delete(s.pendingHTTPRequests, req.RequestID)
		s.requestsMu.Unlock()
		return nil, fmt.Errorf("failed to send HTTP tunnel request: %w", err)
	}

	select {
	case response := <-replyChan:
		return response, nil
	case <-time.After(timeout):
		s.requestsMu.Lock()
		delete(s.pendingHTTPRequests, req.RequestID)
		s.requestsMu.Unlock()
		return nil, fmt.Errorf("HTTP tunnel request timeout")
	}
}

// SendHTTPTunnelRequestForCR sends an HTTP tunnel request to an agent authorized for a specific CR
// and waits for the response. This enforces per-CR security boundaries.
func (s *Server) SendHTTPTunnelRequestForCR(
	planeName, crKey string,
	req *messaging.HTTPTunnelRequest,
	timeout time.Duration,
) (*messaging.HTTPTunnelResponse, error) {
	req.RequestID = messaging.GenerateMessageID()

	replyChan := make(chan *messaging.HTTPTunnelResponse, 1)
	s.requestsMu.Lock()
	s.pendingHTTPRequests[req.RequestID] = replyChan
	s.requestsMu.Unlock()

	s.logger.Debug("sending HTTP tunnel request with CR authorization",
		"requestID", req.RequestID,
		"target", req.Target,
		"method", req.Method,
		"path", req.Path,
		"plane", planeName,
		"cr", crKey,
	)

	conn, err := s.connMgr.GetForCR(planeName, crKey)
	if err != nil {
		s.requestsMu.Lock()
		delete(s.pendingHTTPRequests, req.RequestID)
		s.requestsMu.Unlock()
		return nil, err
	}

	if err := conn.SendHTTPTunnelRequest(req); err != nil {
		s.requestsMu.Lock()
		delete(s.pendingHTTPRequests, req.RequestID)
		s.requestsMu.Unlock()
		return nil, fmt.Errorf("failed to send HTTP tunnel request: %w", err)
	}

	select {
	case response := <-replyChan:
		s.logger.Debug("received HTTP tunnel response",
			"requestID", req.RequestID,
			"plane", planeName,
			"cr", crKey,
			"statusCode", response.StatusCode,
		)
		return response, nil
	case <-time.After(timeout):
		s.requestsMu.Lock()
		delete(s.pendingHTTPRequests, req.RequestID)
		s.requestsMu.Unlock()
		return nil, fmt.Errorf("HTTP tunnel request timeout")
	}
}

func (s *Server) GetConnectionManager() *ConnectionManager {
	return s.connMgr
}

// verifyClientCertificatePerCR validates the client certificate against EACH CR individually
// and returns a list of CRs (namespace/name) that the certificate is valid for.
// This enforces per-CR security boundaries in multi-tenant scenarios.
func (s *Server) verifyClientCertificatePerCR(
	clientCert *x509.Certificate,
	intermediates []*x509.Certificate,
	planeType, planeID string,
) (validCRs []string, err error) {
	clientCN := clientCert.Subject.CommonName
	clientIssuer := clientCert.Issuer.CommonName

	s.logger.Info("performing per-CR certificate validation",
		"planeType", planeType,
		"planeID", planeID,
		"certificateCN", clientCN,
		"certificateIssuer", clientIssuer,
	)

	// Get ALL CRs with matching planeType and planeID
	crsClientCAData, err := s.getAllPlaneClientCAs(planeType, planeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client CA configurations: %w", err)
	}

	if len(crsClientCAData) == 0 {
		s.logger.Warn("connection rejected: no CRs found for plane",
			"planeType", planeType,
			"planeID", planeID,
		)
		return nil, fmt.Errorf("no %s CRs found with planeID '%s'", planeType, planeID)
	}

	var intermediatePool *x509.CertPool
	if len(intermediates) > 0 {
		intermediatePool = x509.NewCertPool()
		for _, ic := range intermediates {
			intermediatePool.AddCert(ic)
		}
	}

	validCRs = []string{}

	// Validate certificate against EACH CR's CA individually
	for crKey, caData := range crsClientCAData {
		if caData == nil {
			s.logger.Debug("skipping CR with no CA configured", "cr", crKey)
			continue
		}

		// Parse CA certificates for this CR (logging only)
		caCerts, parseErr := parseCACertificates(caData)
		if parseErr != nil {
			s.logger.Warn("failed to parse CA certificate for CR; continuing with verification",
				"cr", crKey,
				"error", parseErr,
			)
		}

		// Create separate cert pool for THIS CR only (security isolation)
		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caData) {
			s.logger.Warn("failed to append CA certificate to pool", "cr", crKey)
			continue
		}

		// Verify client cert against THIS CR's CA only
		opts := x509.VerifyOptions{
			Roots:     certPool,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}
		if intermediatePool != nil {
			opts.Intermediates = intermediatePool
		}

		chains, err := clientCert.Verify(opts)
		if err == nil {
			validCRs = append(validCRs, crKey)
			s.logger.Info("certificate valid for CR",
				"cr", crKey,
				"clientCN", clientCN,
				"chainCount", len(chains),
			)

			if len(caCerts) > 0 {
				for i, caCert := range caCerts {
					s.logger.Debug("validated against CA",
						"cr", crKey,
						"caIndex", i,
						"caSubject", caCert.Subject.CommonName,
						"caIssuer", caCert.Issuer.CommonName,
					)
				}
			}
		} else {
			s.logger.Debug("certificate invalid for CR",
				"cr", crKey,
				"clientCN", clientCN,
				"error", err,
			)
		}
	}

	if len(validCRs) == 0 {
		s.logger.Warn("certificate not valid for any CR",
			"planeType", planeType,
			"planeID", planeID,
			"clientCN", clientCN,
			"totalCRs", len(crsClientCAData),
		)
		return nil, fmt.Errorf("certificate not valid for any CR with planeID '%s'", planeID)
	}

	s.logger.Info("per-CR certificate validation successful",
		"planeType", planeType,
		"planeID", planeID,
		"clientCN", clientCN,
		"validCRs", validCRs,
		"totalCRs", len(crsClientCAData),
	)

	return validCRs, nil
}
