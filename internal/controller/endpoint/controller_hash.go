// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/dump"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/hash"
)

// ReleaseSpec captures the immutable inputs that uniquely identify an
// EndpointRelease. Owner is intentionally excluded so the hash represents the
// configuration, not the ownership pointer (mirrors resource.ReleaseSpec).
type ReleaseSpec struct {
	EndpointType openchoreov1alpha1.EndpointReleaseEndpointType `json:"endpointType"`
	Visibility   []openchoreov1alpha1.EndpointVisibility        `json:"visibility"`
	BackendRefs  []openchoreov1alpha1.EndpointBackendRef        `json:"backendRefs"`
	Schema       *openchoreov1alpha1.Schema                     `json:"schema,omitempty"`
	Parameters   *runtime.RawExtension                          `json:"parameters,omitempty"`
}

// computeReleaseHash returns a deterministic hash for a ReleaseSpec. Fields
// added since a release could first be cut are hashed as they were before they
// existed while they are unset, so upgrading doesn't cut a new release for an
// unchanged Endpoint.
func computeReleaseHash(spec ReleaseSpec) string {
	return hash.ComputeHashOfString(hashCompatible.Replace(dump.ForHash(spec)), nil)
}

// hashCompatible rewrites dump.ForHash output to what it was before these
// fields existed. Each pattern is anchored on the field after it, so it can't
// match anywhere else.
var hashCompatible = strings.NewReplacer(
	// A Component backend without a release line.
	" ReleaseLine:(string)} Weight:", "} Weight:",
)
