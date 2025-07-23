// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import "fmt"

// GetStoredKubeConfigValues is a stub function to maintain compatibility
// TODO: Remove this once all direct Kubernetes access is removed from CLI
func GetStoredKubeConfigValues() (string, string, error) {
	return "", "", fmt.Errorf("direct Kubernetes access is deprecated, use choreoctl apply instead")
}
