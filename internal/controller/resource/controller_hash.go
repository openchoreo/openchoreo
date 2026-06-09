// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/hash"
)

// ReleaseSpec captures the immutable inputs that uniquely identify a
// ResourceRelease. Owner is intentionally excluded so the hash represents the
// configuration, not the ownership pointer (mirrors component.ReleaseSpec).
type ReleaseSpec struct {
	ResourceType openchoreov1alpha1.ResourceReleaseResourceType `json:"resourceType"`
	Parameters   *runtime.RawExtension                          `json:"parameters,omitempty"`
}

// computeReleaseHash returns a deterministic hash for a ReleaseSpec. Same
// underlying algorithm as component.ComputeReleaseHash, with a value receiver
// since the spec is small, never mutated, and "no spec" is not a meaningful
// state at any caller.
func computeReleaseHash(spec ReleaseSpec, collisionCount *int32) string {
	return hash.ComputeHash(spec, collisionCount)
}
