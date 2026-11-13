package codeaction

import (
	protocol "github.com/tliron/glsp/protocol_3_16"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestCELRootQuickFix(t *testing.T) {
	for _, text := range []string{"value: '${metdata.name}'", "value: '${metdata.name}' # metdata", "value: '😀 ${metdata.name}'\r"} {
		start := len(utf16.Encode([]rune(text[:strings.Index(text, "metdata")])))
		diag := protocol.Diagnostic{Message: `undeclared reference to 'metdata' (in container ''). Did you mean "metadata"?`, Range: protocol.Range{Start: protocol.Position{Character: protocol.UInteger(start)}, End: protocol.Position{Character: protocol.UInteger(start + 7)}}}
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
