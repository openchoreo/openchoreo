// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import "time"

// AgentAuthMode selects how the public agent listener obtains the client
// certificate chain that is subsequently verified per plane CR.
type AgentAuthMode string

const (
	// AgentAuthModeMTLS takes the client certificate from the TLS handshake
	// (r.TLS.PeerCertificates). This is the default and requires the agent's
	// TLS to reach the gateway unterminated (TLS passthrough / L4 exposure).
	AgentAuthModeMTLS AgentAuthMode = "mtls"
	// AgentAuthModeForwardedHeader takes the client certificate chain from an
	// HTTP header set by a trusted L7 proxy that terminated the agent's TLS
	// (e.g. AWS ALB mTLS passthrough, or Envoy/Istio XFCC). Only the extraction
	// point changes; the trust anchors and per-CR verification are identical to
	// mtls mode.
	AgentAuthModeForwardedHeader AgentAuthMode = "forwarded-header"

	// DefaultForwardedHeaderName is the header inspected in forwarded-header
	// mode when none is configured. It matches Envoy/Istio.
	DefaultForwardedHeaderName = "X-Forwarded-Client-Cert"
)

// Config holds configuration for the agent server
type Config struct {
	Port           int
	InternalPort   int
	ServerCertPath string
	ServerKeyPath  string
	// AgentAuthMode selects how the public listener obtains the agent client
	// certificate: AgentAuthModeMTLS (default, from the TLS handshake) or
	// AgentAuthModeForwardedHeader (from a header set by a trusted
	// TLS-terminating proxy). An empty value is treated as mtls.
	AgentAuthMode AgentAuthMode
	// AgentAuthForwardedHeader is the request header carrying the URL-encoded
	// client certificate chain when AgentAuthMode is
	// AgentAuthModeForwardedHeader. Empty defaults to DefaultForwardedHeaderName.
	AgentAuthForwardedHeader string
	// SkipClientCertVerify has no effect: agent client certificates are always
	// verified per plane CR on the public listener, and internal listener
	// verification is controlled by InternalMTLSEnabled.
	//
	// Deprecated: will be removed in a future release.
	SkipClientCertVerify bool
	// InternalMTLSEnabled requires and verifies client certificates on the
	// internal API listener (/api/*) against InternalClientCAPath.
	InternalMTLSEnabled bool
	// InternalClientCAPath is the path to the CA bundle used to verify
	// internal API clients. Required when InternalMTLSEnabled is true.
	InternalClientCAPath string
	ReadTimeout          time.Duration
	WriteTimeout         time.Duration
	IdleTimeout          time.Duration
	ShutdownTimeout      time.Duration
	HeartbeatInterval    time.Duration
	HeartbeatTimeout     time.Duration
	// DrainWindow is the period over which GOAWAY frames are spread across
	// agent connections during shutdown, so agents re-land on surviving
	// replicas without a reconnect stampede. Must be shorter than
	// ShutdownTimeout. Zero disables the choreographed drain (connections are
	// closed immediately).
	DrainWindow time.Duration
}

// RemoteServerClientConfig holds configuration for RemoteServerClient
type RemoteServerClientConfig struct {
	// ServerURL is the URL of the agent server (e.g., https://cluster-agent-server:8443)
	ServerURL string

	// InsecureSkipVerify disables TLS certificate verification (development only)
	InsecureSkipVerify bool

	// ServerCAPath is the path to the CA certificate for verifying the server's certificate
	// If empty and InsecureSkipVerify is false, system CA pool will be used
	ServerCAPath string

	// ClientCertPath is the path to the client certificate for mTLS (optional)
	ClientCertPath string

	// ClientKeyPath is the path to the client private key for mTLS (optional)
	ClientKeyPath string

	// Timeout is the HTTP client timeout
	Timeout time.Duration
}
