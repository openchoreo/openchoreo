// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// committedMatrixPath is relative to this package's directory.
const committedMatrixPath = "../../docs/audit/coverage-matrix.md"

// TestRender_Deterministic guards the fix for a real bug: renderMCPSection
// used to key a scope-collapsed tool's two bindings by tool name alone, so
// which one survived depended on Go's randomized map iteration order —
// three consecutive runs could each produce a different file. Rendering
// twice in the same process and diffing catches any reintroduction of a
// map-keyed-by-tool-name-alone bug, without needing to actually observe
// iteration-order flakiness (which by definition doesn't reproduce reliably
// in a single run).
func TestRender_Deterministic(t *testing.T) {
	a, err := render()
	if err != nil {
		t.Fatalf("render() error: %v", err)
	}
	b, err := render()
	if err != nil {
		t.Fatalf("render() error: %v", err)
	}
	if a != b {
		t.Fatal("render() produced different output across two calls in the same process — " +
			"non-deterministic map iteration is leaking into the rendered output")
	}
}

// TestRenderMCPSection_FlagsUndocumentedExemption is a direct, isolated unit
// test for a state the real registry can't produce today (every real
// non-read-only, unbound tool has an apiaudit.MCPToolExemptions entry — see
// TestAuditCoverage's assertion 4) but that the coverage gate exists
// specifically to catch if it ever did: a tool that's neither bound nor
// read-only, with no exemption reason on file, must render as visibly
// UNDOCUMENTED rather than silently dropping out of the matrix.
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

// TestCoverageMatrix_IsFresh guards docs/audit/coverage-matrix.md against
// drifting from the code it describes: make audit-coverage-matrix must be
// re-run and the result committed whenever a change to the audit tables
// would change this file's content. Unlike tools/auditgen's generated file,
// this isn't wired into code.gen-check (it's reporting only, doesn't affect
// what's audited) — this test is the freshness gate instead, and only works
// now that render() is deterministic (see TestRender_Deterministic).
func TestCoverageMatrix_IsFresh(t *testing.T) {
	want, err := render()
	if err != nil {
		t.Fatalf("render() error: %v", err)
	}
	got, err := os.ReadFile(committedMatrixPath)
	if err != nil {
		t.Fatalf("failed to read committed matrix at %s: %v", committedMatrixPath, err)
	}
	if string(got) != want {
		t.Errorf("%s is stale — run `make audit-coverage-matrix` and commit the result", committedMatrixPath)
	}
}
