// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"fmt"
	"os"
)

// GetEnv gets an environment variable or returns a default value
func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetEnvBool gets a boolean environment variable or returns a default value.
// Recognizes "true", "1" as true and "false", "0" as false.
func GetEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1"
	}
	return defaultValue
}

// GetEnvInt gets an integer environment variable or returns a default value
func GetEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intValue int
		if _, err := fmt.Sscanf(value, "%d", &intValue); err == nil {
			return intValue
		}
	}
	return defaultValue
}
