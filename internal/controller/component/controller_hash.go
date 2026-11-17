// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"sort"
	"strings"

	"k8s.io/utils/dump"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/hash"
)

// ReleaseSpec represents the immutable data that defines a ComponentRelease.
// This structure is used to compute a hash that uniquely identifies a specific
// configuration of a component ready for release.
type ReleaseSpec struct {
	// ComponentType is the frozen ComponentType snapshot (kind, name, and spec)
	ComponentType openchoreov1alpha1.ComponentReleaseComponentType `json:"componentType"`

	// Traits holds the frozen trait specifications, sorted by (Kind, Name) for deterministic hashing.
	Traits []openchoreov1alpha1.ComponentReleaseTrait `json:"traits,omitempty"`

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
	// Fields added since a release could first be cut are hashed as they were before they existed
	// while they are unset, so upgrading doesn't change the release hash of an unchanged component.
	return hash.ComputeHashOfString(hashCompatible.Replace(dump.ForHash(*template)), collisionCount)
}

// hashCompatible rewrites dump.ForHash output to what it was before these fields existed. Each
// pattern is anchored on the field after it, so it can't match anywhere else.
var hashCompatible = strings.NewReplacer(
	// An endpoint dependency without a release line.
	"ReleaseLine:(string) Name:(string)", "Name:(string)",
	// A ComponentType without allowedEndpoints.
	"AllowedEndpoints:([]v1alpha1.AllowedEndpointType)<nil> Validations:", "Validations:",
	// A workload endpoint without protocol, whose type was named EndpointType before the
	// protocol enum was renamed to EndpointProtocol.
	"Protocol:(v1alpha1.EndpointProtocol) Type:(v1alpha1.EndpointProtocol)", "Type:(v1alpha1.EndpointType)",
)

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
// Traits are sorted by (Kind, Name) to ensure the hash is deterministic regardless of
// the order in which traits were collected.
func ReleaseSpecFromComponentReleaseSpec(spec *openchoreov1alpha1.ComponentReleaseSpec) *ReleaseSpec {
	sortedTraits := make([]openchoreov1alpha1.ComponentReleaseTrait, len(spec.Traits))
	copy(sortedTraits, spec.Traits)
	sort.Slice(sortedTraits, func(i, j int) bool {
		ki, kj := string(sortedTraits[i].Kind), string(sortedTraits[j].Kind)
		if ki != kj {
			return ki < kj
		}
		return sortedTraits[i].Name < sortedTraits[j].Name
	})
	return &ReleaseSpec{
		ComponentType:    spec.ComponentType,
		Traits:           sortedTraits,
		ComponentProfile: spec.ComponentProfile,
		Workload:         spec.Workload,
	}
}
