// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package httputil

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// WriteJSON marshals the provided interface to JSON and writes it to the response.
//
// Cache-Control defaults to no-store. Almost everything this API returns is scoped
// to the caller's identity, and a response with no cache directive is heuristically
// cacheable by a browser - so without this, a shared profile could serve one
// subject's data to the next after a re-login. It matters most on the GET endpoints
// (platform logs, FinOps costs, span details), since POST responses are not
// heuristically cached.
//
// A caller that genuinely serves public, identity-independent data may set
// Cache-Control before calling and it will be left alone.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(status)

	if v == nil {
		return nil
	}

	if err := json.NewEncoder(w).Encode(v); err != nil {
		return fmt.Errorf("failed to encode JSON response: %w", err)
	}

	return nil
}
