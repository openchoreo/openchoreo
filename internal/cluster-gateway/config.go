// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import "time"

// Config holds configuration for the agent server
type Config struct {
	Port                 int
	ServerCertPath       string
	ServerKeyPath        string
	SkipClientCertVerify bool
	ReadTimeout          time.Duration
	WriteTimeout         time.Duration
	IdleTimeout          time.Duration
	ShutdownTimeout      time.Duration
	HeartbeatInterval    time.Duration
	HeartbeatTimeout     time.Duration
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
