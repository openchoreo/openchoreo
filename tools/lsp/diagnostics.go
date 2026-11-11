// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"fmt"
	"time"

	clivalidator "github.com/openchoreo/openchoreo/tools/lint/cli-validator"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// diagnosticDebounceDelay is how long the server waits after the last edit
// before revalidating. Validation parses the YAML and runs the rule engine,
// so bursts of keystrokes collapse into a single publish. Indexer and
// document updates stay synchronous; only the publish is delayed.
const diagnosticDebounceDelay = 250 * time.Millisecond

// Validate parses the given YAML text and runs the rule engine over every
// document, converting each rule diagnostic into an LSP diagnostic. Text that
// does not declare an OpenChoreo apiVersion is not an OpenChoreo document and
// is ignored (no diagnostics are produced).
func Validate(text string) []protocol.Diagnostic {
	if !parser.HasOpenChoreoAPIVersion([]byte(text)) {
		return []protocol.Diagnostic{}
	}

	docs, err := parser.ParseYAML([]byte(text))
	if err != nil {
		return []protocol.Diagnostic{parseErrorDiagnostic(err)}
	}

	return validateDocs(docs)
}

// validateDocs runs the rule engine over already-parsed documents. It is the
// shared core of Validate so callers that parse once (e.g. the validation
// cache) can reuse the AST for both rule-engine and reference checks.
func validateDocs(docs []*parser.DocumentNode) []protocol.Diagnostic {
	var out []protocol.Diagnostic
	for _, result := range clivalidator.ValidateDocuments(docs) {
		for _, finding := range result.Diagnostics {
			out = append(out, toLSPDiagnostic(finding))
		}
	}

	if out == nil {
		out = []protocol.Diagnostic{}
	}

	return out
}

func parseErrorDiagnostic(err error) protocol.Diagnostic {
	severity := protocol.DiagnosticSeverity(ruleengine.SeverityError)
	return protocol.Diagnostic{
		Range:    protocol.Range{},
		Severity: &severity,
		Message:  err.Error(),
	}
}

// toLSPDiagnostic converts a rule engine diagnostic into an LSP diagnostic.
func toLSPDiagnostic(d ruleengine.Diagnostic) protocol.Diagnostic {
	severity := protocol.DiagnosticSeverity(d.Severity)
	return protocol.Diagnostic{
		Range:    toLSPRange(d.Range),
		Severity: &severity,
		Code:     &protocol.IntegerOrString{Value: d.Code},
		Source:   stringPtr("openchoreo"),
		Message:  d.Message,
	}
}

// toLSPRange converts a 1-indexed parser range into a 0-indexed LSP range.
func toLSPRange(r parser.Range) protocol.Range {
	return protocol.Range{
		Start: toLSPPosition(r.Start),
		End:   toLSPPosition(r.End),
	}
}

// toLSPPosition converts a 1-indexed parser position into a 0-indexed
// LSP position.
func toLSPPosition(p parser.Position) protocol.Position {
	line := p.Line - 1
	character := p.Column - 1
	if line < 0 {
		line = 0
	}
	if character < 0 {
		character = 0
	}
	return protocol.Position{
		Line:      protocol.UInteger(line),
		Character: protocol.UInteger(character),
	}
}

// publishDiagnostics sends the given diagnostics for a document to the client.
func (s *Server) publishDiagnostics(context *glsp.Context, uri string, version int, diags []protocol.Diagnostic) {
	if context == nil || !s.validationSnapshotCurrent(uri, version) {
		return
	}

	if s.GetDocument(uri) == nil {
		s.preRenderSchemaMu.Lock()
		diags = append(diags, s.workspaceStaticByURI[uri]...)
		s.preRenderSchemaMu.Unlock()
	}
	diags = s.mergePostRenderDiagnostics(uri, diags)
	diags = s.mergePreRenderSchemaDiagnostics(uri, diags)
	if diags == nil {
		diags = []protocol.Diagnostic{}
	}

	diags = uniqueProjectDiagnostics(diags)
	versionUInt := protocol.UInteger(version)
	context.Notify(protocol.ServerTextDocumentPublishDiagnostics, &protocol.PublishDiagnosticsParams{
		URI:         protocol.DocumentUri(uri),
		Version:     diagnosticVersion(s, uri, &versionUInt),
		Diagnostics: diags,
	})
}

// publishDiagnosticsAsync pushes diagnostics through the retained connection.
// Unlike glsp.Context.Notify, that is valid after a handler has returned,
// which is what debounced publishes rely on.
func (s *Server) publishDiagnosticsAsync(uri string, version int, diags []protocol.Diagnostic) {
	if !s.validationSnapshotCurrent(uri, version) {
		return
	}
	if s.GetDocument(uri) == nil {
		s.preRenderSchemaMu.Lock()
		diags = append(diags, s.workspaceStaticByURI[uri]...)
		s.preRenderSchemaMu.Unlock()
	}
	diags = s.mergePostRenderDiagnostics(uri, diags)
	diags = s.mergePreRenderSchemaDiagnostics(uri, diags)
	if diags == nil {
		diags = []protocol.Diagnostic{}
	}

	diags = uniqueProjectDiagnostics(diags)
	versionUInt := protocol.UInteger(version)
	s.SendAsync(protocol.ServerTextDocumentPublishDiagnostics, &protocol.PublishDiagnosticsParams{
		URI:         protocol.DocumentUri(uri),
		Version:     diagnosticVersion(s, uri, &versionUInt),
		Diagnostics: diags,
	})
}

// scheduleDiagnostics revalidates the given document after a short quiet
// period. Repeated calls within the window reset the timer, so a quick burst
// of edits yields one publish using the final text. The text captured here is
// the freshest state at the last edit, which is exactly what a delayed
// validation would see.
func (s *Server) scheduleDiagnostics(uri string, version int, text string) {
	s.diagMu.Lock()
	defer s.diagMu.Unlock()

	s.diagGen++
	gen := s.diagGen
	s.diagURI = uri
	s.diagVersion = version
	s.diagText = text
	s.diagPending = true

	if s.diagTimer != nil {
		s.diagTimer.Stop()
	}
	s.diagTimer = time.AfterFunc(diagnosticDebounceDelay, func() {
		s.diagMu.Lock()
		if gen != s.diagGen {
			// Superseded by a newer schedule; its timer will fire instead.
			s.diagMu.Unlock()
			return
		}
		uri := s.diagURI
		version := s.diagVersion
		text := s.diagText
		s.diagTimer = nil
		s.diagPending = false
		s.diagMu.Unlock()

		s.publishDiagnosticsAsync(uri, version, s.diagnosticsForCached(uri, text))
		if path := fileURIToPath(uri); path != "" {
			s.triggerPreRenderSchemaValidationForPath(path)
		}
	})
}

// cancelDiagnostics drops a pending (not yet fired) diagnostic publish for the
// given URI. Used on didClose so a stale publish is not sent for a closed doc.
func (s *Server) cancelDiagnostics(uri string) {
	s.diagMu.Lock()
	defer s.diagMu.Unlock()
	if s.diagURI != uri || !s.diagPending {
		return
	}
	if s.diagTimer != nil {
		s.diagTimer.Stop()
		s.diagTimer = nil
	}
	s.diagPending = false
	s.diagGen++
}

// FlushDiagnostics publishes any pending debounced diagnostics immediately.
// Call before shutdown so the final validation state is not lost. With no
// newer schedule pending it is a no-op.
func (s *Server) FlushDiagnostics() {
	s.diagMu.Lock()
	if !s.diagPending {
		s.diagMu.Unlock()
		return
	}
	if s.diagTimer != nil {
		s.diagTimer.Stop()
		s.diagTimer = nil
	}
	uri := s.diagURI
	version := s.diagVersion
	text := s.diagText
	s.diagPending = false
	s.diagGen++
	s.diagMu.Unlock()

	s.publishDiagnosticsAsync(uri, version, s.diagnosticsForCached(uri, text))
}

func stringPtr(s string) *string {
	return &s
}

// otherDocRevalidateDelay is how long an edited document's index change is
// quiet before other open documents are revalidated. Keystroke bursts collapse
// into a single pass; the edited document itself is published by its own
// scheduleDiagnostics debounce.
const otherDocRevalidateDelay = 100 * time.Millisecond

// scheduleOtherDocRevalidation revalidates every open document after the
// edited one has been quiet for a moment, because the edit may have changed
// what those documents reference (e.g. a metadata.name edit). The edited
// document is skipped: its own publish is handled by scheduleDiagnostics.
func (s *Server) scheduleOtherDocRevalidation(editedURI string) {
	s.diagMu.Lock()
	defer s.diagMu.Unlock()

	s.revalGen++
	gen := s.revalGen
	s.revalURI = editedURI

	if s.revalTimer != nil {
		s.revalTimer.Stop()
	}
	s.revalTimer = time.AfterFunc(otherDocRevalidateDelay, func() {
		s.diagMu.Lock()
		if gen != s.revalGen {
			// Superseded by a newer schedule; its timer will fire instead.
			s.diagMu.Unlock()
			return
		}
		edited := s.revalURI
		s.revalURI = ""
		s.revalTimer = nil
		s.diagMu.Unlock()

		s.revalidateOpenDocumentsExcept(edited)
	})
}

// revalidateOpenDocumentsExcept republishes the current diagnostics of every
// open document except exceptURI. Metrics and cached parses are reused, so
// CEL and reference checks re-run when schema dependencies change.
func (s *Server) revalidateOpenDocumentsExcept(exceptURI string) {
	for _, doc := range s.snapshotOpenDocuments() {
		if doc.URI == exceptURI {
			continue
		}
		s.publishDiagnosticsAsync(doc.URI, doc.Version, s.diagnosticsForCached(doc.URI, doc.Text))
	}
}

func uniqueProjectDiagnostics(diags []protocol.Diagnostic) []protocol.Diagnostic {
	seen := map[string]bool{}
	out := make([]protocol.Diagnostic, 0, len(diags))
	for _, d := range diags {
		code := ""
		if d.Code != nil {
			code = fmt.Sprint(d.Code.Value)
		}
		key := fmt.Sprintf("%s:%d:%d:%s", code, d.Range.Start.Line, d.Range.Start.Character, d.Message)
		if !seen[key] {
			seen[key] = true
			out = append(out, d)
		}
	}
	return out
}

// Schema edits do not change a consuming document's LSP version, so guard the
// schema revision as well before publishing its cached static diagnostics.
func (s *Server) validationSnapshotCurrent(uri string, version int) bool {
	if current := s.GetDocument(uri); current != nil && current.Version != version {
		return false
	}
	s.cacheMu.Lock()
	entry := s.cache[uri]
	stale := entry != nil && entry.indexVer != s.indexer.Version()
	s.cacheMu.Unlock()
	return !stale
}

// Closed workspace documents have no client document version.
func diagnosticVersion(s *Server, uri string, version *protocol.UInteger) *protocol.UInteger {
	if s.GetDocument(uri) == nil {
		return nil
	}
	return version
}

// Refresh the automatic lane before merging manual findings. An unrelated index
// revision must not suppress a closed file's Play results or leave stale markers.
func (s *Server) republishCurrentDiagnostics(uri string) {
	if doc := s.GetDocument(uri); doc != nil {
		s.publishDiagnosticsAsync(uri, doc.Version, s.diagnosticsForCached(uri, doc.Text))
		return
	}
	var diags []protocol.Diagnostic
	if text, exists := s.documentText(uri); exists {
		diags = s.diagnosticsForCached(uri, text)
	} else {
		s.deleteValidationEntry(uri)
	}
	s.preRenderSchemaMu.Lock()
	if s.workspaceStaticByURI == nil {
		s.workspaceStaticByURI = make(map[string][]protocol.Diagnostic)
	}
	s.workspaceStaticByURI[uri] = diags
	s.preRenderSchemaMu.Unlock()
	s.publishDiagnosticsAsync(uri, 0, nil)
}
