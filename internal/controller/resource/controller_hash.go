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

// ComputeReleaseHash returns a deterministic hash for a ReleaseSpec. Mirrors
// component.ComputeReleaseHash — same algorithm, different inputs.
func ComputeReleaseHash(spec *ReleaseSpec, collisionCount *int32) string {
	return hash.ComputeHash(*spec, collisionCount)
}
