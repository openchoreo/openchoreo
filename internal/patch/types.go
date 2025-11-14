// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package patch

// JSONPatchOperation represents a single patch operation in a JSON Patch specification.
type JSONPatchOperation struct {
	Op    string `yaml:"op"`
	Path  string `yaml:"path"`
	Value any    `yaml:"value,omitempty"`
}
