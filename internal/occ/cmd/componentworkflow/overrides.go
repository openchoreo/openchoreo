// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentworkflow

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tidwall/sjson"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// mergeParametersWithComponent merges --set parameter values with existing component
// This uses sjson to generically update JSON paths in the existing component
// User-facing paths use "workflow" prefix (e.g., workflow.parameters.key=value)
// These are converted to "componentWorkflow" for the JSON structure
func mergeParametersWithComponent(existingComponent *gen.Component, setValues []string) (*gen.UpdateComponentWorkflowParametersJSONRequestBody, error) {
	// Marshal entire component to JSON
	existingJSON, err := json.Marshal(existingComponent)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal existing component: %w", err)
	}
	jsonStr := string(existingJSON)

	// Apply each --set value using sjson
	for _, setValue := range setValues {
		parts := strings.SplitN(setValue, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --set format '%s', expected: key=value", setValue)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "" {
			return nil, fmt.Errorf("empty key in --set flag")
		}

		// Sanitize: convert "workflow." prefix to "componentWorkflow."
		// TODO: Update API spec to return workflow directly
		if strings.HasPrefix(key, "workflow.") {
			key = "componentWorkflow." + strings.TrimPrefix(key, "workflow.")
		}

		// Use sjson to update the value at the given path
		jsonStr, err = sjson.Set(jsonStr, key, value)
		if err != nil {
			return nil, fmt.Errorf("failed to set value for key '%s': %w", key, err)
		}
	}

	// Unmarshal back to component and extract workflow config
	var updatedComponent gen.Component
	if err := json.Unmarshal([]byte(jsonStr), &updatedComponent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal merged component: %w", err)
	}

	// Build the request body with both parameters and systemParameters
	body := &gen.UpdateComponentWorkflowParametersJSONRequestBody{}

	if updatedComponent.Spec != nil && updatedComponent.Spec.Workflow != nil {
		if updatedComponent.Spec.Workflow.Parameters != nil {
			body.Parameters = updatedComponent.Spec.Workflow.Parameters
		}
		if updatedComponent.Spec.Workflow.SystemParameters != nil {
			body.SystemParameters = updatedComponent.Spec.Workflow.SystemParameters
		}
	}

	return body, nil
}
