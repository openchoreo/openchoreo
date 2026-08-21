// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package protocol

import (
	"testing"
	"time"
)

// TestTimeoutLadderValues pins each rung. The absolute values matter on their
// own: they are what an operator's own gateway and proxy timeouts are sized
// against.
func TestTimeoutLadderValues(t *testing.T) {
	tests := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"agent", AgentRequestTimeout, 25 * time.Second},
		{"gateway tunnel", GatewayTunnelTimeout, 28 * time.Second},
		{"client", ClientRequestTimeout, 30 * time.Second},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("the %s timeout must be %s, got %s", tt.name, tt.want, tt.got)
		}
	}
}

// TestTimeoutLadderOrdering pins the invariant the three values exist to hold.
// Without it the constants have only been collected in one place: each layer
// must give up after the one below it, or the layer that ran out of time cannot
// report through the next one.
func TestTimeoutLadderOrdering(t *testing.T) {
	if AgentRequestTimeout >= GatewayTunnelTimeout {
		t.Errorf("the agent budget (%s) must be shorter than the gateway tunnel timeout (%s), "+
			"or the gateway gives up while the agent is still working and its error never reaches the caller",
			AgentRequestTimeout, GatewayTunnelTimeout)
	}
	if GatewayTunnelTimeout >= ClientRequestTimeout {
		t.Errorf("the gateway tunnel timeout (%s) must be shorter than the client timeout (%s), "+
			"or the caller times out blind instead of receiving the gateway's 502",
			GatewayTunnelTimeout, ClientRequestTimeout)
	}
}
