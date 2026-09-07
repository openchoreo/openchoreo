// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// repoRelative resolves a service's defaultOut (repo-root-relative) against
// this package's directory, which is where `go test` runs.
func repoRelative(p string) string {
	return filepath.Join("..", "..", p)
}

// TestRender_Deterministic guards the fix for a real bug: renderMCPSection
// used to key a scope-collapsed tool's two bindings by tool name alone, so
// which one survived depended on map iteration order. Rendering twice and
// diffing catches a reintroduction without needing to observe the flakiness.
//
// Run per registered service, so a future service is covered on joining.
func TestRender_Deterministic(t *testing.T) {
	for name, svc := range services {
		t.Run(name, func(t *testing.T) {
			a, err := render(svc)
			if err != nil {
				t.Fatalf("render() error: %v", err)
			}
			b, err := render(svc)
			if err != nil {
				t.Fatalf("render() error: %v", err)
			}
			if a != b {
				t.Fatal("render() produced different output across two calls in the same process — " +
					"non-deterministic map iteration is leaking into the rendered output")
			}
		})
	}
}

// TestCoverageMatrix_IsFresh guards each committed matrix against drifting
// from the code it describes. These aren't wired into code.gen-check
// (reporting only), so this is the freshness gate — and it only works because
// render() is deterministic (see TestRender_Deterministic).
func TestCoverageMatrix_IsFresh(t *testing.T) {
	for name, svc := range services {
		t.Run(name, func(t *testing.T) {
			want, err := render(svc)
			if err != nil {
				t.Fatalf("render() error: %v", err)
			}
			path := repoRelative(svc.defaultOut)
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read committed matrix at %s: %v", path, err)
			}
			if string(got) != want {
				t.Errorf("%s is stale — run `make %s` and commit the result", svc.defaultOut, svc.makeTarget)
			}
		})
	}
}

// TestServiceRegistry_IsComplete pins that every registered service declares
// the fields render() reads. A zero-valued title or defaultOut would produce
// a matrix with no heading, or write it to "" — both silent.
func TestServiceRegistry_IsComplete(t *testing.T) {
	for name, svc := range services {
		t.Run(name, func(t *testing.T) {
			if svc.title == "" {
				t.Error("title must not be empty")
			}
			if svc.defaultOut == "" {
				t.Error("defaultOut must not be empty")
			}
			if svc.makeTarget == "" {
				t.Error("makeTarget must not be empty")
			}
			if svc.operations == nil {
				t.Error("operations must not be nil")
			}
			if svc.knownNonEvents == "" {
				t.Error("knownNonEvents must not be empty")
			}
		})
	}
}

// TestRenderMCPSection_FlagsUndocumentedExemption covers a state the real
// registry can't produce today but the gate exists to catch: a tool neither
// bound nor read-only, with no exemption reason, must render as visibly
// UNDOCUMENTED rather than dropping out of the matrix.
func TestRenderMCPSection_FlagsUndocumentedExemption(t *testing.T) {
	perms := map[string]tools.ToolPermission{
		"mystery_tool": {Action: "widget:create"},
	}
	out := renderMCPSection(perms, map[audit.MCPBindingKey]audit.MCPBinding{})

	if !strings.Contains(out, "mystery_tool") {
		t.Fatalf("renderMCPSection() output missing the unbound tool:\n%s", out)
	}
	if !strings.Contains(out, "UNDOCUMENTED") {
		t.Errorf("renderMCPSection() output = %q, want the UNDOCUMENTED fallback for a tool with no exemption reason", out)
	}
}
