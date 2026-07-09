// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"fmt"
	"strings"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// Component wraps v1alpha1.Component with domain-specific helper methods
type Component struct {
	*v1alpha1.Component
}

// NewComponent creates a Component wrapper from a ResourceEntry
func NewComponent(entry *index.ResourceEntry) (*Component, error) {
	comp, err := FromEntry[v1alpha1.Component](entry)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to Component: %w", err)
	}
	return &Component{Component: comp}, nil
}

// ComponentTypeName extracts the type name from componentType
// Example: "http-service" from "deployment/http-service"
func (c *Component) ComponentTypeName() string {
	ct := c.Spec.ComponentType.Name
	if idx := strings.LastIndex(ct, "/"); idx >= 0 {
		return ct[idx+1:]
	}
	return ct
}

// ProjectName returns the project name from spec.owner.projectName
func (c *Component) ProjectName() string {
	return c.Spec.Owner.ProjectName
}
