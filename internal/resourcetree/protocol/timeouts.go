// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package protocol

import "time"

// The timeout ladder for resource tree discovery. It is a single contract with
// four rungs, and every rung is only correct relative to the others, so all four
// are declared here rather than one per package:
//
//	AgentRequestTimeout < GatewayTunnelTimeout < ClientRequestTimeout < WalkTimeout
//
// The first three bound ONE match batch. Each layer gives up after the one below
// it, so whichever layer runs out of time still reports through the next: the
// agent's own error reaches the gateway, and the gateway's timeout reaches the
// caller as a 502 rather than the caller timing out blind. Inverting any pair —
// dropping the client below the agent's budget, say — turns every large batch
// into a hard client-side failure instead of a completed one.
//
// WalkTimeout bounds the WHOLE walk of one API request, which sends batches one
// after another: one per tree level, more when a level is split. The API
// server's write deadline is derived from it, so a walk always ends — with the
// nodes found so far and an error status on whatever was cut short — before the
// connection does. The one batch the walk deadline lands in the middle of is
// abandoned below the agent's own budget; that is the caller's budget running
// out, and it is reported the same way as any other batch failure.
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

	// WalkTimeout is the wall clock one API request's whole tree walk gets,
	// across every batch it sends. Two full-budget batches fit; a walk that
	// needs more is reported incomplete rather than cut off mid-response.
	WalkTimeout = 60 * time.Second
)
