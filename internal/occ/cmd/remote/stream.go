// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// remoteAgentEndpointOverrideEnv, when set, makes occ dial this host:port instead of the
// endpoint the resolve call returned. Used for local development where the remote-agent's
// L4 Service isn't directly routable from the laptop (e.g. a k3d NodePort behind a
// `kubectl port-forward`) — the TLS pin (CA bundle + server name) still comes from the
// resolve response, so the agent's certificate is verified regardless of dial address.
const remoteAgentEndpointOverrideEnv = "OCC_REMOTE_AGENT_ENDPOINT"

// dialRetryTimeout bounds how long occ retries connecting to the remote-agent endpoint.
// It absorbs the window where a freshly-provisioned L4 Service is still coming up (a
// cloud LoadBalancer being assigned, or a local port-forward being established).
const dialRetryTimeout = 30 * time.Second

// dialRemoteAgentTunnel opens a single yamux tunnel to one remote-agent, presenting the
// capability in the Hello handshake. One TunnelClient is opened per remote-agent (a
// workload's dependencies fan out to one agent per provider namespace) and shared across
// that agent's targets; each accepted local connection becomes one yamux stream (see
// connect.go). The agent's SNI + pinned cert come from the resolve response, so
// overriding the dial address stays safe.
func dialRemoteAgentTunnel(ctx context.Context, agent remoteconnect.AgentEndpoint, capability string) (*remoteconnect.TunnelClient, error) {
	endpoint := agent.Endpoint
	if override := os.Getenv(remoteAgentEndpointOverrideEnv); override != "" {
		endpoint = override
	}
	if endpoint == "" {
		return nil, fmt.Errorf("resolve returned no remote-agent endpoint")
	}

	deadline := time.Now().Add(dialRetryTimeout)
	var lastErr error
	for {
		client, err := remoteconnect.DialTunnel(ctx, endpoint, agent.CABundle, agent.ServerName, capability)
		if err == nil {
			return client, nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("connect to remote-agent %s (%s): %w", endpoint, agent.ServerName, lastErr)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("connect to remote-agent %s (%s): %w", endpoint, agent.ServerName, ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}
