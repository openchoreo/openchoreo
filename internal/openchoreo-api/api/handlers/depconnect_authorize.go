// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/openchoreo/openchoreo/internal/depconnect"
)

// DepConnectAuthorizeHandler serves POST /api/v1/dep-connect:authorize — the
// per-stream authorization callback the dep-agent makes. It
// verifies the capability the agent forwarded (minted by DepConnectHandler's resolve
// endpoint) and returns the concrete dial target for the requested key. The dep-agent
// holds no signing/verification key, so the control plane — the sole key holder — is
// the authority on which targets a capability may reach.
//
// The capability itself is the credential (a short-lived CP-signed JWT); this endpoint
// is therefore registered outside the user-JWT middleware, like the exec stream
// handler, and returns only host:port for targets already signed into the capability —
// information the capability holder obtained at resolve time.
type DepConnectAuthorizeHandler struct {
	verifyKey ed25519.PublicKey
	// touch refreshes the dep-agent's last-used annotation so the reaper keeps an
	// actively-used agent alive. Optional; called asynchronously off the dial path.
	// Signature: (ctx, controlPlaneNamespace, env, agentDataPlaneNamespace).
	touch  func(ctx context.Context, namespace, env, dpNamespace string) error
	logger *slog.Logger
}

// NewDepConnectAuthorizeHandler builds the authorize handler. verifyKey is the public
// half of the key DepConnectHandler signs capabilities with; touch (optional)
// refreshes the dep-agent's liveness annotation.
func NewDepConnectAuthorizeHandler(verifyKey ed25519.PublicKey, touch func(ctx context.Context, namespace, env, dpNamespace string) error, logger *slog.Logger) *DepConnectAuthorizeHandler {
	return &DepConnectAuthorizeHandler{
		verifyKey: verifyKey,
		touch:     touch,
		logger:    logger.With("component", "dep-connect-authorize-handler"),
	}
}

func (h *DepConnectAuthorizeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req depconnect.AuthorizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Capability == "" || req.Key == "" {
		http.Error(w, "capability and key are required", http.StatusBadRequest)
		return
	}

	claims, err := depconnect.VerifyCapability(req.Capability, h.verifyKey)
	if err != nil {
		h.logger.Warn("capability rejected", "error", err)
		http.Error(w, "invalid or expired capability", http.StatusUnauthorized)
		return
	}

	target, ok := claims.TargetByKey(req.Key)
	if !ok {
		h.logger.Warn("target not authorized by capability", "key", req.Key)
		http.Error(w, "target not authorized", http.StatusForbidden)
		return
	}

	proto := target.Proto
	if proto == "" {
		proto = "tcp"
	}
	resp := depconnect.AuthorizeResponse{
		Host: target.Host, Port: target.Port, Proto: proto, AgentNamespace: target.AgentNamespace,
	}

	// Refresh the liveness annotation of the agent that served this stream (the one in
	// the target's namespace) off the dial path, so an active session isn't reaped
	// mid-use. Best-effort, detached from the request context.
	if h.touch != nil {
		go func(ns, env, dpNs string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := h.touch(ctx, ns, env, dpNs); err != nil {
				h.logger.Debug("failed to refresh dep-agent liveness", "error", err)
			}
		}(claims.Namespace, claims.Env, target.AgentNamespace)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("failed to encode authorize response", "error", err)
	}
}
