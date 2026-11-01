package lsp

import (
	protocol "github.com/tliron/glsp/protocol_3_16"
	"path/filepath"
	"testing"
)

func TestProjectDiagnosticsMergeAutomaticFindingsOnce(t *testing.T) {
	d := protocol.Diagnostic{Message: "invalid parameter", Code: &protocol.IntegerOrString{Value: "schema-parameters-mismatch"}, Range: protocol.Range{Start: protocol.Position{Line: 4, Character: 2}}}
	manual := d
	manual.Source = stringPtr("openchoreo-post-render")
	other := d
	other.Range.Start.Line = 8
	got := uniqueProjectDiagnostics([]protocol.Diagnostic{d, manual, other})
	if len(got) != 2 {
		t.Fatalf("automatic/manual duplicates must merge, distinct locations must remain: %+v", got)
	}
}
func TestFirstProjectRunInvalidatedByNewFile(t *testing.T) {
	s := NewServer()
	root := t.TempDir()
	s.postRenderScope = root
	s.postRenderRunning = true
	s.invalidatePostRenderForEdit(pathToDocumentURI(filepath.Join(root, "new.yaml")))
	if s.postRenderEpoch != 1 {
		t.Fatal("first project run was not invalidated by a newly created file")
	}
}
func TestProjectWorkspaceBoundary(t *testing.T) {
	s := NewServer()
	root := t.TempDir()
	other := t.TempDir()
	s.workspaceRoots = []string{root, other}
	path := filepath.Join(root, "nested", "component.yaml")
	if got := s.postRenderRoot(path); got != root {
		t.Fatalf("root=%s want=%s", got, root)
	}
	standalone := filepath.Join(t.TempDir(), "component.yaml")
	if got := s.postRenderRoot(standalone); got != filepath.Dir(standalone) {
		t.Fatal("standalone fallback crossed its directory")
	}
}
