// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package httputil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// BindJSON reads JSON from the request body and unmarshals it into the provided interface
func BindJSON(r *http.Request, v interface{}) error {
	if r.Body == nil {
		return fmt.Errorf("request body is empty")
	}
	defer r.Body.Close()

	// Check content type
	contentType := r.Header.Get("Content-Type")
	if contentType != "" && !strings.HasPrefix(contentType, "application/json") {
		return fmt.Errorf("invalid content type: %s", contentType)
	}

	// Read and decode JSON
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	if len(body) == 0 {
		return fmt.Errorf("request body is empty")
	}

	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return nil
}

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

// GetPathParam extracts a path parameter from the URL using the new Go 1.22 pattern matching
func GetPathParam(r *http.Request, key string) string {
	return r.PathValue(key)
}
