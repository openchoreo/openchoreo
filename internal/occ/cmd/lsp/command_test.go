// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package lsp

import (
	"strings"
	"testing"
)

func TestRequiresLogin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmd := NewLSPCmd()
	cmd.SetArgs([]string{"--stdio"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "occ login") {
		t.Fatalf("expected login error, got %v", err)
	}
}
