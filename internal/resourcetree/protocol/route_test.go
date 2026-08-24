// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package protocol

import "testing"

// TestGatewayMatchesPathRoundTrip is the seal on the gateway route: the path the
// client builds is the path the gateway parses. Per-package tests each asserted
// their own spelling, which is exactly the drift a mismatched segment causes —
// the call 404s, and a 404 is read as version skew rather than as a bug.
//
// The literal is asserted too, because a round trip alone stays green if both
// sides move together in a way an already-deployed peer does not follow.
func TestGatewayMatchesPathRoundTrip(t *testing.T) {
	const (
		planeType   = "dataplane"
		planeID     = "prod-cluster"
		crNamespace = "acme"
		crName      = "prod-dp"
	)

	path := BuildGatewayMatchesPath(planeType, planeID, crNamespace, crName)

	if want := "/api/resource-tree/dataplane/prod-cluster/acme/prod-dp/matches"; path != want {
		t.Fatalf("the gateway route must stay byte-identical, got %q, want %q", path, want)
	}

	gotPlaneType, gotPlaneID, gotCRNamespace, gotCRName, ok := ParseGatewayMatchesPath(path)
	if !ok {
		t.Fatalf("the built path %q must parse back", path)
	}
	if gotPlaneType != planeType || gotPlaneID != planeID ||
		gotCRNamespace != crNamespace || gotCRName != crName {
		t.Errorf("round trip lost segments: got (%q, %q, %q, %q), want (%q, %q, %q, %q)",
			gotPlaneType, gotPlaneID, gotCRNamespace, gotCRName,
			planeType, planeID, crNamespace, crName)
	}
}
