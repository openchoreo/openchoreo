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

// GetSchema returns the schema as a map for template processing
func (ct *ComponentType) GetSchema() map[string]interface{} {
	schema := make(map[string]interface{})

	if ct.Spec.Schema.Types != nil {
		schema["types"] = rawExtensionToMap(ct.Spec.Schema.Types)
	}
	if ct.Spec.Schema.Parameters != nil {
		schema["parameters"] = rawExtensionToMap(ct.Spec.Schema.Parameters)
	}
	if ct.Spec.Schema.EnvOverrides != nil {
		schema["envOverrides"] = rawExtensionToMap(ct.Spec.Schema.EnvOverrides)
	}

	return schema
}

// GetResources returns the resource templates as a slice for template processing
func (ct *ComponentType) GetResources() []interface{} {
	if len(ct.Spec.Resources) == 0 {
		return nil
	}

	resources := make([]interface{}, len(ct.Spec.Resources))
	for i, res := range ct.Spec.Resources {
		resourceMap := map[string]interface{}{
			"id": res.ID,
		}

		if res.TargetPlane != "" {
			resourceMap["targetPlane"] = res.TargetPlane
		}
		if res.IncludeWhen != "" {
			resourceMap["includeWhen"] = res.IncludeWhen
		}
		if res.ForEach != "" {
			resourceMap["forEach"] = res.ForEach
		}
		if res.Var != "" {
			resourceMap["var"] = res.Var
		}
		if res.Template != nil {
			resourceMap["template"] = rawExtensionToMap(res.Template)
		}

		resources[i] = resourceMap
	}

	return resources
}

// WorkloadType returns the workload type
func (ct *ComponentType) WorkloadType() string {
	return ct.Spec.WorkloadType
}
