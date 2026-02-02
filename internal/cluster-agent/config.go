// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

import "time"

type Config struct {
	ServerURL         string
	PlaneType         string // "dataplane" or "buildplane" or "observabilityplane"
	PlaneID           string // Logical plane identifier (shared across multiple CRs with same physical plane)
	TLSEnabled        bool
	ClientCertPath    string
	ClientKeyPath     string
	ServerCAPath      string
	ReconnectDelay    time.Duration
	HeartbeatInterval time.Duration
	RequestTimeout    time.Duration
	Routes            []RouteConfig // Backend service routes for HTTP proxy
}
