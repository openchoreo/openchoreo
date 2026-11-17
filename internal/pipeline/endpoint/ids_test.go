// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointpipeline

import (
	"strings"
	"testing"
)

func TestNetworkPolicyID(t *testing.T) {
	if got := NetworkPolicyID("stable"); got != "openchoreo-networkpolicy-stable" {
		t.Fatalf("short backend name = %q, want it kept as is", got)
	}

	long := strings.Repeat("a", 30) + "-" + strings.Repeat("b", 32) // 63 characters, the longest backend name
	id := NetworkPolicyID(long)
	if len(id) > maxResourceIDLength {
		t.Fatalf("ID %q is %d characters, want at most %d", id, len(id), maxResourceIDLength)
	}
	if !strings.HasPrefix(id, NetworkPolicyIDPrefix) {
		t.Fatalf("ID %q lost the reserved prefix", id)
	}
	if id != NetworkPolicyID(long) {
		t.Fatal("ID is not stable for the same backend name")
	}

	// Names sharing the truncated part still get distinct IDs.
	other := strings.Repeat("a", 30) + "-" + strings.Repeat("c", 32)
	if NetworkPolicyID(other) == id {
		t.Fatalf("backends %q and %q share ID %q", long, other, id)
	}

	// A truncation point on a hyphen does not leave a double hyphen.
	hyphen := strings.Repeat("a", 28) + "-" + strings.Repeat("b", 34)
	if got := NetworkPolicyID(hyphen); strings.Contains(got, "--") {
		t.Fatalf("ID %q contains a double hyphen", got)
	}
}
