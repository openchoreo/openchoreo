// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/openchoreo/openchoreo/tools/lsp"
)

// Black-box tests for the validation cache: it must reuse the parse and
// rule-engine result when the text is unchanged, re-run only the reference
// checks when the index moves on, and evict entries when documents close.
// These drove internal/lsp's white-box tests; the exported Server hooks below
// keep the same assertions reachable from this external test package.

const cacheComponentText = "apiVersion: openchoreo.dev/v1alpha1\n" +
	"kind: Component\n" +
	"metadata:\n" +
	"  name: cache-svc\n" +
	"spec:\n" +
	"  componentType: service\n" +
	"  deploymentPipelineRef:\n" +
	"    name: docker-gitops-releasd\n"

const cacheReleaseFile = "apiVersion: openchoreo.dev/v1alpha1\n" +
	"kind: DeploymentPipeline\n" +
	"metadata:\n" +
	"  name: docker-gitops-release\n" +
	"spec: {}\n"

func cacheHasRefDiag(diags []protocol.Diagnostic) bool {
	for _, d := range diags {
		if d.Code != nil && d.Code.Value == "unknown-resource-file" {
			return true
		}
	}
	return false
}

func TestValidationCacheReusesResultWhenNothingChanged(t *testing.T) {
	s := lsp.NewServer()

	first := s.DiagnosticsForText("file:///a.yaml", cacheComponentText)
	second := s.DiagnosticsForText("file:///a.yaml", cacheComponentText)

	misses, hits := cacheStats(t, s)
	if misses != 1 {
		t.Errorf("cache misses = %d, want 1 (single parse)", misses)
	}
	if hits != 1 {
		t.Errorf("cache hits = %d, want 1", hits)
	}
	if len(first) != len(second) {
		t.Errorf("diagnostic counts differ: first=%d second=%d", len(first), len(second))
	}
}

func TestValidationCacheRerunsOnlyRefsWhenIndexChanges(t *testing.T) {
	s := lsp.NewServer()

	before := s.DiagnosticsForText("file:///a.yaml", cacheComponentText)
	if cacheHasRefDiag(before) {
		t.Fatalf("reference diagnostic present before the resource exists")
	}

	// Indexing a file that matches the misspelled name must invalidate only
	// the reference checks (same text, newer index).
	if err := s.IndexFile("file:///docker-gitops-release.yaml", []byte(cacheReleaseFile)); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}

	after := s.DiagnosticsForText("file:///a.yaml", cacheComponentText)

	if !cacheHasRefDiag(after) {
		t.Errorf("reference diagnostic missing after the resource was indexed")
	}
	// The second call must still have been a cache hit: the parse and rule
	// engine were not re-run, only the reference check was.
	misses, hits := cacheStats(t, s)
	if misses != 1 {
		t.Errorf("cache misses = %d, want 1 (parse must be reused)", misses)
	}
	if hits != 1 {
		t.Errorf("cache hits = %d, want 1 (index invalidation is still a hit)", hits)
	}
}

func TestValidationCacheInvalidatesOnTextChange(t *testing.T) {
	s := lsp.NewServer()

	s.DiagnosticsForText("file:///a.yaml", cacheComponentText)
	s.DiagnosticsForText("file:///a.yaml", cacheComponentText+"\n")
	s.DiagnosticsForText("file:///a.yaml", cacheComponentText)

	misses, hits := cacheStats(t, s)
	if misses != 3 {
		t.Errorf("cache misses = %d, want 3 (each distinct text reparsed)", misses)
	}
	if hits != 0 {
		t.Errorf("cache hits = %d, want 0", hits)
	}
}

func TestValidationCacheSeparatesURIs(t *testing.T) {
	s := lsp.NewServer()

	s.DiagnosticsForText("file:///a.yaml", cacheComponentText)
	s.DiagnosticsForText("file:///b.yaml", cacheComponentText)

	misses, _ := cacheStats(t, s)
	if misses != 2 {
		t.Errorf("cache misses = %d, want 2 (entries are per URI)", misses)
	}
}

func TestValidationCacheEvictsOnClose(t *testing.T) {
	s := lsp.NewServer()
	uri := "file:///close.yaml"

	s.OpenDocument(uri, cacheComponentText, 1)
	s.DiagnosticsForText(uri, cacheComponentText)
	if s.CacheSize() != 1 {
		t.Fatalf("default cache size = %d, want 1 entry for the open document", s.CacheSize())
	}

	if err := s.DidClose(nil, &protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	}); err != nil {
		t.Fatalf("DidClose: %v", err)
	}

	if s.CacheSize() != 0 {
		t.Errorf("cache size = %d, want 0 after didClose", s.CacheSize())
	}
}

func TestValidationCacheDoesNotParseNonOpenChoreo(t *testing.T) {
	s := lsp.NewServer()
	uri := "file:///plain.yaml"

	diags := s.DiagnosticsForText(uri, "hello: world\n")
	if len(diags) != 0 {
		t.Errorf("got %d diagnostics, want 0 for non-OpenChoreo text", len(diags))
	}
	if s.CacheHasParsedDocs(uri) {
		t.Fatal("docs parsed for text without an OpenChoreo apiVersion")
	}

	// A later index change must not resurrect anything for this URI.
	if err := s.IndexFile("file:///x.yaml", []byte(cacheReleaseFile)); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}
	again := s.DiagnosticsForText(uri, "hello: world\n")
	if len(again) != 0 {
		t.Errorf("got %d diagnostics after index change, want 0", len(again))
	}
}

func cacheStats(t *testing.T, s *lsp.Server) (misses, hits uint64) {
	t.Helper()
	hits, misses = s.CacheStats()
	return misses, hits
}
