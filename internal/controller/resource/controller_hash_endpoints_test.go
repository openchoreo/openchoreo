// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"testing"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Endpoints must participate in the release hash. ResourceRelease names are
// content-addressed, so if the hash ignored endpoints, adding one to a resource type
// would reuse the existing release and the snapshot would never carry the declaration.
func TestReleaseHashCoversEndpoints(t *testing.T) {
	base := v1alpha1.ResourceTypeSpec{
		Outputs: []v1alpha1.ResourceTypeOutput{
			{Name: "host", Value: "h"},
			{Name: "port", Value: "6379"},
		},
		Resources: []v1alpha1.ResourceTypeManifest{{ID: "marker"}},
	}
	withEP := base
	withEP.Endpoints = []v1alpha1.ResourceTypeEndpoint{
		{Name: "client", HostFrom: "host", PortFrom: "port"},
	}

	h1 := computeReleaseHash(ReleaseSpec{
		ResourceType: v1alpha1.ResourceReleaseResourceType{Kind: "ClusterResourceType", Name: "t", Spec: base},
	}, nil)
	h2 := computeReleaseHash(ReleaseSpec{
		ResourceType: v1alpha1.ResourceReleaseResourceType{Kind: "ClusterResourceType", Name: "t", Spec: withEP},
	}, nil)

	t.Logf("without endpoints=%s  with endpoints=%s", h1, h2)
	if h1 == h2 {
		t.Fatalf("endpoints do not affect the release hash: both %s", h1)
	}
}
