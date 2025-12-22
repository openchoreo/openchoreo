// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package hash provides generic utilities for computing hashes.
// This package contains no domain-specific types and can be used by any package.
package hash

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"

	"k8s.io/apimachinery/pkg/util/dump"
	"k8s.io/apimachinery/pkg/util/rand"
)

// ComputeHash is a generic hash function following Kubernetes patterns.
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
func ComputeHash(obj interface{}, collisionCount *int32) string {
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

// Equal returns true if two objects produce the same hash.
// This is a convenience function for comparing objects without collision count.
func Equal(obj1, obj2 interface{}) bool {
	return ComputeHash(obj1, nil) == ComputeHash(obj2, nil)
}
