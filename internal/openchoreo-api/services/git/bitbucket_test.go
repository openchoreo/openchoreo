// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package git

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func bitbucketSignature(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestBitbucketProvider_ValidateWebhookPayload(t *testing.T) {
	payload := []byte(`{"push":{"changes":[]}}`)
	const secret = "bitbucket-webhook-secret"
	p := NewBitbucketProvider()

	tests := []struct {
		name      string
		signature string
		secret    string
		wantErr   bool
	}{
		{
			name:      "valid signature accepted",
			signature: bitbucketSignature(secret, payload),
			secret:    secret,
			wantErr:   false,
		},
		{
			name:      "missing signature rejected",
			signature: "",
			secret:    secret,
			wantErr:   true,
		},
		{
			name:      "empty secret rejected",
			signature: bitbucketSignature(secret, payload),
			secret:    "",
			wantErr:   true,
		},
		{
			name:      "wrong signature rejected",
			signature: bitbucketSignature("attacker-secret", payload),
			secret:    secret,
			wantErr:   true,
		},
		{
			name:      "malformed signature format rejected",
			signature: "not-a-sha256-prefixed-value",
			secret:    secret,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.ValidateWebhookPayload(payload, tt.signature, tt.secret)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestBitbucketProvider_ForgedWebhookRejected is a regression test for the historical bypass
// where Bitbucket webhook validation was skipped whenever no signature was supplied. A forged
// webhook with no valid signature (or no configured secret) must now be rejected.
func TestBitbucketProvider_ForgedWebhookRejected(t *testing.T) {
	payload := []byte(`{"push":{"changes":[{"new":{"name":"main","type":"branch"}}]}}`)
	p := NewBitbucketProvider()

	// No signature, secret configured: rejected (previously this was silently accepted).
	if err := p.ValidateWebhookPayload(payload, "", "configured-secret"); err == nil {
		t.Error("forged webhook with no signature was accepted; expected rejection")
	}

	// No signature and no secret: still rejected (fail closed).
	if err := p.ValidateWebhookPayload(payload, "", ""); err == nil {
		t.Error("forged webhook with no signature and no secret was accepted; expected rejection")
	}
}
