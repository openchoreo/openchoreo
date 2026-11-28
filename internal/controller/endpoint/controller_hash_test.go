// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"testing"

	"k8s.io/utils/ptr"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestReleaseHash_BackendWithoutReleaseLineKeepsItsHash(t *testing.T) {
	withBackend := func(line string) ReleaseSpec {
		return ReleaseSpec{
			Visibility: []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityExternal},
			BackendRefs: []openchoreov1alpha1.EndpointBackendRef{{
				Name: "stable",
				Kind: openchoreov1alpha1.EndpointBackendKindComponent,
				Component: &openchoreov1alpha1.EndpointComponentBackend{
					Name: "greeter", Endpoint: "http", ReleaseLine: line,
				},
				Weight: ptr.To(int32(90)),
			}},
		}
	}

	// The hash the same spec had before backends could name a release line.
	if got := computeReleaseHash(withBackend("")); got != "7fc656dc79" {
		t.Fatalf("release hash of a backend without a release line = %s, want 7fc656dc79", got)
	}
	if computeReleaseHash(withBackend("next")) == computeReleaseHash(withBackend("")) {
		t.Fatalf("expected naming a release line to change the release hash")
	}
}

func TestReleaseHash_MatchChangesTheHash(t *testing.T) {
	spec := func(match *openchoreov1alpha1.EndpointMatch) ReleaseSpec {
		return ReleaseSpec{BackendRefs: []openchoreov1alpha1.EndpointBackendRef{{
			Name:      "canary",
			Kind:      openchoreov1alpha1.EndpointBackendKindComponent,
			Component: &openchoreov1alpha1.EndpointComponentBackend{Name: "greeter", Endpoint: "http"},
			Weight:    ptr.To(int32(0)),
			Match:     match,
		}}}
	}
	header := &openchoreov1alpha1.EndpointMatch{
		Headers: []openchoreov1alpha1.EndpointHeaderMatch{{Name: "x-canary", Value: "true"}},
	}
	if computeReleaseHash(spec(header)) == computeReleaseHash(spec(nil)) {
		t.Fatalf("expected a match to change the release hash")
	}
}

func TestReleaseHash_MirrorChangesTheHash(t *testing.T) {
	spec := func(mirror *openchoreov1alpha1.EndpointMirror) ReleaseSpec {
		return ReleaseSpec{BackendRefs: []openchoreov1alpha1.EndpointBackendRef{{
			Name:      "shadow",
			Kind:      openchoreov1alpha1.EndpointBackendKindComponent,
			Component: &openchoreov1alpha1.EndpointComponentBackend{Name: "greeter", Endpoint: "http"},
			Weight:    ptr.To(int32(0)),
			Mirror:    mirror,
		}}}
	}
	if computeReleaseHash(spec(&openchoreov1alpha1.EndpointMirror{Percent: 10})) == computeReleaseHash(spec(nil)) {
		t.Fatalf("expected a mirror to change the release hash")
	}
}
