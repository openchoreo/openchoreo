// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"
)

// UnsupportedAPITypeError returns an error for unsupported API types.
func UnsupportedAPITypeError(apiType string) error {
	return fmt.Errorf("unsupported API type: %s", apiType)
}

// MissingAPIClassError returns an error when API class is missing.
func MissingAPIClassError() error {
	return fmt.Errorf("API class is required but not specified")
}

// MissingAPISpecError returns an error when API specification is missing.
func MissingAPISpecError() error {
	return fmt.Errorf("API specification is missing")
}
