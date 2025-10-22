// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"encoding/json"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

type MCPHandler struct {
	Services *services.Services
}

// marshalResponse is a helper function that marshals any data into a JSON string.
// It returns the JSON string representation of the data or an error if marshaling fails.
func marshalResponse(data any) (string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(jsonData), nil
}
