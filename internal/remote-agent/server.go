// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remoteagent

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/yamux"

	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// streamAuthorizer resolves a (capability, key) into a dial target by asking the
// control plane. Implemented by *authorizer; stubbed in tests.
type streamAuthorizer interface {
	authorize(ctx context.Context, capability, key string) (*remoteconnect.AuthorizeResponse, error)
}

// streamHeartbeater refreshes the agent's liveness with the control plane while it has
// live sessions. Implemented by *authorizer; nil (disabled) in tests or when unconfigured.
type streamHeartbeater interface {
	heartbeat(ctx context.Context, capability, agentNamespace string) error
}

// Server terminates remote-connect tunnels and forwards each multiplexed stream to the
// dependency target the control plane authorizes for that stream's key.
type Server struct {
	cfg  Config
	auth streamAuthorizer
	hb   streamHeartbeater // nil disables heartbeats
	log  *slog.Logger

	// sessions tracks live tunnel sessions so the heartbeat loop can keep the agent
	// alive for the whole life of a connection (not just when new streams open).
	sessions *sessionTracker

	// dialer dials upstream dependency targets. Overridable in tests.
	dialer func(ctx context.Context, network, addr string) (net.Conn, error)
}

// sessionTracker records the capability of each live tunnel session. Its count drives
// whether the agent heartbeats; sample() yields any live capability to present.
type sessionTracker struct {
	mu   sync.Mutex
	next uint64
	caps map[uint64]string
}

func newSessionTracker() *sessionTracker { return &sessionTracker{caps: map[uint64]string{}} }

func (t *sessionTracker) add(capability string) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	id := t.next
	t.next++
	t.caps[id] = capability
	return id
}

func (t *sessionTracker) remove(id uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.caps, id)
}

// sample returns the capability of one live session, or ("", false) if none are live.
func (t *sessionTracker) sample() (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, c := range t.caps {
		return c, true
	}
	return "", false
}

// New builds a Server from cfg, constructing the control-plane authorizer from the
// configured authorize URL / CA. Use this from main; tests use NewServer to inject a
// fake authorizer.
func New(cfg Config, log *slog.Logger) (*Server, error) {
	cfg = cfg.withDefaults()
	auth, err := newAuthorizer(cfg.AuthorizeURL, cfg.HeartbeatURL, cfg.AuthorizeCABundlePath, cfg.AuthorizeInsecureSkipVerify, cfg.AuthorizeTimeout)
	if err != nil {
		return nil, err
	}
	srv := NewServer(cfg, auth, log)
	// Heartbeats need both an endpoint and the agent's own namespace to name itself.
	if cfg.HeartbeatURL != "" && cfg.Namespace != "" {
		srv.hb = auth
	} else {
		log.Warn("remote-agent heartbeats disabled (no heartbeat URL or namespace configured); "+
			"the agent may be reaped during a long-lived idle session",
			"heartbeatURL", cfg.HeartbeatURL, "namespace", cfg.Namespace)
	}
	return srv, nil
}

// NewServer builds a Server that authorizes streams via auth. Heartbeats are off unless
// the caller sets hb (New does when configured).
func NewServer(cfg Config, auth streamAuthorizer, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		cfg:      cfg.withDefaults(),
		auth:     auth,
		log:      log,
		sessions: newSessionTracker(),
		dialer:   (&net.Dialer{}).DialContext,
	}
}

// Run binds a TLS listener from the configured cert/key and serves until ctx is done.
func (s *Server) Run(ctx context.Context) error {
	tlsCfg, err := serverTLSConfig(s.cfg.TLSCertPath, s.cfg.TLSKeyPath)
	if err != nil {
		return err
	}
	ln, err := tls.Listen("tcp", s.cfg.ListenAddr, tlsCfg)
	if err != nil {
		return fmt.Errorf("remote-agent: listen on %s: %w", s.cfg.ListenAddr, err)
	}
	return s.Serve(ctx, ln)
}

// Serve accepts tunnel connections on ln until ctx is done. Exposed for tests, which
// can pass a plain (non-TLS) listener since the protocol is transport-agnostic.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	go s.heartbeatLoop(ctx)
	s.log.Info("remote-agent listening", "addr", ln.Addr().String())
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil // graceful shutdown
			}
			return fmt.Errorf("remote-agent: accept: %w", err)
		}
		go s.handleConn(ctx, conn)
	}
}

// handleConn runs the handshake for one tunnel connection, then multiplexes streams.
func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()

	capability, ok := s.handshake(conn, remote)
	if !ok {
		return
	}
	s.log.Info("tunnel established", "remote", remote)

	// Register the session so the heartbeat loop keeps this agent alive for the whole
	// life of the connection — even while it is idle (no new streams), as when a
	// debugger is paused at a breakpoint.
	sessionID := s.sessions.add(capability)
	defer s.sessions.remove(sessionID)

	ycfg := yamux.DefaultConfig()
	ycfg.LogOutput = io.Discard // quiet yamux; our logging is structured
	session, err := yamux.Server(conn, ycfg)
	if err != nil {
		s.log.Warn("yamux session setup failed", "remote", remote, "error", err)
		return
	}
	defer session.Close()

	var sem chan struct{}
	if s.cfg.MaxStreamsPerSession > 0 {
		sem = make(chan struct{}, s.cfg.MaxStreamsPerSession)
	}
	for {
		stream, err := session.AcceptStream()
		if err != nil {
			return // session closed or client gone
		}
		if sem != nil {
			select {
			case sem <- struct{}{}:
			default:
				go s.rejectStream(stream, "too many concurrent streams")
				continue
			}
		}
		go func(st net.Conn) {
			if sem != nil {
				defer func() { <-sem }()
			}
			s.handleStream(ctx, st, capability)
		}(stream)
	}
}

// heartbeatLoop periodically refreshes the agent's liveness with the control plane
// while it has at least one live session, so the reaper keeps it alive for the full
// life of a connection. It is a no-op when heartbeats are unconfigured.
func (s *Server) heartbeatLoop(ctx context.Context) {
	if s.hb == nil || s.cfg.HeartbeatInterval <= 0 {
		return
	}
	ticker := time.NewTicker(s.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			capability, active := s.sessions.sample()
			if !active {
				continue // nothing to keep alive
			}
			hbCtx, cancel := context.WithTimeout(ctx, s.cfg.AuthorizeTimeout)
			err := s.hb.heartbeat(hbCtx, capability, s.cfg.Namespace)
			cancel()
			if err != nil {
				s.log.Debug("heartbeat failed", "error", err)
			}
		}
	}
}

// handshake reads Hello and replies HelloResult. The agent does not verify the
// capability here — it holds no key. It simply checks the protocol version and retains
// the capability to replay to the control plane's authorize endpoint on each stream.
func (s *Server) handshake(conn net.Conn, remote string) (capability string, ok bool) {
	_ = conn.SetDeadline(time.Now().Add(s.cfg.HandshakeTimeout))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	var hello remoteconnect.Hello
	if err := remoteconnect.ReadMessage(conn, &hello); err != nil {
		s.log.Warn("handshake read failed", "remote", remote, "error", err)
		return "", false
	}
	if hello.ProtocolVersion != remoteconnect.ProtocolVersion {
		_ = remoteconnect.WriteMessage(conn, remoteconnect.HelloResult{OK: false, Error: "unsupported protocol version"})
		return "", false
	}
	if hello.Capability == "" {
		_ = remoteconnect.WriteMessage(conn, remoteconnect.HelloResult{OK: false, Error: "missing capability"})
		return "", false
	}
	if err := remoteconnect.WriteMessage(conn, remoteconnect.HelloResult{OK: true}); err != nil {
		s.log.Warn("handshake reply failed", "remote", remote, "error", err)
		return "", false
	}
	return hello.Capability, true
}

func (s *Server) rejectStream(stream net.Conn, reason string) {
	defer stream.Close()
	_ = remoteconnect.WriteMessage(stream, remoteconnect.StreamResult{OK: false, Error: reason})
}

func serverTLSConfig(certPath, keyPath string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("remote-agent: load TLS keypair: %w", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}
