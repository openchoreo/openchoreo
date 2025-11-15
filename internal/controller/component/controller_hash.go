// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"

	"k8s.io/apimachinery/pkg/util/dump"
	"k8s.io/apimachinery/pkg/util/rand"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
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
	ComponentProfile openchoreov1alpha1.ComponentProfile `json:"componentProfile"`

	// Workload is the embedded Workload template specification
	Workload openchoreov1alpha1.WorkloadTemplateSpec `json:"workload"`
}

// computeHash is a generic hash function following Kubernetes patterns.
// It computes a hash value from any object using dump.ForHash() for deterministic
// string representation and an optional collisionCount to avoid hash collisions.
// The hash will be safe encoded to avoid bad words.
//
// This follows the same algorithm as Kubernetes controller.ComputeHash but
// works with any type instead of being specific to PodTemplateSpec.
//
// Typical usage pattern from k8s.io/apimachinery/pkg/util/dump:
//
//	hashableString := dump.ForHash(myObject)
//	// Then pass to a hash function
//
// This is an internal helper function. Use type-specific wrappers like
// ComputeReleaseHash for better type safety.
func computeHash(obj interface{}, collisionCount *int32) string {
	hasher := fnv.New32a()

	// Get deterministic string representation using dump.ForHash
	// This is the Kubernetes standard way to get hashable representations
	hashableStr := dump.ForHash(obj)
	hasher.Write([]byte(hashableStr))

	// Add collisionCount in the hash if it exists and is non-negative.
	// Collision count should always be >= 0 in practice (it's a counter).
	if collisionCount != nil && *collisionCount >= 0 {
		collisionCountBytes := make([]byte, 8)
		binary.LittleEndian.PutUint32(collisionCountBytes, uint32(*collisionCount))
		hasher.Write(collisionCountBytes)
	}

	return rand.SafeEncodeString(fmt.Sprint(hasher.Sum32()))
}

// ComputeReleaseHash returns a hash value calculated from the release spec and
// a collisionCount to avoid hash collision. The hash will be safe encoded to
// avoid bad words.
//
// This is a type-safe wrapper around computeHash for ReleaseSpec.
// This follows the same pattern as Kubernetes controller.ComputeHash.
func ComputeReleaseHash(template *ReleaseSpec, collisionCount *int32) string {
	return computeHash(*template, collisionCount)
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

// BuildReleaseSpec constructs a ReleaseSpec from the component's
// related resources. This extracts the immutable snapshot data needed for hashing.
//
// This follows the pattern of building template specs in Kubernetes controllers.
func BuildReleaseSpec(
	ct *openchoreov1alpha1.ComponentType,
	traits []openchoreov1alpha1.Trait,
	comp *openchoreov1alpha1.Component,
	workload *openchoreov1alpha1.Workload,
) (*ReleaseSpec, error) {
	if ct == nil {
		return nil, fmt.Errorf("componentType cannot be nil")
	}
	if workload == nil {
		return nil, fmt.Errorf("workload cannot be nil")
	}
	if comp == nil {
		return nil, fmt.Errorf("component cannot be nil")
	}

	// Build traits map from trait slice
	traitsMap := make(map[string]openchoreov1alpha1.TraitSpec)
	if len(traits) == 0 || traits == nil {
		traitsMap = nil
	} else {
		for _, trait := range traits {
			traitsMap[trait.Name] = trait.Spec
		}
	}

	// Build component profile
	componentProfile := openchoreov1alpha1.ComponentProfile{
		Parameters: comp.Spec.Parameters,
		Traits:     comp.Spec.Traits,
	}

	// Construct ReleaseSpec
	releaseSpec := &ReleaseSpec{
		ComponentType:    ct.Spec,
		Traits:           traitsMap,
		ComponentProfile: componentProfile,
		Workload:         workload.Spec.WorkloadTemplateSpec,
	}

	return releaseSpec, nil
}
