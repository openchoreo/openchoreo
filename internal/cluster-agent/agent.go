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
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

type Agent struct {
	config     *Config
	clientCert tls.Certificate
	serverCA   *x509.CertPool
	conn       *websocket.Conn
	k8sClient  client.Client
	router     *Router
	mu         sync.Mutex
	logger     *slog.Logger
	stopChan   chan struct{}
}

func New(cfg *Config, k8sClient client.Client, k8sConfig *rest.Config, logger *slog.Logger) (*Agent, error) {
	var cert tls.Certificate
	var serverCertPool *x509.CertPool

	if cfg.TLSEnabled {
		// Load client certificate
		var err error
		cert, err = tls.LoadX509KeyPair(cfg.ClientCertPath, cfg.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}

		// Load server CA certificate
		serverCertPool = x509.NewCertPool()
		if cfg.ServerCAPath != "" {
			serverCACert, err := os.ReadFile(cfg.ServerCAPath)
			if err != nil {
				logger.Warn("failed to read server CA certificate",
					"path", cfg.ServerCAPath,
					"error", err,
				)
				logger.Warn("agent will connect without server verification")
			} else {
				if !serverCertPool.AppendCertsFromPEM(serverCACert) {
					logger.Warn("failed to parse server CA certificate")
					serverCertPool = nil
				} else {
					logger.Info("server CA certificate loaded successfully")
				}
			}
		}
	} else {
		logger.Info("TLS disabled, connecting without mTLS")
	}

	// Create router for HTTP proxy support
	router, err := NewRouter(k8sConfig, cfg.Routes, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create router: %w", err)
	}

	return &Agent{
		config:     cfg,
		clientCert: cert,
		serverCA:   serverCertPool,
		k8sClient:  k8sClient,
		router:     router,
		logger:     logger.With("component", "agent", "planeID", cfg.PlaneID),
		stopChan:   make(chan struct{}),
	}, nil
}

func (a *Agent) Start(ctx context.Context) error {
	a.logger.Info("starting agent",
		"planeType", a.config.PlaneType,
		"planeID", a.config.PlaneID,
		"serverURL", a.config.ServerURL,
	)

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
		a.handleConnection(ctx)

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

// handleConnection handles an established WebSocket connection
func (a *Agent) handleConnection(ctx context.Context) {
	// Setup ping/pong handlers for connection health
	a.conn.SetPingHandler(func(appData string) error {
		a.logger.Debug("received ping from server")
		return a.conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second))
	})

	// Handle context cancellation asynchronously by closing the connection
	// This causes ReadMessage() to unblock with an error, terminating the loop
	go func() {
		<-ctx.Done()
		a.logger.Debug("context canceled, closing connection")
		a.closeConnection()
	}()

	// Main message processing loop
	for {
		_, message, err := a.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				a.logger.Error("websocket error", "error", err)
			} else {
				a.logger.Debug("connection closed", "error", err)
			}
			return
		}

		// Parse as HTTPTunnelRequest
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
