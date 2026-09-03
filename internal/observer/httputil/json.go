// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package httputil

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// WriteJSON marshals the provided interface to JSON and writes it to the response
func WriteJSON(w http.ResponseWriter, status int, v interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if v == nil {
		return nil
	}

	if err := json.NewEncoder(w).Encode(v); err != nil {
		return fmt.Errorf("failed to encode JSON response: %w", err)
	}

	return nil
}
