// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"testing"
)

func TestSupportedKinds(t *testing.T) {
	kinds := supportedKinds()
	if len(kinds) == 0 {
		t.Fatal("supportedKinds() returned empty list")
	}
	for i := 1; i < len(kinds); i++ {
		if kinds[i] < kinds[i-1] {
			t.Errorf("supportedKinds() not sorted: %q before %q", kinds[i-1], kinds[i])
		}
	}
}
