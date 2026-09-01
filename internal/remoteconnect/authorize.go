// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remoteconnect

// AuthorizeRequest is what the remote-agent POSTs to the control plane's authorize
// endpoint for each StreamOpen it receives. The
// agent holds no signing/verification key: it presents the session capability and
// the requested target key, and the control plane — the sole holder of the signing
// key — verifies the capability and returns the concrete dial target. This keeps
// the signing key on the control plane and makes per-stream authorization an online
// check.
type AuthorizeRequest struct {
	// Capability is the session capability occ presented to the agent in Hello.
	Capability string `json:"capability"`
	// Key identifies which of the capability's authorized targets to dial.
	Key string `json:"key"`
}

// AuthorizeResponse is the control plane's reply: the concrete host:port the agent
// should net.Dial for the requested key. Only returned when the capability is valid
// and the key is one of its signed targets; otherwise the endpoint returns a non-2xx
// status and the agent refuses the stream.
type AuthorizeResponse struct {
	Host  string `json:"host"`
	Port  int    `json:"port"`
	Proto string `json:"proto"` // "tcp" for v1
	// AgentNamespace is the data-plane namespace the capability designated to serve this
	// target. The agent refuses a target routed to a namespace other than its own, so the
	// "dial from the provider's own namespace" invariant is enforced by the agent rather
	// than trusted from the client's choice of which agent to open the stream against.
	AgentNamespace string `json:"agentNamespace,omitempty"`
}

// AuthorizePath is the control-plane route the remote-agent calls to authorize a stream.
const AuthorizePath = "/api/v1/remote-connect:authorize"

// HeartbeatRequest is what a remote-agent POSTs periodically while it has at least one
// live tunnel session, so the control plane refreshes the agent's liveness for the
// whole life of the connection — not just when a new stream is opened. It presents a
// capability the agent currently holds (proof it is legitimately serving a session)
// and its own data-plane namespace (the agent to refresh). This closes the gap where a
// long-lived-but-quiet connection (e.g. a debugger paused at a breakpoint) would stop
// refreshing its stream-open-driven liveness and be reaped mid-transport.
type HeartbeatRequest struct {
	// Capability is a capability the agent currently holds for an active session.
	Capability string `json:"capability"`
	// AgentNamespace is the agent's own data-plane namespace (from the downward API).
	AgentNamespace string `json:"agentNamespace"`
}

// HeartbeatPath is the control-plane route a remote-agent calls to refresh its liveness.
const HeartbeatPath = "/api/v1/remote-connect:heartbeat"
