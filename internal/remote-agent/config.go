// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package remoteagent implements the OpenChoreo remote-agent: a per-project+env
// data-plane component that terminates TLS + yamux tunnels from `occ remote`, and for
// each stream calls the control plane's authorize endpoint to turn the session
// capability + target key into a concrete host:port, then dials it and pipes bytes.
// The agent holds no capability-signing/verification
// key: per-stream authorization is an online check against the control plane.
//
// A stream may also be a value fetch rather than a tunnel: for a key the control plane
// answers with Secret/ConfigMap coordinates, the agent reads that key from its own
// namespace through the Kubernetes API and returns the value on the stream. This is what
// keeps secret material on the path data plane -> occ, out of the control plane. The
// agent's ServiceAccount is granted `get` on exactly the object names the control plane
// authorized, so it cannot enumerate its namespace.
package remoteagent

import "time"

// Config configures the remote-agent server.
type Config struct {
	// ListenAddr is the TCP address the TLS tunnel listener binds to (e.g. ":8443").
	ListenAddr string

	// TLSCertPath / TLSKeyPath are the server certificate and key presented to occ.
	// The certificate is self-signed by the control plane at provisioning time; occ
	// pins it.
	TLSCertPath string
	TLSKeyPath  string

	// AuthorizeURL is the control plane's authorize endpoint the agent calls per
	// stream (e.g. "https://api.openchoreo.example/api/v1/remote-connect:authorize").
	AuthorizeURL string
	// AuthorizeCABundlePath, when set, pins the CA the agent trusts when calling the
	// control plane over TLS. Empty uses the system roots.
	AuthorizeCABundlePath string
	// AuthorizeInsecureSkipVerify disables TLS verification of the control plane
	// (development only).
	AuthorizeInsecureSkipVerify bool

	// HeartbeatURL is the control plane's heartbeat endpoint the agent calls
	// periodically while it has at least one live session, so the reaper keeps the
	// agent alive for the whole life of a connection. Empty disables heartbeats.
	HeartbeatURL string
	// HeartbeatInterval is how often the agent refreshes its liveness while it has live
	// sessions. Must be well under the reaper TTL. Zero uses DefaultHeartbeatInterval.
	HeartbeatInterval time.Duration
	// Namespace is the agent's own data-plane namespace (from the downward API), sent
	// in heartbeats so the control plane refreshes the right agent. Empty disables
	// heartbeats (the control plane could not identify which agent to refresh).
	Namespace string

	// HandshakeTimeout bounds how long the Hello/HelloResult exchange may take.
	HandshakeTimeout time.Duration
	// StreamOpenTimeout bounds how long occ may take to send StreamOpen after opening
	// a yamux stream.
	StreamOpenTimeout time.Duration
	// AuthorizeTimeout bounds a single authorize call to the control plane.
	AuthorizeTimeout time.Duration
	// DialTimeout bounds dialing an upstream dependency target.
	DialTimeout time.Duration
	// ReadTimeout bounds a single Secret/ConfigMap read against the Kubernetes API.
	ReadTimeout time.Duration

	// MaxStreamsPerSession caps concurrent streams on a single tunnel connection
	// (0 = unlimited).
	MaxStreamsPerSession int
	// MaxSessions caps concurrent tunnel connections to this agent (0 = unlimited).
	// One agent serves a whole project+env.
	MaxSessions int
	// AuthorizeRatePerSecond caps the sustained rate of authorize calls to the control
	// plane (0 = unlimited). Every new stream costs one call.
	AuthorizeRatePerSecond float64
	// AuthorizeBurst is how many authorize calls may arrive at once before the rate
	// applies. Ignored when AuthorizeRatePerSecond is 0.
	AuthorizeBurst int
}

// Default timeouts / limits.
const (
	DefaultHandshakeTimeout  = 10 * time.Second
	DefaultStreamOpenTimeout = 10 * time.Second
	DefaultAuthorizeTimeout  = 10 * time.Second
	DefaultDialTimeout       = 10 * time.Second
	DefaultReadTimeout       = 10 * time.Second
	// DefaultHeartbeatInterval is comfortably under the control plane's default reaper
	// TTL (30 min), so a couple of missed heartbeats don't reap a live agent.
	DefaultHeartbeatInterval = 60 * time.Second
	// DefaultMaxSessions bounds concurrent sessions on one project+env agent.
	DefaultMaxSessions = 64
	// DefaultAuthorizeRatePerSecond bounds the steady authorize load one agent puts on
	// the control plane.
	DefaultAuthorizeRatePerSecond = 20
	// DefaultAuthorizeBurst absorbs a session's start-up fan-out without raising the
	// sustained rate.
	DefaultAuthorizeBurst = 40
)

// withDefaults returns a copy of the config with zero-valued timeouts filled in.
func (c Config) withDefaults() Config {
	if c.HandshakeTimeout == 0 {
		c.HandshakeTimeout = DefaultHandshakeTimeout
	}
	if c.StreamOpenTimeout == 0 {
		c.StreamOpenTimeout = DefaultStreamOpenTimeout
	}
	if c.AuthorizeTimeout == 0 {
		c.AuthorizeTimeout = DefaultAuthorizeTimeout
	}
	if c.DialTimeout == 0 {
		c.DialTimeout = DefaultDialTimeout
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = DefaultReadTimeout
	}
	if c.HeartbeatInterval == 0 {
		c.HeartbeatInterval = DefaultHeartbeatInterval
	}
	return c
}
