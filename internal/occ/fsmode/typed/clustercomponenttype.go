// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"fmt"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// ClusterComponentType wraps v1alpha1.ClusterComponentType with domain-specific helper methods
type ClusterComponentType struct {
	*v1alpha1.ClusterComponentType
}

// NewClusterComponentType creates a ClusterComponentType wrapper from a ResourceEntry
func NewClusterComponentType(entry *index.ResourceEntry) (*ClusterComponentType, error) {
	cct, err := FromEntry[v1alpha1.ClusterComponentType](entry)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to ClusterComponentType: %w", err)
	}
	return &ClusterComponentType{ClusterComponentType: cct}, nil
}

// WorkloadType returns the workload type
func (cct *ClusterComponentType) WorkloadType() string {
	return cct.Spec.WorkloadType
}
