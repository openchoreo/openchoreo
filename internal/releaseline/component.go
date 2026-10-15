// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"fmt"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// ScopedName names an object of a component's release line: "{component}-{line}-{suffix}",
// or "{component}-{suffix}" when the component has no lines.
func ScopedName(componentName, line, suffix string) string {
	if line == "" {
		return fmt.Sprintf("%s-%s", componentName, suffix)
	}
	return fmt.Sprintf("%s-%s-%s", componentName, line, suffix)
}

// ComponentLineName names the component-level objects of a release line, such as its network
// policies and derived ConfigMaps and Secrets: the component name without a line, and
// "{component}-{line}", bounded to a DNS label, otherwise.
func ComponentLineName(componentName, line string) string {
	if line == "" {
		return componentName
	}
	name := componentName + "-" + line
	if len(name) > dpkubernetes.MaxServiceNameLength {
		name = dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxServiceNameLength, componentName, line)
	}
	return name
}

// ObjectBaseName names a release line's objects in an environment: "{component}-{line}-{environment}-{hash}".
// The hash covers the line on its own, so component "a-b" on line "c", component "a" on line "b-c" and
// component "a-b-c" without lines never share a name.
func ObjectBaseName(componentName, line, environment string) string {
	return dpkubernetes.GenerateK8sNameWithExtraHashInput(dpkubernetes.MaxResourceNameLength,
		"release-line="+line, componentName, line, environment)
}

// maxGeneratedNamePrefix is how much of a generateName Kubernetes keeps, leaving room for its
// random suffix.
const maxGeneratedNamePrefix = 58

// ObjectNamePrefix is the name prefix of every workload object of a release line in an environment,
// including pods whose generated names Kubernetes truncated.
func ObjectNamePrefix(componentName, line, environment string) string {
	prefix := ObjectBaseName(componentName, line, environment)
	if len(prefix) > maxGeneratedNamePrefix {
		prefix = prefix[:maxGeneratedNamePrefix]
	}
	return prefix
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
