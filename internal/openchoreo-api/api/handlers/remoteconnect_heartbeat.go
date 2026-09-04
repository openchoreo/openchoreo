// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// maxHeartbeatBodyBytes bounds the request body; a capability plus a namespace is
// well under this.
const maxHeartbeatBodyBytes = 16 << 10

// RemoteConnectHeartbeatHandler serves POST /api/v1/remote-connect:heartbeat — the periodic
// liveness callback a remote-agent makes while it has at least one live tunnel session.
// It refreshes the agent's last-used annotation so the reaper keeps it alive for the
// full life of a connection, not merely while new streams are being opened (which is
// what the per-stream authorize callback covers). This is what keeps a long-lived or
// paused-at-a-breakpoint session from being reaped mid-transport.
//
// Like the authorize endpoint, this is registered outside the user-JWT middleware: the
// agent holds no user token. The capability it presents is the credential — verified
// here for signature and audience (expiry is tolerated, since a live session may
// outlast its capability's dial-authorization window), and the agent may only refresh a
// namespace its capability actually references. Worst case for an unauthenticated
// attacker who somehow holds a valid capability: it keeps an idle agent alive longer.
type RemoteConnectHeartbeatHandler struct {
	verifyKey ed25519.PublicKey
	// touch refreshes the named remote-agent's last-used annotation.
	// Signature: (ctx, controlPlaneNamespace, env, agentDataPlaneNamespace).
	touch  func(ctx context.Context, namespace, env, dpNamespace string) error
	logger *slog.Logger
}

// NewRemoteConnectHeartbeatHandler builds the heartbeat handler. verifyKey is the public
// half of the key RemoteConnectHandler signs capabilities with; touch refreshes the
// remote-agent's liveness annotation.
func NewRemoteConnectHeartbeatHandler(verifyKey ed25519.PublicKey, touch func(ctx context.Context, namespace, env, dpNamespace string) error, logger *slog.Logger) *RemoteConnectHeartbeatHandler {
	return &RemoteConnectHeartbeatHandler{
		verifyKey: verifyKey,
		touch:     touch,
		logger:    logger.With("component", "remote-connect-heartbeat-handler"),
	}
}

func (h *RemoteConnectHeartbeatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// The capability in the body is the credential, so the body is read before any
	// authentication and must be bounded.
	var req remoteconnect.HeartbeatRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxHeartbeatBodyBytes)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Capability == "" || req.AgentNamespace == "" {
		http.Error(w, "capability and agentNamespace are required", http.StatusBadRequest)
		return
	}

	claims, err := remoteconnect.VerifyCapabilityAllowExpired(req.Capability, h.verifyKey)
	if err != nil {
		h.logger.Warn("heartbeat capability rejected", "error", err)
		http.Error(w, "invalid capability", http.StatusUnauthorized)
		return
	}

	// An agent may only refresh a namespace its capability serves — it cannot keep
	// arbitrary agents alive.
	if !claims.HasAgentNamespace(req.AgentNamespace) {
		h.logger.Warn("heartbeat namespace not served by capability", "namespace", req.AgentNamespace)
		http.Error(w, "agent namespace not authorized", http.StatusForbidden)
		return
	}

	if h.touch != nil {
		if err := h.touch(r.Context(), claims.Namespace, claims.Env, req.AgentNamespace); err != nil {
			h.logger.Warn("heartbeat touch failed", "namespace", req.AgentNamespace, "error", err)
			http.Error(w, "failed to refresh liveness", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
