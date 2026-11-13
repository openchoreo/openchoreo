package codeaction

import (
	protocol "github.com/tliron/glsp/protocol_3_16"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestCELFieldFix(t *testing.T) {
	for _, prefix := range []string{"", "# 😀\r\n"} {
		line := "          path: /spec/template/spec/containers/[?(@.name=='${parameters.contaimnerName}')]/volumeMounts/-"
		text := prefix + line
		start := len(utf16.Encode([]rune(line[:strings.Index(line, "contaimnerName")])))
		row := protocol.UInteger(strings.Count(prefix, "\n"))
		diag := protocol.Diagnostic{Message: `undefined field 'contaimnerName'. Did you mean "containerName"?`, Range: protocol.Range{Start: protocol.Position{Line: row, Character: protocol.UInteger(start)}, End: protocol.Position{Line: row, Character: protocol.UInteger(start + len("contaimnerName"))}}}
		action := fixCELField("file:///trait.yaml", text, diag)
		if action == nil {
			t.Fatal("missing action")
		}
		edits := action.Edit.Changes["file:///trait.yaml"]
		if len(edits) != 1 || edits[0].Range != diag.Range || edits[0].NewText != "containerName" {
			t.Fatal(edits)
		}
		if fixCELField("file:///trait.yaml", strings.ReplaceAll(text, "contaimnerName", "containerName"), diag) != nil {
			t.Fatal("stale edit")
		}
		diag.Message = "undefined field 'contaimnerName'"
		if fixCELField("file:///trait.yaml", text, diag) != nil {
			t.Fatal("no suggestion")
		}
	}
}
