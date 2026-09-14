// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// renewalTestHandler builds a handler with no Kubernetes fixtures, for the lifetime
// decisions resolve makes that do not depend on resolving anything.
func renewalTestHandler(t *testing.T, cfg config.RemoteConnectConfig) *RemoteConnectHandler {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &RemoteConnectHandler{
		signer: &capabilitySigner{
			privKey: priv, keyID: "k1", issuer: "cp",
			ttl:        cfg.CapabilityTTL(false),
			secretTTL:  time.Duration(cfg.SecretTTLSeconds) * time.Second,
			maxSession: cfg.MaxSession(),
		},
		cfg:    cfg,
		logger: logger,
	}
}

// One grant drops a capability's lifetime to secret_ttl_seconds, so a session that
// declares it will not fetch must be signed no grants and keep the full dial TTL.
func TestSkipSecretsSuppressesGrantsAndRestoresDialTTL(t *testing.T) {
	resolveSkipping := func(skip bool) *remoteconnect.CapabilityClaims {
		t.Helper()
		env, dp := testEnvironmentAndDataPlane()
		cl := fake.NewClientBuilder().WithScheme(remoteConnectScheme(t)).
			WithObjects(env, dp, secretResourceFixture()).Build()

		_, priv, _ := ed25519.GenerateKey(nil)
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		h := &RemoteConnectHandler{
			k8sClient:    cl,
			authzChecker: svcpkg.NewAuthzChecker(&actionPDP{}, logger),
			signer: &capabilitySigner{
				privKey: priv, keyID: "k1", issuer: "cp",
				ttl: 30 * time.Minute, secretTTL: 10 * time.Minute,
			},
			secretsEnabled: true,
			cfg:            config.RemoteConnectDefaults(),
			logger:         logger,
		}

		resp, err := h.resolve(context.Background(), remoteconnect.ResolveRequest{
			Namespace: "default", Project: "doclet", Component: "doclet-document",
			Environment: "development",
			Resources: []remoteconnect.ResourceDep{{
				Ref:         docletPostgres,
				EnvBindings: map[string]string{"password": "DB_PASSWORD", "caCert": "CA_PEM"},
			}},
			SkipSecrets: skip,
		}, "user:alice")
		if err != nil {
			t.Fatalf("resolve(skipSecrets=%v): %v", skip, err)
		}
		claims, verr := remoteconnect.VerifyCapability(resp.Capability, priv.Public().(ed25519.PublicKey))
		if verr != nil {
			t.Fatalf("verify capability: %v", verr)
		}
		return claims
	}

	withGrants := resolveSkipping(false)
	if len(withGrants.Secrets) == 0 {
		t.Fatal("fixture should produce grants when they are not skipped; test proves nothing otherwise")
	}
	grantLife := withGrants.ExpiresAt.Sub(withGrants.IssuedAt.Time)

	skipped := resolveSkipping(true)
	if len(skipped.Secrets) != 0 {
		t.Errorf("skipSecrets must suppress every grant, got %+v", skipped.Secrets)
	}
	dialLife := skipped.ExpiresAt.Sub(skipped.IssuedAt.Time)

	if dialLife <= grantLife {
		t.Errorf("suppressing grants should restore the full dial TTL: skipped=%s, with grants=%s",
			dialLife, grantLife)
	}
	if dialLife != 30*time.Minute {
		t.Errorf("skipSecrets capability lifetime = %s, want the full 30m dial TTL", dialLife)
	}
}

// The bound measures a session, not a capability, so every renewal inherits the original
// start.
func TestSessionStartCarriedForwardAcrossRenewals(t *testing.T) {
	cfg := config.RemoteConnectDefaults()
	cfg.MaxSessionSeconds = 0 // unbounded: this test is about the value, not the refusal
	h := renewalTestHandler(t, cfg)

	started := time.Now().Add(-40 * time.Minute).Truncate(time.Second)
	first, err := h.signer.sign("user:alice", "default",
		remoteconnect.ComponentRef{Project: "doclet", Name: "doc"}, "development", nil, nil, started)
	if err != nil {
		t.Fatal(err)
	}

	got, err := h.sessionStart(remoteconnect.ResolveRequest{
		Purpose: remoteconnect.PurposeRenew, PreviousCapability: first,
	})
	if err != nil {
		t.Fatalf("sessionStart: %v", err)
	}
	if !got.Equal(started) {
		t.Errorf("sessionStart = %s, want the original %s", got, started)
	}

	// And it survives a second hop.
	second, err := h.signer.sign("user:alice", "default",
		remoteconnect.ComponentRef{Project: "doclet", Name: "doc"}, "development", nil, nil, got)
	if err != nil {
		t.Fatal(err)
	}
	again, err := h.sessionStart(remoteconnect.ResolveRequest{
		Purpose: remoteconnect.PurposeRenew, PreviousCapability: second,
	})
	if err != nil {
		t.Fatalf("sessionStart after two renewals: %v", err)
	}
	if !again.Equal(started) {
		t.Errorf("sessionStart after two renewals = %s, want the original %s", again, started)
	}
}

// The refusal must be distinguishable, so occ can stop retrying and say why.
func TestRenewalRefusedPastSessionBound(t *testing.T) {
	cfg := config.RemoteConnectDefaults()
	cfg.MaxSessionSeconds = 1800
	h := renewalTestHandler(t, cfg)

	old, err := h.signer.sign("user:alice", "default",
		remoteconnect.ComponentRef{Project: "doclet", Name: "doc"}, "development", nil, nil,
		time.Now().Add(-2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := h.sessionStart(remoteconnect.ResolveRequest{
		Purpose: remoteconnect.PurposeRenew, PreviousCapability: old,
	}); !errors.Is(err, ErrSessionBoundReached) {
		t.Fatalf("expected ErrSessionBoundReached, got %v", err)
	}

	// A session still inside the bound renews normally.
	young, err := h.signer.sign("user:alice", "default",
		remoteconnect.ComponentRef{Project: "doclet", Name: "doc"}, "development", nil, nil,
		time.Now().Add(-5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.sessionStart(remoteconnect.ResolveRequest{
		Purpose: remoteconnect.PurposeRenew, PreviousCapability: young,
	}); err != nil {
		t.Errorf("a session inside the bound must renew, got %v", err)
	}
}

// The previous capability is read for its session start, not as an authorization, so its
// expiry must not block a renewal that arrives just after it lapsed.
func TestRenewalAcceptsExpiredPreviousCapability(t *testing.T) {
	cfg := config.RemoteConnectDefaults()
	cfg.TTLSeconds = 1
	cfg.SecretTTLSeconds = 0
	cfg.MaxSessionSeconds = 43200
	h := renewalTestHandler(t, cfg)

	started := time.Now().Add(-time.Minute).Truncate(time.Second)
	expired, err := h.signer.sign("user:alice", "default",
		remoteconnect.ComponentRef{Project: "doclet", Name: "doc"}, "development", nil, nil, started)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, verr := remoteconnect.VerifyCapability(expired, h.VerifyKey()); verr == nil {
		t.Fatal("capability should have expired; test proves nothing otherwise")
	}

	got, err := h.sessionStart(remoteconnect.ResolveRequest{
		Purpose: remoteconnect.PurposeRenew, PreviousCapability: expired,
	})
	if err != nil {
		t.Fatalf("an expired capability must still be renewable: %v", err)
	}
	if !got.Equal(started) {
		t.Errorf("sessionStart = %s, want %s", got, started)
	}
}

// The bound is hygiene, not an access control, so an unreadable previous capability
// starts a fresh clock rather than failing the call.
func TestSessionStartIgnoresUnusablePreviousCapability(t *testing.T) {
	cfg := config.RemoteConnectDefaults()
	h := renewalTestHandler(t, cfg)

	for _, prev := range []string{"", "not-a-jwt", "a.b.c"} {
		got, err := h.sessionStart(remoteconnect.ResolveRequest{
			Purpose: remoteconnect.PurposeRenew, PreviousCapability: prev,
		})
		if err != nil {
			t.Errorf("previous capability %q: unexpected error %v", prev, err)
		}
		if time.Since(got) > time.Minute {
			t.Errorf("previous capability %q: expected a fresh session start, got %s", prev, got)
		}
	}
}

// occ obeys the server's interval, so resolve has to send one.
func TestResolveReturnsRenewalCadence(t *testing.T) {
	cfg := config.RemoteConnectDefaults()
	if want := cfg.RenewAfter(false); want <= 0 {
		t.Fatalf("defaults should produce a positive cadence, got %s", want)
	}

	// A capability carrying no grants renews against the dial TTL.
	h := renewalTestHandler(t, cfg)
	if got, want := int(h.cfg.RenewAfter(false).Seconds()), 1200; got != want {
		t.Errorf("RenewAfter(false) = %ds, want %ds", got, want)
	}
}

func TestCapabilityExpiryCappedAtSessionDeadline(t *testing.T) {
	cfg := config.RemoteConnectDefaults()
	cfg.TTLSeconds = 1800
	cfg.MaxSessionSeconds = 3600
	h := renewalTestHandler(t, cfg)

	// One minute of session budget left, against a thirty minute capability lifetime.
	started := time.Now().Add(-59 * time.Minute).Truncate(time.Second)
	capability, err := h.signer.sign("user:alice", "default",
		remoteconnect.ComponentRef{Project: "doclet", Name: "doc"}, "development", nil, nil, started)
	if err != nil {
		t.Fatal(err)
	}

	expiry, ok := remoteconnect.CapabilityExpiry(capability)
	if !ok {
		t.Fatal("capability carries no expiry")
	}
	deadline := started.Add(cfg.MaxSession())
	if expiry.After(deadline) {
		t.Errorf("capability expires %s, past the session deadline %s", expiry, deadline)
	}
	if !expiry.After(time.Now()) {
		t.Errorf("capability expires %s, which is already past", expiry)
	}
}
