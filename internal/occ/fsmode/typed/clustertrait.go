// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"fmt"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// ClusterTrait wraps v1alpha1.ClusterTrait with domain-specific helper methods
type ClusterTrait struct {
	*v1alpha1.ClusterTrait
}

// NewClusterTrait creates a ClusterTrait wrapper from a ResourceEntry
func NewClusterTrait(entry *index.ResourceEntry) (*ClusterTrait, error) {
	trait, err := FromEntry[v1alpha1.ClusterTrait](entry)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to ClusterTrait: %w", err)
	}
	return &ClusterTrait{ClusterTrait: trait}, nil
}
