package codeaction

import (
	protocol "github.com/tliron/glsp/protocol_3_16"
	"testing"
)

func TestCELFunctionFix(t *testing.T) {
	text := "value: '${configurations.toConfigEvsByContainer()}'"
	diag := protocol.Diagnostic{Message: `undeclared reference to 'toConfigEvsByContainer' (in container ''). Did you mean "toConfigEnvsByContainer"?`}
	action := fixCELFunction("file:///demo.yaml", text, diag)
	if action == nil {
		t.Fatal("missing quick fix")
	}
	edits := action.Edit.Changes["file:///demo.yaml"]
	if len(edits) != 1 {
		t.Fatal(edits)
	}
	edit := edits[0]
	got := text[:edit.Range.Start.Character] + edit.NewText + text[edit.Range.End.Character:]
	if got != "value: '${configurations.toConfigEnvsByContainer()}'" {
		t.Fatal(got)
	}
	if fixCELFunction("file:///demo.yaml", text+text, diag) != nil {
		t.Fatal("ambiguous edit")
	}
}
