// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func UnsupportedEndpointTypeError(epType openchoreov1alpha1.EndpointType) error {
	return fmt.Errorf("unsupported endpoint type: %s", epType)
}

func MergeError(err error) error {
	return fmt.Errorf("failed to merge: %w", err)
}
