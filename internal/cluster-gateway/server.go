// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
	"github.com/openchoreo/openchoreo/internal/cluster-gateway/fabric"
)

const (
	planeTypeDataPlane            = "dataplane"
	planeTypeWorkflowPlane        = "workflowplane"
	planeTypeObservabilityPlane   = "observabilityplane"
	crNamespaceClusterPlaceholder = "_cluster" // Special placeholder for cluster-scoped CRs (no namespace)

	// drainGracePeriod is how long the drain waits after the last GOAWAY for
	// agents to close on their own before sockets are forced shut. It is spent
	// inside the shutdown budget, so Start validates that it fits.
	drainGracePeriod = 2 * time.Second
)

// Connection abstracts a WebSocket connection for testability.
// *websocket.Conn satisfies this interface.
type Connection interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	SetReadDeadline(t time.Time) error
	SetPongHandler(h func(appData string) error)
	Close() error
}

// pendingRequest is an in-flight tunnel request awaiting its response. It
// records which agent connection the request was dispatched on so a response
// can only be accepted from that same connection.
type pendingRequest struct {
	ch     chan *messaging.HTTPTunnelResponse
	connID string
}

type Server struct {
	config                *Config
	httpServer            *http.Server
	internalServer        *http.Server
	healthServer          *http.Server
	upgrader              websocket.Upgrader
	connMgr               *ConnectionManager
	pendingHTTPRequests   map[string]*pendingRequest
	requestsMu            sync.Mutex
	pendingStreamSessions map[string]*streamSession
	streamSessionsMu      sync.RWMutex
	validator             *RequestValidator
	logger                *slog.Logger
	agentAuth             AgentAuthenticator // How the public listener extracts the agent client certificate
	k8sClient             client.Client      // Kubernetes client for querying DataPlane/WorkflowPlane CRs

	// Gateway mesh fabric (optional): replicates the connection registry
	// across gateway replicas and forwards requests to the pod that owns the
	// target agent connection. Nil when running as a singleton.
	fabricMesh     *fabric.Mesh
	fabricRegistry *fabric.Registry
	draining       atomic.Bool
}

// SetFabric wires the gateway into the mesh fabric. Must be called before
// Start. The server mirrors its local connections into the fabric registry
// and serves forwarded requests from peer replicas.
func (s *Server) SetFabric(mesh *fabric.Mesh, registry *fabric.Registry) {
	s.fabricMesh = mesh
	s.fabricRegistry = registry
	s.connMgr.SetListener(s)
}

// OnAgentRegistered implements ConnectionListener.
func (s *Server) OnAgentRegistered(planeIdentifier, connID string, validCRs []string) {
	s.fabricMesh.LocalUpsert(fabric.AgentEntry{
		PlaneIdentifier: planeIdentifier,
		ConnID:          connID,
		ValidCRs:        validCRs,
	})
}

// OnAgentUnregistered implements ConnectionListener.
func (s *Server) OnAgentUnregistered(planeIdentifier, connID string) {
	s.fabricMesh.LocalRemove(planeIdentifier, connID)
}

// OnAgentCRsChanged implements ConnectionListener.
func (s *Server) OnAgentCRsChanged(planeIdentifier, connID string, validCRs []string) {
	s.fabricMesh.LocalUpsert(fabric.AgentEntry{
		PlaneIdentifier: planeIdentifier,
		ConnID:          connID,
		ValidCRs:        validCRs,
	})
}

func New(config *Config, k8sClient client.Client, logger *slog.Logger) *Server {
	return &Server{
		config: config,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		connMgr:               NewConnectionManager(logger),
		pendingHTTPRequests:   make(map[string]*pendingRequest),
		pendingStreamSessions: make(map[string]*streamSession),
		validator:             NewRequestValidator(),
		logger:                logger.With("component", "agent-server"),
		k8sClient:             k8sClient,
		// Safe default; Start replaces this with the configured mode (and
		// surfaces an invalid mode as an error).
		agentAuth: mtlsAuthenticator{},
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
	// The drain spends DrainWindow plus drainGracePeriod in unconditional
	// sleeps, with no context to cut it short, while the shutdown deadline is
	// already ticking. A window that does not fit therefore does not degrade
	// gracefully: the remaining shutdown steps run against an expired context
	// and, under Kubernetes, the pod is killed part-way through the drain -
	// dropping the agents it was meant to hand over gently. Reject it here,
	// before any port is bound, rather than discover it during a rollout.
	if s.config.DrainWindow > 0 && s.config.DrainWindow+drainGracePeriod > s.config.ShutdownTimeout {
		return fmt.Errorf("drain window does not fit in the shutdown timeout: "+
			"DrainWindow (%s) plus %s of drain grace exceeds ShutdownTimeout (%s); "+
			"shorten the drain window, raise the shutdown timeout, or set the drain window to 0 to disable the drain",
			s.config.DrainWindow, drainGracePeriod, s.config.ShutdownTimeout)
	}

	cert, err := tls.LoadX509KeyPair(s.config.ServerCertPath, s.config.ServerKeyPath)
	if err != nil {
		return fmt.Errorf("failed to load server certificate: %w", err)
	}

	// Configure TLS - request client certificates but don't verify at TLS level
	// Verification is done at application level based on DataPlane/WorkflowPlane CR configuration
	// This allows per-plane CA configuration for enhanced security
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequestClientCert, // Request cert but don't verify at TLS level
		MinVersion:   tls.VersionTLS12,
	}

	s.logger.Info("TLS configured",
		"clientAuth", "RequestClientCert",
		"note", "Client certificate verification performed at application level per DataPlane/WorkflowPlane CR",
	)

	// Resolve how the public listener obtains the agent client certificate. An
	// invalid mode fails startup rather than silently falling back.
	agentAuth, err := buildAgentAuthenticator(s.config)
	if err != nil {
		return fmt.Errorf("failed to configure agent authentication: %w", err)
	}
	s.agentAuth = agentAuth
	if fh, ok := agentAuth.(forwardedHeaderAuthenticator); ok {
		s.logger.Warn("agent authentication using forwarded header",
			"mode", AgentAuthModeForwardedHeader,
			"header", fh.header,
			"note", "the gateway trusts this header for agent identity; the public listener MUST be reachable only from the trusted TLS-terminating proxy, "+
				"and every other ingress path must strip it",
		)
	} else {
		s.logger.Info("agent authentication using client certificate from TLS handshake",
			"mode", AgentAuthModeMTLS,
		)
	}

	internalTLSConfig, err := buildInternalTLSConfig(tlsConfig, s.config)
	if err != nil {
		return fmt.Errorf("failed to configure internal listener TLS: %w", err)
	}
	if s.config.InternalMTLSEnabled {
		s.logger.Info("internal API mTLS enabled",
			"clientAuth", "RequireAndVerifyClientCert",
			"clientCA", s.config.InternalClientCAPath,
		)
	} else {
		s.logger.Warn("internal API mTLS disabled",
			"note", "internal /api/* endpoints accept unauthenticated callers; enable with --internal-mtls",
		)
	}

	// Public listener: agent WebSocket only (reached by remote data planes).
	publicMux := http.NewServeMux()
	publicMux.HandleFunc("/ws", s.handleWebSocket)

	// Internal listener: caller-facing /api/* for in-cluster components only.
	internalMux := http.NewServeMux()
	internalMux.HandleFunc("/api/proxy/", s.handleHTTPProxy)   // HTTP proxy to data plane services
	internalMux.HandleFunc("/api/exec/", s.handleExec)         // WebSocket exec proxy to data plane pods
	internalMux.HandleFunc("/api/wirelogs/", s.handleWirelogs) // WebSocket wirelogs (Cilium Hubble flow) stream

	// Register plane lifecycle API (for controller notifications and status queries)
	planeAPI := NewPlaneAPI(s.connMgr, s, s.logger)
	planeAPI.RegisterRoutes(internalMux)
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
		Handler:      publicMux,
		TLSConfig:    tlsConfig,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	s.internalServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.InternalPort),
		Handler:      internalMux,
		TLSConfig:    internalTLSConfig,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	// Setup health server (separate, no TLS, no client cert verification)
	// /ready reports 503 while draining so the pod leaves Service endpoints.
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/health", s.handleHealth)
	healthMux.HandleFunc("/ready", s.handleReady)

	s.healthServer = &http.Server{
		Addr:         ":8080",
		Handler:      healthMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The mesh outlives the shutdown signal: draining needs the links to carry
	// DRAINING, removal deltas, and in-flight forward responses.
	var meshCancel context.CancelFunc
	if s.fabricMesh != nil {
		var meshCtx context.Context
		meshCtx, meshCancel = context.WithCancel(context.Background())
		defer meshCancel()
		if err := s.fabricMesh.Start(meshCtx); err != nil {
			return fmt.Errorf("failed to start gateway mesh: %w", err)
		}
		// Bound to the signal context, not the mesh context: a draining pod
		// must stop shedding for balance while it is shedding everything.
		go s.runRebalancer(ctx)
	}

	serverErrors := make(chan error, 3)

	go func() {
		s.logger.Info("public agent server starting",
			"port", s.config.Port,
			"endpoints", "/ws",
			"tls", "enabled",
		)
		serverErrors <- s.httpServer.ListenAndServeTLS("", "")
	}()

	go func() {
		s.logger.Info("internal API server starting",
			"port", s.config.InternalPort,
			"endpoints", "/api/*",
			"tls", "enabled",
		)
		serverErrors <- s.internalServer.ListenAndServeTLS("", "")
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

		// Choreographed drain: readiness drops (pod leaves Service
		// endpoints), peers stop routing new forwards to us, new agent
		// connections stop, then GOAWAY frames spread agents onto surviving
		// replicas before remaining sockets are closed.
		s.draining.Store(true)
		if s.fabricMesh != nil {
			s.fabricMesh.Drain()
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
		defer cancel()

		var shutdownErr error
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("main server shutdown error", "error", err)
			shutdownErr = fmt.Errorf("main server shutdown failed: %w", err)
		}

		if s.fabricMesh != nil {
			s.drainAgentConnections(shutdownCtx, s.config.DrainWindow)
		}

		if err := s.internalServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("internal server shutdown error", "error", err)
			if shutdownErr != nil {
				shutdownErr = fmt.Errorf("%w; internal server shutdown failed: %w", shutdownErr, err)
			} else {
				shutdownErr = fmt.Errorf("internal server shutdown failed: %w", err)
			}
		}

		if err := s.healthServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("health server shutdown error", "error", err)
			if shutdownErr != nil {
				shutdownErr = fmt.Errorf("%w; health server shutdown failed: %w", shutdownErr, err)
			} else {
				shutdownErr = fmt.Errorf("health server shutdown failed: %w", err)
			}
		}

		if s.fabricMesh != nil {
			if err := s.fabricMesh.Shutdown(shutdownCtx); err != nil {
				s.logger.Error("mesh shutdown error", "error", err)
			}
			meshCancel()
		}

		if shutdownErr != nil {
			return shutdownErr
		}

		s.logger.Info("server shutdown completed")
		return nil
	}
}

// buildInternalTLSConfig derives the TLS configuration for the internal API
// listener from the shared base config. When internal mTLS is enabled, callers
// must present a certificate signed by the internal client CA
// (RequireAndVerifyClientCert); the internal CA is distinct from the per-plane
// agent CAs verified on the public listener, so agent certificates cannot
// authenticate to the internal API.
func buildInternalTLSConfig(base *tls.Config, cfg *Config) (*tls.Config, error) {
	tlsConfig := base.Clone()
	if !cfg.InternalMTLSEnabled {
		return tlsConfig, nil
	}

	if cfg.InternalClientCAPath == "" {
		return nil, fmt.Errorf("internal mTLS is enabled but no client CA is configured: " +
			"set --internal-client-ca-cert (helm: clusterGateway.internalMtls) or disable with --internal-mtls=false")
	}

	caData, err := os.ReadFile(cfg.InternalClientCAPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read internal client CA %s: %w", cfg.InternalClientCAPath, err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("failed to parse internal client CA %s: no valid certificates found", cfg.InternalClientCAPath)
	}

	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	tlsConfig.ClientCAs = caPool
	return tlsConfig, nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		s.logger.Warn("failed to write health response", "error", err)
	}
}

// handleReady reports readiness: 503 while draining so the pod is removed
// from Service endpoints before its agent connections are spread onto
// surviving replicas.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.draining.Load() {
		http.Error(w, "draining", http.StatusServiceUnavailable)
		return
	}
	// Stay out of Service endpoints until the registry knows where agents
	// live. A freshly started replica can reach its own health server long
	// before its first peer snapshots arrive, and routing to it in that window
	// yields "no agents found" for planes whose agents are connected to peers.
	if s.fabricMesh != nil && !s.fabricMesh.Converged() {
		http.Error(w, "mesh registry not converged", http.StatusServiceUnavailable)
		return
	}
	s.handleHealth(w, r)
}

// waitOrDone waits for d, or until ctx ends. It reports whether the full wait
// elapsed; false means the caller is out of budget and should stop waiting.
func waitOrDone(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// drainAgentConnections spreads GOAWAY frames over the drain window so agents
// re-land on surviving replicas without a reconnect stampede, then closes
// whatever connections remain (the backstop for agents that ignore GOAWAY).
//
// Every wait is bounded by ctx. Start rejects a drain window that cannot fit
// inside ShutdownTimeout, but that only bounds the configuration: the main
// server's own shutdown runs first and can spend most of the budget, leaving
// the drain less time than it was promised. When that happens the polite path
// is over - stop spreading, skip the grace, and close - because the
// alternative is sleeping past the deadline until the kubelet sends SIGKILL
// mid-drain, which drops the agents this choreography exists to hand over.
func (s *Server) drainAgentConnections(ctx context.Context, window time.Duration) {
	conns := s.connMgr.GetAll()
	if len(conns) == 0 {
		return
	}

	goAway, err := json.Marshal(messaging.GoAway{
		Type:   messaging.MessageTypeGoAway,
		Reason: "gateway draining",
	})
	if err != nil {
		s.logger.Error("failed to marshal GOAWAY", "error", err)
		goAway = nil
	}

	s.logger.Info("draining agent connections",
		"connections", len(conns),
		"window", window,
	)

	interval := window / time.Duration(len(conns)+1)
	spreadFully := true
	for i, conn := range conns {
		if goAway != nil {
			if err := s.connMgr.DrainConnection(conn, goAway); err != nil {
				s.logger.Debug("failed to send GOAWAY", "connectionID", conn.ID, "error", err)
			}
		}
		if !waitOrDone(ctx, interval) {
			s.logger.Warn("drain cut short: shutdown budget exhausted",
				"notified", i+1,
				"connections", len(conns),
				"note", "remaining connections are closed without a spread GOAWAY",
			)
			spreadFully = false
			break
		}
	}

	// Short grace for agents acting on GOAWAY to finish closing on their own.
	// Skipped when the spread was already cut short: there is no budget left
	// to be patient with.
	if spreadFully && window > 0 {
		waitOrDone(ctx, drainGracePeriod)
	}

	for _, conn := range conns {
		conn.Close()
	}
	s.logger.Info("agent connections drained")
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

	if planeType != planeTypeDataPlane && planeType != planeTypeWorkflowPlane && planeType != planeTypeObservabilityPlane {
		s.logger.Warn("connection rejected: invalid planeType",
			"planeType", planeType,
		)
		http.Error(w, "invalid planeType: must be 'dataplane', 'workflowplane', or 'observabilityplane'", http.StatusBadRequest)
		return
	}

	// Extract the client certificate for per-CR validation. Where the chain
	// comes from (TLS handshake vs a header set by a trusted TLS-terminating
	// proxy) depends on the configured agent auth mode; the per-CR verification
	// below is identical in every mode.
	authenticator := s.agentAuth
	if authenticator == nil {
		authenticator = mtlsAuthenticator{}
	}
	creds, err := authenticator.Authenticate(r)
	if err != nil {
		s.logger.Warn("connection rejected: no client certificate presented",
			"planeType", planeType,
			"planeID", planeID,
			"error", err,
		)
		http.Error(w, "no client certificate presented", http.StatusUnauthorized)
		return
	}

	clientCert := creds.clientCert
	intermediates := creds.intermediates

	// Build the handshake intermediate pool once; reused for connect-time validation
	// and later incremental re-validation (see AgentConnection.UpdateCRValidity).
	intermediatePool := buildIntermediatePool(intermediates)

	// Per-CR certificate validation enforces security boundaries
	// Each CR is validated independently to prevent cross-tenant access
	validCRs, err := s.verifyClientCertificatePerCR(clientCert, intermediatePool, planeType, planeID)
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
	connID, err := s.connMgr.Register(planeType, planeID, conn, validCRs, clientCert, intermediatePool)
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

func (s *Server) handleConnection(planeName, connID string, conn Connection) {
	defer s.connMgr.Unregister(planeName, connID)
	// Release anything still waiting on this connection once it goes away.
	defer s.failPendingForConnection(planeName, connID)

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

		// Try to route as a stream chunk (has "data" field and no "statusCode" at top level)
		var streamChunk messaging.HTTPTunnelStreamChunk
		if err := json.Unmarshal(data, &streamChunk); err == nil && streamChunk.RequestID != "" && (streamChunk.Data != nil || streamChunk.IsClose) {
			s.handleStreamChunk(&streamChunk)
			continue
		}

		var httpResp messaging.HTTPTunnelResponse
		if err := json.Unmarshal(data, &httpResp); err != nil {
			s.logger.Warn("failed to parse HTTP tunnel response", "plane", planeName, "error", err)
			continue
		}

		if httpResp.RequestID == "" {
			s.logger.Warn("received HTTP tunnel response without requestID", "plane", planeName)
			continue
		}

		s.handleHTTPTunnelResponse(planeName, connID, &httpResp)
	}
}

// failPendingForConnection releases every request still waiting on an agent
// connection that has gone away.
//
// Responses are only accepted from the connection a request was dispatched on,
// so once that connection dies the answer can never arrive: without this the
// caller would block for the full tunnel timeout waiting for a reply that no
// longer has a sender. That matters most while agents are rolling, which is
// exactly when in-flight requests get stranded — and when several replicas
// serve the plane, failing fast lets the caller retry against a live one
// instead of holding a worker for 30s.
func (s *Server) failPendingForConnection(planeName, connID string) {
	s.requestsMu.Lock()
	orphaned := make([]*pendingRequest, 0)
	for id, p := range s.pendingHTTPRequests {
		if p.connID == connID {
			orphaned = append(orphaned, p)
			delete(s.pendingHTTPRequests, id)
		}
	}
	s.requestsMu.Unlock()

	if len(orphaned) == 0 {
		return
	}

	s.logger.Info("failing in-flight requests for closed agent connection",
		"plane", planeName,
		"connectionID", connID,
		"requests", len(orphaned),
	)

	// A nil response tells the waiter the connection died; a full buffer means
	// it was already answered and is about to wake up anyway.
	for _, p := range orphaned {
		select {
		case p.ch <- nil:
		default:
		}
	}
}

// handleHTTPTunnelResponse resolves the pending request matching resp, but only
// when the response arrived on the same connection the request was dispatched
// on. connID is the connection that delivered this response.
//
// Without that check, any connected agent could answer any in-flight request by
// naming its ID — safety would rest solely on request IDs being unguessable.
// That matters most when a plane runs several agent replicas: they share a
// plane identifier and are all simultaneously connected, so a buggy or hostile
// replica is already holding an accepted connection.
func (s *Server) handleHTTPTunnelResponse(planeName, connID string, resp *messaging.HTTPTunnelResponse) {
	s.logger.Debug("received HTTP tunnel response",
		"plane", planeName,
		"requestID", resp.RequestID,
		"statusCode", resp.StatusCode,
	)

	s.requestsMu.Lock()
	p, ok := s.pendingHTTPRequests[resp.RequestID]
	// Only consume the entry when the responder owns it; otherwise leave it
	// pending so the rightful connection can still answer.
	if ok && p.connID == connID {
		delete(s.pendingHTTPRequests, resp.RequestID)
	}
	s.requestsMu.Unlock()

	if !ok {
		s.logger.Warn("received HTTP tunnel response for unknown request", "requestID", resp.RequestID)
		return
	}

	if p.connID != connID {
		s.logger.Warn("dropping HTTP tunnel response from unexpected connection",
			"plane", planeName,
			"requestID", resp.RequestID,
			"dispatchedTo", p.connID,
			"receivedFrom", connID,
		)
		return
	}

	select {
	case p.ch <- resp:
	default:
		s.logger.Warn("HTTP tunnel reply channel full", "requestID", resp.RequestID)
	}
}

// handleHTTPProxy handles HTTP proxy requests to data plane services
// URL format: /api/proxy/{planeType}/{planeID}/{namespace}/{crName}/{target}/{path...}
// Examples:
//   - /api/proxy/dataplane/prod-cluster/namespace-a/namespace-a-dataplane/k8s/api/v1/pods
//   - /api/proxy/workflowplane/default/default/default/k8s/api/v1/namespaces
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
	// Handle cluster-scoped CR namespace placeholder: crNamespaceClusterPlaceholder maps to empty namespace
	// to match the key format "/name" used by getAllPlaneClientCAs for cluster-scoped resources
	if crNamespace == crNamespaceClusterPlaceholder {
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
	response, servedBy, err := s.SendHTTPTunnelRequestForCR(planeIdentifier, crKey, tunnelReq, 30*time.Second)
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

	// Identify the request's path through the mesh: which pod received it vs.
	// which pod actually owned the agent connection and served it. Equal
	// values mean it was served locally; differing values mean it was
	// forwarded one hop over the gateway mesh.
	w.Header().Set("X-Cluster-Gateway-Entry-Pod", s.selfID())
	w.Header().Set("X-Cluster-Gateway-Served-By", servedBy)

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
		"servedBy", servedBy,
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

// dispatchTunnelRequest sends a tunnel request over the given agent connection
// and waits for the correlated response. It stamps a fresh RequestID so the
// response can be matched on this pod regardless of where the request
// originated (direct or forwarded over the mesh).
func (s *Server) dispatchTunnelRequest(
	conn *AgentConnection,
	req *messaging.HTTPTunnelRequest,
	timeout time.Duration,
) (*messaging.HTTPTunnelResponse, error) {
	req.RequestID = messaging.GenerateMessageID()

	replyChan := make(chan *messaging.HTTPTunnelResponse, 1)
	s.requestsMu.Lock()
	s.pendingHTTPRequests[req.RequestID] = &pendingRequest{ch: replyChan, connID: conn.ID}
	s.requestsMu.Unlock()

	cleanup := func() {
		s.requestsMu.Lock()
		delete(s.pendingHTTPRequests, req.RequestID)
		s.requestsMu.Unlock()
	}

	if err := conn.SendHTTPTunnelRequest(req); err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to send HTTP tunnel request: %w", err)
	}

	select {
	case response := <-replyChan:
		// nil means the agent connection closed with this request in flight
		// (see failPendingForConnection) — surface it now rather than making
		// the caller wait out the timeout for an answer that cannot come.
		if response == nil {
			return nil, fmt.Errorf("agent connection closed before responding")
		}
		return response, nil
	case <-time.After(timeout):
		cleanup()
		return nil, fmt.Errorf("HTTP tunnel request timeout")
	}
}

// SendHTTPTunnelRequest sends an HTTP tunnel request to an agent and waits for the response
func (s *Server) SendHTTPTunnelRequest(planeName string, req *messaging.HTTPTunnelRequest, timeout time.Duration) (*messaging.HTTPTunnelResponse, error) {
	s.logger.Debug("sending HTTP tunnel request",
		"target", req.Target,
		"method", req.Method,
		"path", req.Path,
		"plane", planeName,
	)

	conn, err := s.connMgr.Get(planeName)
	if err != nil {
		if response, _, fwdErr := s.forwardViaMesh(planeName, "", req, timeout); fwdErr == nil {
			return response, nil
		} else if s.fabricMesh != nil {
			s.logger.Debug("mesh forward failed", "plane", planeName, "error", fwdErr)
		}
		return nil, err
	}

	return s.dispatchTunnelRequest(conn, req, timeout)
}

// selfID returns this pod's mesh identity, or a best-effort fallback (hostname)
// when the mesh is disabled. Used to report which pod actually served a
// request, distinct from which pod received it.
func (s *Server) selfID() string {
	if s.fabricMesh != nil {
		return s.fabricMesh.SelfID()
	}
	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}
	return "unknown"
}

// SendHTTPTunnelRequestForCR sends an HTTP tunnel request to an agent authorized for a specific CR
// and waits for the response. This enforces per-CR security boundaries.
// When no locally-owned agent qualifies, the request is forwarded one hop over
// the gateway mesh to a replica that owns a qualifying connection.
// The returned string identifies the pod that actually served the request
// (this pod, or the mesh peer it was forwarded to) — callers can surface it
// to make the mesh's routing decision observable per-request.
func (s *Server) SendHTTPTunnelRequestForCR(
	planeName, crKey string,
	req *messaging.HTTPTunnelRequest,
	timeout time.Duration,
) (*messaging.HTTPTunnelResponse, string, error) {
	s.logger.Debug("sending HTTP tunnel request with CR authorization",
		"target", req.Target,
		"method", req.Method,
		"path", req.Path,
		"plane", planeName,
		"cr", crKey,
	)

	// Prefer an agent owned by this pod: the forward hop vanishes entirely.
	conn, err := s.connMgr.GetForCR(planeName, crKey)
	if err != nil {
		if response, servedBy, fwdErr := s.forwardViaMesh(planeName, crKey, req, timeout); fwdErr == nil {
			return response, servedBy, nil
		} else if s.fabricMesh != nil {
			s.logger.Debug("mesh forward failed", "plane", planeName, "cr", crKey, "error", fwdErr)
		}
		return nil, "", err
	}

	response, err := s.dispatchTunnelRequest(conn, req, timeout)
	return response, s.selfID(), err
}

// forwardViaMesh routes a request to a gateway replica that owns a qualifying
// agent connection. The registry is eventually consistent, so stale entries
// are expected rather than exceptional: a forward that fails (target pod just
// died, agent just disconnected) is retried once against a different
// candidate — preferring a different owner pod — before giving up.
// On success, the returned string is the owner pod that served the request.
func (s *Server) forwardViaMesh(
	planeIdentifier, crKey string,
	req *messaging.HTTPTunnelRequest,
	timeout time.Duration,
) (*messaging.HTTPTunnelResponse, string, error) {
	if s.fabricMesh == nil || s.fabricRegistry == nil {
		return nil, "", fmt.Errorf("gateway mesh not enabled")
	}

	candidates := s.fabricRegistry.Lookup(planeIdentifier, crKey)
	if len(candidates) == 0 {
		return nil, "", fmt.Errorf("no remote agents found for plane %s", planeIdentifier)
	}

	attempts := []fabric.RemoteAgent{candidates[0]}
	for _, c := range candidates[1:] {
		if c.Owner != candidates[0].Owner {
			attempts = append(attempts, c)
			break
		}
	}
	if len(attempts) == 1 && len(candidates) > 1 {
		attempts = append(attempts, candidates[1])
	}

	var lastErr error
	for _, cand := range attempts {
		fwdReq := &fabric.ForwardRequest{
			PlaneIdentifier: planeIdentifier,
			CRKey:           crKey,
			TimeoutMillis:   timeout.Milliseconds(),
			Request:         req,
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		rsp, err := s.fabricMesh.Forward(ctx, cand.Owner, fwdReq)
		cancel()

		if err != nil {
			lastErr = err
			if !forwardIsSafeToRetry(err, req) {
				return nil, "", err
			}
			continue
		}
		if rsp.Response == nil {
			lastErr = fmt.Errorf("forward to %s failed: %s", cand.Owner, rsp.Error)
			// NoAgent means the owner never dispatched it - its registry view
			// simply disagreed with ours - so another candidate is free to
			// serve it. Any other failure came back from an owner that did
			// dispatch to an agent, and may have run.
			if !rsp.NoAgent && !isSafeHTTPMethod(req.Method) {
				return nil, "", lastErr
			}
			continue
		}

		s.logger.Debug("request served via mesh forward",
			"plane", planeIdentifier,
			"cr", crKey,
			"owner", cand.Owner,
			"statusCode", rsp.Response.StatusCode,
		)
		return rsp.Response, cand.Owner, nil
	}

	return nil, "", lastErr
}

// forwardIsSafeToRetry reports whether a failed mesh forward may be retried on
// another candidate without risking a second execution.
//
// Retrying is unconditionally safe when the failure is proven to have happened
// before the request reached the owner: no link, or a frame that never made it
// onto the wire. Once it is in flight the outcome is unknown, and only HTTP's
// safe methods may be replayed - repeating a POST or a DELETE could apply it
// twice, which no amount of availability is worth.
func forwardIsSafeToRetry(err error, req *messaging.HTTPTunnelRequest) bool {
	if errors.Is(err, fabric.ErrNoLink) || errors.Is(err, fabric.ErrForwardNotSent) {
		return true
	}
	return req != nil && isSafeHTTPMethod(req.Method)
}

// isSafeHTTPMethod reports whether the method carries no side effects, so a
// request with an unknown outcome can be repeated.
func isSafeHTTPMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// ServeForward implements fabric.Delegate: it handles a request forwarded from
// a peer replica using a locally-owned agent connection. The receiving pod
// answers as if it had served the request itself.
func (s *Server) ServeForward(freq *fabric.ForwardRequest) *fabric.ForwardResponse {
	timeout := 30 * time.Second
	if freq.TimeoutMillis > 0 {
		timeout = time.Duration(freq.TimeoutMillis) * time.Millisecond
	}

	var conn *AgentConnection
	var err error
	if freq.CRKey != "" {
		conn, err = s.connMgr.GetForCR(freq.PlaneIdentifier, freq.CRKey)
	} else {
		conn, err = s.connMgr.Get(freq.PlaneIdentifier)
	}
	if err != nil {
		// The sender's registry view was stale; let it retry elsewhere.
		return &fabric.ForwardResponse{CorrID: freq.CorrID, NoAgent: true, Error: err.Error()}
	}

	response, err := s.dispatchTunnelRequest(conn, freq.Request, timeout)
	if err != nil {
		return &fabric.ForwardResponse{CorrID: freq.CorrID, Error: err.Error()}
	}
	return &fabric.ForwardResponse{CorrID: freq.CorrID, Response: response}
}

// ApplyPlaneEvent implements fabric.Delegate: it applies a plane lifecycle
// event propagated from the replica that received the original notification.
func (s *Server) ApplyPlaneEvent(ev fabric.PlaneEvent) {
	s.logger.Info("applying propagated plane event",
		"planeType", ev.PlaneType,
		"planeID", ev.PlaneID,
		"event", ev.Event,
	)

	switch ev.Event {
	case "created", "updated":
		caData, err := s.fetchCRClientCA(ev.PlaneType, ev.PlaneID, ev.Namespace, ev.Name)
		if err != nil {
			s.logger.Warn("failed to fetch CR CA for propagated re-validation",
				"planeType", ev.PlaneType,
				"planeID", ev.PlaneID,
				"error", err,
			)
			return
		}
		if _, _, err := s.connMgr.RevalidateCR(ev.PlaneType, ev.PlaneID, ev.Namespace, ev.Name, caData); err != nil {
			s.logger.Warn("propagated CR re-validation failed", "error", err)
		}
	case "deleted", "reconnect":
		s.connMgr.DisconnectAllForPlane(ev.PlaneType, ev.PlaneID)
	default:
		s.logger.Warn("unknown propagated plane event", "event", ev.Event)
	}
}

// propagatePlaneEvent broadcasts a plane lifecycle event to peer replicas so
// agent connections owned by other pods are revalidated or disconnected too.
func (s *Server) propagatePlaneEvent(ev fabric.PlaneEvent) {
	if s.fabricMesh == nil {
		return
	}
	s.fabricMesh.BroadcastPlaneEvent(ev)
}

// fetchCRClientCA fetches the client CA configured for one CR of a plane.
func (s *Server) fetchCRClientCA(planeType, planeID, namespace, name string) ([]byte, error) {
	allCRs, err := s.getAllPlaneClientCAs(planeType, planeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get CRs: %w", err)
	}

	crKey := fmt.Sprintf("%s/%s", namespace, name)
	caData, exists := allCRs[crKey]
	if !exists {
		return nil, fmt.Errorf("CR %s not found", crKey)
	}
	if caData == nil {
		return nil, fmt.Errorf("CR %s has no CA configured", crKey)
	}
	return caData, nil
}

// PlaneStatus returns the fleet-wide connection status for a plane: local
// connections plus registry entries owned by peer replicas. LastSeen reflects
// only locally-owned connections (liveness is tracked by the owning pod).
func (s *Server) PlaneStatus(planeType, planeID string) *PlaneConnectionStatus {
	status := s.connMgr.GetPlaneStatus(planeType, planeID)
	if s.fabricRegistry != nil {
		if remote := s.fabricRegistry.CountForPlane(planeType + "/" + planeID); remote > 0 {
			status.ConnectedAgents += remote
			status.Connected = true
		}
	}
	return status
}

// CRAuthorizationStatus returns the fleet-wide authorization status for a
// specific CR within a plane.
func (s *Server) CRAuthorizationStatus(planeType, planeID, namespace, name string) *PlaneConnectionStatus {
	status := s.connMgr.GetCRAuthorizationStatus(planeType, planeID, namespace, name)
	if s.fabricRegistry != nil {
		crKey := fmt.Sprintf("%s/%s", namespace, name)
		if remote := s.fabricRegistry.CountForCR(planeType+"/"+planeID, crKey); remote > 0 {
			status.ConnectedAgents += remote
			status.Connected = true
		}
	}
	return status
}

// AllPlaneStatuses returns the fleet-wide connection status for all planes.
func (s *Server) AllPlaneStatuses() []PlaneConnectionStatus {
	statuses := s.connMgr.GetAllPlaneStatuses()
	if s.fabricRegistry == nil {
		return statuses
	}

	remote := s.fabricRegistry.AllPlaneCounts()
	for i := range statuses {
		key := statuses[i].PlaneType + "/" + statuses[i].PlaneID
		if count, ok := remote[key]; ok {
			statuses[i].ConnectedAgents += count
			statuses[i].Connected = true
			delete(remote, key)
		}
	}
	for key, count := range remote {
		parts := splitPlaneIdentifier(key)
		if len(parts) != 2 {
			continue
		}
		statuses = append(statuses, PlaneConnectionStatus{
			PlaneType:       parts[0],
			PlaneID:         parts[1],
			Connected:       true,
			ConnectedAgents: count,
		})
	}
	return statuses
}

func (s *Server) GetConnectionManager() *ConnectionManager {
	return s.connMgr
}

// buildIntermediatePool builds an x509 cert pool from the intermediate certificates
// presented during the TLS handshake. Returns nil when there are no intermediates.
func buildIntermediatePool(intermediates []*x509.Certificate) *x509.CertPool {
	if len(intermediates) == 0 {
		return nil
	}
	pool := x509.NewCertPool()
	for _, ic := range intermediates {
		pool.AddCert(ic)
	}
	return pool
}

// verifyClientCertificatePerCR validates the client certificate against EACH CR individually
// and returns a list of CRs (namespace/name) that the certificate is valid for.
// This enforces per-CR security boundaries in multi-tenant scenarios.
func (s *Server) verifyClientCertificatePerCR(
	clientCert *x509.Certificate,
	intermediatePool *x509.CertPool,
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

	s.warnOnSharedClientCA(validCRs, crsClientCAData, planeType, planeID, clientCN)

	s.logger.Info("per-CR certificate validation successful",
		"planeType", planeType,
		"planeID", planeID,
		"clientCN", clientCN,
		"validCRs", validCRs,
		"totalCRs", len(crsClientCAData),
	)

	return validCRs, nil
}

// warnOnSharedClientCA reports when one agent certificate authenticated to
// several CRs that trust byte-identical CA material.
//
// Per-CR validation only isolates tenants when each plane CR references its
// own CA. Point two CRs at the same CA and any certificate it signs is
// automatically valid for both — the isolation still appears to be enforced,
// because validation genuinely runs per CR, but the trust domains have merged.
// Nothing else in the system surfaces that, so it is worth saying out loud.
func (s *Server) warnOnSharedClientCA(
	validCRs []string,
	crsClientCAData map[string][]byte,
	planeType, planeID, clientCN string,
) {
	if len(validCRs) < 2 {
		return
	}

	byCA := make(map[[sha256.Size]byte][]string, len(validCRs))
	for _, crKey := range validCRs {
		caData, ok := crsClientCAData[crKey]
		if !ok || len(caData) == 0 {
			continue
		}
		digest := sha256.Sum256(caData)
		byCA[digest] = append(byCA[digest], crKey)
	}

	for _, sharing := range byCA {
		if len(sharing) < 2 {
			continue
		}
		slices.Sort(sharing)
		s.logger.Warn("plane CRs share identical client CA material",
			"planeType", planeType,
			"planeID", planeID,
			"clientCN", clientCN,
			"crs", sharing,
			"note", "any certificate signed by this CA is valid for all of these CRs; "+
				"give each CR its own CA to keep their trust domains separate",
		)
	}
}

// Connection rebalancing. Agents pick a replica through the Service, and
// nothing pulls them apart afterwards: a herd that reconnects together (a
// rollout evicting a whole pod's worth of agents) can land on whichever
// replica happens to be ready and simply stay there. The fleet then has one
// pod holding every connection, so restarting that pod takes every plane
// offline at once while a rolling restart of the other replicas costs
// nothing — the intermittent-outage pattern this exists to remove.
const (
	// rebalanceInterval is comfortably longer than a reconnect (sub-second),
	// so each cycle observes where the previous cycle's agents actually landed
	// before deciding again. Reacting faster would keep shedding agents that
	// are still in flight and oscillate.
	rebalanceInterval = 15 * time.Second
	// rebalanceMaxShedPerCycle caps disruption per decision. A shed agent
	// re-picks a replica at random, so some land back here and convergence
	// takes several cycles — but evicting the whole excess at once would
	// recreate the very stampede that causes concentration.
	rebalanceMaxShedPerCycle = 3
	// rebalanceSlack is the tolerance above fair share. Without it a fleet
	// that cannot divide evenly (4 connections across 3 pods) would shed
	// forever, since some pod must always hold the remainder.
	rebalanceSlack = 1
)

// runRebalancer periodically sheds surplus agent connections so they
// redistribute across replicas.
func (s *Server) runRebalancer(ctx context.Context) {
	ticker := time.NewTicker(rebalanceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.rebalanceOnce()
		}
	}
}

// rebalanceOnce sheds at most rebalanceMaxShedPerCycle connections if this pod
// holds more than its fair share of the fleet's agent connections.
func (s *Server) rebalanceOnce() {
	if s.fabricMesh == nil || s.fabricRegistry == nil || s.draining.Load() {
		return
	}

	allLocal := s.connMgr.GetAll()
	var local []*AgentConnection
	for _, c := range allLocal {
		if !c.IsDraining() {
			local = append(local, c)
		}
	}
	// Peers' connections, from the replicated registry. Excludes draining
	// owners, so a fleet mid-rollout is measured against pods that will
	// actually still be there.
	peerCounts := s.fabricRegistry.CountByOwner()
	self := s.fabricMesh.SelfID()
	delete(peerCounts, self) // guard against our own mirrored entries

	pods := len(peerCounts) + 1 // peers + this pod
	if pods < 2 {
		return // nothing to balance against
	}

	total := len(local)
	for _, n := range peerCounts {
		total += n
	}
	if total == 0 {
		return
	}

	// Ceiling division: with 4 connections over 3 pods, fair share is 2, so
	// the pod holding the remainder is not treated as an offender.
	fairShare := (total + pods - 1) / pods
	if len(local) <= fairShare+rebalanceSlack {
		return
	}

	// Move roughly half the excess per cycle rather than all of it: a shed
	// agent may well land back here, so overshooting would evict agents that
	// were about to be fine anyway. Halving converges in a few cycles while
	// keeping each step small.
	excess := len(local) - (fairShare + rebalanceSlack)
	shed := min(max(1, (excess+1)/2), rebalanceMaxShedPerCycle)

	s.logger.Info("rebalancing agent connections",
		"localConnections", len(local),
		"fleetConnections", total,
		"pods", pods,
		"fairShare", fairShare,
		"shedding", shed,
	)

	goAway, err := json.Marshal(messaging.GoAway{
		Type:   messaging.MessageTypeGoAway,
		Reason: "rebalancing connections across gateway replicas",
	})
	if err != nil {
		s.logger.Error("failed to marshal rebalance GOAWAY", "error", err)
		return
	}

	for i := 0; i < shed && i < len(local); i++ {
		conn := local[i]
		if err := s.connMgr.DrainConnection(conn, goAway); err != nil {
			s.logger.Debug("failed to send rebalance GOAWAY", "connectionID", conn.ID, "error", err)
			continue
		}
		s.logger.Info("shed agent connection for rebalancing",
			"connectionID", conn.ID,
			"plane", conn.PlaneIdentifier,
		)
	}
}
