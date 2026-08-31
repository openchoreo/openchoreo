// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remoteconnect

import (
	"crypto/ed25519"
	"fmt"
	"slices"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// capabilitySigningMethod is EdDSA (Ed25519).
var capabilitySigningMethod = jwt.SigningMethodEdDSA

// CapabilityAudience is the JWT audience for remote-connect capabilities — a
// defense-in-depth check against token reuse across unrelated JWT purposes.
const CapabilityAudience = "openchoreo-api:remote-connect"

// ComponentRef identifies the consuming component a capability is scoped to.
type ComponentRef struct {
	Project string `json:"project"`
	Name    string `json:"name"`
}

// Target is a single dialable destination the capability authorizes. The control
// plane resolves the concrete host:port and the owning data plane at resolve time
// and signs them here, so the remote-connect stream endpoint dials only CP-authorized
// targets and never a free-form destination from the client. Key is the
// stable identifier the client references when opening a stream.
type Target struct {
	Key   string `json:"key"`
	Proto string `json:"proto"` // "tcp" for v1
	Host  string `json:"host"`
	Port  int    `json:"port"`

	// AgentNamespace is the data-plane namespace of the remote-agent that serves this
	// target — the provider's project+env namespace, so a target dials its dependency
	// from within the dependency's own namespace. occ routes the target's streams to
	// that agent, and the authorize callback refreshes that agent's liveness. Targets
	// in different provider projects fan out to different agents.
	AgentNamespace string `json:"agentNamespace,omitempty"`
}

// CapabilityClaims are the custom claims of the remote-connect capability JWT. The
// registered claims carry iss/sub/aud/exp/iat/jti.
type CapabilityClaims struct {
	jwt.RegisteredClaims
	Namespace string       `json:"namespace"`
	Component ComponentRef `json:"component"`
	Env       string       `json:"env"`
	Targets   []Target     `json:"targets"`
}

// TargetByKey returns the authorized target with the given key, if present.
func (c *CapabilityClaims) TargetByKey(key string) (Target, bool) {
	for _, t := range c.Targets {
		if t.Key == key {
			return t, true
		}
	}
	return Target{}, false
}

// HasAgentNamespace reports whether any target is served by the given data-plane
// namespace. The heartbeat path uses it so an agent may only refresh the liveness of a
// namespace its capability actually references (it cannot keep arbitrary agents alive).
func (c *CapabilityClaims) HasAgentNamespace(ns string) bool {
	for _, t := range c.Targets {
		if t.AgentNamespace == ns {
			return true
		}
	}
	return false
}

// SignCapability mints a compact capability JWT signed with the control plane's
// Ed25519 private key. kid is set in the JWT header so the verifier can select the
// matching public key during rotation.
func SignCapability(claims *CapabilityClaims, priv ed25519.PrivateKey, kid string) (string, error) {
	tok := jwt.NewWithClaims(capabilitySigningMethod, claims)
	if kid != "" {
		tok.Header["kid"] = kid
	}
	return tok.SignedString(priv)
}

// VerifyCapability parses and validates a capability JWT against the control plane's
// Ed25519 public key, enforcing the signing method, a present-and-unexpired exp, and
// the remote-connect audience. On success it returns the claims.
func VerifyCapability(token string, pub ed25519.PublicKey) (*CapabilityClaims, error) {
	claims := &CapabilityClaims{}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{capabilitySigningMethod.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithAudience(CapabilityAudience),
	)
	if _, err := parser.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		return pub, nil
	}); err != nil {
		return nil, fmt.Errorf("remoteconnect: verify capability: %w", err)
	}
	return claims, nil
}

// VerifyCapabilityAllowExpired verifies the capability's signature and audience but
// ignores expiry. The heartbeat path uses it: a long session's capability may expire
// while its tunnel is still live, and refreshing an agent's liveness only needs proof
// the capability was genuinely control-plane-signed for that agent — not that it is
// still within its (dial-authorizing) validity window. Dialing still requires an
// unexpired capability via VerifyCapability on the authorize path.
func VerifyCapabilityAllowExpired(token string, pub ed25519.PublicKey) (*CapabilityClaims, error) {
	claims := &CapabilityClaims{}
	// WithoutClaimsValidation skips exp/nbf/aud/iss checks (signature is still verified
	// via the keyfunc); we re-assert the audience by hand below.
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{capabilitySigningMethod.Alg()}),
		jwt.WithoutClaimsValidation(),
	)
	if _, err := parser.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		return pub, nil
	}); err != nil {
		return nil, fmt.Errorf("remoteconnect: verify capability: %w", err)
	}
	if !slices.Contains(claims.Audience, CapabilityAudience) {
		return nil, fmt.Errorf("remoteconnect: capability audience mismatch")
	}
	// WithoutClaimsValidation drops every temporal check, not only expiry; re-assert the
	// two that still apply.
	if claims.ExpiresAt == nil {
		return nil, fmt.Errorf("remoteconnect: capability has no expiry")
	}
	if claims.NotBefore != nil && claims.NotBefore.After(time.Now()) {
		return nil, fmt.Errorf("remoteconnect: capability is not valid yet")
	}
	return claims, nil
}

// CapabilityExpiry reads a capability's expiry without verifying its signature. occ has
// no verification key, but it still needs to tell the developer when the session ends —
// otherwise expiry looks like the dependency going away. Never use this to make a
// security decision; only the control plane's verified check governs access.
func CapabilityExpiry(token string) (time.Time, bool) {
	claims := &CapabilityClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err != nil {
		return time.Time{}, false
	}
	if claims.ExpiresAt == nil {
		return time.Time{}, false
	}
	return claims.ExpiresAt.Time, true
}
