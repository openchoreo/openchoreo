// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package protocol

import "time"

// The timeout ladder for one match batch. It is a single contract with three
// rungs, and every rung is only correct relative to the others, so all three are
// declared here rather than one per package:
//
//	AgentRequestTimeout < GatewayTunnelTimeout < ClientRequestTimeout
//
// Each layer gives up after the one below it, so whichever layer runs out of
// time still reports through the next: the agent's own error reaches the
// gateway, and the gateway's timeout reaches the caller as a 502 rather than the
// caller timing out blind. Inverting any pair — dropping the client below the
// agent's budget, say — turns every large batch into a hard client-side failure
// instead of a completed one.
const (
	// AgentRequestTimeout bounds one resource tree match request inside the
	// cluster agent.
	AgentRequestTimeout = 25 * time.Second

	// GatewayTunnelTimeout bounds the gateway's wait on the agent's answer.
	GatewayTunnelTimeout = 28 * time.Second

	// ClientRequestTimeout is the wall clock the control-plane client gives a
	// match batch. It is deliberately larger than the shared gateway client's
	// 10s default: the agent alone budgets AgentRequestTimeout per batch, so
	// reusing the shared client would abort calls the agent was still working
	// on.
	ClientRequestTimeout = 30 * time.Second
)
