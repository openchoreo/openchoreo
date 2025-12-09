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
	"encoding/pem"
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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
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

	if planeType != planeTypeDataPlane && planeType != planeTypeBuildPlane && planeType != planeTypeObservabilityPlane {
		s.logger.Warn("connection rejected: invalid planeType",
			"planeType", planeType,
		)
		http.Error(w, "invalid planeType: must be 'dataplane', 'buildplane', or 'observabilityplane'", http.StatusBadRequest)
		return
	}

	if err := s.verifyClientCertificate(r, planeType, planeName); err != nil {
		s.logger.Warn("client certificate verification failed",
			"planeType", planeType,
			"planeName", planeName,
			"error", err,
		)
		http.Error(w, fmt.Sprintf("client certificate verification failed: %v", err), http.StatusUnauthorized)
		return
	}

	planeIdentifier := fmt.Sprintf("%s/%s", planeType, planeName)

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

	s.logger.Info("agent connected successfully",
		"planeType", planeType,
		"planeName", planeName,
		"planeIdentifier", planeIdentifier,
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
// URL format: /api/proxy/{planeType}/{planeName}/{target}/{path...}
// Examples:
//   - /api/proxy/dataplane/my-dataplane/k8s/api/v1/pods
//   - /api/proxy/buildplane/default/k8s/api/v1/namespaces
func (s *Server) handleHTTPProxy(w http.ResponseWriter, r *http.Request) {
	requestID := getOrGenerateRequestID(r)
	logger := s.logger.With("requestId", requestID)

	// Parse URL: /api/proxy/{planeType}/{planeName}/{target}/{path...}
	path := strings.TrimPrefix(r.URL.Path, "/api/proxy/")
	parts := strings.SplitN(path, "/", 4)

	if len(parts) < 4 {
		logger.Warn("invalid proxy URL format",
			"path", r.URL.Path,
			"expected", "/api/proxy/{planeType}/{planeName}/{target}/{path}",
		)
		http.Error(w, "invalid proxy URL format: /api/proxy/{planeType}/{planeName}/{target}/{path}", http.StatusBadRequest)
		return
	}

	planeType := parts[0]
	planeName := parts[1]
	target := parts[2]
	targetPath := "/" + parts[3]

	if err := s.validator.ValidateRequest(r, target, targetPath); err != nil {
		var valErr *ValidationError
		if errors.As(err, &valErr) {
			logger.Warn("request validation failed",
				"plane", planeName,
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

	// Construct planeIdentifier from planeType and planeName (e.g., "dataplane/default" or "buildplane/default")
	// This matches how agents register themselves with the gateway
	planeIdentifier := fmt.Sprintf("%s/%s", planeType, planeName)

	isStreaming := s.isStreamingRequest(r, targetPath)

	if isStreaming {
		s.handleStreamingProxy(w, r, planeIdentifier, target, targetPath)
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
		"plane", planeIdentifier,
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

	response, err := s.SendHTTPTunnelRequest(planeIdentifier, tunnelReq, 30*time.Second)
	if err != nil {
		logger.Error("HTTP tunnel request failed",
			"plane", planeIdentifier,
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
func (s *Server) handleStreamingProxy(w http.ResponseWriter, r *http.Request, planeIdentifier, target, targetPath string) {
	requestID := getOrGenerateRequestID(r)
	logger := s.logger.With("requestId", requestID)

	logger.Info("HTTP streaming proxy request received",
		"plane", planeIdentifier,
		"target", target,
		"path", targetPath,
		"method", r.Method,
		"query", r.URL.RawQuery,
	)

	http.Error(w, "Streaming operations (watch, logs -f, exec, port-forward) are not yet supported through the HTTP proxy. "+
		"Use the CQRS API (/api/k8s-resources/) for resource operations, or connect directly to the data plane for streaming operations.",
		http.StatusNotImplemented)

	// TODO: Implement full streaming support
	// 1. Send HTTPTunnelStreamInit to agent
	// 2. Set up bidirectional channel for stream chunks
	// 3. Stream data chunks back and forth
	// 4. Handle connection close gracefully
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

func (s *Server) GetConnectionManager() *ConnectionManager {
	return s.connMgr
}

// verifyClientCertificate verifies the client certificate against the CA configured in the plane's CR
func (s *Server) verifyClientCertificate(r *http.Request, planeType, planeName string) error {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return fmt.Errorf("no client certificate presented")
	}

	clientCert := r.TLS.PeerCertificates[0]
	clientCN := clientCert.Subject.CommonName
	clientIssuer := clientCert.Issuer.CommonName

	s.logger.Info("verifying client certificate",
		"planeType", planeType,
		"planeName", planeName,
		"certificateCN", clientCN,
		"certificateIssuer", clientIssuer,
		"certificateNotBefore", clientCert.NotBefore,
		"certificateNotAfter", clientCert.NotAfter,
		"certificateSerialNumber", clientCert.SerialNumber.String(),
	)

	clientCAData, err := s.getPlaneClientCA(planeType, planeName)
	if err != nil {
		return fmt.Errorf("failed to get client CA configuration: %w", err)
	}

	if clientCAData == nil {
		s.logger.Warn("no client CA configured for plane, skipping certificate verification",
			"planeType", planeType,
			"planeName", planeName,
		)
		return nil
	}

	caCerts, err := parseCACertificates(clientCAData)
	if err != nil {
		s.logger.Error("failed to parse CA certificate for logging",
			"error", err,
		)
	} else {
		for i, caCert := range caCerts {
			s.logger.Info("CA certificate details",
				"index", i,
				"subject", caCert.Subject.CommonName,
				"issuer", caCert.Issuer.CommonName,
				"notBefore", caCert.NotBefore,
				"notAfter", caCert.NotAfter,
				"isCA", caCert.IsCA,
			)
		}
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(clientCAData) {
		return fmt.Errorf("failed to parse client CA certificate")
	}

	opts := x509.VerifyOptions{
		Roots:     certPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	chains, err := clientCert.Verify(opts)
	if err != nil {
		s.logger.Error("certificate verification failed with details",
			"clientCN", clientCN,
			"clientIssuer", clientIssuer,
			"error", err,
			"errorType", fmt.Sprintf("%T", err),
		)
		return fmt.Errorf("certificate verification failed: %w", err)
	}

	s.logger.Info("certificate verification successful",
		"clientCN", clientCN,
		"chainCount", len(chains),
	)
	for i, chain := range chains {
		s.logger.Debug("certificate chain",
			"chainIndex", i,
			"chainLength", len(chain),
		)
		for j, cert := range chain {
			s.logger.Debug("chain certificate",
				"chainIndex", i,
				"certIndex", j,
				"subject", cert.Subject.CommonName,
				"issuer", cert.Issuer.CommonName,
			)
		}
	}

	if clientCN != planeName {
		s.logger.Warn("certificate CN does not match plane name",
			"certificateCN", clientCN,
			"planeName", planeName,
			"note", "This is allowed but may indicate a configuration issue",
		)
	}

	s.logger.Info("client certificate verified successfully",
		"planeType", planeType,
		"planeName", planeName,
		"certificateCN", clientCN,
	)

	return nil
}

// getPlaneClientCA retrieves the client CA configuration from DataPlane or BuildPlane CR
func (s *Server) getPlaneClientCA(planeType, planeName string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Todo: Need to optimize to get the plane from the correct namespace
	namespacesToCheck := []string{"default"}

	if planeType == planeTypeDataPlane {
		var dataPlane openchoreov1alpha1.DataPlane

		for _, namespace := range namespacesToCheck {
			key := client.ObjectKey{
				Name:      planeName,
				Namespace: namespace,
			}

			if err := s.k8sClient.Get(ctx, key, &dataPlane); err == nil {
				s.logger.Debug("found DataPlane CR",
					"name", planeName,
					"namespace", namespace,
				)

				if dataPlane.Spec.Agent != nil && dataPlane.Spec.Agent.ClientCA != nil {
					return s.extractCADataWithNamespace(dataPlane.Spec.Agent.ClientCA, namespace)
				}

				return nil, nil
			}
		}

		var dataPlaneList openchoreov1alpha1.DataPlaneList
		if err := s.k8sClient.List(ctx, &dataPlaneList); err != nil {
			return nil, fmt.Errorf("failed to list DataPlane CRs: %w", err)
		}

		for _, dp := range dataPlaneList.Items {
			if dp.Name == planeName {
				s.logger.Debug("found DataPlane CR via list",
					"name", planeName,
					"namespace", dp.Namespace,
				)

				if dp.Spec.Agent != nil && dp.Spec.Agent.ClientCA != nil {
					return s.extractCADataWithNamespace(dp.Spec.Agent.ClientCA, dp.Namespace)
				}

				return nil, nil
			}
		}

		return nil, fmt.Errorf("DataPlane %s not found", planeName)
	}

	if planeType == planeTypeBuildPlane {
		var buildPlane openchoreov1alpha1.BuildPlane

		// Todo: Need to optimize to get the plane from the correct namespace
		for _, namespace := range namespacesToCheck {
			key := client.ObjectKey{
				Name:      planeName,
				Namespace: namespace,
			}

			if err := s.k8sClient.Get(ctx, key, &buildPlane); err == nil {
				s.logger.Debug("found BuildPlane CR",
					"name", planeName,
					"namespace", namespace,
				)

				if buildPlane.Spec.Agent != nil && buildPlane.Spec.Agent.ClientCA != nil {
					return s.extractCADataWithNamespace(buildPlane.Spec.Agent.ClientCA, namespace)
				}

				return nil, nil
			}
		}

		var buildPlaneList openchoreov1alpha1.BuildPlaneList
		if err := s.k8sClient.List(ctx, &buildPlaneList); err != nil {
			return nil, fmt.Errorf("failed to list BuildPlane CRs: %w", err)
		}

		for _, bp := range buildPlaneList.Items {
			if bp.Name == planeName {
				s.logger.Debug("found BuildPlane CR via list",
					"name", planeName,
					"namespace", bp.Namespace,
				)

				if bp.Spec.Agent != nil && bp.Spec.Agent.ClientCA != nil {
					return s.extractCADataWithNamespace(bp.Spec.Agent.ClientCA, bp.Namespace)
				}

				return nil, nil
			}
		}

		return nil, fmt.Errorf("BuildPlane %s not found", planeName)
	}

	if planeType == planeTypeObservabilityPlane {
		var observabilityPlane openchoreov1alpha1.ObservabilityPlane

		// Todo: Need to optimize to get the plane from the correct namespace
		for _, namespace := range namespacesToCheck {
			key := client.ObjectKey{
				Name:      planeName,
				Namespace: namespace,
			}

			if err := s.k8sClient.Get(ctx, key, &observabilityPlane); err == nil {
				s.logger.Debug("found ObservabilityPlane CR",
					"name", planeName,
					"namespace", namespace,
				)

				if observabilityPlane.Spec.Agent != nil && observabilityPlane.Spec.Agent.ClientCA != nil {
					return s.extractCADataWithNamespace(observabilityPlane.Spec.Agent.ClientCA, namespace)
				}

				return nil, nil
			}
		}

		var observabilityPlaneList openchoreov1alpha1.ObservabilityPlaneList
		if err := s.k8sClient.List(ctx, &observabilityPlaneList); err != nil {
			return nil, fmt.Errorf("failed to list ObservabilityPlane CRs: %w", err)
		}

		for _, op := range observabilityPlaneList.Items {
			if op.Name == planeName {
				s.logger.Debug("found ObservabilityPlane CR via list",
					"name", planeName,
					"namespace", op.Namespace,
				)

				if op.Spec.Agent != nil && op.Spec.Agent.ClientCA != nil {
					return s.extractCADataWithNamespace(op.Spec.Agent.ClientCA, op.Namespace)
				}

				return nil, nil
			}
		}

		return nil, fmt.Errorf("ObservabilityPlane %s not found", planeName)
	}

	return nil, fmt.Errorf("unsupported plane type: %s", planeType)
}

// extractCAData extracts CA certificate data from ValueFrom configuration
// planeNamespace is used as default namespace for SecretRef if not specified
func (s *Server) extractCADataWithNamespace(valueFrom *openchoreov1alpha1.ValueFrom, planeNamespace string) ([]byte, error) {
	if valueFrom.Value != "" {
		return []byte(valueFrom.Value), nil
	}

	if valueFrom.SecretRef != nil {
		return s.extractCAFromSecret(valueFrom.SecretRef, planeNamespace)
	}

	return nil, fmt.Errorf("no valid CA data found in ValueFrom")
}

func parseCACertificates(pemData []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate

	for len(pemData) > 0 {
		block, rest := pem.Decode(pemData)
		if block == nil {
			break
		}

		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse certificate: %w", err)
			}
			certs = append(certs, cert)
		}

		pemData = rest
	}

	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates found in PEM data")
	}

	return certs, nil
}

// extractCAFromSecret extracts CA certificate data from a Kubernetes secret
// planeNamespace is used as default if secretRef.Namespace is not specified
func (s *Server) extractCAFromSecret(secretRef *openchoreov1alpha1.SecretKeyReference, planeNamespace string) ([]byte, error) {
	if secretRef.Name == "" {
		return nil, fmt.Errorf("secret name is required")
	}

	if secretRef.Key == "" {
		return nil, fmt.Errorf("secret key is required")
	}

	// Determine namespace: use specified namespace, or default to plane's namespace
	namespace := secretRef.Namespace
	if namespace == "" {
		namespace = planeNamespace
	}

	s.logger.Debug("loading CA certificate from secret",
		"secretName", secretRef.Name,
		"namespace", namespace,
		"key", secretRef.Key,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var secret corev1.Secret
	secretKey := types.NamespacedName{
		Name:      secretRef.Name,
		Namespace: namespace,
	}

	if err := s.k8sClient.Get(ctx, secretKey, &secret); err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", namespace, secretRef.Name, err)
	}

	caData, ok := secret.Data[secretRef.Key]
	if !ok {
		return nil, fmt.Errorf("key %s not found in secret %s/%s", secretRef.Key, namespace, secretRef.Name)
	}

	if len(caData) == 0 {
		return nil, fmt.Errorf("CA data is empty in secret %s/%s key %s", namespace, secretRef.Name, secretRef.Key)
	}

	s.logger.Info("loaded CA certificate from secret",
		"secretName", secretRef.Name,
		"namespace", namespace,
		"key", secretRef.Key,
		"dataSize", len(caData),
	)

	return caData, nil
}
