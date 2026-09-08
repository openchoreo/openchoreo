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

	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

func signTestCapability(t *testing.T, priv ed25519.PrivateKey, targets []remoteconnect.Target) string {
	t.Helper()
	signer := &capabilitySigner{privKey: priv, keyID: "k1", issuer: "cp", ttl: time.Minute}
	cap, err := signer.sign("user:alice", "default",
		remoteconnect.ComponentRef{Project: "doclet", Name: "doc"}, "development", targets, nil)
	if err != nil {
		t.Fatalf("sign capability: %v", err)
	}
	return cap
}

func postAuthorize(t *testing.T, h *RemoteConnectAuthorizeHandler, body remoteconnect.AuthorizeRequest) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, remoteconnect.AuthorizePath, bytes.NewReader(b))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthorizeReturnsSignedTarget(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	cap := signTestCapability(t, priv, []remoteconnect.Target{
		{Key: "ep/greeter/http", Proto: "tcp", Host: "greeter.dp-ns.svc.cluster.local", Port: 8080},
	})
	h := NewRemoteConnectAuthorizeHandler(priv.Public().(ed25519.PublicKey), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rec := postAuthorize(t, h, remoteconnect.AuthorizeRequest{Capability: cap, Key: "ep/greeter/http"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp remoteconnect.AuthorizeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Host != "greeter.dp-ns.svc.cluster.local" || resp.Port != 8080 || resp.Proto != "tcp" {
		t.Fatalf("unexpected target: %+v", resp)
	}
}

func TestAuthorizeRejectsUnknownKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	cap := signTestCapability(t, priv, []remoteconnect.Target{{Key: "ep/greeter/http", Proto: "tcp", Host: "greeter", Port: 8080}})
	h := NewRemoteConnectAuthorizeHandler(priv.Public().(ed25519.PublicKey), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rec := postAuthorize(t, h, remoteconnect.AuthorizeRequest{Capability: cap, Key: "ep/unknown"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestAuthorizeRejectsBadCapability(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	// Capability signed by a different key → verification fails.
	_, other, _ := ed25519.GenerateKey(nil)
	cap := signTestCapability(t, other, []remoteconnect.Target{{Key: "ep/greeter/http", Host: "greeter", Port: 8080}})
	h := NewRemoteConnectAuthorizeHandler(priv.Public().(ed25519.PublicKey), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rec := postAuthorize(t, h, remoteconnect.AuthorizeRequest{Capability: cap, Key: "ep/greeter/http"})
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
		remoteconnect.ComponentRef{Project: "doclet", Name: "doc"}, "development",
		[]remoteconnect.Target{{Key: "ep/greeter/greeter-svc/http", Proto: "tcp", Host: "h", Port: 8080}}, nil)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	h := NewRemoteConnectAuthorizeHandler(priv.Public().(ed25519.PublicKey), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := postAuthorize(t, h, remoteconnect.AuthorizeRequest{Capability: expired, Key: "ep/greeter/greeter-svc/http"})

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
	cap := signTestCapability(t, priv, []remoteconnect.Target{{
		Key: "ep/greeter/greeter-svc/http", Proto: "tcp", Host: "h", Port: 8080,
		AgentNamespace: "dp-default-greeter-development",
	}})
	h := NewRemoteConnectAuthorizeHandler(priv.Public().(ed25519.PublicKey), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rec := postAuthorize(t, h, remoteconnect.AuthorizeRequest{Capability: cap, Key: "ep/greeter/greeter-svc/http"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp remoteconnect.AuthorizeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.AgentNamespace != "dp-default-greeter-development" {
		t.Errorf("agentNamespace = %q, want the capability's target namespace", resp.AgentNamespace)
	}
}

// Only a read refreshes the read Role; tunnel traffic refreshes only the agent.
func TestAuthorizeRefreshesReadGrantsOnlyForReads(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	signer := &capabilitySigner{privKey: priv, keyID: "k1", issuer: "cp", ttl: time.Hour}
	grant := remoteconnect.SecretGrant{
		Key: "sec/doclet-postgres/password", AgentNamespace: "dp-default-doclet-development",
		SourceKind: remoteconnect.SourceKindSecret, SourceName: "pg-creds", SourceKey: "password",
	}
	cap, err := signer.sign("user:alice", "default",
		remoteconnect.ComponentRef{Project: "doclet", Name: "doc"}, "development",
		[]remoteconnect.Target{{
			Key: "ep/greeter/greeter-svc/http", Proto: "tcp", Host: "h", Port: 8080,
			AgentNamespace: "dp-default-doclet-development",
		}},
		[]remoteconnect.SecretGrant{grant})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	for _, tc := range []struct {
		name  string
		key   string
		reads bool
	}{
		{"dial stream", "ep/greeter/greeter-svc/http", false},
		{"read stream", grant.Key, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := make(chan bool, 1)
			h := NewRemoteConnectAuthorizeHandler(priv.Public().(ed25519.PublicKey),
				func(_ context.Context, _, _, _ string, readsSecret bool) error {
					got <- readsSecret
					return nil
				}, slog.New(slog.NewTextHandler(io.Discard, nil)))

			rec := postAuthorize(t, h, remoteconnect.AuthorizeRequest{Capability: cap, Key: tc.key})
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
			}
			select {
			case readsSecret := <-got:
				if readsSecret != tc.reads {
					t.Fatalf("touch got readsSecret=%v, want %v", readsSecret, tc.reads)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("touch was never called")
			}
		})
	}
}
