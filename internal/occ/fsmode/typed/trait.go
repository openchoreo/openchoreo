// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"fmt"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// Trait wraps v1alpha1.Trait with domain-specific helper methods
type Trait struct {
	*v1alpha1.Trait
}

// NewTrait creates a Trait wrapper from a ResourceEntry
func NewTrait(entry *index.ResourceEntry) (*Trait, error) {
	trait, err := FromEntry[v1alpha1.Trait](entry)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to Trait: %w", err)
	}
	return &Trait{Trait: trait}, nil
}

// GetSpec returns the full trait spec as a map for template processing
func (t *Trait) GetSpec() map[string]interface{} {
	spec := make(map[string]interface{})

	// Add schema
	if t.Spec.Schema.Types != nil || t.Spec.Schema.Parameters != nil || t.Spec.Schema.EnvOverrides != nil {
		schema := make(map[string]interface{})
		if t.Spec.Schema.Types != nil {
			schema["types"] = rawExtensionToMap(t.Spec.Schema.Types)
		}
		if t.Spec.Schema.Parameters != nil {
			schema["parameters"] = rawExtensionToMap(t.Spec.Schema.Parameters)
		}
		if t.Spec.Schema.EnvOverrides != nil {
			schema["envOverrides"] = rawExtensionToMap(t.Spec.Schema.EnvOverrides)
		}
		spec["schema"] = schema
	}

	// Add creates
	if len(t.Spec.Creates) > 0 {
		creates := make([]interface{}, len(t.Spec.Creates))
		for i, c := range t.Spec.Creates {
			createMap := make(map[string]interface{})
			if c.TargetPlane != "" {
				createMap["targetPlane"] = c.TargetPlane
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
