// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/hash"
)

// ReleaseSpec represents the immutable data that defines a ComponentRelease.
// This structure is used to compute a hash that uniquely identifies a specific
// configuration of a component ready for release.
type ReleaseSpec struct {
	// ComponentType is the embedded ComponentType specification
	ComponentType openchoreov1alpha1.ComponentTypeSpec `json:"componentType"`

	// Traits maps trait names to their specifications
	Traits map[string]openchoreov1alpha1.TraitSpec `json:"traits,omitempty"`

	// ComponentProfile contains parameter values and trait configurations
	ComponentProfile *openchoreov1alpha1.ComponentProfile `json:"componentProfile,omitempty"`

	// Workload is the embedded Workload template specification
	Workload openchoreov1alpha1.WorkloadTemplateSpec `json:"workload"`
}

// ComputeReleaseHash returns a hash value calculated from the release spec and
// a collisionCount to avoid hash collision. The hash will be safe encoded to
// avoid bad words.
//
// This is a type-safe wrapper around hash.ComputeHash for ReleaseSpec.
// This follows the same pattern as Kubernetes controller.ComputeHash.
func ComputeReleaseHash(template *ReleaseSpec, collisionCount *int32) string {
	return hash.ComputeHash(*template, collisionCount)
}

// EqualReleaseTemplate returns true if lhs and rhs have the same hash.
// This is used to determine if two release specs are semantically equivalent.
//
// This follows the pattern of EqualRevision in Kubernetes controller_history.go.
func EqualReleaseTemplate(lhs, rhs *ReleaseSpec) bool {
	if lhs == nil || rhs == nil {
		return lhs == rhs
	}
	// Compute hash without collision count for comparison
	return ComputeReleaseHash(lhs, nil) == ComputeReleaseHash(rhs, nil)
}

// ReleaseSpecFromComponentReleaseSpec derives a ReleaseSpec from a ComponentReleaseSpec,
// excluding the Owner field to preserve hash compatibility.
func ReleaseSpecFromComponentReleaseSpec(spec *openchoreov1alpha1.ComponentReleaseSpec) *ReleaseSpec {
	return &ReleaseSpec{
		ComponentType:    spec.ComponentType,
		Traits:           spec.Traits,
		ComponentProfile: spec.ComponentProfile,
		Workload:         spec.Workload,
	}
}
