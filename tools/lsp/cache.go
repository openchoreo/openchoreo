// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lsp/code-action"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// Validation cache.
//
// Validating a document is the most expensive per-edit operation: it parses
// the YAML and runs the rule engine. The parse result and the rule-engine
// diagnostics depend only on the text, so they are worth caching per document
// URI. Only the reference diagnostics (FileNameRefDiagnostics) depend on the
// indexer, which is why every entry records the index version it was computed
// against and re-runs only that part when the index moves on.
//
// The cache is bounded in practice: entries are created for documents that are
// actually open and evicted on didClose.

// validationEntry caches the validation state of one document URI.
type validationEntry struct {
	// text is the document text this entry was computed from. Entries are
	// invalidated by comparing it on every lookup.
	text string
	// docs is the parsed AST for text; nil when parsing was skipped (no
	// OpenChoreo apiVersion) or failed (parse error).
	docs []*parser.DocumentNode
	// validationDiags is the rule-engine result. It depends only on text.
	validationDiags []protocol.Diagnostic
	// refDiags is the resource-file reference result. It depends on the
	// indexer state at indexVer.
	refDiags []protocol.Diagnostic
	// indexVer is indexer.Version() at the time refDiags was computed.
	indexVer uint64
	// combined is validationDiags + refDiags, pre-joined for cheap hits.
	combined []protocol.Diagnostic
}

// diagnosticsForCached returns the combined diagnostics for a document,
// reusing the cached parse and rule-engine result whenever the text is
// unchanged. The returned slice must be treated as read-only.
func (s *Server) diagnosticsForCached(uri, text string) []protocol.Diagnostic {
	indexVer := s.indexer.Version()

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	entry := s.cache[uri]

	if entry != nil && entry.text == text {
		if entry.indexVer == indexVer {
			s.cacheHits++
			return entry.combined
		}

		// The text did not change but the index did: reuse the parse and the
		// rule-engine result, recompute only the reference diagnostics.
		s.cacheHits++
		if entry.docs != nil {
			entry.refDiags = codeaction.FileNameRefDiagnosticsDocs(entry.docs, s.indexer.ResourceNames(), s.indexer.FileNames(), codeaction.ReferenceScope{Index: s.indexer, URI: uri})
			entry.combined = combineDiagnostics(entry.validationDiags, entry.refDiags)
		}
		entry.indexVer = indexVer
		return entry.combined
	}

	s.cacheMisses++

	rebuilt := s.computeValidation(uri, text, indexVer)
	s.cache[uri] = rebuilt
	return rebuilt.combined
}

// invalidateValidationCache drops every cached validation entry, forcing a
// full re-computation against the current template version on next lookup.
func (s *Server) invalidateValidationCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cache = make(map[string]*validationEntry)
}

// deleteValidationEntry drops the cached state for a document (on close).
func (s *Server) deleteValidationEntry(uri string) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	delete(s.cache, uri)
}

// computeValidation parses and validates a document from scratch. It returns a
// fully populated entry that the caller stores under cacheMu.
func (s *Server) computeValidation(uri, text string, indexVer uint64) *validationEntry {
	if !parser.HasOpenChoreoAPIVersion([]byte(text)) {
		return &validationEntry{
			text:     text,
			indexVer: indexVer,
			combined: []protocol.Diagnostic{},
		}
	}

	docs, err := parser.ParseYAML([]byte(text))
	if err != nil {
		return &validationEntry{
			text:     text,
			indexVer: indexVer,
			combined: []protocol.Diagnostic{parseErrorDiagnostic(err)},
		}
	}

	validationDiags := validateDocs(docs)
	refDiags := codeaction.FileNameRefDiagnosticsDocs(docs, s.indexer.ResourceNames(), s.indexer.FileNames(), codeaction.ReferenceScope{Index: s.indexer, URI: uri})

	return &validationEntry{
		text:            text,
		docs:            docs,
		validationDiags: validationDiags,
		refDiags:        refDiags,
		indexVer:        indexVer,
		combined:        combineDiagnostics(validationDiags, refDiags),
	}
}

// combineDiagnostics joins two diagnostic sets into one slice.
func combineDiagnostics(a, b []protocol.Diagnostic) []protocol.Diagnostic {
	combined := make([]protocol.Diagnostic, 0, len(a)+len(b))
	combined = append(combined, a...)
	combined = append(combined, b...)
	return combined
}

// DiagnosticsForText validates text for a URI through the validation cache,
// exercising the exact lookup a live didChange performs. Exported so the
// integration tests in tests/lsp can exercise the cache black-box.
func (s *Server) DiagnosticsForText(uri, text string) []protocol.Diagnostic {
	return s.diagnosticsForCached(uri, text)
}

// IndexFile adds or replaces the indexer entry for a URI, the same operation a
// watched on-disk change performs. Exported for the tests/lsp integration
// tests.
func (s *Server) IndexFile(uri string, content []byte) error {
	return s.indexer.UpdateFile(uri, content)
}

// CacheSize reports how many document URIs currently hold a validation cache
// entry. Exported for the tests/lsp integration tests.
func (s *Server) CacheSize() int {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	return len(s.cache)
}

// CacheStats reports the accumulated (hits, misses) counters. Exported for the
// tests/lsp integration tests.
func (s *Server) CacheStats() (hits uint64, misses uint64) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	return s.cacheHits, s.cacheMisses
}

// CacheHasParsedDocs reports whether the cached entry for a URI holds parsed
// AST nodes: true for OpenChoreo text, false when parsing was skipped. It
// returns false when no entry exists. Exported for the tests/lsp integration
// tests.
func (s *Server) CacheHasParsedDocs(uri string) bool {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	entry := s.cache[uri]
	return entry != nil && entry.docs != nil
}

// ResourceNames returns the resource names currently in the index. Exported
// for the tests/lsp integration tests.
func (s *Server) ResourceNames() []string {
	return s.indexer.ResourceNames()
}
