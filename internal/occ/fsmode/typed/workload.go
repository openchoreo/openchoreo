// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"fmt"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// Workload wraps v1alpha1.Workload with domain-specific helper methods
type Workload struct {
	*v1alpha1.Workload
}

// NewWorkload creates a Workload wrapper from a ResourceEntry
func NewWorkload(entry *index.ResourceEntry) (*Workload, error) {
	wl, err := FromEntry[v1alpha1.Workload](entry)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to Workload: %w", err)
	}
	return &Workload{Workload: wl}, nil
}
