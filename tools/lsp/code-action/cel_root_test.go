// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package codeaction

import (
	"strings"
	"testing"
	"unicode/utf16"

	protocol "github.com/tliron/glsp/protocol_3_16"

	lsputil "github.com/openchoreo/openchoreo/tools/lsp/lsp-util"
)

func TestCELRootQuickFix(t *testing.T) {
	for _, text := range []string{"value: '${metdata.name}'", "value: '${metdata.name}' # metdata", "value: '😀 ${metdata.name}'\r"} {
		index := strings.Index(text, "metdata")
		if index < 0 {
			t.Fatal("fixture token missing")
		}
		start := len(utf16.Encode([]rune(text[:index])))
		diag := protocol.Diagnostic{Message: `undeclared reference to 'metdata' (in container ''). Did you mean "metadata"?`, Range: protocol.Range{Start: protocol.Position{Character: lsputil.PositionIndex(start)}, End: protocol.Position{Character: lsputil.PositionIndex(start + 7)}}}
		action := fixCELRoot("file:///demo.yaml", text, diag)
		if action == nil {
			t.Fatal("missing fix", text)
		}
		edits := action.Edit.Changes["file:///demo.yaml"]
		if len(edits) != 1 || edits[0].NewText != "metadata" || edits[0].Range != diag.Range {
			t.Fatal(edits)
		}
		if fixCELRoot("file:///demo.yaml", strings.ReplaceAll(text, "metdata", "metadata"), diag) != nil {
			t.Fatal("stale fix")
		}
	}
}
