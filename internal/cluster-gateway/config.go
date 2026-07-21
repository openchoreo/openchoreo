// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import "time"

// Config holds configuration for the agent server
type Config struct {
	Port                 int
	InternalPort         int
	ServerCertPath       string
	ServerKeyPath        string
	SkipClientCertVerify bool
	ReadTimeout          time.Duration
	WriteTimeout         time.Duration
	IdleTimeout          time.Duration
	ShutdownTimeout      time.Duration
	HeartbeatInterval    time.Duration
	HeartbeatTimeout     time.Duration

	// TrustClientCertHeader enables falling back to a caller-supplied HTTP header for the
	// agent's client certificate when the TLS handshake itself carries none. This is for
	// deployments where a TLS-terminating L7 load balancer sits in front of the public
	// listener and forwards the certificate it received from the agent as a header instead
	// of passing the TLS session through untouched.
	//
	// This is a network-trust decision, not an application-layer one: the header is trusted
	// verbatim, so it MUST only be enabled when the gateway's public listener is unreachable
	// except through the load balancer (e.g. via NetworkPolicy or security group rules).
	// Anyone who can reach the listener directly can set this header to impersonate any agent.
	TrustClientCertHeader bool

	// ClientCertHeaderName is the HTTP header the load balancer is expected to populate with
	// the client certificate when TrustClientCertHeader is enabled. The value must be the
	// standard base64 encoding of one or more concatenated PEM-encoded certificates (leaf
	// first, followed by any intermediates).
	ClientCertHeaderName string
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
