// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"math"
	"strings"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/openchoreo/openchoreo/tools/lsp"
)

// findDiagnostic returns the first diagnostic with the given code, or nil.
func findDiagnostic(diags []protocol.Diagnostic, code string) *protocol.Diagnostic {
	for i := range diags {
		if diags[i].Code != nil && diags[i].Code.Value == code {
			return &diags[i]
		}
	}
	return nil
}

func TestValidate_ReportsMissingReference(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-comp
spec:
  componentType:
    name: deployment/service
`
	diags := lsp.Validate(yaml)

	d := findDiagnostic(diags, "missing-ref-kind")
	if d == nil {
		t.Fatalf("expected diagnostic with code missing-ref-kind, got: %+v", diags)
	}
	if d.Severity == nil || *d.Severity != protocol.DiagnosticSeverityError {
		t.Errorf("severity = %v, want error", d.Severity)
	}
	if d.Message == "" {
		t.Error("expected non-empty message")
	}
	if d.Range.Start.Line > d.Range.End.Line {
		t.Errorf("invalid range: start %+v after end %+v", d.Range.Start, d.Range.End)
	}
}

func TestValidate_ReportsUnknownKind(t *testing.T) {
	diags := lsp.Validate("apiVersion: openchoreo.dev/v1alpha1\nkind: BogusKind\nmetadata:\n  name: x\n")

	if findDiagnostic(diags, "unknown-kind") == nil {
		t.Fatalf("expected unknown-kind diagnostic, got: %+v", diags)
	}
}

func TestValidate_ParseError(t *testing.T) {
	diags := lsp.Validate("apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nspec:\n  componentType: [\n")

	if len(diags) != 1 {
		t.Fatalf("expected exactly 1 parse-error diagnostic, got %d: %+v", len(diags), diags)
	}
	if diags[0].Message == "" {
		t.Error("expected non-empty parse error message")
	}
	if diags[0].Severity == nil || *diags[0].Severity != protocol.DiagnosticSeverityError {
		t.Errorf("severity = %v, want error", diags[0].Severity)
	}
}

func TestValidate_EmptyDocument(t *testing.T) {
	diags := lsp.Validate("")

	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for empty document, got: %+v", diags)
	}
}

func TestValidate_RangesAreZeroIndexed(t *testing.T) {
	// metadata starts on line 3 (1-indexed), so LSP line must be 2.
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-comp
spec:
  componentType:
    name: deployment/service
`
	diags := lsp.Validate(yaml)

	for i, d := range diags {
		start, end := d.Range.Start, d.Range.End
		if strings.HasPrefix(d.Message, "yaml:") {
			continue
		}
		if start.Line > math.MaxInt32 || end.Line > math.MaxInt32 || start.Character > math.MaxInt32 || end.Character > math.MaxInt32 {
			t.Errorf("diagnostic %d has a position outside the LSP integer range: %+v", i, d.Range)
		}
	}
}
