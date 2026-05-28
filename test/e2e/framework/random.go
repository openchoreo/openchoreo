// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// RandSuffix returns a lowercase hex string of the requested length. Used by
// the observability and alerts suites to mint marker tokens that won't collide
// with anything else in the OpenSearch index.
func RandSuffix(n int) string {
	if n <= 0 {
		n = 4
	}
	// hex encoding doubles size, so request ceil(n/2) bytes.
	buf := make([]byte, (n+1)/2)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("failed to read random suffix bytes: %v", err))
	}
	out := hex.EncodeToString(buf)
	if len(out) > n {
		out = out[:n]
	}
	return out
}
