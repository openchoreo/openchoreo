// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
)

// maxReconnectJitter spreads a herd of agents reconnecting after GOAWAY so
// they do not all land on the same replica. Non-cryptographic randomness is
// fine here: this only needs to break synchrony, not be unpredictable.
const maxReconnectJitter = 750 * time.Millisecond

// resourceTreeRequestTimeout bounds one resource tree match request. It is the
// bottom rung of protocol's timeout ladder, which owns the ordering against the
// gateway's tunnel timeout and the caller's client timeout: the agent answers —
// with whatever it has — before either gives up on it.
const resourceTreeRequestTimeout = protocol.AgentRequestTimeout

// heartbeatMisses is how many heartbeat intervals may pass with nothing at all
// heard from the gateway before the agent treats the connection as dead. It
// matches the gateway's own 3x ratio (30s ping, 90s timeout).
const heartbeatMisses = 3

// writeTimeout bounds one message write. Writers hold a.mu, which
// closeConnection also needs, so a write stuck on a dead peer would otherwise
// stall every other response and the reconnect itself. Generous enough for a
// large list response; gorilla fails the writes queued behind a timed-out one
// immediately, so a dead socket holds the lock for at most this long.
const writeTimeout = 30 * time.Second

// Connection abstracts a WebSocket connection for testability.
// *websocket.Conn satisfies this interface.
type Connection interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	SetPingHandler(h func(appData string) error)
	SetWriteDeadline(t time.Time) error
	Close() error
}

// activityConn pushes the read deadline forward whenever bytes arrive from the
// gateway. Watching frames is not enough: gorilla sends each server message as
// a single frame, so no ping or pong can arrive while a large one is still in
// flight, and a slow but healthy transfer would be mistaken for a dead peer.
// It sits under TLS, so it sees every byte on the wire.
type activityConn struct {
	net.Conn
	// timeout is zero until connect arms it after the handshake, leaving the
	// dialer's handshake deadline alone.
	timeout atomic.Int64
}

func (c *activityConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if t := time.Duration(c.timeout.Load()); n > 0 && t > 0 {
		_ = c.Conn.SetReadDeadline(time.Now().Add(t))
	}
	return n, err
}

type Agent struct {
	config     *Config
	clientCert tls.Certificate
	serverCA   *x509.CertPool
	conn       Connection
	k8sClient  client.Client
	k8sConfig  *rest.Config
	router     *Router
	// resourceTree answers resource tree match queries locally. It is nil when
	// the agent was built without a dynamic client, in which case those
	// requests fall through to the router and get its unknown-target 404.
	resourceTree *resourceTreeHandler
	mu           sync.Mutex
	logger       *slog.Logger
	stopChan     chan struct{}
	// activeStreams tracks active exec streaming sessions indexed by requestID
	activeStreams   map[string]*execSession
	activeStreamsMu sync.Mutex
	// hubbleStreams tracks active hubble flow streaming sessions indexed by requestID
	hubbleStreams   map[string]*hubbleSession
	hubbleStreamsMu sync.Mutex
}

func New(cfg *Config, k8sClient client.Client, k8sConfig *rest.Config, dyn dynamic.Interface, logger *slog.Logger) (*Agent, error) {
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

	var resourceTree *resourceTreeHandler
	if dyn != nil {
		resourceTree = newResourceTreeHandler(dyn, logger)
	} else {
		logger.Warn("no dynamic client provided; resource tree child discovery is disabled")
	}

	return &Agent{
		config:        cfg,
		clientCert:    cert,
		serverCA:      serverCertPool,
		k8sClient:     k8sClient,
		k8sConfig:     k8sConfig,
		router:        router,
		resourceTree:  resourceTree,
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

	// Detect a dead connection from this side. The gateway already drops an
	// agent it stops hearing from, but if the path goes half-open (a proxy,
	// load balancer or NAT forgets the flow without sending a reset) the
	// agent would otherwise sit in ReadMessage until TCP gives up - minutes
	// at best, never if a proxy keeps our leg alive - while the gateway
	// reports the plane as having no agents. Any bytes from the gateway push
	// the read deadline forward; handleConnection pings on the agent's own
	// interval so an idle connection does not depend on the gateway's ping
	// settings to stay alive.
	var activity *activityConn
	if a.config.HeartbeatInterval > 0 {
		dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			c, err := (&net.Dialer{}).DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			activity = &activityConn{Conn: c}
			return activity, nil
		}
	}

	a.logger.Info("connecting to control plane", "url", u.String())

	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	if activity != nil {
		timeout := heartbeatMisses * a.config.HeartbeatInterval
		activity.timeout.Store(int64(timeout))
		// This only fails on a socket that is already closed, and then the
		// first read fails too and Start reconnects.
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
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

	if interval := a.config.HeartbeatInterval; interval > 0 {
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
				}
				// The gateway's pong is what keeps the read deadline set in
				// connect moving on an idle connection. WriteControl is safe
				// alongside the a.mu-guarded writers, and a failed ping needs
				// no handling: the read deadline is what ends a dead session.
				// The ping holds gorilla's write lock, which message writers
				// wait on under a.mu, so it gets the same bound they do.
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(min(interval, writeTimeout))); err != nil {
					a.logger.Debug("failed to send ping", "error", err)
				}
			}
		}()
	}

	// Main message processing loop
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				a.logger.Warn("nothing heard from gateway within heartbeat timeout, reconnecting",
					"timeout", heartbeatMisses*a.config.HeartbeatInterval,
				)
			} else if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
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

	// Resource tree matching is answered by the agent itself rather than
	// proxied to a backend: it needs many list calls the tunnel should not pay
	// a round trip for each of.
	if req.Target == protocol.Target && a.resourceTree != nil {
		ctx, cancel := context.WithTimeout(context.Background(), resourceTreeRequestTimeout)
		defer cancel()

		if err := a.sendHTTPTunnelResponse(a.resourceTree.Handle(ctx, req)); err != nil {
			a.logger.Error("failed to send resource tree response",
				"requestID", req.RequestID,
				"error", err,
			)
		}
		return
	}

	// Bound the backend call: the gateway stops waiting after its own tunnel
	// timeout, and without a deadline a hung backend pins this goroutine for
	// good.
	ctx := context.Background()
	if a.config.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.config.RequestTimeout)
		defer cancel()
	}

	// Route the request to the appropriate backend service
	response := a.router.Route(ctx, req)

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

	if err := a.writeMessageLocked(data); err != nil {
		return fmt.Errorf("sendHTTPTunnelResponse: failed to write message: %w", err)
	}
	return nil
}

// writeMessageLocked writes one text message under the write deadline.
// The caller must hold a.mu and have checked that a.conn is set.
//
// A failed write closes the connection. gorilla makes a write error permanent,
// so the socket can carry no further responses, yet the read loop may sit on
// it until the heartbeat timeout - or indefinitely with heartbeats off. Closing
// it ends the read and sends Start straight to reconnecting.
func (a *Agent) writeMessageLocked(data []byte) error {
	err := a.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	if err == nil {
		err = a.conn.WriteMessage(websocket.TextMessage, data)
	}
	if err != nil {
		a.conn.Close()
		a.conn = nil
	}
	return err
}

func (a *Agent) closeConnection() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
	}
}
