// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"fmt"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// BuildInput holds the resolved resources needed to assemble a ComponentReleaseSpec.
// Traits and ClusterTraits are separate maps keyed by trait name.
// Callers handle fetching, ClusterTrait→TraitSpec conversion, and deduplication.
// BuildSpec merges both maps into a single Traits field on the ComponentReleaseSpec.
type BuildInput struct {
	Component     *openchoreov1alpha1.Component
	ComponentType *openchoreov1alpha1.ComponentTypeSpec
	Traits        map[string]openchoreov1alpha1.TraitSpec
	ClusterTraits map[string]openchoreov1alpha1.ClusterTraitSpec
	Workload      *openchoreov1alpha1.WorkloadTemplateSpec
}

// BuildSpec assembles a ComponentReleaseSpec from resolved resources.
// Both the controller and API service use this to ensure consistent spec construction.
func BuildSpec(input BuildInput) (*openchoreov1alpha1.ComponentReleaseSpec, error) {
	if input.Component == nil {
		return nil, fmt.Errorf("component cannot be nil")
	}
	if input.ComponentType == nil {
		return nil, fmt.Errorf("componentType cannot be nil")
	}
	if input.Workload == nil {
		return nil, fmt.Errorf("workload cannot be nil")
	}

	// Validate that all required traits are present in the correct map based on kind
	for _, et := range input.ComponentType.Traits {
		if !hasTraitByKind(input, et.Kind, et.Name) {
			return nil, fmt.Errorf("embedded trait %q required by ComponentType is missing", et.Name)
		}
	}
	for _, ct := range input.Component.Spec.Traits {
		if !hasTraitByKind(input, ct.Kind, ct.Name) {
			return nil, fmt.Errorf("component trait %q is missing", ct.Name)
		}
	}

	// Merge both maps into a single traits slice for the spec
	traits := mergeTraits(input.Traits, input.ClusterTraits)

	return &openchoreov1alpha1.ComponentReleaseSpec{
		Owner: openchoreov1alpha1.ComponentReleaseOwner{
			ProjectName:   input.Component.Spec.Owner.ProjectName,
			ComponentName: input.Component.Name,
		},
		ComponentType:    *input.ComponentType,
		Traits:           traits,
		ComponentProfile: buildComponentProfile(input.Component),
		Workload:         *input.Workload,
	}, nil
}

// hasTraitByKind checks whether the named trait exists in the correct map based on its kind.
func hasTraitByKind(input BuildInput, kind openchoreov1alpha1.TraitRefKind, name string) bool {
	if kind == openchoreov1alpha1.TraitRefKindClusterTrait {
		_, ok := input.ClusterTraits[name]
		return ok
	}
	_, ok := input.Traits[name]
	return ok
}

// mergeTraits combines Traits and ClusterTraits into a []ComponentReleaseTrait slice.
// Each entry preserves its Kind (Trait or ClusterTrait) so that a namespace-scoped Trait
// and a cluster-scoped ClusterTrait with the same name can coexist as separate entries.
// Returns nil if both inputs are empty.
func mergeTraits(traits map[string]openchoreov1alpha1.TraitSpec, clusterTraits map[string]openchoreov1alpha1.ClusterTraitSpec) []openchoreov1alpha1.ComponentReleaseTrait {
	total := len(traits) + len(clusterTraits)
	if total == 0 {
		return nil
	}
	merged := make([]openchoreov1alpha1.ComponentReleaseTrait, 0, total)
	for name, spec := range traits {
		merged = append(merged, openchoreov1alpha1.ComponentReleaseTrait{
			Kind: openchoreov1alpha1.TraitRefKindTrait,
			Name: name,
			Spec: spec,
		})
	}
	for name, spec := range clusterTraits {
		merged = append(merged, openchoreov1alpha1.ComponentReleaseTrait{
			Kind: openchoreov1alpha1.TraitRefKindClusterTrait,
			Name: name,
			Spec: openchoreov1alpha1.TraitSpec(spec),
		})
	}
	return merged
}

// FindTraitSpec searches the release traits slice for an entry matching kind and name,
// returning a copy of its Spec. Returns an empty TraitSpec and false if not found.
func FindTraitSpec(traits []openchoreov1alpha1.ComponentReleaseTrait, kind openchoreov1alpha1.TraitRefKind, name string) (openchoreov1alpha1.TraitSpec, bool) {
	for i := range traits {
		if traits[i].Kind == kind && traits[i].Name == name {
			return traits[i].Spec, true
		}
	}
	return openchoreov1alpha1.TraitSpec{}, false
}

// buildComponentProfile extracts the ComponentProfile from the Component.
// Returns nil if the component has no parameters and no traits.
func buildComponentProfile(comp *openchoreov1alpha1.Component) *openchoreov1alpha1.ComponentProfile {
	if comp.Spec.Parameters == nil && len(comp.Spec.Traits) == 0 {
		return nil
	}
	profileTraits := make([]openchoreov1alpha1.ComponentProfileTrait, 0, len(comp.Spec.Traits))
	for _, ct := range comp.Spec.Traits {
		kind := ct.Kind
		if kind == "" {
			kind = openchoreov1alpha1.TraitRefKindTrait
		}
		profileTraits = append(profileTraits, openchoreov1alpha1.ComponentProfileTrait{
			Kind:         kind,
			Name:         ct.Name,
			InstanceName: ct.InstanceName,
			Parameters:   ct.Parameters,
		})
	}
	return &openchoreov1alpha1.ComponentProfile{
		Parameters: comp.Spec.Parameters,
		Traits:     profileTraits,
	}
}
