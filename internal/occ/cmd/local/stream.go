// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/openchoreo/openchoreo/internal/depconnect"
)

// depAgentEndpointOverrideEnv, when set, makes occ dial this host:port instead of the
// endpoint the resolve call returned. Used for local development where the dep-agent's
// L4 Service isn't directly routable from the laptop (e.g. a k3d NodePort behind a
// `kubectl port-forward`) — the TLS pin (CA bundle + server name) still comes from the
// resolve response, so the agent's certificate is verified regardless of dial address.
const depAgentEndpointOverrideEnv = "OCC_DEV_AGENT_ENDPOINT"

// dialRetryTimeout bounds how long occ retries connecting to the dep-agent endpoint.
// It absorbs the window where a freshly-provisioned L4 Service is still coming up (a
// cloud LoadBalancer being assigned, or a local port-forward being established).
const dialRetryTimeout = 30 * time.Second

// dialDepAgentTunnel opens a single yamux tunnel to one dep-agent, presenting the
// capability in the Hello handshake. One TunnelClient is opened per dep-agent (a
// workload's dependencies fan out to one agent per provider namespace) and shared across
// that agent's targets; each accepted local connection becomes one yamux stream (see
// connect.go). The agent's SNI + pinned cert come from the resolve response, so
// overriding the dial address stays safe.
func dialDepAgentTunnel(ctx context.Context, agent depconnect.AgentEndpoint, capability string) (*depconnect.TunnelClient, error) {
	endpoint := agent.Endpoint
	if override := os.Getenv(depAgentEndpointOverrideEnv); override != "" {
		endpoint = override
	}
	if endpoint == "" {
		return nil, fmt.Errorf("resolve returned no dep-agent endpoint")
	}

	deadline := time.Now().Add(dialRetryTimeout)
	var lastErr error
	for {
		client, err := depconnect.DialTunnel(ctx, endpoint, agent.CABundle, agent.ServerName, capability)
		if err == nil {
			return client, nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("connect to dep-agent %s (%s): %w", endpoint, agent.ServerName, lastErr)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("connect to dep-agent %s (%s): %w", endpoint, agent.ServerName, ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}
