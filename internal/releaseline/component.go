// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"fmt"
	"strings"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// ScopedName names a control plane object of a component's line, such as its Workload, ComponentReleases
// and ReleaseBindings: "{component}-{suffix}" for the default line, and "{componentLineName}-{suffix}" for a
// release line, so a line's objects never take the names of another component's.
func ScopedName(componentName, line, suffix string) string {
	return ComponentLineName(componentName, line) + "-" + suffix
}

// lineName names an object of a release line from parts that start with the component and the line, with a
// hash of "{parts}_{line}". A name can't contain "_", so the line after it is exact: component "a-b" on line
// "c", component "a" on line "b-c" and component "a-b-c" without lines never share a hash.
func lineName(limit int, line string, parts ...string) string {
	return dpkubernetes.GenerateK8sNameWithHashInput(limit, strings.Join(parts, "-")+"_"+line, parts...)
}

// ComponentLineName names the component-level objects of a release line, such as its network
// policies and derived ConfigMaps and Secrets: the component name without a line, and
// "{component}-{line}-{hash}", bounded to a DNS label, otherwise.
func ComponentLineName(componentName, line string) string {
	if line == "" {
		return componentName
	}
	return lineName(dpkubernetes.MaxServiceNameLength, line, componentName, line)
}

// releaseNameBaseLimit leaves room in a release name for a suffix of up to 12 characters, a release hash
// or "{date}-{counter}", so the name fits a label value.
const releaseNameBaseLimit = dpkubernetes.MaxServiceNameLength - 13

// ReleaseName names a ComponentRelease of a component's line: "{component}-{suffix}" for the default line,
// and "{ReleaseNameBase}-{suffix}" for a release line. The suffix is the release hash or its date and counter.
func ReleaseName(componentName, line, suffix string) string {
	return ReleaseNameBase(componentName, line) + "-" + suffix
}

// ReleaseNameBase is the part of a release name before its suffix: the component name for the default line,
// and the line's ComponentLineName for a release line, shortened when needed so the release name fits a label value.
func ReleaseNameBase(componentName, line string) string {
	if line == "" {
		return componentName
	}
	if name := ComponentLineName(componentName, line); len(name) <= releaseNameBaseLimit {
		return name
	}
	return lineName(releaseNameBaseLimit, line, componentName, line)
}

// ObjectBaseName names a release line's objects in an environment: "{component}-{line}-{environment}-{hash}".
func ObjectBaseName(componentName, line, environment string) string {
	return lineName(dpkubernetes.MaxResourceNameLength, line, componentName, line, environment)
}

// renderedReleaseNameLimit leaves room for the "-observability" suffix of an observability plane release, so a
// RenderedRelease name fits the label that names it on every object it applies.
const renderedReleaseNameLimit = dpkubernetes.MaxServiceNameLength - len("-observability")

// RenderedReleaseName names the data plane RenderedRelease of a component's line in an environment:
// "{component}-{environment}" for the default line, and for a release line its ObjectBaseName, shortened when
// needed so that the name, also with an "-observability" suffix, fits a label value.
func RenderedReleaseName(componentName, line, environment string) string {
	if line == "" {
		return componentName + "-" + environment
	}
	if name := ObjectBaseName(componentName, line, environment); len(name) <= renderedReleaseNameLimit {
		return name
	}
	return lineName(renderedReleaseNameLimit, line, componentName, line, environment)
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
