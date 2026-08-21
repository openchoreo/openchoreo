// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/depconnect"
)

func signTestCapabilityTTL(t *testing.T, priv ed25519.PrivateKey, targets []depconnect.Target, ttl time.Duration) string {
	t.Helper()
	signer := &capabilitySigner{privKey: priv, keyID: "k1", issuer: "cp", ttl: ttl}
	cap, err := signer.sign("user:alice", defaultPlaneName,
		depconnect.ComponentRef{Project: "doclet", Name: "doc"}, "development", targets)
	if err != nil {
		t.Fatalf("sign capability: %v", err)
	}
	return cap
}

type touchRecord struct {
	called                    bool
	namespace, env, dpNamespc string
}

func postHeartbeat(t *testing.T, h *DepConnectHeartbeatHandler, body depconnect.HeartbeatRequest) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, depconnect.HeartbeatPath, bytes.NewReader(b))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHeartbeatRefreshesTargetAgent(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	cap := signTestCapabilityTTL(t, priv, []depconnect.Target{
		{Key: "ep/greeter/http", Host: "greeter", Port: 80, AgentNamespace: "dp-default-greeter-development"},
	}, time.Minute)

	var rec touchRecord
	touch := func(_ context.Context, ns, env, dpNs string) error {
		rec = touchRecord{called: true, namespace: ns, env: env, dpNamespc: dpNs}
		return nil
	}
	h := NewDepConnectHeartbeatHandler(priv.Public().(ed25519.PublicKey), touch, slog.New(slog.NewTextHandler(io.Discard, nil)))

	res := postHeartbeat(t, h, depconnect.HeartbeatRequest{Capability: cap, AgentNamespace: "dp-default-greeter-development"})
	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", res.Code, res.Body.String())
	}
	if !rec.called || rec.namespace != defaultPlaneName || rec.env != "development" || rec.dpNamespc != "dp-default-greeter-development" {
		t.Fatalf("touch called with unexpected args: %+v", rec)
	}
}

// A live session can outlast its capability's dial-authorization window; the heartbeat
// must still refresh liveness so the agent isn't reaped mid-connection.
func TestHeartbeatAcceptsExpiredCapability(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	cap := signTestCapabilityTTL(t, priv, []depconnect.Target{
		{Key: "ep/greeter/http", Host: "greeter", Port: 80, AgentNamespace: "dp-ns"},
	}, -time.Minute) // already expired

	var rec touchRecord
	touch := func(_ context.Context, ns, env, dpNs string) error {
		rec = touchRecord{called: true, namespace: ns, env: env, dpNamespc: dpNs}
		return nil
	}
	h := NewDepConnectHeartbeatHandler(priv.Public().(ed25519.PublicKey), touch, slog.New(slog.NewTextHandler(io.Discard, nil)))

	res := postHeartbeat(t, h, depconnect.HeartbeatRequest{Capability: cap, AgentNamespace: "dp-ns"})
	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (expired capability tolerated); body=%s", res.Code, res.Body.String())
	}
	if !rec.called {
		t.Fatal("touch not called for expired-but-valid capability")
	}
}

// An agent may only refresh a namespace its capability actually serves.
func TestHeartbeatRejectsUnservedNamespace(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	cap := signTestCapabilityTTL(t, priv, []depconnect.Target{
		{Key: "ep/greeter/http", Host: "greeter", Port: 80, AgentNamespace: "dp-ns"},
	}, time.Minute)

	var called bool
	touch := func(context.Context, string, string, string) error { called = true; return nil }
	h := NewDepConnectHeartbeatHandler(priv.Public().(ed25519.PublicKey), touch, slog.New(slog.NewTextHandler(io.Discard, nil)))

	res := postHeartbeat(t, h, depconnect.HeartbeatRequest{Capability: cap, AgentNamespace: "dp-someone-else"})
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", res.Code)
	}
	if called {
		t.Fatal("touch must not be called for an unserved namespace")
	}
}

func TestHeartbeatRejectsBadCapability(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	_, other, _ := ed25519.GenerateKey(nil) // signed by a different key
	cap := signTestCapabilityTTL(t, other, []depconnect.Target{
		{Key: "ep/greeter/http", Host: "greeter", Port: 80, AgentNamespace: "dp-ns"},
	}, time.Minute)

	h := NewDepConnectHeartbeatHandler(priv.Public().(ed25519.PublicKey), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	res := postHeartbeat(t, h, depconnect.HeartbeatRequest{Capability: cap, AgentNamespace: "dp-ns"})
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.Code)
	}
}

// TestHeartbeatRejectsOversizedBody: the body is read before any authentication, so it
// must be bounded.
func TestHeartbeatRejectsOversizedBody(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	h := NewDepConnectHeartbeatHandler(priv.Public().(ed25519.PublicKey), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	huge := bytes.NewReader(bytes.Repeat([]byte("a"), maxHeartbeatBodyBytes+1024))
	req := httptest.NewRequest(http.MethodPost, depconnect.HeartbeatPath, huge)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d for an oversized body", rec.Code, http.StatusBadRequest)
	}
}
