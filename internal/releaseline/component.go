// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"fmt"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ScopedName names an object of a component's release line: "{component}-{line}-{suffix}",
// or "{component}-{suffix}" when the component has no lines.
func ScopedName(componentName, line, suffix string) string {
	if line == "" {
		return fmt.Sprintf("%s-%s", componentName, suffix)
	}
	return fmt.Sprintf("%s-%s-%s", componentName, line, suffix)
}

// EffectiveComponent returns a copy of the Component with the line's overrides applied.
// Parameters deep-merge; traits merge on (kind, name, instanceName) with parameters deep-merged.
func EffectiveComponent(comp *openchoreov1alpha1.Component, line *openchoreov1alpha1.ReleaseLine) (*openchoreov1alpha1.Component, error) {
	effective := comp.DeepCopy()
	effective.Status = openchoreov1alpha1.ComponentStatus{}

	params, err := MergeParameters(comp.Spec.Parameters, line.Spec.Parameters)
	if err != nil {
		return nil, fmt.Errorf("invalid parameters override: %w", err)
	}
	effective.Spec.Parameters = params

	for _, override := range line.Spec.Traits {
		idx := -1
		for i, t := range effective.Spec.Traits {
			if traitKind(t.Kind) == traitKind(override.Kind) && t.Name == override.Name && t.InstanceName == override.InstanceName {
				idx = i
				break
			}
		}
		if idx < 0 {
			effective.Spec.Traits = append(effective.Spec.Traits, *override.DeepCopy())
			continue
		}
		merged, err := MergeParameters(effective.Spec.Traits[idx].Parameters, override.Parameters)
		if err != nil {
			return nil, fmt.Errorf("invalid parameters override for trait instance %q: %w", override.InstanceName, err)
		}
		effective.Spec.Traits[idx].Parameters = merged
	}
	return effective, nil
}

func traitKind(k openchoreov1alpha1.TraitRefKind) openchoreov1alpha1.TraitRefKind {
	if k == "" {
		return openchoreov1alpha1.TraitRefKindTrait
	}
	return k
}
