// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package git

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// verifyHMACSHA256 validates an HMAC-SHA256 webhook signature of the form "sha256=<hex>"
// against the given payload and secret. It fails closed: an empty secret or a missing or
// malformed signature is treated as invalid — never skipped.
func verifyHMACSHA256(payload []byte, signature, secret string) error {
	if secret == "" {
		return fmt.Errorf("webhook secret not configured")
	}
	if signature == "" {
		return fmt.Errorf("missing signature header")
	}
	if !strings.HasPrefix(signature, "sha256=") {
		return fmt.Errorf("invalid signature format")
	}
	signature = strings.TrimPrefix(signature, "sha256=")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedMAC)) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}
