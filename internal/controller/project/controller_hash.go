// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/hash"
)

// ReleaseSpec captures the immutable inputs that uniquely identify a
// ProjectRelease. Owner is intentionally excluded so the hash represents the
// configuration, not the ownership pointer. Mirrors
// internal/controller/resource/controller_hash.go:ReleaseSpec.
type ReleaseSpec struct {
	ProjectType openchoreov1alpha1.ProjectReleaseProjectType `json:"projectType"`
	Parameters  *runtime.RawExtension                        `json:"parameters,omitempty"`
}

// computeReleaseHash returns a deterministic hash for a ReleaseSpec. Same
// underlying algorithm as resource.computeReleaseHash; uses pkg/hash.ComputeHash
// for canonical JSON serialization. Value receiver since the spec is small,
// never mutated, and "no spec" is not a meaningful state at any caller.
func computeReleaseHash(spec ReleaseSpec, collisionCount *int32) string {
	return hash.ComputeHash(spec, collisionCount)
}
