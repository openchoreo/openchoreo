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
	"golang.org/x/time/rate"

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
	// alive for the whole life of a connection (not just when new streams open), and
	// caps how many are admitted at once.
	sessions *sessionTracker

	// authLimiter paces authorize calls to the control plane. Nil when unlimited.
	authLimiter *rate.Limiter

	// dialer dials upstream dependency targets. Overridable in tests.
	dialer func(ctx context.Context, network, addr string) (net.Conn, error)

	// values reads authorized Secret/ConfigMap keys from the agent's own namespace. Nil
	// when the agent has no Kubernetes identity — tunnels still work and fetches are
	// refused, rather than the whole agent failing to start.
	values valueReader
}

// sessionTracker records the most recent capability seen on each live tunnel session.
// Its count drives whether the agent heartbeats and enforces the concurrent-session
// cap; sample() yields a live capability to present.
//
// A session enters with no capability (it arrives with the first StreamOpen) and cannot
// be heartbeated until then. Not a gap: occ's own renewals resolve on a cadence shorter
// than the reaper's TTL, which refreshes the agent.
type sessionTracker struct {
	// max caps live sessions; 0 is unlimited.
	max int

	mu   sync.Mutex
	next uint64
	caps map[uint64]string
}

func newSessionTracker(maxSessions int) *sessionTracker {
	return &sessionTracker{max: maxSessions, caps: map[uint64]string{}}
}

// add admits a session and returns its id, or false when the agent is already at its
// session cap.
func (t *sessionTracker) add() (uint64, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.max > 0 && len(t.caps) >= t.max {
		return 0, false
	}
	id := t.next
	t.next++
	t.caps[id] = ""
	return id, true
}

// update records the capability a stream on this session presented, so the heartbeat
// presents a genuine one. Any capability will do as a liveness proof, so the freshest
// one seen simply wins.
func (t *sessionTracker) update(id uint64, capability string) {
	if capability == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, live := t.caps[id]; live {
		t.caps[id] = capability
	}
}

func (t *sessionTracker) remove(id uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.caps, id)
}

// sample returns a capability from one live session, or ("", false) when no live session
// has presented one yet. A session that has opened no stream is skipped rather than
// yielding an empty token the control plane would reject.
func (t *sessionTracker) sample() (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, c := range t.caps {
		if c != "" {
			return c, true
		}
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
	// Value reads need a Kubernetes identity and the agent's own namespace. A missing one
	// is a warning, not a failure: an agent that can only tunnel is still useful, and the
	// fetch path reports "this agent cannot read values" per stream.
	if cfg.Namespace != "" {
		reader, rerr := newK8sValueReader(cfg.Namespace)
		if rerr != nil {
			log.Warn("remote-agent value reads disabled (no usable Kubernetes client); "+
				"secret- and configmap-backed bindings will not resolve",
				"namespace", cfg.Namespace, "error", rerr)
		} else {
			srv.values = reader
		}
	} else {
		log.Warn("remote-agent value reads disabled (no namespace configured); " +
			"secret- and configmap-backed bindings will not resolve")
	}
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
	cfg = cfg.withDefaults()
	return &Server{
		cfg:         cfg,
		auth:        auth,
		log:         log,
		sessions:    newSessionTracker(cfg.MaxSessions),
		authLimiter: newAuthorizeLimiter(cfg),
		dialer:      (&net.Dialer{}).DialContext,
	}
}

// newAuthorizeLimiter builds the token bucket pacing authorize calls, or nil when the
// rate is unset. Burst is floored to 1, since a zero-burst limiter refuses every call.
func newAuthorizeLimiter(cfg Config) *rate.Limiter {
	if cfg.AuthorizeRatePerSecond <= 0 {
		return nil
	}
	burst := cfg.AuthorizeBurst
	if burst < 1 {
		burst = 1
	}
	return rate.NewLimiter(rate.Limit(cfg.AuthorizeRatePerSecond), burst)
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

	// The session registration keeps the agent alive for the whole life of the
	// connection — even while it is idle (no new streams), as when a debugger is paused
	// at a breakpoint. The capability arrives with the first stream.
	sessionID, ok := s.handshake(conn, remote)
	if !ok {
		return
	}
	defer s.sessions.remove(sessionID)
	s.log.Info("tunnel established", "remote", remote)

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
			s.handleStream(ctx, st, sessionID)
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

// handshake reads Hello, admits the session, and replies HelloResult, checking only that
// the peer speaks this protocol version. Hello carries no capability: the client is
// admitted by the TLS handshake against the agent's pinned certificate, and every stream
// is authorized on its own by the control plane. Returns the admitted session id.
func (s *Server) handshake(conn net.Conn, remote string) (uint64, bool) {
	_ = conn.SetDeadline(time.Now().Add(s.cfg.HandshakeTimeout))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	var hello remoteconnect.Hello
	if err := remoteconnect.ReadMessage(conn, &hello); err != nil {
		s.log.Warn("handshake read failed", "remote", remote, "error", err)
		return 0, false
	}
	if hello.ProtocolVersion != remoteconnect.ProtocolVersion {
		s.log.Warn("handshake rejected: protocol version",
			"remote", remote, "client", hello.ProtocolVersion, "agent", remoteconnect.ProtocolVersion)
		_ = remoteconnect.WriteMessage(conn, remoteconnect.HelloResult{OK: false, Error: fmt.Sprintf(
			"unsupported protocol version: this remote-agent speaks %d, occ sent %d; upgrade whichever is older",
			remoteconnect.ProtocolVersion, hello.ProtocolVersion)})
		return 0, false
	}
	// Claimed once the peer has sent a valid Hello, so a connection that stalls before
	// then holds no capacity.
	sessionID, admitted := s.sessions.add()
	if !admitted {
		s.log.Warn("tunnel refused: session limit reached", "remote", remote, "limit", s.cfg.MaxSessions)
		_ = remoteconnect.WriteMessage(conn, remoteconnect.HelloResult{OK: false,
			Error: "this remote-agent is already serving its maximum number of sessions"})
		return 0, false
	}
	if err := remoteconnect.WriteMessage(conn, remoteconnect.HelloResult{OK: true}); err != nil {
		s.sessions.remove(sessionID)
		s.log.Warn("handshake reply failed", "remote", remote, "error", err)
		return 0, false
	}
	return sessionID, true
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
