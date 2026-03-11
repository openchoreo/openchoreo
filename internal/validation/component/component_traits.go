// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"
	"strings"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ValidateAllowedTraits checks that all component-level traits are permitted by the ComponentType's allowedTraits list.
// If allowedTraits is empty, no component-level traits are allowed.
func ValidateAllowedTraits(compTraits []v1alpha1.ComponentTrait, allowedTraits []v1alpha1.TraitRef) error {
	if len(compTraits) == 0 {
		return nil
	}

	if len(allowedTraits) == 0 {
		return fmt.Errorf("no traits are allowed, but component has %d trait(s)", len(compTraits))
	}

	allowedSet := make(map[string]bool, len(allowedTraits))
	for _, ref := range allowedTraits {
		key := traitRefKey(ref.Kind, ref.Name)
		allowedSet[key] = true
	}

	var disallowed []string
	for _, trait := range compTraits {
		key := traitRefKey(trait.Kind, trait.Name)
		if !allowedSet[key] {
			disallowed = append(disallowed, string(trait.Kind)+":"+trait.Name)
		}
	}

	if len(disallowed) > 0 {
		return fmt.Errorf("traits %v are not in the allowed list %v", disallowed, formatAllowedTraits(allowedTraits))
	}
	return nil
}

// ValidateTraitInstanceNameUniqueness checks that:
// - embedded trait instance names are unique among themselves
// - component-level trait instance names are unique among themselves
// - component-level trait instance names do not collide with embedded trait instance names
func ValidateTraitInstanceNameUniqueness(compTraits []v1alpha1.ComponentTrait, embeddedTraits []v1alpha1.ComponentTypeTrait) error {
	// Check for duplicates within embedded traits
	seen := make(map[string]bool, len(embeddedTraits)+len(compTraits))
	var duplicates []string
	for _, et := range embeddedTraits {
		if seen[et.InstanceName] {
			duplicates = append(duplicates, et.InstanceName)
		}
		seen[et.InstanceName] = true
	}
	if len(duplicates) > 0 {
		return fmt.Errorf("duplicate trait instance names %v in embedded traits", duplicates)
	}

	// Check for duplicates within component-level traits
	compSeen := make(map[string]bool, len(compTraits))
	for _, t := range compTraits {
		if compSeen[t.InstanceName] {
			duplicates = append(duplicates, t.InstanceName)
		}
		compSeen[t.InstanceName] = true
	}
	if len(duplicates) > 0 {
		return fmt.Errorf("duplicate trait instance names %v in component traits", duplicates)
	}

	// Check for collisions between component-level and embedded traits
	var colliding []string
	for _, t := range compTraits {
		if seen[t.InstanceName] {
			colliding = append(colliding, t.InstanceName)
		}
	}
	if len(colliding) > 0 {
		return fmt.Errorf("trait instance names %v collide with embedded traits", colliding)
	}

	return nil
}

// ValidateTraitNameKindConsistency checks that the same trait name is not referenced with
// different kinds (Trait vs ClusterTrait) within embedded traits, within component-level
// traits, or across the two lists. This is required because ComponentRelease uses trait
// name as the map key, so a Trait and ClusterTrait with the same name would collide.
func ValidateTraitNameKindConsistency(compTraits []v1alpha1.ComponentTrait, embeddedTraits []v1alpha1.ComponentTypeTrait) error {
	kindByName := make(map[string]v1alpha1.TraitRefKind, len(embeddedTraits)+len(compTraits))

	// Check within embedded traits
	var conflicts []string
	for _, et := range embeddedTraits {
		if prevKind, exists := kindByName[et.Name]; exists && prevKind != et.Kind {
			conflicts = append(conflicts, fmt.Sprintf("%q referenced as both %s and %s in embedded traits", et.Name, prevKind, et.Kind))
		}
		kindByName[et.Name] = et.Kind
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("trait name kind mismatch: %s", strings.Join(conflicts, ", "))
	}

	// Check within component traits
	compKindByName := make(map[string]v1alpha1.TraitRefKind, len(compTraits))
	for _, t := range compTraits {
		if prevKind, exists := compKindByName[t.Name]; exists && prevKind != t.Kind {
			conflicts = append(conflicts, fmt.Sprintf("%q referenced as both %s and %s in component traits", t.Name, prevKind, t.Kind))
		}
		compKindByName[t.Name] = t.Kind
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("trait name kind mismatch: %s", strings.Join(conflicts, ", "))
	}

	// Check across embedded and component traits
	for name, compKind := range compKindByName {
		if embeddedKind, exists := kindByName[name]; exists && embeddedKind != compKind {
			conflicts = append(conflicts, fmt.Sprintf("%q referenced as %s in embedded traits and %s in component traits", name, embeddedKind, compKind))
		}
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("trait name kind mismatch: %s", strings.Join(conflicts, ", "))
	}

	return nil
}

func traitRefKey(kind v1alpha1.TraitRefKind, name string) string {
	return string(kind) + ":" + name
}

func formatAllowedTraits(refs []v1alpha1.TraitRef) []string {
	result := make([]string, len(refs))
	for i, ref := range refs {
		result[i] = string(ref.Kind) + ":" + ref.Name
	}
	return result
}
