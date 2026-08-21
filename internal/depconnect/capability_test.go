// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package depconnect

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func testClaims(exp time.Time) *CapabilityClaims {
	return &CapabilityClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "https://cp.test",
			Subject:   "user:alice",
			Audience:  jwt.ClaimStrings{CapabilityAudience},
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(exp.Add(-time.Hour)),
		},
		Component: ComponentRef{Project: "doclet", Name: "doclet-document"},
		Env:       "development",
		Targets: []Target{
			{
				Key: "ep/backend-api/http", Proto: "tcp", Host: "10.0.0.5", Port: 8080,
				AgentNamespace: "dp-default-doclet-development",
			},
		},
	}
}

func TestCapabilityRoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	token, err := SignCapability(testClaims(time.Now().Add(30*time.Minute)), priv, "kid-1")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	got, err := VerifyCapability(token, pub)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Component.Name != "doclet-document" || got.Env != "development" {
		t.Fatalf("unexpected claims: %+v", got)
	}
	target, ok := got.TargetByKey("ep/backend-api/http")
	if !ok || target.Port != 8080 || target.Host != "10.0.0.5" || target.AgentNamespace != "dp-default-doclet-development" {
		t.Fatalf("target lookup failed: %+v ok=%v", target, ok)
	}
	if _, ok := got.TargetByKey("nope"); ok {
		t.Fatal("expected missing target to be absent")
	}
}

func TestCapabilityExpiredRejected(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	token, _ := SignCapability(testClaims(time.Now().Add(-time.Minute)), priv, "kid-1")
	if _, err := VerifyCapability(token, pub); err == nil {
		t.Fatal("expected expired capability to be rejected")
	}
}

func TestCapabilityWrongKeyRejected(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	otherPub, _, _ := ed25519.GenerateKey(nil)
	token, _ := SignCapability(testClaims(time.Now().Add(30*time.Minute)), priv, "kid-1")
	if _, err := VerifyCapability(token, otherPub); err == nil {
		t.Fatal("expected verification with wrong key to fail")
	}
}

// TestVerifyCapabilityAllowExpiredStillChecksTemporalClaims: a passed expiry is
// tolerated, but a missing expiry or a not-yet-valid token is not.
func TestVerifyCapabilityAllowExpiredStillChecksTemporalClaims(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)

	base := func() CapabilityClaims {
		return CapabilityClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:   "cp",
				Subject:  "user:alice",
				Audience: jwt.ClaimStrings{CapabilityAudience},
				IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			},
		}
	}

	tests := []struct {
		name    string
		mutate  func(*CapabilityClaims)
		wantErr bool
	}{
		{
			name:   "expired is still accepted",
			mutate: func(c *CapabilityClaims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour)) },
		},
		{
			name:   "unexpired is accepted",
			mutate: func(c *CapabilityClaims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour)) },
		},
		{
			name:    "no expiry at all is rejected",
			mutate:  func(c *CapabilityClaims) { c.ExpiresAt = nil },
			wantErr: true,
		},
		{
			name: "not yet valid is rejected",
			mutate: func(c *CapabilityClaims) {
				c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(2 * time.Hour))
				c.NotBefore = jwt.NewNumericDate(time.Now().Add(time.Hour))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := base()
			tt.mutate(&claims)
			token, err := SignCapability(&claims, priv, "k1")
			if err != nil {
				t.Fatalf("sign: %v", err)
			}
			_, err = VerifyCapabilityAllowExpired(token, pub)
			if tt.wantErr && err == nil {
				t.Error("expected rejection")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected acceptance, got %v", err)
			}
		})
	}
}
