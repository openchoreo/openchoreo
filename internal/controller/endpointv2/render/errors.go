// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"

	corev1 "github.com/openchoreo/openchoreo/api/v1"
)

func UnsupportedEndpointTypeError(epType corev1.EndpointType) error {
	return fmt.Errorf("unsupported endpoint type: %s", epType)
}

func MergeError(err error) error {
	return fmt.Errorf("failed to merge: %w", err)
}
