// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package postrendervalidator

import "testing"

func TestFixForDiagnostic(t *testing.T) {
	if got := fixForDiagnostic("image-latest", ""); got == "" {
		t.Fatal("image-latest should include fix guidance")
	}
	if got := fixForDiagnostic("post-render-validation-failed", "replicas must be one"); got != "" {
		t.Fatalf("validation failures should not include generic fix guidance, got %q", got)
	}
}
