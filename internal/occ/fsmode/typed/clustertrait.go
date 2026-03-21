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

// GetSpec returns the full cluster trait spec as a map for template processing
func (t *ClusterTrait) GetSpec() map[string]interface{} {
	spec := make(map[string]interface{})

	// Add schema sections
	if params := t.Spec.Parameters.GetRaw(); params != nil {
		spec["parameters"] = rawExtensionToMap(params)
	}
	if envConfig := t.Spec.EnvironmentConfigs.GetRaw(); envConfig != nil {
		spec["environmentConfigs"] = rawExtensionToMap(envConfig)
	}

	// Add creates
	if len(t.Spec.Creates) > 0 {
		creates := make([]interface{}, len(t.Spec.Creates))
		for i, c := range t.Spec.Creates {
			createMap := make(map[string]interface{})
			if c.TargetPlane != "" {
				createMap["targetPlane"] = c.TargetPlane
			}
			if c.IncludeWhen != "" {
				createMap["includeWhen"] = c.IncludeWhen
			}
			if c.ForEach != "" {
				createMap["forEach"] = c.ForEach
			}
			if c.Var != "" {
				createMap["var"] = c.Var
			}
			if c.Template != nil {
				createMap["template"] = rawExtensionToMap(c.Template)
			}
			creates[i] = createMap
		}
		spec["creates"] = creates
	}

	// Add patches
	if len(t.Spec.Patches) > 0 {
		patches := make([]interface{}, len(t.Spec.Patches))
		for i, p := range t.Spec.Patches {
			patchMap := make(map[string]interface{})
			if p.ForEach != "" {
				patchMap["forEach"] = p.ForEach
			}
			if p.Var != "" {
				patchMap["var"] = p.Var
			}
			if p.TargetPlane != "" {
				patchMap["targetPlane"] = p.TargetPlane
			}

			// Add target
			patchMap["target"] = map[string]interface{}{
				"group":   p.Target.Group,
				"version": p.Target.Version,
				"kind":    p.Target.Kind,
			}
			if p.Target.Where != "" {
				patchMap["target"].(map[string]interface{})["where"] = p.Target.Where
			}

			// Add operations
			if len(p.Operations) > 0 {
				operations := make([]interface{}, len(p.Operations))
				for j, op := range p.Operations {
					opMap := map[string]interface{}{
						"op":   op.Op,
						"path": op.Path,
					}
					if op.Value != nil {
						opMap["value"] = rawExtensionToMap(op.Value)
					}
					operations[j] = opMap
				}
				patchMap["operations"] = operations
			}

			patches[i] = patchMap
		}
		spec["patches"] = patches
	}

	return spec
}
