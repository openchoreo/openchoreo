// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

// New creates a new API client, wrapping any error with a standard message.
func New() (*client.Client, error) {
	cl, err := client.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}
	return cl, nil
}
