// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"encoding/json"
	"net/http"
)

// errorResponse represents the structure of an error response
type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// writeErrorResponse writes a JSON error response
func writeErrorResponse(w http.ResponseWriter, statusCode int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(errorResponse{
		Error:   code,
		Message: message,
	})
}
