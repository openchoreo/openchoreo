// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"fmt"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// ComponentType wraps v1alpha1.ComponentType with domain-specific helper methods
type ComponentType struct {
	*v1alpha1.ComponentType
}

// NewComponentType creates a ComponentType wrapper from a ResourceEntry
func NewComponentType(entry *index.ResourceEntry) (*ComponentType, error) {
	ct, err := FromEntry[v1alpha1.ComponentType](entry)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to ComponentType: %w", err)
	}
	return &ComponentType{ComponentType: ct}, nil
}

// WorkloadType returns the workload type
func (ct *ComponentType) WorkloadType() string {
	return ct.Spec.WorkloadType
}
