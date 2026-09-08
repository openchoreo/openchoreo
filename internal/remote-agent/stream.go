// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remoteagent

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// handleStream services one multiplexed stream: read StreamOpen, ask the control
// plane to authorize the key against the session capability (returning the concrete
// dial target), dial it, and pipe bytes both ways. The agent never trusts a
// client-supplied host — the target comes only from the control plane.
func (s *Server) handleStream(ctx context.Context, stream net.Conn, capability string) {
	defer stream.Close()

	_ = stream.SetReadDeadline(time.Now().Add(s.cfg.StreamOpenTimeout))
	var open remoteconnect.StreamOpen
	if err := remoteconnect.ReadMessage(stream, &open); err != nil {
		s.log.Debug("stream open read failed", "error", err)
		return
	}
	_ = stream.SetReadDeadline(time.Time{})

	authCtx, cancelAuth := context.WithTimeout(ctx, s.cfg.AuthorizeTimeout)
	target, err := s.auth.authorize(authCtx, capability, open.Key)
	cancelAuth()
	if err != nil {
		s.log.Warn("stream authorization failed", "key", open.Key, "error", err)
		_ = remoteconnect.WriteMessage(stream, remoteconnect.StreamResult{OK: false, Error: "not authorized"})
		return
	}

	// The client picks which agent to open a stream against; the capability says which
	// namespace should serve the target. Refuse the mismatch rather than dialing from
	// the wrong namespace.
	if target.AgentNamespace != "" && s.cfg.Namespace != "" && target.AgentNamespace != s.cfg.Namespace {
		s.log.Warn("stream target belongs to another agent", "key", open.Key,
			"target_namespace", target.AgentNamespace, "agent_namespace", s.cfg.Namespace)
		_ = remoteconnect.WriteMessage(stream, remoteconnect.StreamResult{OK: false, Error: "not authorized"})
		return
	}

	// The key's space and the control plane's answer must agree. A fetch key answered
	// with a dial target would open a TCP connection to a host the client never had a
	// grant for; a dial key answered with a grant would read a value it never had one
	// for. Either way the agent is being made someone's deputy, so refuse instead of
	// guessing which side is right.
	wantSecret := remoteconnect.IsSecretGrantKey(open.Key)
	if gotSecret := target.ResolvedKind() == remoteconnect.AuthorizeKindSecret; gotSecret != wantSecret {
		s.log.Warn("stream authorization kind mismatch", "key", open.Key,
			"kind", target.ResolvedKind())
		_ = remoteconnect.WriteMessage(stream, remoteconnect.StreamResult{OK: false, Error: "not authorized"})
		return
	}
	if wantSecret {
		s.serveFetch(ctx, stream, open.Key, target.Secret)
		return
	}

	network := target.Proto
	if network == "" {
		network = "tcp"
	}
	addr := net.JoinHostPort(target.Host, strconv.Itoa(target.Port))

	dialCtx, cancel := context.WithTimeout(ctx, s.cfg.DialTimeout)
	upstream, err := s.dialer(dialCtx, network, addr)
	cancel()
	if err != nil {
		s.log.Warn("dial upstream failed", "key", open.Key, "addr", addr, "error", err)
		_ = remoteconnect.WriteMessage(stream, remoteconnect.StreamResult{OK: false, Error: "dial failed"})
		return
	}
	defer upstream.Close()

	if err := remoteconnect.WriteMessage(stream, remoteconnect.StreamResult{OK: true}); err != nil {
		s.log.Debug("stream result write failed", "key", open.Key, "error", err)
		return
	}
	s.log.Debug("stream connected", "key", open.Key, "addr", addr)
	remoteconnect.Pipe(stream, upstream)
}

// serveFetch answers one value-fetch stream: read the authorized key from the agent's own
// namespace and write exactly one SecretResult. Nothing is piped — a fetch stream is a
// single request/response, not a byte channel.
//
// Errors are reported to the client as short, non-value-bearing reasons and logged
// without the value, so neither the log nor the wire can leak what was being read.
func (s *Server) serveFetch(ctx context.Context, stream net.Conn, key string, grant *remoteconnect.SecretGrant) {
	if grant == nil {
		s.log.Warn("fetch authorized with no grant", "key", key)
		_ = remoteconnect.WriteMessage(stream, remoteconnect.SecretResult{OK: false, Error: "not authorized"})
		return
	}
	if s.values == nil {
		// The agent has no Kubernetes identity (not running in-cluster, or the client
		// failed to build). Tunnels still work; say so plainly rather than timing out.
		s.log.Warn("fetch requested but no value reader is configured", "key", key)
		_ = remoteconnect.WriteMessage(stream, remoteconnect.SecretResult{
			OK: false, Error: "this agent cannot read values",
		})
		return
	}

	readCtx, cancel := context.WithTimeout(ctx, s.cfg.ReadTimeout)
	value, err := s.values.read(readCtx, *grant)
	cancel()
	if err != nil {
		// The reason is deliberately coarse. A caller who reaches here is already
		// authorized for this grant, but the API server's error text can name objects and
		// fields, and there is no reason to relay that to a developer's terminal.
		s.log.Warn("value read failed", "key", key,
			"sourceKind", grant.SourceKind, "sourceName", grant.SourceName, "error", err)
		_ = remoteconnect.WriteMessage(stream, remoteconnect.SecretResult{OK: false, Error: "read failed"})
		return
	}
	if len(value) > remoteconnect.MaxSecretValueSize {
		// Refuse rather than write a frame the peer will reject for length, which would
		// look like a transport fault instead of an oversized value.
		s.log.Warn("value too large to return", "key", key, "size", len(value))
		_ = remoteconnect.WriteMessage(stream, remoteconnect.SecretResult{OK: false, Error: "value too large"})
		return
	}

	s.log.Debug("value fetched", "key", key, "sourceKind", grant.SourceKind, "bytes", len(value))
	_ = remoteconnect.WriteMessage(stream, remoteconnect.SecretResult{OK: true, Value: value})
}
