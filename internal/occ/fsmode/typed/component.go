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

// ComponentTypeCategory extracts the category from componentType
// Example: "deployment" from "deployment/http-service"
func (c *Component) ComponentTypeCategory() string {
	ct := c.Spec.ComponentType.Name
	if idx := strings.Index(ct, "/"); idx >= 0 {
		return ct[:idx]
	}
	return ""
}

// GetParameters returns the component parameters as a map for template processing
func (c *Component) GetParameters() map[string]interface{} {
	return rawExtensionToMap(c.Spec.Parameters)
}

// GetTraitRefs returns trait references converted to TraitRef for easier use
func (c *Component) GetTraitRefs() []TraitRef {
	if len(c.Spec.Traits) == 0 {
		return nil
	}

	refs := make([]TraitRef, len(c.Spec.Traits))
	for i, trait := range c.Spec.Traits {
		refs[i] = TraitRef{
			Name:         trait.Name,
			InstanceName: trait.InstanceName,
			Parameters:   rawExtensionToMap(trait.Parameters),
		}
	}
	return refs
}

// ProjectName returns the project name from spec.owner.projectName
func (c *Component) ProjectName() string {
	return c.Spec.Owner.ProjectName
}
