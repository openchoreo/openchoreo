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

	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// RemoteConnectAuthorizeHandler serves POST /api/v1/remote-connect:authorize — the
// per-stream authorization callback the remote-agent makes. It
// verifies the capability the agent forwarded (minted by RemoteConnectHandler's resolve
// endpoint) and returns the concrete dial target for the requested key. The remote-agent
// holds no signing/verification key, so the control plane — the sole key holder — is
// the authority on which targets a capability may reach.
//
// The capability itself is the credential (a short-lived CP-signed JWT); this endpoint
// is therefore registered outside the user-JWT middleware, like the exec stream
// handler, and returns only what is already signed into the capability — a target's
// host:port, or a grant's Secret/ConfigMap coordinates — information the capability
// holder obtained at resolve time. It never returns a secret VALUE: the agent reads that
// from its own cluster, so no secret material passes through this endpoint.
//
// This endpoint is an echo of the capability, not a policy decision point: it holds no
// Kubernetes client and no authorization enforcer, and re-checks nothing. Authorization
// is decided once, at resolve, and the capability's expiry is its revocation window.
type RemoteConnectAuthorizeHandler struct {
	verifyKey ed25519.PublicKey
	// touch refreshes the remote-agent's last-used annotation so the reaper keeps an
	// actively-used agent alive. Optional; called asynchronously off the dial path.
	// Signature: (ctx, controlPlaneNamespace, env, agentDataPlaneNamespace).
	touch  func(ctx context.Context, namespace, env, dpNamespace string, readsSecret bool) error
	logger *slog.Logger
}

// NewRemoteConnectAuthorizeHandler builds the authorize handler. verifyKey is the public
// half of the key RemoteConnectHandler signs capabilities with; touch (optional)
// refreshes the remote-agent's liveness annotation.
func NewRemoteConnectAuthorizeHandler(verifyKey ed25519.PublicKey, touch func(ctx context.Context, namespace, env, dpNamespace string, readsSecret bool) error, logger *slog.Logger) *RemoteConnectAuthorizeHandler {
	return &RemoteConnectAuthorizeHandler{
		verifyKey: verifyKey,
		touch:     touch,
		logger:    logger.With("component", "remote-connect-authorize-handler"),
	}
}

func (h *RemoteConnectAuthorizeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req remoteconnect.AuthorizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Capability == "" || req.Key == "" {
		http.Error(w, "capability and key are required", http.StatusBadRequest)
		return
	}

	claims, err := remoteconnect.VerifyCapability(req.Capability, h.verifyKey)
	if err != nil {
		h.logger.Warn("capability rejected", "error", err)
		http.Error(w, "invalid or expired capability", http.StatusUnauthorized)
		return
	}

	// A key resolves in exactly one space, chosen by its prefix — never by searching
	// both. Looking a fetch key up in the dial table (or the reverse) is what would let
	// a caller turn one kind of grant into the other, with the agent as the deputy.
	var (
		resp        remoteconnect.AuthorizeResponse
		agentNs     string
		readsSecret bool
	)
	if remoteconnect.IsSecretGrantKey(req.Key) {
		grant, ok := claims.SecretByKey(req.Key)
		if !ok {
			h.logger.Warn("secret read not authorized by capability", "key", req.Key)
			http.Error(w, "target not authorized", http.StatusForbidden)
			return
		}
		agentNs = grant.AgentNamespace
		readsSecret = true
		resp = remoteconnect.AuthorizeResponse{
			Kind:           remoteconnect.AuthorizeKindSecret,
			AgentNamespace: grant.AgentNamespace,
			Secret:         &grant,
		}
	} else {
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
		agentNs = target.AgentNamespace
		resp = remoteconnect.AuthorizeResponse{
			Kind: remoteconnect.AuthorizeKindTCP,
			Host: target.Host, Port: target.Port, Proto: proto, AgentNamespace: target.AgentNamespace,
		}
	}

	// Refresh the liveness annotation of the agent that served this stream (the one in
	// the target's namespace) off the dial path, so an active session isn't reaped
	// mid-use. Best-effort, detached from the request context.
	if h.touch != nil {
		go func(ns, env, dpNs string, reads bool) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := h.touch(ctx, ns, env, dpNs, reads); err != nil {
				h.logger.Debug("failed to refresh remote-agent liveness", "error", err)
			}
		}(claims.Namespace, claims.Env, agentNs, readsSecret)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("failed to encode authorize response", "error", err)
	}
}
