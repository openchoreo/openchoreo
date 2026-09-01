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
