// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"testing"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// The declarations must participate in the release hash. Release names are
// content-addressed, so if the hash ignored the annotation, editing it would reuse the
// existing release and never reach a binding.
func TestReleaseHashCoversLocalDevAddressesAnnotation(t *testing.T) {
	rt := v1alpha1.ResourceReleaseResourceType{
		Kind: "ClusterResourceType",
		Name: "t",
		Spec: v1alpha1.ResourceTypeSpec{
			Outputs: []v1alpha1.ResourceTypeOutput{
				{Name: "host", Value: "h"},
				{Name: "port", Value: "6379"},
			},
			Resources: []v1alpha1.ResourceTypeManifest{{ID: "marker"}},
		},
	}

	h1 := computeReleaseHash(ReleaseSpec{ResourceType: rt})
	h2 := computeReleaseHash(ReleaseSpec{ResourceType: rt, LocalDevAddresses: "database=outputs.host:outputs.port"})
	h3 := computeReleaseHash(ReleaseSpec{ResourceType: rt, LocalDevAddresses: "database=outputs.host:outputs.otherPort"})

	t.Logf("without=%s  with=%s  edited=%s", h1, h2, h3)
	if h1 == h2 {
		t.Fatalf("declaring endpoints does not affect the release hash: both %s", h1)
	}
	if h2 == h3 {
		t.Fatalf("editing the endpoints declaration does not affect the release hash: both %s", h2)
	}
}
