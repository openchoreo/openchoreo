// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

// maxReconnectJitter spreads a herd of agents reconnecting after GOAWAY so
// they do not all land on the same replica. Non-cryptographic randomness is
// fine here: this only needs to break synchrony, not be unpredictable.
const maxReconnectJitter = 750 * time.Millisecond

// Connection abstracts a WebSocket connection for testability.
// *websocket.Conn satisfies this interface.
type Connection interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	SetPingHandler(h func(appData string) error)
	Close() error
}

type Agent struct {
	config     *Config
	clientCert tls.Certificate
	serverCA   *x509.CertPool
	conn       Connection
	k8sClient  client.Client
	k8sConfig  *rest.Config
	router     *Router
	mu         sync.Mutex
	logger     *slog.Logger
	stopChan   chan struct{}
	// activeStreams tracks active exec streaming sessions indexed by requestID
	activeStreams   map[string]*execSession
	activeStreamsMu sync.Mutex
	// hubbleStreams tracks active hubble flow streaming sessions indexed by requestID
	hubbleStreams   map[string]*hubbleSession
	hubbleStreamsMu sync.Mutex
}

func New(cfg *Config, k8sClient client.Client, k8sConfig *rest.Config, logger *slog.Logger) (*Agent, error) {
	var cert tls.Certificate
	var serverCertPool *x509.CertPool

	if cfg.TLSEnabled {
		// The URL scheme, not this flag, is what actually selects the
		// transport: the websocket dialer speaks plaintext for ws:// and
		// ignores TLSClientConfig entirely. Left unchecked, the agent would
		// load its certificates, log that the CA was accepted, and then
		// forward control-plane instructions to the plane's Kubernetes API
		// over an unencrypted socket, presenting no client certificate and
		// verifying no server - the same silent downgrade the CA checks below
		// exist to prevent, so fail the same way.
		u, err := url.Parse(cfg.ServerURL)
		if err != nil {
			return nil, fmt.Errorf("invalid server URL %q: %w", cfg.ServerURL, err)
		}
		if u.Scheme != "wss" {
			return nil, fmt.Errorf("TLS is enabled but the server URL scheme is %q: "+
				"use wss:// to connect over TLS, or --tls-enabled=false to disable TLS", u.Scheme)
		}

		// Load client certificate
		cert, err = tls.LoadX509KeyPair(cfg.ClientCertPath, cfg.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}

		// Load server CA certificate.
		//
		// Failures here are fatal rather than degraded: falling back to an
		// unverified connection would silently turn the tunnel into something
		// any host able to answer the gateway's address could impersonate,
		// and the agent forwards control-plane instructions straight into the
		// plane's Kubernetes API. A misconfigured CA path must be loud. Use
		// --tls-enabled=false to opt out of TLS explicitly for dev.
		if cfg.ServerCAPath == "" {
			return nil, fmt.Errorf("TLS is enabled but no server CA is configured: " +
				"set --server-ca to verify the gateway, or --tls-enabled=false to disable TLS")
		}
		serverCACert, err := os.ReadFile(cfg.ServerCAPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read server CA certificate %s: %w", cfg.ServerCAPath, err)
		}
		serverCertPool = x509.NewCertPool()
		if !serverCertPool.AppendCertsFromPEM(serverCACert) {
			return nil, fmt.Errorf("failed to parse server CA certificate %s: no valid certificates found",
				cfg.ServerCAPath)
		}
		logger.Info("server CA certificate loaded successfully")
	} else {
		logger.Info("TLS disabled, connecting without mTLS")
	}

	// Create router for HTTP proxy support
	router, err := NewRouter(k8sConfig, cfg.Routes, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create router: %w", err)
	}

	return &Agent{
		config:        cfg,
		clientCert:    cert,
		serverCA:      serverCertPool,
		k8sClient:     k8sClient,
		k8sConfig:     k8sConfig,
		router:        router,
		logger:        logger.With("component", "agent", "planeID", cfg.PlaneID),
		stopChan:      make(chan struct{}),
		activeStreams: make(map[string]*execSession),
		hubbleStreams: make(map[string]*hubbleSession),
	}, nil
}

func (a *Agent) Start(ctx context.Context) error {
	a.logger.Info("starting agent",
		"planeType", a.config.PlaneType,
		"planeID", a.config.PlaneID,
		"serverURL", a.config.ServerURL,
	)

	// The loop below is the only owner of a.conn, so it closes the socket on
	// every way out. Several exits used to rely on the connection watcher
	// started by handleConnection, which no longer outlives its connection.
	defer a.closeConnection()

	for {
		// Check for cancellation before attempting connection
		select {
		case <-ctx.Done():
			a.logger.Info("agent stopping due to context cancellation")
			a.closeConnection()
			return ctx.Err()
		case <-a.stopChan:
			a.logger.Info("agent stopping")
			a.closeConnection()
			return nil
		default:
		}

		// Release the previous session's socket before dialing a new one.
		// connect() overwrites a.conn, so anything still open here would never
		// be closed. Deliberately not done when handleConnection returns: on a
		// GOAWAY the gateway keeps serving for its drain window, and responses
		// to requests already dispatched can still be delivered over the old
		// socket while we wait out the jitter.
		a.closeConnection()

		// Attempt to connect
		if err := a.connect(); err != nil {
			a.logger.Error("connection failed",
				"error", err,
				"retryAfter", a.config.ReconnectDelay,
			)

			// Wait before retrying, checking for cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-a.stopChan:
				return nil
			case <-time.After(a.config.ReconnectDelay):
				continue
			}
		}

		// Handle messages on the established connection
		// This will block until connection is lost or context is canceled
		graceful := a.handleConnection(ctx)

		// A GOAWAY is a planned handover: the gateway replica told us it is
		// draining and is still serving other agents. Waiting out the reconnect
		// backoff here would leave this plane unroutable for the whole delay
		// even though a healthy replica is ready right now — exactly what the
		// drain choreography exists to avoid. Only back off when the
		// connection dropped unexpectedly, where retrying immediately would
		// risk hammering a gateway that is actually unhealthy.
		if graceful {
			// Reconnect promptly, but not in lockstep with every other agent
			// that was just evicted. A whole replica's worth of agents receive
			// GOAWAY together, and if they all redial at the same instant they
			// tend to land on whichever replica is ready right then — the
			// concentration that makes one pod's restart take every plane down
			// at once. A brief random spread lets the load balancer place them
			// independently. Kept well under a second so this stays far cheaper
			// than the reconnect backoff it replaces.
			//nolint:gosec // Breaking reconnect synchrony, not a security decision.
			jitter := time.Duration(rand.Int64N(int64(maxReconnectJitter)))
			a.logger.Info("reconnecting after GOAWAY", "jitter", jitter)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-a.stopChan:
				return nil
			case <-time.After(jitter):
			}
			continue
		}

		// Connection lost, wait before reconnecting
		a.logger.Info("connection lost, reconnecting",
			"delay", a.config.ReconnectDelay,
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.stopChan:
			return nil
		case <-time.After(a.config.ReconnectDelay):
			continue
		}
	}
}

func (a *Agent) Stop() {
	close(a.stopChan)
}

// connect establishes a WebSocket connection to the control plane
func (a *Agent) connect() error {
	u, err := url.Parse(a.config.ServerURL)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	query := u.Query()
	query.Set("planeType", a.config.PlaneType)
	query.Set("planeID", a.config.PlaneID)
	u.RawQuery = query.Encode()

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	if a.config.TLSEnabled {
		dialer.TLSClientConfig = &tls.Config{
			Certificates: []tls.Certificate{a.clientCert},
			RootCAs:      a.serverCA,
			MinVersion:   tls.VersionTLS12,
		}
	} else {
		dialer.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, //nolint:gosec // Intentional: TLS disabled via configs
		}
	}

	a.logger.Info("connecting to control plane", "url", u.String())

	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	// No lock needed here - connect() is only called from the single-threaded Start() loop
	// and no other goroutine accesses a.conn during connection establishment
	a.conn = conn

	a.logger.Info("connected to control plane")
	return nil
}

// handleConnection handles an established WebSocket connection. It reports
// whether the connection ended gracefully — i.e. the gateway sent a GOAWAY
// asking us to move to another replica — so the caller can reconnect
// immediately instead of serving out the reconnect backoff.
func (a *Agent) handleConnection(ctx context.Context) (graceful bool) {
	// Bind this session's connection once. The watcher below clears a.conn to
	// unblock the read, so re-reading the field on every loop turn would race
	// with that write - and read from nil once it lands. Holding the socket
	// locally keeps the unblock working: the close still makes ReadMessage
	// return an error, which is what ends the loop.
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		a.logger.Debug("no connection to handle")
		return false
	}

	// Setup ping/pong handlers for connection health
	conn.SetPingHandler(func(appData string) error {
		a.logger.Debug("received ping from server")
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second))
	})

	// Handle context cancellation asynchronously by closing the connection.
	// This causes ReadMessage() to unblock with an error, terminating the loop.
	//
	// done retires the watcher when the connection ends for any other reason.
	// Without it every reconnect leaves one behind, parked on a context that
	// only fires at agent shutdown - and a long-lived agent reconnects on
	// every gateway rollout. The stragglers are not just idle: each closes
	// whatever connection is current when the context finally fires, so a
	// healthy session inherits the closes owed to sessions long gone.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-done:
			return
		case <-ctx.Done():
		}
		// Both channels can be ready before this goroutine is first scheduled,
		// and select would then pick between them at random. Re-check done so
		// a retired watcher never closes a socket: past this point the
		// connection has ended and Start owns what replaces it.
		select {
		case <-done:
			return
		default:
		}
		a.logger.Debug("context canceled, closing connection")
		a.closeConnection()
	}()

	// Main message processing loop
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				a.logger.Error("websocket error", "error", err)
			} else {
				a.logger.Debug("connection closed", "error", err)
			}
			return false
		}

		// GOAWAY: the gateway replica is draining — return so the reconnect
		// loop dials again through the load balancer and lands on a surviving
		// replica. Reported as graceful so the caller skips the backoff.
		var goAway messaging.GoAway
		if err := json.Unmarshal(message, &goAway); err == nil && goAway.Type == messaging.MessageTypeGoAway {
			a.logger.Info("received GOAWAY from gateway, reconnecting", "reason", goAway.Reason)
			return true
		}

		// Try to parse as stream init (exec / hubble requests)
		var streamInit messaging.HTTPTunnelStreamInit
		if err := json.Unmarshal(message, &streamInit); err == nil && streamInit.IsUpgrade && streamInit.RequestID != "" {
			switch streamInit.Target {
			case "hubble":
				go a.handleHubbleStreamInit(ctx, &streamInit)
			default:
				go a.handleHTTPTunnelStreamInit(&streamInit)
			}
			continue
		}

		// Try to parse as stream chunk (stdin data for active exec sessions, or
		// the close signal for exec/hubble sessions).
		var streamChunk messaging.HTTPTunnelStreamChunk
		if err := json.Unmarshal(message, &streamChunk); err == nil && streamChunk.RequestID != "" && (streamChunk.Data != nil || streamChunk.IsClose) {
			if !a.routeHubbleChunk(&streamChunk) {
				a.routeStreamChunk(&streamChunk)
			}
			continue
		}

		// Parse as regular HTTPTunnelRequest
		var httpReq messaging.HTTPTunnelRequest
		if err := json.Unmarshal(message, &httpReq); err != nil {
			a.logger.Warn("failed to parse HTTP tunnel request", "error", err)
			continue
		}

		if httpReq.RequestID == "" {
			a.logger.Warn("received HTTP tunnel request without requestID")
			continue
		}

		go a.handleHTTPTunnelRequest(&httpReq)
	}
}

// handleHTTPTunnelRequest handles HTTPTunnelRequest
func (a *Agent) handleHTTPTunnelRequest(req *messaging.HTTPTunnelRequest) {
	a.logger.Info("received HTTP tunnel request",
		"target", req.Target,
		"method", req.Method,
		"path", req.Path,
		"requestID", req.RequestID,
	)

	// Route the request to the appropriate backend service
	response := a.router.Route(req)

	if err := a.sendHTTPTunnelResponse(response); err != nil {
		a.logger.Error("failed to send HTTP tunnel response",
			"requestID", req.RequestID,
			"error", err,
		)
	}
}

func (a *Agent) sendHTTPTunnelResponse(resp *messaging.HTTPTunnelResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.conn == nil {
		return messaging.ErrNotConnected
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("sendHTTPTunnelResponse: failed to marshal response: %w", err)
	}

	a.logger.Debug("sending HTTP tunnel response",
		"requestID", resp.RequestID,
		"statusCode", resp.StatusCode,
	)

	if err := a.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("sendHTTPTunnelResponse: failed to write message: %w", err)
	}
	return nil
}

func (a *Agent) closeConnection() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
	}
}
