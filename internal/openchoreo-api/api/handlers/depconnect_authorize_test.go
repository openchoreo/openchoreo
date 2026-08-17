// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
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

func signTestCapability(t *testing.T, priv ed25519.PrivateKey, targets []depconnect.Target) string {
	t.Helper()
	signer := &capabilitySigner{privKey: priv, keyID: "k1", issuer: "cp", ttl: time.Minute}
	cap, err := signer.sign("user:alice", "default",
		depconnect.ComponentRef{Project: "doclet", Name: "doc"}, "development", targets)
	if err != nil {
		t.Fatalf("sign capability: %v", err)
	}
	return cap
}

func postAuthorize(t *testing.T, h *DepConnectAuthorizeHandler, body depconnect.AuthorizeRequest) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, depconnect.AuthorizePath, bytes.NewReader(b))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthorizeReturnsSignedTarget(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	cap := signTestCapability(t, priv, []depconnect.Target{
		{Key: "ep/greeter/http", Proto: "tcp", Host: "greeter.dp-ns.svc.cluster.local", Port: 8080},
	})
	h := NewDepConnectAuthorizeHandler(priv.Public().(ed25519.PublicKey), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rec := postAuthorize(t, h, depconnect.AuthorizeRequest{Capability: cap, Key: "ep/greeter/http"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp depconnect.AuthorizeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Host != "greeter.dp-ns.svc.cluster.local" || resp.Port != 8080 || resp.Proto != "tcp" {
		t.Fatalf("unexpected target: %+v", resp)
	}
}

func TestAuthorizeRejectsUnknownKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	cap := signTestCapability(t, priv, []depconnect.Target{{Key: "ep/greeter/http", Proto: "tcp", Host: "greeter", Port: 8080}})
	h := NewDepConnectAuthorizeHandler(priv.Public().(ed25519.PublicKey), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rec := postAuthorize(t, h, depconnect.AuthorizeRequest{Capability: cap, Key: "ep/unknown"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestAuthorizeRejectsBadCapability(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	// Capability signed by a different key → verification fails.
	_, other, _ := ed25519.GenerateKey(nil)
	cap := signTestCapability(t, other, []depconnect.Target{{Key: "ep/greeter/http", Host: "greeter", Port: 8080}})
	h := NewDepConnectAuthorizeHandler(priv.Public().(ed25519.PublicKey), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rec := postAuthorize(t, h, depconnect.AuthorizeRequest{Capability: cap, Key: "ep/greeter/http"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// TestAuthorizeRejectsExpiredCapability is the control that bounds a session: the
// heartbeat path deliberately tolerates expiry, so the dial path is the only place it is
// enforced. Without this, an expired capability would keep opening streams forever.
func TestAuthorizeRejectsExpiredCapability(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	signer := &capabilitySigner{privKey: priv, keyID: "k1", issuer: "cp", ttl: -time.Minute}
	expired, err := signer.sign("user:alice", "default",
		depconnect.ComponentRef{Project: "doclet", Name: "doc"}, "development",
		[]depconnect.Target{{Key: "ep/greeter/greeter-svc/http", Proto: "tcp", Host: "h", Port: 8080}})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	h := NewDepConnectAuthorizeHandler(priv.Public().(ed25519.PublicKey), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := postAuthorize(t, h, depconnect.AuthorizeRequest{Capability: expired, Key: "ep/greeter/greeter-svc/http"})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d for an expired capability", rec.Code, http.StatusUnauthorized)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("host")) {
		t.Fatalf("expired capability leaked a dial target: %s", rec.Body.String())
	}
}

// TestAuthorizeReturnsAgentNamespace: the agent refuses a target routed to a namespace
// other than its own, which only works if the response carries it.
func TestAuthorizeReturnsAgentNamespace(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	cap := signTestCapability(t, priv, []depconnect.Target{{
		Key: "ep/greeter/greeter-svc/http", Proto: "tcp", Host: "h", Port: 8080,
		AgentNamespace: "dp-default-greeter-development",
	}})
	h := NewDepConnectAuthorizeHandler(priv.Public().(ed25519.PublicKey), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rec := postAuthorize(t, h, depconnect.AuthorizeRequest{Capability: cap, Key: "ep/greeter/greeter-svc/http"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp depconnect.AuthorizeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.AgentNamespace != "dp-default-greeter-development" {
		t.Errorf("agentNamespace = %q, want the capability's target namespace", resp.AgentNamespace)
	}
}
