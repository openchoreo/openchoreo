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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
	"github.com/openchoreo/openchoreo/internal/cluster-gateway/fabric"
)

const (
	planeTypeDataPlane            = "dataplane"
	planeTypeWorkflowPlane        = "workflowplane"
	planeTypeObservabilityPlane   = "observabilityplane"
	crNamespaceClusterPlaceholder = "_cluster" // Special placeholder for cluster-scoped CRs (no namespace)

	// headerGatewayEntryPod and headerGatewayServedBy name the pair of
	// response headers that make a request's path through the mesh visible:
	// the pod the caller reached, and the pod that owned the agent connection.
	// The handlers that echo them share the names from here rather than
	// spelling out the literals, so the two cannot drift apart.
	headerGatewayEntryPod = "X-Cluster-Gateway-Entry-Pod"
	headerGatewayServedBy = "X-Cluster-Gateway-Served-By"

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
	SetWriteDeadline(t time.Time) error
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
	// planeReader lists plane CRs for an agent connect and for the deleted-CR
	// sweep, both of which look at every CR of a plane type. In the gateway
	// binary it is an informer cache, so those lists do not reach the API
	// server. Anything decided about a single CR is read through k8sClient.
	// It is unset in a Server built without SetPlaneReader, which then lists
	// through k8sClient: see planes.
	planeReader client.Reader
	caCache     *secretCACache // Client CAs read from Secrets, reused by agent connects: see cas
	caCacheInit sync.Once

	// liveCRs remembers what this replica last read from the API server about
	// plane CRs, by plane type and then "namespace/name", so that an agent
	// connect does not decide on a planeReader copy that is older: see
	// reconcilePlaneReader.
	liveMu  sync.Mutex
	liveCRs map[string]map[string]liveCR

	// unreadJobs holds, per plane, the job that authorizes the CRs agent
	// connects left unread: see authorizeUnreadCRs.
	unreadMu   sync.Mutex
	unreadJobs map[string]*unreadCRJob
	// runCtx ends when the server shuts down. Start sets it: see lifecycle.
	runCtx context.Context

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

// SetPlaneReader sets where plane CRs are listed from. It must be called before
// Start. The reader may trail the API server, as an informer cache does.
func (s *Server) SetPlaneReader(reader client.Reader) {
	s.planeReader = reader
}

// planes returns where plane CRs are listed from: the reader given to
// SetPlaneReader, or the API server client when there is none.
func (s *Server) planes() client.Reader {
	if s.planeReader != nil {
		return s.planeReader
	}
	return s.k8sClient
}

// lifecycle returns a context that ends when the server shuts down. A Server
// that was never started has nothing to shut down.
func (s *Server) lifecycle() context.Context {
	if s.runCtx != nil {
		return s.runCtx
	}
	return context.Background()
}

// liveCRRetention is how long a liveCR is kept after planeReader has caught up
// with it. An agent connect that was already under way when the CR was read
// needs the record once it is registered, so it has to outlast a connect.
const liveCRRetention = time.Minute

// liveCR is what this replica last read from the API server about a plane CR.
type liveCR struct {
	uid        types.UID
	generation int64
	gone       bool // the read found no such CR
	readAt     time.Time
}

// noteLiveCR records a plane CR as it was just read from the API server.
func (s *Server) noteLiveCR(planeType, crKey string, obj client.Object) {
	s.setLiveCR(planeType, crKey, liveCR{uid: obj.GetUID(), generation: obj.GetGeneration(), readAt: time.Now()})
}

// noteLiveCRGone records that a read from the API server found no such CR.
func (s *Server) noteLiveCRGone(planeType, crKey string) {
	s.setLiveCR(planeType, crKey, liveCR{gone: true, readAt: time.Now()})
}

func (s *Server) setLiveCR(planeType, crKey string, cr liveCR) {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()

	if s.liveCRs == nil {
		s.liveCRs = make(map[string]map[string]liveCR)
	}
	if s.liveCRs[planeType] == nil {
		s.liveCRs[planeType] = make(map[string]liveCR)
	}
	s.liveCRs[planeType][crKey] = cr
}

// liveCRsReadSince returns the CRs of a plane type that this replica has read
// from the API server since the given time.
func (s *Server) liveCRsReadSince(planeType string, since time.Time) []string {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()

	var crKeys []string
	for crKey, cr := range s.liveCRs[planeType] {
		if !cr.readAt.Before(since) {
			crKeys = append(crKeys, crKey)
		}
	}
	return crKeys
}

// reconcilePlaneReader makes the plane CRs an agent connect has listed from
// planeReader at least as new as what this replica has already read from the
// API server. cached holds the listed CRs of one scope by "namespace/name" and
// is corrected in place.
//
// planeReader is an informer cache and can trail the API server. A notification
// reads its CR from the API server and applies it to the connected agents. A
// connect that then took the same CR from a cache that had not seen the change
// would undo it: after a CA rotation it would authorize an agent the
// notification had just refused, and nothing would take that back. The same
// goes for a CR the cache does not list yet, lists under the planeID it had
// before it moved, or still lists after it was deleted.
//
// So every CR recorded by noteLiveCR is compared with its cached copy, whether
// or not that copy passes for one of this plane's. Where the cache has not
// caught up, the CR is read from the API server and that copy takes the cached
// one's place. A CR that cannot be read is taken out of the connect and
// returned, to be authorized once the connection is registered.
func (s *Server) reconcilePlaneReader(
	ctx context.Context, planeType string, clusterScoped bool, cached map[string]client.Object,
) (unread []string) {
	now := time.Now()
	var behind []string

	s.liveMu.Lock()
	for crKey, cr := range s.liveCRs[planeType] {
		if strings.HasPrefix(crKey, "/") != clusterScoped {
			continue // The other scope's list covers it.
		}
		obj, listed := cached[crKey]
		caughtUp := !listed
		if !cr.gone {
			// A different UID is another CR of the same name. Which of the
			// two is the newer cannot be told from here, so it is read.
			caughtUp = listed && obj.GetUID() == cr.uid && obj.GetGeneration() >= cr.generation
		}
		if !caughtUp {
			behind = append(behind, crKey)
			continue
		}
		if now.Sub(cr.readAt) > liveCRRetention {
			delete(s.liveCRs[planeType], crKey)
		}
	}
	s.liveMu.Unlock()

	for _, crKey := range behind {
		namespace, name, _ := strings.Cut(crKey, "/")
		obj, err := s.getPlaneCR(ctx, planeType, namespace, name)
		switch {
		case apierrors.IsNotFound(err):
			s.noteLiveCRGone(planeType, crKey)
			delete(cached, crKey)
		case err != nil:
			delete(cached, crKey)
			unread = append(unread, crKey)
		default:
			s.noteLiveCR(planeType, crKey, obj)
			cached[crKey] = obj
		}
	}
	return unread
}

// authorizeAfterConnect re-validates, on a plane whose agent has just been
// registered, the CRs that the connect could not settle:
//
//   - the CRs its lookup left unread, and
//   - the CRs this replica read from the API server while the connect was under
//     way. A notification applies a CR to the connections that are registered,
//     and this one was not yet: it was authorized from what the lookup had
//     read before the change.
func (s *Server) authorizeAfterConnect(planeType, planeID string, unread []string, lookupStart time.Time) {
	pending := slices.Concat(unread, s.liveCRsReadSince(planeType, lookupStart))
	if len(pending) > 0 {
		s.authorizeUnreadCRs(planeType, planeID, pending)
	}
}

// cas returns the CA cache, created on first use.
func (s *Server) cas() *secretCACache {
	s.caCacheInit.Do(func() { s.caCache = newSecretCACache() })
	return s.caCache
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
	internalMux.HandleFunc("/api/proxy/", s.handleHTTPProxy)                    // HTTP proxy to data plane services
	internalMux.HandleFunc("/api/exec/", s.handleExec)                          // WebSocket exec proxy to data plane pods
	internalMux.HandleFunc("/api/wirelogs/", s.handleWirelogs)                  // WebSocket wirelogs (Cilium Hubble flow) stream
	internalMux.HandleFunc(resourceTreePathPrefix, s.handleResourceTreeMatches) // Resource tree child match queries

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
	s.runCtx = ctx

	go s.runDeletedCRSweep(ctx)

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
			if err := conn.SendRawMessage(goAway); err != nil {
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
	lookupStart := time.Now()
	validCRs, unreadCRs, err := s.verifyClientCertificatePerCR(clientCert, intermediatePool, planeType, planeID)
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

	// The agent is connected and serves the CRs above. What the lookup could
	// not settle is done now, without the handshake waiting on it.
	go s.authorizeAfterConnect(planeType, planeID, unreadCRs, lookupStart)

	go s.handleConnection(planeIdentifier, connID, conn)
}

// unreadCRJob is the work left over from agent connects on one plane that ran
// out of lookup time: the CRs still to be read and authorized.
type unreadCRJob struct {
	pending map[string]struct{}
	cancel  context.CancelFunc
}

// authorizeUnreadCRs finishes what an agent connect could not do within its
// lookup time: it reads each CR left unread from the API server and re-validates
// it on the plane's connections, caReadConcurrency CRs at a time. Without it a
// CR left out of a connect stays unauthorized on that connection until
// something else notifies the gateway about the CR.
//
// A plane has one such job at a time. A connect reaches its lookup bound when
// the API server is slow, and agents reconnect at such times too; a job per
// connect would read the same CRs once for each of them and add load exactly
// when the API server can least take it. A call made while the plane's job is
// running adds its CRs to that job and returns.
//
// The job is not tied to the connection that started it, because it authorizes
// CRs on every connection of the plane. It ends when the plane has no agent
// connected to this replica, when the server shuts down, or when no CR is left.
func (s *Server) authorizeUnreadCRs(planeType, planeID string, crKeys []string) {
	planeIdentifier := planeType + "/" + planeID

	s.unreadMu.Lock()
	job, running := s.unreadJobs[planeIdentifier]
	if !running {
		job = &unreadCRJob{pending: make(map[string]struct{}, len(crKeys))}
		if s.unreadJobs == nil {
			s.unreadJobs = make(map[string]*unreadCRJob)
		}
		s.unreadJobs[planeIdentifier] = job
	}
	for _, crKey := range crKeys {
		job.pending[crKey] = struct{}{}
	}
	var ctx context.Context
	if !running {
		ctx, job.cancel = context.WithCancel(s.lifecycle())
	}
	s.unreadMu.Unlock()
	if running {
		return
	}
	defer job.cancel()

	var authorized atomic.Int64
	for {
		var wg sync.WaitGroup
		for range caReadConcurrency {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					crKey, ok := s.nextUnreadCR(ctx, planeType, planeID, job)
					if !ok {
						return
					}
					namespace, name, _ := strings.Cut(crKey, "/")
					s.revalidateCRWithRetry(ctx, fabric.PlaneEvent{
						PlaneType: planeType, PlaneID: planeID, Event: "updated",
						Namespace: namespace, Name: name,
					})
					authorized.Add(1)
				}
			}()
		}
		wg.Wait()

		// The job stays registered until its workers have finished, so a call
		// that arrives meanwhile adds to it and does not start a second set
		// of workers. CRs added after the last worker left are picked up by
		// another round.
		s.unreadMu.Lock()
		retired := s.unreadJobs[planeIdentifier] != job
		if !retired && len(job.pending) == 0 {
			delete(s.unreadJobs, planeIdentifier)
			retired = true
		}
		s.unreadMu.Unlock()
		if retired {
			break
		}
	}
	s.logger.Info("finished authorizing the CRs agent connects left unread",
		"planeType", planeType,
		"planeID", planeID,
		"crs", authorized.Load(),
	)
}

// nextUnreadCR takes one CR off the plane's unread job, or reports that there
// is none to take. It also ends the job early when the job's context has ended
// or the plane has no agent connected to this replica: an agent that connects
// later is checked against every CR again.
//
// A job that ends early is retired at once, before its workers have wound
// down. An agent can connect in that interval, and its unread CRs must start a
// job of their own: added to this one, they would be dropped with the rest.
func (s *Server) nextUnreadCR(ctx context.Context, planeType, planeID string, job *unreadCRJob) (string, bool) {
	planeIdentifier := planeType + "/" + planeID

	s.unreadMu.Lock()
	defer s.unreadMu.Unlock()

	if s.unreadJobs[planeIdentifier] != job {
		return "", false // Retired by another worker.
	}
	if ctx.Err() != nil || s.connMgr.GetPlaneStatus(planeType, planeID).ConnectedAgents == 0 {
		delete(s.unreadJobs, planeIdentifier)
		job.cancel() // Ends the lookups and backoffs the other workers are in.
		return "", false
	}
	for crKey := range job.pending {
		delete(job.pending, crKey)
		return crKey, true
	}
	return "", false
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

	// Stopping the ticker does not close its channel, so the ping sender needs
	// its own signal to end with the connection.
	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			select {
			case <-done:
				return
			case <-pingTicker.C:
				if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
					s.logger.Debug("failed to send ping", "plane", planeName, "error", err)
					return
				}
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
	w.Header().Set(headerGatewayEntryPod, s.selfID())
	w.Header().Set(headerGatewayServedBy, servedBy)

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
		response, servedBy, fwdErr := s.forwardViaMesh(planeName, crKey, req, timeout)
		if fwdErr == nil {
			return response, servedBy, nil
		}
		if s.fabricMesh != nil {
			s.logger.Debug("mesh forward failed", "plane", planeName, "cr", crKey, "error", fwdErr)
		}
		// A forward that was attempted and failed is the real cause. This
		// pod's local view would hide it, and would turn a transport failure
		// into a 403 when the local agents are not authorized for the CR.
		if !errors.Is(fwdErr, errNoForwardCandidate) {
			return nil, "", fwdErr
		}
		if s.fabricRegistry != nil {
			// Both counts come from one view of the registry, so an agent
			// registering now cannot be missed by one and seen by the other.
			forCR, forPlane := s.fabricRegistry.CountForCRAndPlane(planeName, crKey)
			// The forward skips draining replicas, but their agents are still
			// connected and about to reconnect elsewhere. A CR they are
			// authorized for is unroutable for a moment, not unauthorized.
			if forCR > 0 {
				return nil, "", fmt.Errorf("CR %s is served only by agents on a draining gateway replica", crKey)
			}
			// No replica has an agent authorized for this CR. When peers hold
			// agents for the plane, report the missing authorization: "no
			// agents found" would read as a disconnected agent.
			if forPlane > 0 {
				return nil, "", fmt.Errorf("no agents authorized for CR %s", crKey)
			}
		}
		return nil, "", err
	}

	response, err := s.dispatchTunnelRequest(conn, req, timeout)
	return response, s.selfID(), err
}

// errNoForwardCandidate reports that a mesh forward was never attempted: the
// mesh is off, or no peer replica advertises an agent that can serve the request.
var errNoForwardCandidate = errors.New("no forward candidate")

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
		return nil, "", fmt.Errorf("gateway mesh not enabled: %w", errNoForwardCandidate)
	}

	candidates := s.fabricRegistry.Lookup(planeIdentifier, crKey)
	if len(candidates) == 0 {
		return nil, "", fmt.Errorf("no remote agents found for plane %s: %w", planeIdentifier, errNoForwardCandidate)
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

	// failure is the last attempt that failed for a reason other than the owner
	// answering NoAgent. It outranks a NoAgent answer: an owner that could not
	// be reached may still hold an authorized agent, while NoAgent speaks only
	// for the owner that sent it and can read as an authorization miss.
	var lastErr, failure error
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
			failure = err
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
			if !rsp.NoAgent {
				failure = lastErr
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

	if failure != nil {
		return nil, "", failure
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
		s.revalidateCRWithRetry(s.lifecycle(), ev)
	case "deleted":
		if ev.Name == "" {
			s.connMgr.DisconnectAllForPlane(ev.PlaneType, ev.PlaneID)
			return
		}
		s.revokeCR(ev.PlaneType, ev.PlaneID, ev.Namespace, ev.Name)
	case "reconnect":
		s.connMgr.DisconnectAllForPlane(ev.PlaneType, ev.PlaneID)
	default:
		s.logger.Warn("unknown propagated plane event", "event", ev.Event)
	}
}

// planeEventAttempts bounds how many times the re-validation of one CR is
// tried, and planeEventRetryBackoff is the wait before the second attempt,
// doubled before each one after it.
const planeEventAttempts = 5

var planeEventRetryBackoff = time.Second

// errPlaneCRGone marks a lookup that failed because the plane CR itself does
// not exist. It is told apart from a CA Secret that does not exist, which can
// still appear: a CR is often created just before the Secret it points at.
var errPlaneCRGone = errors.New("plane CR does not exist")

// errPlaneCRElsewhere marks a lookup of a CR that exists but is on another
// plane than the one asked about.
var errPlaneCRElsewhere = errors.New("does not belong to plane")

// revalidateCRWithRetry re-validates one CR on this replica's connections from
// what the API server holds, for callers that nothing prompts a second time:
//
//   - A propagated "created" or "updated" event. The replica that received the
//     notification has already answered the controller, so the event is not
//     sent again.
//   - A CR that an agent connect left unread.
//
// In both cases a lookup that fails is retried, or the agents this replica owns
// stay unauthorized for the CR, possibly while the CR reports as connected
// through another replica's agent.
func (s *Server) revalidateCRWithRetry(ctx context.Context, ev fabric.PlaneEvent) {
	backoff := planeEventRetryBackoff
	for attempt := 1; ; attempt++ {
		err := s.revalidateCROnce(ctx, ev)
		if err == nil || ctx.Err() != nil {
			return
		}
		// Neither a CR that no longer exists nor one that is on another plane
		// has anything to grant here, and trying again does not change that.
		// Both are expected: a connect re-validates CRs it could not place.
		if errors.Is(err, errPlaneCRGone) || errors.Is(err, errPlaneCRElsewhere) {
			s.logger.Debug("nothing to re-validate for CR",
				"planeType", ev.PlaneType,
				"planeID", ev.PlaneID,
				"cr", fmt.Sprintf("%s/%s", ev.Namespace, ev.Name),
				"reason", err,
			)
			return
		}
		if attempt >= planeEventAttempts {
			s.logger.Warn("giving up on CR re-validation",
				"planeType", ev.PlaneType,
				"planeID", ev.PlaneID,
				"cr", fmt.Sprintf("%s/%s", ev.Namespace, ev.Name),
				"attempts", attempt,
				"error", err,
			)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
	}
}

// revalidateCROnce makes one attempt at re-validating a CR. It returns an error
// only for a failed lookup, which is the part worth retrying. Every attempt
// reads the CR from the API server and re-reads the revocation count first, so
// an attempt that runs after the CR's "deleted" event is held back by the same
// staleness guard as the notification handler: see there.
func (s *Server) revalidateCROnce(ctx context.Context, ev fabric.PlaneEvent) error {
	revocations := s.connMgr.CRRevocations(ev.PlaneType, ev.PlaneID, ev.Namespace, ev.Name)
	if revocations > 0 {
		terminating, err := s.planeCRTerminating(ctx, ev.PlaneType, ev.Namespace, ev.Name)
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to look up CR for re-validation: %w: %w", errPlaneCRGone, err)
		}
		if err != nil {
			return fmt.Errorf("failed to look up CR for re-validation: %w", err)
		}
		if terminating {
			s.logger.Info("skipping re-validation of a deleted CR",
				"planeType", ev.PlaneType,
				"planeID", ev.PlaneID,
				"cr", fmt.Sprintf("%s/%s", ev.Namespace, ev.Name),
			)
			return nil
		}
	}
	caData, err := s.fetchCRClientCA(ctx, ev.PlaneType, ev.PlaneID, ev.Namespace, ev.Name)
	if err != nil {
		return fmt.Errorf("failed to fetch CR CA for re-validation: %w", err)
	}
	if _, _, err := s.connMgr.RevalidateCR(ev.PlaneType, ev.PlaneID, ev.Namespace, ev.Name, caData, revocations); err != nil {
		s.logger.Warn("CR re-validation failed", "error", err)
	}
	return nil
}

// revokeCR withdraws a deleted CR from this pod: its authorization on the
// plane's agent connections and the streaming sessions opened for it.
func (s *Server) revokeCR(planeType, planeID, namespace, name string) (revoked int, disconnected int) {
	revoked, disconnected = s.connMgr.RevokeCR(planeType, planeID, namespace, name)

	planeIdentifier := fmt.Sprintf("%s/%s", planeType, planeID)
	crKey := fmt.Sprintf("%s/%s", namespace, name)
	// Recorded, so that a connect does not grant the CR again from a
	// planeReader copy that still lists it.
	s.noteLiveCRGone(planeType, crKey)
	if closed := s.closeStreamSessionsForCR(planeIdentifier, crKey); closed > 0 {
		s.logger.Info("closed streaming sessions of deleted CR",
			"planeType", planeType,
			"planeID", planeID,
			"cr", crKey,
			"sessions", closed,
		)
	}
	return revoked, disconnected
}

// deletedCRSweepInterval bounds how long a deleted CR can stay authorized on
// this pod after its "deleted" event was missed.
const deletedCRSweepInterval = time.Minute

// runDeletedCRSweep periodically revokes locally authorized CRs that no
// longer exist.
func (s *Server) runDeletedCRSweep(ctx context.Context) {
	ticker := time.NewTicker(deletedCRSweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.revokeDeletedCRs(ctx)
		}
	}
}

// revokeDeletedCRs revokes every CR this pod has authorized that no longer
// exists. A "deleted" event reaches a pod at most once: the mesh drops it when
// its outbox is full or the peer link is down, and no later event withdraws a
// CR-scoped authorization. Checking against the API server recovers from that.
// A terminating CR still exists and stays authorized: its finalizer cleans up
// through the agent. The pass stops as soon as ctx is canceled.
//
// A pass lists each plane type once, so its cost does not grow with the number
// of authorized CRs.
func (s *Server) revokeDeletedCRs(ctx context.Context) {
	type authorizedCR struct{ planeType, planeID, crKey string }

	// Collected before listing: a CR that is created and authorized after the
	// listing is then not in this set, and cannot be taken for a deleted one.
	authorized := make(map[authorizedCR]struct{})
	for _, conn := range s.connMgr.GetAll() {
		for _, crKey := range conn.GetValidCRs() {
			authorized[authorizedCR{conn.PlaneType, conn.PlaneID, crKey}] = struct{}{}
		}
	}

	existing := make(map[string]map[string]string)
	for cr := range authorized {
		if _, listed := existing[cr.planeType]; listed {
			continue
		}
		crs, err := s.existingPlaneCRs(ctx, cr.planeType)
		if err != nil {
			// A failed listing says nothing about which CRs exist. Skip the
			// whole pass rather than revoke on a partial view.
			s.logger.Warn("skipping the deleted-CR sweep, failed to list plane CRs",
				"planeType", cr.planeType,
				"error", err,
			)
			return
		}
		existing[cr.planeType] = crs
	}

	for cr := range authorized {
		// A CR moved to another planeID is no longer served by this plane's
		// agents. Its events revalidate only its new plane, so this check is
		// the only thing that removes the old grant.
		if planeID, exists := existing[cr.planeType][cr.crKey]; exists && planeID == cr.planeID {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		s.revokeStaleCR(ctx, cr.planeType, cr.planeID, cr.crKey)
	}
}

// revokeStaleCR revokes a CR that the sweep's listing reports as deleted or
// moved to another planeID. The listing can be stale, so the CR is looked up
// first: one re-created on this plane since then is left alone, and so is one
// whose lookup fails.
func (s *Server) revokeStaleCR(ctx context.Context, planeType, planeID, crKey string) {
	namespace, name, _ := strings.Cut(crKey, "/")
	obj, err := s.getPlaneCR(ctx, planeType, namespace, name)
	switch {
	case apierrors.IsNotFound(err):
		s.logger.Warn("revoking a CR that no longer exists, its deleted event was missed",
			"planeType", planeType,
			"planeID", planeID,
			"cr", crKey,
		)
	case err != nil:
		s.logger.Warn("skipping a CR the sweep could not look up",
			"planeType", planeType,
			"planeID", planeID,
			"cr", crKey,
			"error", err,
		)
		return
	default:
		current := planeCRPlaneID(obj)
		if current == planeID {
			return // re-created since the listing: still served here
		}
		s.logger.Warn("revoking a CR that moved to another planeID",
			"planeType", planeType,
			"planeID", planeID,
			"newPlaneID", current,
			"cr", crKey,
		)
		s.revokeCR(planeType, planeID, namespace, name)
		return
	}

	s.revokeCR(planeType, planeID, namespace, name)

	// The CR can still be re-created between the lookup and the revoke. The
	// revoke then removed the new CR's grant, and no later event would grant
	// it again. Grant it here if it exists on this plane now.
	if obj, err := s.getPlaneCR(ctx, planeType, namespace, name); err == nil {
		if planeCRPlaneID(obj) == planeID {
			s.ApplyPlaneEvent(fabric.PlaneEvent{
				PlaneType: planeType, PlaneID: planeID, Event: "updated",
				Namespace: namespace, Name: name,
			})
		}
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
// It reads only that CR. Listing every CR that shares the planeID, as an agent
// connect does, would make each notification cost grow with the number of CRs.
func (s *Server) fetchCRClientCA(ctx context.Context, planeType, planeID, namespace, name string) ([]byte, error) {
	obj, extractInfo, err := planeCRObject(planeType, namespace)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	crKey := fmt.Sprintf("%s/%s", namespace, name)
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, obj); err != nil {
		if apierrors.IsNotFound(err) {
			s.noteLiveCRGone(planeType, crKey)
			return nil, fmt.Errorf("failed to get CR %s: %w: %w", crKey, errPlaneCRGone, err)
		}
		return nil, fmt.Errorf("failed to get CR %s: %w", crKey, err)
	}

	s.noteLiveCR(planeType, crKey, obj)

	// A CR that has moved to another planeID is not served by this plane's agents.
	info, ok := extractInfo(obj)
	if !ok || info.planeID != planeID {
		return nil, fmt.Errorf("CR %s %w %s/%s", crKey, errPlaneCRElsewhere, planeType, planeID)
	}

	caData, err := s.extractCAFromPlane(ctx, info.agent, info.namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to extract CA from CR %s: %w", crKey, err)
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
	// The plane-wide count lets a caller tell "no agent connected" apart from
	// "agents connected, none authorized for this CR".
	status.PlaneConnectedAgents = s.PlaneStatus(planeType, planeID).ConnectedAgents
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
//
// unread lists the CRs whose CA was not read within the lookup's time. They are
// not in validCRs, and the caller authorizes them once the connection is
// registered: see authorizeUnreadCRs.
func (s *Server) verifyClientCertificatePerCR(
	clientCert *x509.Certificate,
	intermediatePool *x509.CertPool,
	planeType, planeID string,
) (validCRs []string, unread []string, err error) {
	clientCN := clientCert.Subject.CommonName
	clientIssuer := clientCert.Issuer.CommonName

	s.logger.Info("performing per-CR certificate validation",
		"planeType", planeType,
		"planeID", planeID,
		"certificateCN", clientCN,
		"certificateIssuer", clientIssuer,
	)

	// Get ALL CRs with matching planeType and planeID
	crsClientCAData, unread, err := s.getAllPlaneClientCAs(planeType, planeID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get client CA configurations: %w", err)
	}

	if len(crsClientCAData) == 0 {
		s.logger.Warn("connection rejected: no CRs found for plane",
			"planeType", planeType,
			"planeID", planeID,
		)
		return nil, nil, fmt.Errorf("no %s CRs found with planeID '%s'", planeType, planeID)
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
			s.logger.Debug("certificate valid for CR",
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
		return nil, nil, fmt.Errorf("certificate not valid for any CR with planeID '%s'", planeID)
	}

	s.warnOnSharedClientCA(validCRs, crsClientCAData, planeType, planeID, clientCN)

	s.logger.Info("per-CR certificate validation successful",
		"planeType", planeType,
		"planeID", planeID,
		"clientCN", clientCN,
		"validCRs", validCRs,
		"totalCRs", len(crsClientCAData),
	)

	return validCRs, unread, nil
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

	local := s.connMgr.GetAll()
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
		if err := conn.SendRawMessage(goAway); err != nil {
			s.logger.Debug("failed to send rebalance GOAWAY", "connectionID", conn.ID, "error", err)
			continue
		}
		s.logger.Info("shed agent connection for rebalancing",
			"connectionID", conn.ID,
			"plane", conn.PlaneIdentifier,
		)
	}
}
