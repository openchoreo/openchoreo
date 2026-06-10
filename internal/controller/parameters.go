// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
)

// GetNestedStringInParams navigates a runtime.RawExtension JSON blob using a dotted path
// and returns the string value. The leading "parameters." prefix is stripped if present.
func GetNestedStringInParams(raw *runtime.RawExtension, dottedPath string) (string, error) {
	if raw == nil || raw.Raw == nil {
		return "", fmt.Errorf("parameters is nil")
	}

	path := strings.TrimPrefix(dottedPath, "parameters.")

	var data map[string]interface{}
	if err := json.Unmarshal(raw.Raw, &data); err != nil {
		return "", fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	parts := strings.Split(path, ".")
	current := interface{}(data)
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("path %s: expected object at %s", dottedPath, part)
		}
		current, ok = m[part]
		if !ok {
			return "", fmt.Errorf("path %s: key %s not found", dottedPath, part)
		}
	}

	str, ok := current.(string)
	if !ok {
		return "", fmt.Errorf("path %s: value is not a string", dottedPath)
	}
	return str, nil
}

// SetNestedStringInParams sets a string value at the given dotted path in a runtime.RawExtension.
// The leading "parameters." prefix is stripped if present.
func SetNestedStringInParams(raw *runtime.RawExtension, dottedPath, value string) (*runtime.RawExtension, error) {
	if raw == nil || raw.Raw == nil {
		return nil, fmt.Errorf("parameters is nil")
	}

	path := strings.TrimPrefix(dottedPath, "parameters.")

	var data map[string]interface{}
	if err := json.Unmarshal(raw.Raw, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	parts := strings.Split(path, ".")
	current := data
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part]
		if !ok {
			newObj := make(map[string]interface{})
			current[part] = newObj
			current = newObj
			continue
		}
		m, ok := next.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("path %s: expected object at %s", dottedPath, part)
		}
		current = m
	}

	current[parts[len(parts)-1]] = value

	rawBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal parameters: %w", err)
	}

	return &runtime.RawExtension{Raw: rawBytes}, nil
}
